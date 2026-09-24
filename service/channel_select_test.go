package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/dynamic_routing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoDeliveryPreferenceDoesNotOverrideStrictAffinity(t *testing.T) {
	for _, mode := range []string{"prefer", "strict"} {
		t.Run(mode, func(t *testing.T) {
			db := setupChannelSelectAutoGroupsTest(t)
			configureDynamicRoutingForTest(t, false)
			for _, id := range []int{9401, 9402} {
				createChannelSelectAutoGroupsChannel(t, db, id, "default", "gpt-5")
				var channel model.Channel
				require.NoError(t, db.First(&channel, id).Error)
				channel.SetOtherSettings(kitdto.ChannelOtherSettings{ParameterCapabilities: &kitdto.ParameterCapabilityConfig{
					Defaults: map[string]kitdto.ParameterCapability{"messages.*.content.*.video_url": {
						ParticipateInSelection: common.GetPointer(true),
						Media: &kitdto.MediaCapability{Kind: "video", Formats: map[string]kitdto.MediaFormatCapability{
							"url": {Supported: common.GetPointer(true)}, "base64": {Supported: common.GetPointer(id == 9402)},
						}, Conversions: kitdto.MediaConversions{Base64ToURL: common.GetPointer(true)}},
					}},
				}})
				require.NoError(t, db.Model(&channel).Update("settings", channel.OtherSettings).Error)
			}
			model.InitChannelCache()
			affinity := operation_setting.GetChannelAffinitySetting()
			previous := *affinity
			t.Cleanup(func() { *affinity = previous })
			rule := operation_setting.ChannelAffinityRule{Name: t.Name(), ModelRegex: []string{"^gpt-5$"}, SessionMode: mode,
				KeySources: []operation_setting.ChannelAffinityKeySource{{Type: "request_header", Key: "X-Affinity-Key"}}}
			affinity.Enabled, affinity.Rules = true, []operation_setting.ChannelAffinityRule{rule}
			cacheKey := buildChannelAffinityCacheKeySuffix(rule, "gpt-5", "default", t.Name())
			cache := getChannelAffinityCache()
			require.NoError(t, cache.SetWithTTL(cacheKey, 9401, time.Minute))
			t.Cleanup(func() { _, _ = cache.DeleteMany([]string{cacheKey}) })
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			c.Request.Header.Set("X-Affinity-Key", t.Name())
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
			selected, _, selectionErr := SelectChannelForRequest(c, "gpt-5", &RetryParam{
				Ctx: c, ModelName: "gpt-5", TokenGroup: "default", RequestPath: c.Request.URL.Path,
				RequestBody: []byte(`{"messages":[{"content":[{"video_url":{"url":"data:video/mp4;base64,AAAA"}}]}]}`),
			})
			require.Nil(t, selectionErr)
			require.NotNil(t, selected)
			if mode == "strict" {
				assert.Equal(t, 9401, selected.Id, "strict sessions retain their bound channel")
			} else {
				assert.Equal(t, 9402, selected.Id, "best-effort affinity must not bypass native video delivery")
			}
		})
	}
}

