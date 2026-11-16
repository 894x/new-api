package controller

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelRequestPolicyDatabaseMatrix(t *testing.T) {
	for _, dialect := range []struct{ kind, env string }{{"sqlite", ""}, {"mysql", "TEST_MYSQL_DSN"}, {"postgres", "TEST_POSTGRES_DSN"}} {
		t.Run(dialect.kind, func(t *testing.T) {
			if dialect.env != "" && os.Getenv(dialect.env) == "" {
				t.Skip("set " + dialect.env + " to run this database")
			}
			db := modelManagementDB(t, dialect.kind, os.Getenv(dialect.env))
			var response struct {
				Success bool                     `json:"success"`
				Data    model.ModelRequestPolicy `json:"data"`
			}
			policy := model.ModelRequestPolicy{
				Model: "public-policy-model",
				ParameterCapabilities: &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{
					"temperature": {Max: common.GetPointer(1.0), OnViolation: "clamp"},
					"top_p":       {Supported: common.GetPointer(false), ParticipateInSelection: common.GetPointer(true)},
				}},
				ParamOverride: map[string]any{
					"temperature": 0.7, "literal.key": false,
					"operations": []any{map[string]any{"mode": "set", "path": "temperature", "value": 0.6}},
				},
			}
			recorder := modelManagementRequest(t, SaveModelRequestPolicy, http.MethodPut, "/api/models/request-policy", policy, &response)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.True(t, response.Success, recorder.Body.String())
			var persisted []model.Option
			require.NoError(t, db.Find(&persisted).Error)
			require.Len(t, persisted, 1)
			stored, err := model.ParseModelRequestPolicy(persisted[0].Value)
			require.NoError(t, err)
			assert.Equal(t, policy.Model, stored.Model)
			// Rebuild options as another node/startup does, without copying policy
			// into channel rows or depending on a metadata record.
			model.InitOptionMap()
			recorder = modelManagementRequest(t, GetModelRequestPolicy, http.MethodGet, "/api/models/request-policy?model=public-policy-model", nil, &response)
			require.True(t, response.Success, recorder.Body.String())
			assert.Equal(t, 0.7, response.Data.ParamOverride["temperature"])
			modified := model.GetModelRequestPolicy(policy.Model + "@temperature:0.2")
			require.NotNil(t, modified)
			assert.Equal(t, policy.Model, modified.Model)
			assert.Error(t, model.UpdateOption(persisted[0].Key, `{"model":"different-model"}`))

			mapping := `{"public-policy-model":"provider-policy-model"}`
			channelOverride := `{"temperature":0.4,"operations":[{"mode":"set","path":"temperature","value":0.2}]}`
			channel := &model.Channel{Name: "policy-channel", Key: "test-key", Models: policy.Model, ModelMapping: &mapping, Group: "default", Status: 1, Type: constant.ChannelTypeOpenAI, ParamOverride: &channelOverride}
			require.NoError(t, db.Create(channel).Error)
			require.NoError(t, channel.AddAbilities(nil))
			supported, err := channel.SupportsSelectionParameters(policy.Model, []byte(`{"top_p":0.5}`))
			require.Error(t, err)
			assert.False(t, supported)
			channel.SetOtherSettings(dto.ChannelOtherSettings{ParameterCapabilities: &dto.ParameterCapabilityConfig{Rules: []dto.ModelParameterCapabilityRule{{
				Selector:   dto.ParameterCapabilitySelector{Type: "exact", Value: "provider-policy-model"},
				Parameters: map[string]dto.ParameterCapability{"top_p": {Supported: common.GetPointer(true)}},
			}}}})
			supported, err = channel.SupportsSelectionParameters(policy.Model, []byte(`{"top_p":0.5}`))
			require.NoError(t, err)
			assert.True(t, supported)
			require.NoError(t, db.Model(channel).Update("settings", channel.OtherSettings).Error)

			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			context.Set("id", 1)
			context.Set("role", common.RoleAdminUser)
			common.SetContextKey(context, constant.ContextKeyUserStatus, common.UserStatusEnabled)
			common.SetContextKey(context, constant.ContextKeyOriginalModel, policy.Model)
			require.Nil(t, middleware.SetupContextForChannelProbe(context, channel, policy.Model))
			info := &relaycommon.RelayInfo{OriginModelName: policy.Model}
			info.InitChannelMeta(context)
			info.UpstreamModelName = "provider-policy-model"
			out, err := relaycommon.ApplyRequestPoliciesWithRelayInfo([]byte(`{"temperature":1.8,"top_p":0.5}`), info)
			require.NoError(t, err)
			var body map[string]any
			require.NoError(t, common.Unmarshal(out, &body))
			assert.Equal(t, 0.2, body["temperature"], "channel legacy must run after model operations, then channel operations")
			assert.Equal(t, false, body["literal.key"], "legacy keys remain literal and explicit false survives")
			assert.NotContains(t, body, "literal")
			// Re-selecting a channel starts from its own config, not a merged
			// previous attempt; cached channel settings remain sparse.
			require.Nil(t, middleware.SetupContextForChannelProbe(context, channel, policy.Model))
			info.InitChannelMeta(context)
			assert.Len(t, info.ParamOverride["operations"], 5)
			assert.Nil(t, channel.GetOtherSettings().ParameterCapabilities.Defaults)

			capabilities, err := model.ListModelChannelCapabilities(policy.Model)
			require.NoError(t, err)
			require.Len(t, capabilities.Channels, 1)
			assert.Equal(t, 1.0, *capabilities.Channels[0].ParameterCapabilities["temperature"].Max)
			assert.Len(t, capabilities.Channels[0].ParameterOverrideOperations, 5)
			// Updated defaults reach a newly selected attempt immediately, and
			// capability validation sees the final body after both override layers.
			policy.ParameterCapabilities.Defaults["temperature"] = dto.ParameterCapability{Max: common.GetPointer(0.1), OnViolation: "clamp"}
			require.NoError(t, model.SaveModelRequestPolicy(policy))
			require.Nil(t, middleware.SetupContextForChannelProbe(context, channel, policy.Model))
			info.InitChannelMeta(context)
			info.UpstreamModelName = "provider-policy-model"
			out, err = relaycommon.ApplyRequestPoliciesWithRelayInfo([]byte(`{"temperature":1.8}`), info)
			require.NoError(t, err)
			require.NoError(t, common.Unmarshal(out, &body))
			assert.Equal(t, 0.1, body["temperature"])
			assert.NotEmpty(t, info.ParameterCapabilityAudit)

			invalid := map[string]any{"model": policy.Model, "parameter_capabilities": map[string]any{"defaults": map[string]any{"n": map[string]any{"max": 10, "on_violation": "clamp"}}}}
			recorder = modelManagementRequest(t, SaveModelRequestPolicy, http.MethodPut, "/api/models/request-policy", invalid, nil)
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			assert.NotEmpty(t, model.GetModelRequestPolicy(policy.Model).ParamOverride)

			// A second model has an independent option; clearing one is immediate
			// and does not modify channel exceptions or the other model.
			require.NoError(t, model.SaveModelRequestPolicy(model.ModelRequestPolicy{Model: "other-policy-model", ParamOverride: map[string]any{"temperature": 0}}))
			require.NoError(t, model.SaveModelRequestPolicy(model.ModelRequestPolicy{Model: policy.Model}))
			assert.Nil(t, model.GetModelRequestPolicy(policy.Model).ParameterCapabilities)
			assert.Equal(t, float64(0), model.GetModelRequestPolicy("other-policy-model").ParamOverride["temperature"])
			assert.Equal(t, channel.GetParamOverride(), model.EffectiveModelParamOverride(policy.Model, channel.GetParamOverride()))

			for _, tc := range []struct {
				name, modelOverride, channelOverride, input, want string
				operationCount                                    int
			}{
				{
					name:            "copied append executes once despite different descriptions and explicit defaults",
					modelOverride:   `{"operations":[{"description":"shared","mode":"append","path":"tags","value":"x"}]}`,
					channelOverride: `{"operations":[{"description":"channel copy","mode":"append","path":"tags","value":"x","keep_origin":false,"conditions":[],"logic":"OR","from":"","to":""}]}`,
					input:           `{"tags":[]}`, want: `{"tags":["x"]}`, operationCount: 1,
				},
				{
					name:            "different append values remain ordered",
					modelOverride:   `{"operations":[{"mode":"append","path":"tags","value":"x"}]}`,
					channelOverride: `{"operations":[{"mode":"append","path":"tags","value":"y"}]}`,
					input:           `{"tags":[]}`, want: `{"tags":["x","y"]}`, operationCount: 2,
				},
				{
					name:            "different conditions do not discard the matching model rule",
					modelOverride:   `{"operations":[{"mode":"append","path":"tags","value":"x","conditions":[{"path":"kind","mode":"full","value":"alpha"}]}]}`,
					channelOverride: `{"operations":[{"mode":"append","path":"tags","value":"x","conditions":[{"path":"kind","mode":"full","value":"beta"}]}]}`,
					input:           `{"tags":[],"kind":"alpha"}`, want: `{"tags":["x"],"kind":"alpha"}`, operationCount: 2,
				},
				{
					name:            "legacy and operation objects merge regardless of object key order",
					modelOverride:   `{"settings":{"b":false,"a":0}}`,
					channelOverride: `{"operations":[{"mode":"set","path":"settings","value":{"a":0,"b":false},"keep_origin":false}]}`,
					input:           `{}`, want: `{"settings":{"a":0,"b":false}}`, operationCount: 1,
				},
				{
					name:            "channel duplicate retains its final precedence",
					modelOverride:   `{"operations":[{"mode":"set","path":"temperature","value":0.7},{"mode":"set","path":"temperature","value":0.6}]}`,
					channelOverride: `{"temperature":0.7}`,
					input:           `{}`, want: `{"temperature":0.7}`, operationCount: 2,
				},
				{
					name:            "intentional repeats within one layer are preserved",
					modelOverride:   `{"operations":[{"mode":"append","path":"tags","value":"x"},{"mode":"append","path":"tags","value":"x"}]}`,
					channelOverride: `{"operations":[{"mode":"append","path":"tags","value":"x"}]}`,
					input:           `{"tags":[]}`, want: `{"tags":["x","x"]}`, operationCount: 2,
				},
			} {
				t.Run(tc.name, func(t *testing.T) {
					var shared, local map[string]any
					require.NoError(t, common.UnmarshalJsonStr(tc.modelOverride, &shared))
					require.NoError(t, common.UnmarshalJsonStr(tc.channelOverride, &local))
					require.NoError(t, model.SaveModelRequestPolicy(model.ModelRequestPolicy{Model: policy.Model, ParamOverride: shared}))
					effective := model.EffectiveModelParamOverride(policy.Model, local)
					out, err := relaycommon.ApplyParamOverride([]byte(tc.input), effective, nil)
					require.NoError(t, err)
					assert.JSONEq(t, tc.want, string(out))
					channel.ParamOverride = &tc.channelOverride
					require.NoError(t, db.Model(channel).Update("param_override", tc.channelOverride).Error)
					view, err := model.ListModelChannelCapabilities(policy.Model)
					require.NoError(t, err)
					require.Len(t, view.Channels, 1)
					assert.Len(t, view.Channels[0].ParameterOverrideOperations, tc.operationCount)
					// The saved model/channel configs remain editable as entered.
					encoded, err := common.Marshal(model.GetModelRequestPolicy(policy.Model).ParamOverride)
					require.NoError(t, err)
					assert.JSONEq(t, tc.modelOverride, string(encoded))
					encoded, err = common.Marshal(local)
					require.NoError(t, err)
					assert.JSONEq(t, tc.channelOverride, string(encoded))
				})
			}
		})
	}
}
