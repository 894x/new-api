package common

import (
	"os"
	"testing"

	commonjson "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParamOverrideMembershipConditions(t *testing.T) {
	for _, tc := range []struct {
		name, input, condition string
		matched                bool
		errorText              string
	}{
		{"exact name", `{"name":"weather","allowed":["weather"]}`, `{"path":"name","mode":"in","value_path":"allowed"}`, true, ""},
		{"no substring", `{"name":"weather_admin"}`, `{"path":"name","mode":"in","value":["weather"]}`, false, ""},
		{"typed values", `{"name":1}`, `{"path":"name","mode":"in","value":["1"]}`, false, ""},
		{"numeric equality", `{"name":1.0}`, `{"path":"name","mode":"in","value":[1]}`, true, ""},
		{"empty set", `{"name":"weather"}`, `{"path":"name","mode":"in","value":[]}`, false, ""},
		{"subset", `{"names":["b","a","a"],"allowed":["a","b","c"]}`, `{"path":"names","mode":"subset","value_path":"allowed"}`, true, ""},
		{"not subset", `{"names":["b","unknown"]}`, `{"path":"names","mode":"subset","value":["b"]}`, false, ""},
		{"empty subset", `{"names":[]}`, `{"path":"names","mode":"subset","value":[]}`, true, ""},
		{"dynamic full", `{"left":2,"right":2}`, `{"path":"left","mode":"full","value_path":"right"}`, true, ""},
		{"missing reference", `{"name":"weather"}`, `{"path":"name","mode":"in","value_path":"missing","invert":true}`, false, "does not exist"},
		{"non-array", `{"name":"weather","allowed":"weather"}`, `{"path":"name","mode":"in","value_path":"allowed"}`, false, "must be an array"},
		{"complex set", `{"name":"weather"}`, `{"path":"name","mode":"in","value":[{}]}`, false, "only scalars"},
		{"complex candidate", `{"name":{}}`, `{"path":"name","mode":"in","value":["weather"]}`, false, "compare scalars"},
		{"non-array subset", `{"name":"weather"}`, `{"path":"name","mode":"subset","value":[]}`, false, "resolve to an array"},
		{"conflicting operands", `{"name":"a","allowed":["a"]}`, `{"path":"name","mode":"in","value":[],"value_path":"allowed"}`, false, "invalid parameter override"},
		{"empty reference", `{"name":"a"}`, `{"path":"name","mode":"in","value_path":""}`, false, "invalid parameter override"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var config map[string]interface{}
			require.NoError(t, commonjson.UnmarshalJsonStr(`{"operations":[{"mode":"set","path":"matched","value":true,"conditions":[`+tc.condition+`]}]}`, &config))
			out, err := ApplyParamOverride([]byte(tc.input), config, nil)
			if tc.errorText != "" {
				require.ErrorContains(t, err, tc.errorText)
				return
			}
			require.NoError(t, err)
			var result map[string]interface{}
			require.NoError(t, commonjson.Unmarshal(out, &result))
			assert.Equal(t, tc.matched, result["matched"] == true)
		})
	}
}

func TestParamOverridePruneUsesRequestSnapshot(t *testing.T) {
	var config map[string]interface{}
	require.NoError(t, commonjson.UnmarshalJsonStr(`{"operations":[
		{"mode":"copy","from":"next","to":"allowed"},
		{"mode":"prune_objects","path":"tools","value":{"recursive":false,"conditions":[
			{"path":"name","mode":"in","value_path":"allowed","invert":true,"pass_missing_key":true}
		]}}
	]}`, &config))
	input := []byte(`{"allowed":["old"],"next":["b","a"],"tools":[
		{"name":"a","allowed":["evil"],"schema":{"name":"nested"}},
		{"name":"old"},{"name":"b"},{}
	]}`)
	out, err := ApplyParamOverride(input, config, nil)
	require.NoError(t, err)
	assert.JSONEq(t, `{"allowed":["b","a"],"next":["b","a"],"tools":[{"name":"a","allowed":["evil"],"schema":{"name":"nested"}},{"name":"b"}]}`, string(out))
	// Reuse the saved config for another request: resolved sets must not leak.
	out, err = ApplyParamOverride([]byte(`{"next":["old"],"tools":[{"name":"a"},{"name":"old"}]}`), config, nil)
	require.NoError(t, err)
	assert.JSONEq(t, `{"next":["old"],"allowed":["old"],"tools":[{"name":"old"}]}`, string(out))
}