func TestSelectChannelStrictAffinityPrecedesDynamicRouting(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	createChannelSelectAutoGroupsChannel(t, db, 9301, "default", "gpt-5")
	createChannelSelectAutoGroupsChannel(t, db, 9302, "default", "gpt-5")
	model.InitChannelCache()
	dynamic := dynamic_routing_setting.GetSetting()
	t.Cleanup(func() { require.NoError(t, dynamic_routing_setting.ReplaceAndSync(dynamic)) })
	enabled := dynamic
	enabled.Enabled = true
	require.NoError(t, dynamic_routing_setting.ReplaceAndSync(enabled))
	affinity := operation_setting.GetChannelAffinitySetting()
	original := *affinity
	t.Cleanup(func() { *affinity = original })
	rule := operation_setting.ChannelAffinityRule{Name: t.Name(), ModelRegex: []string{"^gpt-5$"}, SessionMode: "strict",
		KeySources: []operation_setting.ChannelAffinityKeySource{{Type: "request_header", Key: "X-Affinity-Key"}}}
	affinity.Enabled, affinity.Rules = true, []operation_setting.ChannelAffinityRule{rule}
	cacheKey := buildChannelAffinityCacheKeySuffix(rule, "gpt-5", "default", t.Name())
	cache := getChannelAffinityCache()
	require.NoError(t, cache.SetWithTTL(cacheKey, 9301, time.Minute))
	t.Cleanup(func() { _, _ = cache.DeleteMany([]string{cacheKey}) })
	for _, available := range []bool{true, false} {
		if !available {
			require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 9301).Update("status", common.ChannelStatusManuallyDisabled).Error)
			model.InitChannelCache()
		}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		c.Request.Header.Set("X-Affinity-Key", t.Name())
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		selected, _, selectErr := SelectChannelForRequest(c, "gpt-5", &RetryParam{Ctx: c, TokenGroup: "default", ModelName: "gpt-5", DynamicRoutingEligible: true})
		if available {
			require.Nil(t, selectErr)
			require.NotNil(t, selected)
			assert.Equal(t, 9301, selected.Id)
		} else {
			assert.Nil(t, selected)
			require.NotNil(t, selectErr)
			assert.Equal(t, http.StatusServiceUnavailable, selectErr.StatusCode)
			assert.Equal(t, "strict_session_binding_unavailable", selectErr.Message)
		}
	}
}

func TestPinnedTaskPluginChannelTypesUsesPinnedGenerationIndex(t *testing.T) {
	registry := jsplugin.NewRegistry()
	plugin, err := registry.Register(channelSelectTaskPluginSource("legacy-select", constant.ChannelTypeKling), jsplugin.Options{})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{
		Generation: registry.Generation(),
		Plugin:     plugin,
	})

	types, keys := pinnedTaskPluginIdentities(c, "legacy-select")
	assert.Equal(t, []int{constant.ChannelTypeKling}, types)
	assert.Equal(t, []string{"legacy-select"}, keys)
	types, keys = pinnedTaskPluginIdentities(c, "another-plugin")
	assert.Empty(t, types)
	assert.Empty(t, keys)
	types, keys = pinnedTaskPluginIdentities(nil, "legacy-select")
	assert.Empty(t, types)
	assert.Empty(t, keys)
}

func TestPinnedTaskPluginChannelTypesLeavesGenericChannelsKeyed(t *testing.T) {
	registry := jsplugin.NewRegistry()
	plugin, err := registry.Register(channelSelectTaskPluginSource("generic-select", constant.ChannelTypeTaskPlugin), jsplugin.Options{})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{
		Generation: registry.Generation(),
		Plugin:     plugin,
	})

	types, keys := pinnedTaskPluginIdentities(c, "generic-select")
	assert.Empty(t, types)
	assert.Equal(t, []string{"generic-select"}, keys)
}

func TestPinnedTaskPluginChannelTypesIncludesSharedEndpointProviders(t *testing.T) {
	registry := jsplugin.NewRegistry()
	_, err := registry.Register(channelSelectEndpointPluginSource("gemini-select", constant.ChannelTypeGemini), jsplugin.Options{})
	require.NoError(t, err)
	_, err = registry.Register(channelSelectEndpointPluginSource("vertex-select", constant.ChannelTypeVertexAi), jsplugin.Options{})
	require.NoError(t, err)
	candidates := registry.Generation().LookupEndpointCandidates("POST", "/v1/responses", "task-model")
	require.Len(t, candidates, 2)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{
		Generation: registry.Generation(),
		Plugin:     candidates[0].Plugin,
	})
	c.Set(jsplugin.ContextKeyPinnedEndpoint, jsplugin.PinnedEndpoint{
		Generation: registry.Generation(),
		Plugin:     candidates[0].Plugin,
		Protocol:   candidates[0].Protocol,
		Operation:  candidates[0].Operation,
		Model:      "task-model",
		Candidates: candidates,
	})

	AppendTaskPluginIdentityFilter(c, candidates[0].Plugin.Meta.Key)
	filters := GetChannelConstraints(c).Filters
	require.Len(t, filters, 1)
	assert.Equal(t, []int{constant.ChannelTypeGemini, constant.ChannelTypeVertexAi}, filters[0].TaskPluginChannelTypes)
	assert.Equal(t, []string{"gemini-select", "vertex-select"}, filters[0].TaskPluginKeys)
}

