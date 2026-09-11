package router

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAllowedToolsChatToKimiGatewayE2E(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelOverride{}, &model.Log{}, &model.UserSubscription{}))
	ratio_setting.InitRatioSettings()
	oldRatios := ratio_setting.ModelRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"kimi-k3":1}`))
	settings := model_setting.GetGlobalSettings()
	oldPass, oldMemory := settings.PassThroughRequestEnabled, common.MemoryCacheEnabled
	settings.PassThroughRequestEnabled, common.MemoryCacheEnabled = false, true
	t.Cleanup(func() {
		settings.PassThroughRequestEnabled, common.MemoryCacheEnabled = oldPass, oldMemory
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatios))
	})

	bodies := make(chan []byte, 8)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(500)
			return
		}
		bodies <- body
		if gjson.GetBytes(body, "stream").Bool() {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-e2e\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"kimi-k3\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"chatcmpl-e2e\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"kimi-k3\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			return
		}
		writeKimiK3ChatE2EResponse(w, "chatcmpl-allowed-tools")
	}))
	t.Cleanup(upstream.Close)
	user := model.User{Username: "allowed-tools-e2e", Status: common.UserStatusEnabled, Group: "default", Quota: 1_000_000}
	require.NoError(t, model.DB.Create(&user).Error)
	require.NoError(t, model.DB.Create(&model.Token{UserId: user.Id, Key: "parametercapabilitye2ekey", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}).Error)
	config, err := os.ReadFile("../docs/examples/kimi-k3-allowed-tools.json")
	require.NoError(t, err)
	configText := string(config)
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "allowed-tools-kimi", Key: "test-key", Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(upstream.URL), Models: "kimi-k3", Group: "default", ParamOverride: &configText}
	channel.SetOtherSettings(dto.ChannelOtherSettings{ParameterCapabilities: &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{
		"tool_choice": {AllowedValues: []string{"auto", "required", "none"}, OnViolation: dto.ParameterCapabilityActionReject},
	}}})
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	model.InitChannelCache()
	engine := gin.New()
	SetRelayRouter(engine)

	for _, tc := range []struct {
		name, mode      string
		stream, invalid bool
	}{
		{"auto non-stream", "auto", false, false},
		{"required non-stream", "required", false, false},
		{"auto stream", "auto", true, false},
		{"invalid whitelist", "auto", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			allowedName := "weather"
			if tc.invalid {
				allowedName = "unknown"
			}
			var request map[string]interface{}
			require.NoError(t, common.UnmarshalJsonStr(`{"model":"kimi-k3","messages":[{"role":"user","content":"Hello"}],"max_tokens":200,"thinking":{"type":"enabled"},"reasoning_effort":"high","tools":[{"type":"function","function":{"name":"weather","description":"Weather","parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}},{"type":"function","function":{"name":"time","parameters":{"type":"object","properties":{}}}}]}`, &request))
			request["stream"] = tc.stream
			request["tool_choice"] = map[string]interface{}{"type": "allowed_tools", "allowed_tools": map[string]interface{}{"mode": tc.mode, "tools": []interface{}{map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": allowedName}}}}}
			input, err := common.Marshal(request)
			require.NoError(t, err)
			response := serveParameterCapabilityE2ERequest(engine, input)
			if tc.invalid {
				assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
				assert.Empty(t, bodies, "invalid whitelist must not reach upstream")
				return
			}
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.Len(t, bodies, 1)
			sent := <-bodies
			assert.Equal(t, tc.mode, gjson.GetBytes(sent, "tool_choice").String())
			assert.Equal(t, int64(1), gjson.GetBytes(sent, "tools.#").Int())
			assert.JSONEq(t, gjson.GetBytes(input, "tools.0").Raw, gjson.GetBytes(sent, "tools.0").Raw)
			assert.JSONEq(t, gjson.GetBytes(input, "messages").Raw, gjson.GetBytes(sent, "messages").Raw)
			assert.Equal(t, "enabled", gjson.GetBytes(sent, "thinking.type").String())
			assert.Equal(t, "high", gjson.GetBytes(sent, "reasoning_effort").String())
			if tc.stream {
				assert.Contains(t, response.Body.String(), "data: [DONE]")
			}
		})
	}
}