func TestParamOverrideConditionGuardsDynamicReference(t *testing.T) {
	for _, tc := range []struct {
		logic   string
		guard   bool
		matched bool
	}{
		{"AND", false, false}, {"OR", true, true},
	} {
		t.Run(tc.logic, func(t *testing.T) {
			config := map[string]interface{}{"operations": []interface{}{map[string]interface{}{
				"mode": "set", "path": "matched", "value": true, "logic": tc.logic,
				"conditions": []interface{}{
					map[string]interface{}{"path": "guard", "mode": "full", "value": tc.guard},
					map[string]interface{}{"path": "name", "mode": "in", "value_path": "missing"},
				},
			}}}
			out, err := ApplyParamOverride([]byte(`{"guard":true}`), config, nil)
			require.NoError(t, err)
			if tc.matched {
				assert.JSONEq(t, `{"guard":true,"matched":true}`, string(out))
			} else {
				assert.JSONEq(t, `{"guard":true}`, string(out))
			}
		})
	}
}

func TestKimiAllowedToolsConfigurationWithoutDeclaredTools(t *testing.T) {
	configBytes, err := os.ReadFile("../../docs/examples/kimi-k3-allowed-tools.json")
	require.NoError(t, err)
	var config map[string]interface{}
	require.NoError(t, commonjson.Unmarshal(configBytes, &config))
	for _, input := range []string{
		`{"model":"kimi-k3","messages":[]}`,
		`{"model":"other","tool_choice":{"type":"allowed_tools"}}`,
	} {
		out, err := ApplyParamOverride([]byte(input), config, nil)
		require.NoError(t, err)
		assert.JSONEq(t, input, string(out))
	}
	_, err = ApplyParamOverride([]byte(`{"model":"kimi-k3","tool_choice":{"type":"allowed_tools","allowed_tools":{"mode":"auto","tools":[]}}}`), config, nil)
	var rejected *ParamOverrideReturnError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, 400, rejected.StatusCode)
}