func channelSelectTaskPluginSource(key string, channelType int) string {
	return fmt.Sprintf(`
export const meta = {
  apiVersion: 1,
  key: %q,
  name: %q,
  version: "1.0.0",
  author: {name: "Test"},
  %s
  models: ["task-model"],
  fetchMode: "per_task",
};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {taskId: "task"}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {status: "SUCCESS"}; }
`, key, key, channelSelectChannelTypesField(channelType))
}

func channelSelectEndpointPluginSource(key string, channelType int) string {
	return fmt.Sprintf(`
export const meta = {
  apiVersion: 1,
  key: %q,
  name: %q,
  version: "1.0.0",
  author: {name: "Test"},
  %s
  models: ["task-model"],
  fetchMode: "per_task",
  protocols: [{name: "openai_responses", supports: ["stream", "sync", "background"]}],
};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {taskId: "task"}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {status: "SUCCESS"}; }
export const protocols = {openai_responses: {
  decodeRequest: function(ctx) { return {kind: "submit", model: "task-model", requestBody: ctx.body.value}; },
  renderEvents: function() { return {events: [], state: null, done: false}; },
  renderFinal: function() { return {output: []}; },
}};
`, key, key, channelSelectChannelTypesField(channelType))
}

func channelSelectChannelTypesField(channelType int) string {
	if channelType <= 0 || channelType == constant.ChannelTypeTaskPlugin {
		return ""
	}
	return fmt.Sprintf("channelTypes: [%d],", channelType)
}

func TestPinnedTaskPluginChannelTypesIncludesCompatibleTypes(t *testing.T) {
	registry := jsplugin.NewRegistry()
	plugin, err := registry.Register(channelSelectCompatiblePluginSource("sora-select", constant.ChannelTypeSora, constant.ChannelTypeOpenAI), jsplugin.Options{})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{
		Generation: registry.Generation(),
		Plugin:     plugin,
	})

	types, keys := pinnedTaskPluginIdentities(c, "sora-select")
	assert.Equal(t, []int{constant.ChannelTypeSora, constant.ChannelTypeOpenAI}, types)
	assert.Equal(t, []string{"sora-select"}, keys)
}

func channelSelectCompatiblePluginSource(key string, channelType, compatibleType int) string {
	return fmt.Sprintf(`
export const meta = {
  apiVersion: 1,
  key: %q,
  name: %q,
  version: "1.0.0",
  author: {name: "Test"},
  channelTypes: [%d, %d],
  models: ["task-model"],
  fetchMode: "per_task",
};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {taskId: "task"}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {status: "SUCCESS"}; }
`, key, key, channelType, compatibleType)
}

func TestSharedType61IdentityFilterContainsAllCandidateKeys(t *testing.T) {
	registry := jsplugin.NewRegistry()
	for _, key := range []string{"alpha", "beta"} {
		_, err := registry.Register(channelSelectEndpointPluginSource(key, 0), jsplugin.Options{})
		require.NoError(t, err)
	}
	generation := registry.Generation()
	candidates := generation.LookupEndpointCandidates("POST", "/v1/responses", "task-model")
	require.Len(t, candidates, 2)
	c, _ := gin.CreateTestContext(nil)
	c.Set(jsplugin.ContextKeyPinnedEndpoint, jsplugin.PinnedEndpoint{Generation: generation, Plugin: candidates[0].Plugin, Candidates: candidates})
	AppendTaskPluginIdentityFilter(c, "alpha")
	filters := GetChannelConstraints(c).Filters
	require.Len(t, filters, 1)
	assert.Equal(t, "alpha", filters[0].TaskPluginKey)
	assert.Equal(t, []string{"alpha", "beta"}, filters[0].TaskPluginKeys)
	assert.Empty(t, filters[0].TaskPluginChannelTypes)
}