func TestKimiAllowedToolsConfiguration(t *testing.T) {
	configBytes, err := os.ReadFile("../../docs/examples/kimi-k3-allowed-tools.json")
	require.NoError(t, err)
	var config map[string]interface{}
	require.NoError(t, commonjson.Unmarshal(configBytes, &config))
	for _, tc := range []struct {
		name, choice, model, expectedChoice string
		indices                             []int
		wantError                           bool
	}{
		{"auto multiple", `{"type":"allowed_tools","allowed_tools":{"mode":"auto","tools":[{"type":"function","function":{"name":"search"}},{"type":"function","function":{"name":"weather"}}]}}`, "kimi-k3", `"auto"`, []int{0, 2}, false},
		{"required single", `{"type":"allowed_tools","allowed_tools":{"mode":"required","tools":[{"type":"function","function":{"name":"time"}}]}}`, "kimi-k3", `"required"`, []int{1}, false},
		{"empty auto", `{"type":"allowed_tools","allowed_tools":{"mode":"auto","tools":[]}}`, "kimi-k3", `"none"`, []int{}, false},
		{"empty required", `{"type":"allowed_tools","allowed_tools":{"mode":"required","tools":[]}}`, "kimi-k3", "", nil, true},
		{"unknown name", `{"type":"allowed_tools","allowed_tools":{"mode":"auto","tools":[{"type":"function","function":{"name":"unknown"}}]}}`, "kimi-k3", "", nil, true},
		{"mixed dialect", `{"type":"allowed_tools","allowed_tools":{"mode":"auto","tools":[{"type":"function","name":"weather"}]}}`, "kimi-k3", "", nil, true},
		{"unsupported type", `{"type":"allowed_tools","allowed_tools":{"mode":"auto","tools":[{"type":"custom","function":{"name":"weather"}}]}}`, "kimi-k3", "", nil, true},
		{"missing type", `{"type":"allowed_tools","allowed_tools":{"mode":"auto","tools":[{"function":{"name":"weather"}}]}}`, "kimi-k3", "", nil, true},
		{"partial malformed list", `{"type":"allowed_tools","allowed_tools":{"mode":"auto","tools":[{"type":"function","function":{"name":"weather"}},{}]}}`, "kimi-k3", "", nil, true},
		{"missing list", `{"type":"allowed_tools","allowed_tools":{"mode":"auto"}}`, "kimi-k3", "", nil, true},
		{"non-array list", `{"type":"allowed_tools","allowed_tools":{"mode":"auto","tools":{}}}`, "kimi-k3", "", nil, true},
		{"invalid mode", `{"type":"allowed_tools","allowed_tools":{"mode":"none","tools":[]}}`, "kimi-k3", "", nil, true},
		{"ordinary auto", `"auto"`, "kimi-k3", `"auto"`, []int{0, 1, 2}, false},
		{"specified function", `{"type":"function","function":{"name":"weather"}}`, "kimi-k3", `{"type":"function","function":{"name":"weather"}}`, []int{0, 1, 2}, false},
		{"other model", `{"type":"allowed_tools","allowed_tools":{"mode":"auto","tools":[]}}`, "other", `{"type":"allowed_tools","allowed_tools":{"mode":"auto","tools":[]}}`, []int{0, 1, 2}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var request map[string]interface{}
			require.NoError(t, commonjson.UnmarshalJsonStr(`{"messages":[{"role":"user","content":"hello"}],"max_tokens":200,"stream":true,"parallel_tool_calls":false,"thinking":{"type":"enabled"},"reasoning_effort":"high","tools":[
				{"type":"function","function":{"name":"weather","description":"Weather","strict":true,"parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}},
				{"type":"function","function":{"name":"time","parameters":{"type":"object","properties":{}}}},
				{"type":"function","function":{"name":"search","parameters":{"type":"object","properties":{}}}}
			]}`, &request))
			request["model"] = tc.model
			var choice interface{}
			require.NoError(t, commonjson.UnmarshalJsonStr(tc.choice, &choice))
			request["tool_choice"] = choice
			input, err := commonjson.Marshal(request)
			require.NoError(t, err)
			info := &RelayInfo{
				OriginModelName: tc.model,
				ChannelMeta: &ChannelMeta{
					UpstreamModelName: tc.model,
					ParamOverride:     config,
					ChannelOtherSettings: dto.ChannelOtherSettings{
						ParameterCapabilities: &dto.ParameterCapabilityConfig{
							Rules: []dto.ModelParameterCapabilityRule{{
								Selector: dto.ParameterCapabilitySelector{Type: dto.ParameterCapabilitySelectorExact, Value: "kimi-k3"},
								Parameters: map[string]dto.ParameterCapability{
									"tool_choice": {AllowedValues: []string{"auto", "required", "none"}, OnViolation: dto.ParameterCapabilityActionReject},
								},
							}},
						},
					},
				},
			}
			// Specified-function requests are independent of this whitelist adapter.
			if tc.name == "specified function" {
				info.ChannelOtherSettings.ParameterCapabilities = nil
			}
			out, err := ApplyRequestPoliciesWithRelayInfo(input, info)
			if tc.wantError {
				var rejected *ParamOverrideReturnError
				require.ErrorAs(t, err, &rejected)
				assert.Equal(t, 400, rejected.StatusCode)
				assert.Equal(t, "invalid_allowed_tools", rejected.Code)
				return
			}
			require.NoError(t, err)
			allTools := request["tools"].([]interface{})
			filtered := make([]interface{}, 0, len(tc.indices))
			for _, index := range tc.indices {
				filtered = append(filtered, allTools[index])
			}
			request["tools"] = filtered
			require.NoError(t, commonjson.UnmarshalJsonStr(tc.expectedChoice, &choice))
			request["tool_choice"] = choice
			expected, err := commonjson.Marshal(request)
			require.NoError(t, err)
			assert.JSONEq(t, string(expected), string(out))
		})
	}
}

func TestKimiChatResponsesAllowedToolsConfiguration(t *testing.T) {
	configBytes, err := os.ReadFile("../../docs/examples/kimi-k3-allowed-tools-chat-responses.json")
	require.NoError(t, err)
	var config map[string]interface{}
	require.NoError(t, commonjson.Unmarshal(configBytes, &config))
	for _, protocol := range []string{"chat/completions", "responses"} {
		for _, tc := range []struct {
			name, mode, allowed, expected string
			wantError                     bool
		}{
			{"auto", "auto", "weather", "auto", false},
			{"required", "required", "weather", "required", false},
			{"empty auto", "auto", "", "none", false},
			{"empty required", "required", "", "", true},
			{"unknown", "auto", "unknown", "", true},
			{"invalid mode", "none", "weather", "", true},
		} {
			t.Run(protocol+"/"+tc.name, func(t *testing.T) {
				var request map[string]interface{}
				require.NoError(t, commonjson.UnmarshalJsonStr(`{"model":"kimi-k3","input":"hello","max_output_tokens":1024,"reasoning":{"effort":"high"},"stream":true,"store":false,"tools":[{"type":"function","name":"weather","parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}},{"type":"function","name":"time","parameters":{"type":"object","properties":{}}}]}`, &request))
				allowed := []interface{}{}
				if tc.allowed != "" {
					entry := map[string]interface{}{"type": "function", "name": tc.allowed}
					if protocol == "chat/completions" {
						delete(entry, "name")
						entry["function"] = map[string]interface{}{"name": tc.allowed}
					}
					allowed = append(allowed, entry)
				}
				choice := map[string]interface{}{"type": "allowed_tools", "mode": tc.mode, "tools": allowed}
				if protocol == "chat/completions" {
					delete(request, "input")
					delete(request, "max_output_tokens")
					delete(request, "reasoning")
					delete(request, "store")
					request["messages"] = []interface{}{map[string]interface{}{"role": "user", "content": "hello"}}
					request["thinking"] = map[string]interface{}{"type": "enabled"}
					for i, tool := range request["tools"].([]interface{}) {
						definition := tool.(map[string]interface{})
						delete(definition, "type")
						request["tools"].([]interface{})[i] = map[string]interface{}{"type": "function", "function": definition}
					}
					choice = map[string]interface{}{"type": "allowed_tools", "allowed_tools": map[string]interface{}{"mode": tc.mode, "tools": allowed}}
				}
				request["tool_choice"] = choice
				input, err := commonjson.Marshal(request)
				require.NoError(t, err)
				info := &RelayInfo{RequestURLPath: "/v1/" + protocol, ChannelMeta: &ChannelMeta{ParamOverride: config}}
				out, err := ApplyRequestPoliciesWithRelayInfo(input, info)
				if tc.wantError {
					var rejected *ParamOverrideReturnError
					require.ErrorAs(t, err, &rejected)
					assert.Equal(t, 400, rejected.StatusCode)
					return
				}
				require.NoError(t, err)
				count := 1
				if tc.allowed == "" {
					count = 0
				}
				request["tools"] = request["tools"].([]interface{})[:count]
				request["tool_choice"] = tc.expected
				expected, err := commonjson.Marshal(request)
				require.NoError(t, err)
				assert.JSONEq(t, string(expected), string(out))
			})
		}
	}
	input := []byte(`{"model":"kimi-k3","tool_choice":{"type":"allowed_tools"}}`)
	out, err := ApplyRequestPoliciesWithRelayInfo(input, &RelayInfo{RequestURLPath: "/v1/responses/compact", ChannelMeta: &ChannelMeta{ParamOverride: config}})
	require.NoError(t, err)
	assert.JSONEq(t, string(input), string(out))
}
