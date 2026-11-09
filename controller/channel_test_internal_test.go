package controller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestChannelMonitoringRequestUsesConfiguredPromptAndBoundedOutput(t *testing.T) {
	settings := *operation_setting.GetMonitorSetting()
	settings.ChannelMonitoringPrompt = "Return a short confirmation."
	settings.ChannelMonitoringMaxTokens = 128
	for _, tc := range []struct{ protocol, promptPath, limitPath string }{
		{string(constant.EndpointTypeOpenAI), "messages.0.content", "max_tokens"},
		{string(constant.EndpointTypeOpenAIResponse), "input.0.content", "max_output_tokens"},
		{string(constant.EndpointTypeAnthropic), "messages.0.content", "max_tokens"},
		{string(constant.EndpointTypeGemini), "contents.0.parts.0.text", "generationConfig.maxOutputTokens"},
		{string(constant.EndpointTypeEmbeddings), "input.0", ""},
		{string(constant.EndpointTypeJinaRerank), "query", ""},
	} {
		t.Run(tc.protocol, func(t *testing.T) {
			request := buildTestRequest("model-a", tc.protocol, &model.Channel{}, true)
			require.NoError(t, configureChannelMonitoringRequest(request, settings))
			encoded, err := common.Marshal(request)
			require.NoError(t, err)
			assert.Equal(t, settings.ChannelMonitoringPrompt, gjson.GetBytes(encoded, tc.promptPath).String())
			if tc.limitPath != "" {
				assert.Equal(t, int64(128), gjson.GetBytes(encoded, tc.limitPath).Int())
			}
		})
	}
	settings.ChannelMonitoringMaxTokens = 513
	assert.Error(t, configureChannelMonitoringRequest(&dto.GeneralOpenAIRequest{}, settings))
}

func TestChannelMonitoringLocalFailureDoesNotBecomeHealthyOrExposeUpstreamSecrets(t *testing.T) {
	started := time.Unix(1700000000, 0)
	unsupported := testChannel(context.Background(), &model.Channel{Type: constant.ChannelTypeTaskPlugin}, 0, "model-a", "", false)
	require.Error(t, unsupported.localErr)
	observation := channelMonitoringObservation(unsupported, started, started.Add(time.Second))
	assert.Equal(t, "unsupported", observation.State)
	assert.Nil(t, observation.TTFTMs)
	assert.Nil(t, observation.TokensPerSecond)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("channel_test_upstream_started", true)
	ctx.Set("channel_test_upstream_status", 503)
	failed := channelMonitoringObservation(testResult{context: ctx, localErr: fmt.Errorf("upstream body with secret-credential")}, started, started.Add(time.Second))
	assert.Equal(t, "unavailable", failed.State)
	assert.Equal(t, 503, failed.StatusCode)
	encoded, err := common.Marshal(failed)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "secret-credential")
	local := channelMonitoringObservation(testResult{localErr: fmt.Errorf("missing local configuration")}, started, started.Add(time.Second))
	assert.Equal(t, "configuration", local.State)
}

func TestChannelMonitoringExclusionsFollowConfiguredStatusCodes(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	withSelfUseModeEnabled(t)
	require.NoError(t, db.AutoMigrate(&model.ChannelMonitorRecord{}, &model.UserChannelRoutingOverride{}, &model.UserGroupChannelRoutingOverride{}, &model.UserRoutingStrategy{}))
	root := model.User{Id: 11501, Username: "monitor-rejection-fixture", Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default", Quota: 1000000}
	require.NoError(t, db.Create(&root).Error)
	settings := *operation_setting.GetMonitorSetting()
	for _, tc := range []struct {
		name, code, rules, wantState string
		status                       int
	}{
		{"configured bad request", "unsupported_parameter", "400", "excluded", 400},
		{"bad request without an exclusion", "unsupported_parameter", "", "unavailable", 400},
		{"bad request with a different exclusion", "unsupported_parameter", "422", "unavailable", 400},
		{"configured range", "rate_limit_exceeded", "420-429", "excluded", 429},
		{"unmatched rate limit", "rate_limit_exceeded", "400", "unavailable", 429},
		{"policy controls credential responses too", "invalid_api_key", "400", "excluded", 400},
		{"configured server error range", "service_unavailable", "500-599", "excluded", 503},
		{"unmatched server error", "service_unavailable", "400", "unavailable", 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprintf(w, `{"error":{"message":"fixture rejection","type":"invalid_request_error","code":%q}}`, tc.code)
			}))
			t.Cleanup(upstream.Close)
			baseURL := upstream.URL
			channel := model.Channel{Type: constant.ChannelTypeOpenAI, Key: "fixture-key", BaseURL: &baseURL, Models: "gpt-4o-mini", Group: "default", Status: common.ChannelStatusEnabled}
			require.NoError(t, db.Create(&channel).Error)
			ctx := context.WithValue(context.Background(), channelMonitoringRequestKey{}, settings)
			started := time.Now()
			result := testChannel(ctx, &channel, root.Id, "gpt-4o-mini", string(constant.EndpointTypeOpenAI), false)
			require.Error(t, result.newAPIError)
			observation := channelMonitoringObservation(result, started, time.Now())
			require.Equal(t, "unavailable", observation.State, "capture the original outcome independently of exclusion policy")
			assert.Equal(t, tc.status, observation.StatusCode)
			assert.Nil(t, observation.TTFTMs)
			assert.Nil(t, observation.TokensPerSecond)
			observation.ChannelID = channel.Id
			observation.Model = "gpt-4o-mini"
			observation.SampleKey = fmt.Sprintf("exclusion-%d", channel.Id)
			require.NoError(t, model.SaveChannelMonitorRecord(context.Background(), &observation))
			excluded, err := operation_setting.ParseHTTPStatusCodeRanges(tc.rules)
			require.NoError(t, err)
			summaries, err := model.ChannelMonitorSummaries(context.Background(), []*model.Channel{&channel}, "gpt-4o-mini", true, time.Minute, excluded, time.Now())
			require.NoError(t, err)
			require.Len(t, summaries, 1)
			assert.Equal(t, tc.wantState, summaries[0].State)
			assert.Equal(t, tc.status, summaries[0].Latest.StatusCode)
			if tc.wantState == "excluded" {
				assert.Nil(t, summaries[0].SuccessRate)
			} else {
				require.NotNil(t, summaries[0].SuccessRate)
				assert.Zero(t, *summaries[0].SuccessRate)
			}
		})
	}
}

func TestChannelMonitoringAPISelectsCurrentModelAndNeverReturnsChannelCredentials(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.ChannelMonitorRecord{}))
	channel := model.Channel{Key: "usable-upstream-secret", Models: "model-a,model-b", Status: common.ChannelStatusEnabled, MonitorConfig: `{"targets":[{"model":"model-a","endpoint_type":"openai"}]}`}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, model.SaveChannelMonitorRecord(context.Background(), &model.ChannelMonitorRecord{SampleKey: "api-observation", ChannelID: channel.Id, Model: "model-a", EndpointType: "openai", CreatedAt: time.Now().Unix(), State: "healthy", LatencyMs: 800}))
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/channel/monitoring?model=model-a&channel_ids=%d", channel.Id), nil)
	GetChannelMonitoring(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "usable-upstream-secret")
	assert.NotContains(t, recorder.Body.String(), "api-observation")
	assert.True(t, gjson.Get(recorder.Body.String(), "success").Bool())
	assert.Equal(t, "healthy", gjson.Get(recorder.Body.String(), "data.0.state").String())
	assert.Equal(t, int64(96), gjson.Get(recorder.Body.String(), "data.0.buckets.#").Int())
	recorder = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/monitoring?model=model-a&channel_ids=-1", nil)
	GetChannelMonitoring(ctx)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	health := testChannelForHealthCheck(context.Background(), &model.Channel{Id: channel.Id, Type: constant.ChannelTypeTaskPlugin, Status: common.ChannelStatusEnabled}, 0, false, 10000)
	assert.Equal(t, 1, health.Failed, "a local unsupported-test error must not count as a successful health check")
	assert.Zero(t, health.Succeeded)
}

func TestChannelMonitoringTaskUsesExistingRelayAndPreservesChannelStatus(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	withSelfUseModeEnabled(t)
	require.NoError(t, db.AutoMigrate(&model.ChannelMonitorRecord{}, &model.SystemTask{}, &model.SystemTaskLock{}, &model.UserChannelRoutingOverride{}, &model.UserGroupChannelRoutingOverride{}, &model.UserRoutingStrategy{}))
	settings := operation_setting.GetMonitorSetting()
	previous := *settings
	t.Cleanup(func() { *settings = previous })
	settings.ChannelMonitoringEnabled = true
	settings.ChannelMonitoringPrompt = "Return OK for monitoring."
	settings.ChannelMonitoringMaxTokens = 128
	settings.ChannelMonitoringStream = true
	settings.ChannelMonitoringTimeoutSeconds = 30
	settings.ChannelTestConcurrency = 1
	requests := make(chan []byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- body
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"monitor-test\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o-mini\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"OK\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"monitor-test\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":2,\"total_tokens\":4}}\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(upstream.Close)
	root := model.User{Id: 11500, Username: "monitor-fixture", Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default", Quota: 1000000}
	require.NoError(t, db.Create(&root).Error)
	baseURL := upstream.URL
	channel := model.Channel{Type: constant.ChannelTypeOpenAI, Key: "fixture-key", Name: "observed channel", BaseURL: &baseURL, Models: "gpt-4o-mini", Group: "default", Status: common.ChannelStatusAutoDisabled}
	require.NoError(t, db.Create(&channel).Error)
	task := model.SystemTask{TaskID: "channel-monitor-fixture", Type: model.SystemTaskTypeChannelMonitor, Status: model.SystemTaskStatusRunning, LockedBy: "monitor-fixture"}
	require.NoError(t, db.Create(&task).Error)
	require.NoError(t, db.Create(&model.SystemTaskLock{Type: task.Type, TaskID: task.TaskID, LockedBy: task.LockedBy, LockedUntil: time.Now().Add(time.Minute).Unix()}).Error)
	channelMonitoringHandler{}.Run(context.Background(), &task, "monitor-fixture")
	var persistedTask model.SystemTask
	require.NoError(t, db.First(&persistedTask, task.ID).Error)
	require.Equal(t, model.SystemTaskStatusSucceeded, persistedTask.Status, persistedTask.Error)
	var observations []model.ChannelMonitorRecord
	require.NoError(t, db.Find(&observations).Error)
	require.Len(t, observations, 1)
	require.Equal(t, "healthy", observations[0].State)
	assert.Equal(t, "gpt-4o-mini", observations[0].Model)
	assert.Equal(t, 200, observations[0].StatusCode)
	require.NotNil(t, observations[0].TTFTMs)
	assert.GreaterOrEqual(t, *observations[0].TTFTMs, int64(0))
	select {
	case body := <-requests:
		assert.Equal(t, settings.ChannelMonitoringPrompt, gjson.GetBytes(body, "messages.0.content").String())
		assert.Equal(t, int64(128), gjson.GetBytes(body, "max_tokens").Int())
		assert.True(t, gjson.GetBytes(body, "stream").Bool())
	default:
		t.Fatal("monitoring did not reach the configured upstream")
	}
	var preserved model.Channel
	require.NoError(t, db.First(&preserved, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusAutoDisabled, preserved.Status, "observation never enables or disables a channel")
}

func TestGetChannelDefaultBaseURLsUsesBuiltInDefaults(t *testing.T) {
	originalBaseURLs := constant.ChannelBaseURLs
	constant.ChannelBaseURLs = append([]string(nil), originalBaseURLs...)
	constant.ChannelBaseURLs[constant.ChannelTypeDeepSeek] = "https://deepseek.server.example"
	t.Cleanup(func() {
		constant.ChannelBaseURLs = originalBaseURLs
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/channel/default_base_urls", nil)
	GetChannelDefaultBaseURLs(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool           `json:"success"`
		Data    map[int]string `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.Equal(t, "https://deepseek.server.example", response.Data[constant.ChannelTypeDeepSeek])
	assert.Equal(t, "https://api.openai.com", response.Data[constant.ChannelTypeOpenAI])
	assert.NotContains(t, response.Data, constant.ChannelTypeAzure)
	assert.NotContains(t, response.Data, constant.ChannelTypeNewAPI)
	assert.NotContains(t, response.Data, constant.ChannelTypeTaskPlugin)
}

func TestValidateChannelProxy(t *testing.T) {
	tests := []struct {
		name    string
		proxy   string
		wantErr bool
	}{
		{name: "empty"},
		{name: "http", proxy: "http://proxy.example:8080"},
		{name: "https", proxy: "https://proxy.example:8443"},
		{name: "socks5", proxy: "socks5://proxy.example"},
		{name: "socks5h", proxy: "socks5h://proxy.example:1080/"},
		{name: "unsupported", proxy: "ftp://proxy.example", wantErr: true},
		{name: "path", proxy: "socks5://proxy.example:1080/path", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setting, err := common.Marshal(dto.ChannelSettings{Proxy: test.proxy})
			require.NoError(t, err)
			channel := &model.Channel{
				Type:    constant.ChannelTypeOpenAI,
				Setting: common.GetPointer(string(setting)),
			}

			err = validateChannel(channel, false)

			if test.wantErr {
				require.ErrorContains(t, err, "invalid channel proxy")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateChannelRequiresNewAPIBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL *string
		wantErr bool
	}{
		{name: "missing", wantErr: true},
		{name: "blank", baseURL: common.GetPointer("  "), wantErr: true},
		{name: "configured", baseURL: common.GetPointer("https://new-api.example")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := &model.Channel{
				Type:    constant.ChannelTypeNewAPI,
				BaseURL: test.baseURL,
			}

			err := validateChannel(channel, false)

			if test.wantErr {
				require.ErrorContains(t, err, "New API channel base URL cannot be empty")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateChannelRejectsOversizedDefaultWeight(t *testing.T) {
	oversized := model.MaxChannelWeight + 1
	err := validateChannel(&model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Weight: &oversized,
	}, false)

	require.ErrorContains(t, err, "channel weight exceeds")
}

func TestManageMultiKeysKeepsChannelAbilityAndCacheStatusInSync(t *testing.T) {
	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "database", true: "memory cache"}[memoryCacheEnabled], func(t *testing.T) {
			db := setupModelListControllerTestDB(t)
			originalMemoryCacheEnabled := common.MemoryCacheEnabled
			common.MemoryCacheEnabled = memoryCacheEnabled
			t.Cleanup(func() {
				common.MemoryCacheEnabled = originalMemoryCacheEnabled
				if originalMemoryCacheEnabled {
					model.InitChannelCache()
				}
			})

			priority := int64(1)
			weight := uint(2)
			channel := &model.Channel{
				Type:     constant.ChannelTypeOpenAI,
				Key:      "key-a\nkey-b",
				Status:   common.ChannelStatusEnabled,
				Name:     "managed-multi-key",
				Models:   "model-a",
				Group:    "default",
				Priority: &priority,
				Weight:   &weight,
				ChannelInfo: model.ChannelInfo{
					IsMultiKey:         true,
					MultiKeySize:       2,
					MultiKeyStatusList: map[int]int{},
				},
			}
			require.NoError(t, channel.Insert())
			model.InitChannelCache()

			call := func(action string, keyIndex int) {
				t.Helper()
				body, err := common.Marshal(MultiKeyManageRequest{
					ChannelId: channel.Id,
					Action:    action,
					KeyIndex:  common.GetPointer(keyIndex),
				})
				require.NoError(t, err)
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/multi-key/manage", bytes.NewReader(body))
				ctx.Request.Header.Set("Content-Type", "application/json")
				ManageMultiKeys(ctx)
				var response struct {
					Success bool   `json:"success"`
					Message string `json:"message"`
				}
				require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
				require.True(t, response.Success, response.Message)
			}
			assertState := func(status int, enabled bool) {
				t.Helper()
				var persisted model.Channel
				require.NoError(t, db.First(&persisted, channel.Id).Error)
				assert.Equal(t, status, persisted.Status)
				var ability model.Ability
				require.NoError(t, db.Where("channel_id = ? AND model = ?", channel.Id, "model-a").First(&ability).Error)
				assert.Equal(t, enabled, ability.Enabled)
				assert.Equal(t, enabled, model.IsChannelEnabledForGroupModel("default", "model-a", channel.Id))
			}

			call("disable_key", 0)
			assertState(common.ChannelStatusEnabled, true)
			call("disable_key", 1)
			assertState(common.ChannelStatusAutoDisabled, false)
			call("enable_key", 0)
			assertState(common.ChannelStatusEnabled, true)
		})
	}
}

func TestManageMultiKeysDoesNotEnableManuallyDisabledChannel(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	channel := &model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    "key-a\nkey-b",
		Status: common.ChannelStatusManuallyDisabled,
		Name:   "manual-multi-key",
		Models: "model-a",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:         true,
			MultiKeySize:       2,
			MultiKeyStatusList: map[int]int{0: common.ChannelStatusManuallyDisabled},
		},
	}
	require.NoError(t, channel.Insert())
	body, err := common.Marshal(MultiKeyManageRequest{
		ChannelId: channel.Id,
		Action:    "enable_key",
		KeyIndex:  common.GetPointer(0),
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/multi-key/manage", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	ManageMultiKeys(ctx)

	var persisted model.Channel
	require.NoError(t, db.First(&persisted, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, persisted.Status)
	var ability model.Ability
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&ability).Error)
	assert.False(t, ability.Enabled)
}

func TestNewAPIChannelRegistration(t *testing.T) {
	apiType, ok := common.ChannelType2APIType(constant.ChannelTypeNewAPI)

	require.True(t, ok)
	assert.Equal(t, constant.APITypeNewAPI, apiType)
	assert.Equal(t, "New API", constant.GetChannelTypeName(constant.ChannelTypeNewAPI))
	require.Greater(t, len(constant.ChannelBaseURLs), constant.ChannelTypeNewAPI)
	assert.Empty(t, constant.ChannelBaseURLs[constant.ChannelTypeNewAPI])
}

func TestAstraFlowImageChannelRegistration(t *testing.T) {
	apiType, ok := common.ChannelType2APIType(constant.ChannelTypeAstraFlowImage)

	require.True(t, ok)
	assert.Equal(t, constant.APITypeAstraFlowImage, apiType)
	assert.Equal(t, "AstraFlow Image", constant.GetChannelTypeName(constant.ChannelTypeAstraFlowImage))
	require.Greater(t, len(constant.ChannelBaseURLs), constant.ChannelTypeAstraFlowImage)
	assert.Equal(t, "https://api.modelverse.cn", constant.ChannelBaseURLs[constant.ChannelTypeAstraFlowImage])
	assert.Equal(t, string(constant.EndpointTypeImageGeneration), normalizeChannelTestEndpoint(
		&model.Channel{Type: constant.ChannelTypeAstraFlowImage},
		"gpt-image-2",
		"",
	))
}

func TestAstraFlowGeminiChannelRegistration(t *testing.T) {
	apiType, ok := common.ChannelType2APIType(constant.ChannelTypeAstraFlowGemini)

	require.True(t, ok)
	assert.Equal(t, constant.APITypeAstraFlowGemini, apiType)
	assert.Equal(t, "AstraFlow Gemini", constant.GetChannelTypeName(constant.ChannelTypeAstraFlowGemini))
	require.Greater(t, len(constant.ChannelBaseURLs), constant.ChannelTypeAstraFlowGemini)
	assert.Equal(t, "https://api.modelverse.cn", constant.ChannelBaseURLs[constant.ChannelTypeAstraFlowGemini])
	assert.Equal(t, []constant.EndpointType{
		constant.EndpointTypeImageGeneration,
		constant.EndpointTypeGemini,
		constant.EndpointTypeOpenAI,
	}, common.GetEndpointTypesByChannelType(constant.ChannelTypeAstraFlowGemini, "gemini-2.5-flash-image"))
	assert.Equal(t, string(constant.EndpointTypeImageGeneration), normalizeChannelTestEndpoint(
		&model.Channel{Type: constant.ChannelTypeAstraFlowGemini},
		"gemini-2.5-flash-image",
		"",
	))
}

func TestResponsesCompactChannelSupport(t *testing.T) {
	tests := []struct {
		name        string
		channelType int
		apiType     int
		want        bool
	}{
		{name: "OpenAI", channelType: constant.ChannelTypeOpenAI, apiType: constant.APITypeOpenAI, want: true},
		{name: "Azure", channelType: constant.ChannelTypeAzure, apiType: constant.APITypeOpenAI, want: true},
		{name: "Codex", channelType: constant.ChannelTypeCodex, apiType: constant.APITypeCodex, want: true},
		{name: "Advanced Custom", channelType: constant.ChannelTypeAdvancedCustom, apiType: constant.APITypeAdvancedCustom, want: true},
		{name: "Sub2API", channelType: constant.ChannelTypeSub2API, apiType: constant.APITypeSub2API, want: true},
		{name: "New API", channelType: constant.ChannelTypeNewAPI, apiType: constant.APITypeNewAPI, want: true},
		{name: "Anthropic", channelType: constant.ChannelTypeAnthropic, apiType: constant.APITypeAnthropic, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, common.SupportsResponsesCompact(test.channelType, test.apiType))
		})
	}
}

func TestMultiprotocolGatewayEndpointTypes(t *testing.T) {
	want := []constant.EndpointType{
		constant.EndpointTypeOpenAI,
		constant.EndpointTypeOpenAIResponse,
		constant.EndpointTypeOpenAIResponseCompact,
		constant.EndpointTypeAnthropic,
		constant.EndpointTypeGemini,
		constant.EndpointTypeOpenAIAlphaSearch,
	}

	assert.Equal(t, want, common.GetEndpointTypesByChannelType(constant.ChannelTypeNewAPI, "gpt-5"))
	assert.Equal(t, want, common.GetEndpointTypesByChannelType(constant.ChannelTypeSub2API, "gpt-5"))
}

func TestCopyChannelRejectsInvalidLegacyProxySettings(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	settingBytes, err := common.Marshal(dto.ChannelSettings{
		Proxy: "socks5://proxy.example/legacy-path",
	})
	require.NoError(t, err)
	setting := string(settingBytes)
	origin := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		Name:    "legacy proxy channel",
		Key:     "test-key",
		Models:  "gpt-test",
		Group:   "default",
		Setting: &setting,
	}
	require.NoError(t, db.Create(origin).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", origin.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/copy", nil)

	CopyChannel(ctx)

	assert.Contains(t, recorder.Body.String(), "invalid channel settings")
	var channelCount int64
	require.NoError(t, db.Model(&model.Channel{}).Count(&channelCount).Error)
	assert.Equal(t, int64(1), channelCount)
}

func TestCopyChannelUsesCurrentSourceStateAndCopiesRouting(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	priority := int64(3)
	weight := uint(4)
	origin := &model.Channel{
		Type:      constant.ChannelTypeOpenAI,
		Name:      "current source",
		Key:       "test-key",
		Models:    "gpt-test",
		Group:     "default",
		Status:    common.ChannelStatusEnabled,
		Priority:  &priority,
		Weight:    &weight,
		Balance:   11,
		UsedQuota: 23,
	}
	require.NoError(t, origin.Insert())
	overridePriority := int64(9)
	require.NoError(t, model.PatchChannelModelOverrides([]model.ChannelModelOverridePatch{
		{ChannelId: origin.Id, Model: "gpt-test", Priority: &overridePriority},
	}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", origin.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/copy?suffix=-copy&reset_balance=true", nil)
	CopyChannel(ctx)

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Id int `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.NotZero(t, response.Data.Id)
	var clone model.Channel
	require.NoError(t, db.First(&clone, response.Data.Id).Error)
	assert.Equal(t, "current source-copy", clone.Name)
	assert.Zero(t, clone.Balance)
	assert.Zero(t, clone.UsedQuota)
	var override model.ChannelModelOverride
	require.NoError(t, db.Where("channel_id = ? AND model = ?", clone.Id, "gpt-test").First(&override).Error)
	require.NotNil(t, override.Priority)
	assert.Equal(t, int64(9), *override.Priority)
	var ability model.Ability
	require.NoError(t, db.Where("channel_id = ? AND model = ?", clone.Id, "gpt-test").First(&ability).Error)
	require.NotNil(t, ability.Priority)
	assert.Equal(t, int64(9), *ability.Priority)
}

func TestDeleteChannelResetsProxyCacheWhenPreReadFails(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.Log{}, &model.AuditLog{}, &model.ChannelAssetConfig{}, &model.UserAssetGroupReplica{}, &model.UserAssetReplica{},
	))
	service.ResetProxyClientCache()
	t.Cleanup(service.ResetProxyClientCache)

	proxyURL := "http://proxy.example:8080"
	beforeDelete, err := service.GetHttpClientWithProxy(proxyURL)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "999999"}}
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/channel/999999", nil)

	DeleteChannel(ctx)

	assert.Contains(t, recorder.Body.String(), `"success":true`)
	afterDelete, err := service.GetHttpClientWithProxy(proxyURL)
	require.NoError(t, err)
	assert.NotSame(t, beforeDelete, afterDelete)
}

func TestDeleteChannelBatchReportsAndAuditsActualDeletedCount(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.Log{}, &model.AuditLog{}, &model.ChannelAssetConfig{}, &model.UserAssetGroupReplica{}, &model.UserAssetReplica{},
	))
	channel := &model.Channel{Name: "existing", Key: "test-key"}
	require.NoError(t, db.Create(channel).Error)

	requestBody, err := common.Marshal(ChannelBatch{Ids: []int{channel.Id, 999999}})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/channel/batch", bytes.NewReader(requestBody))
	ctx.Request.Header.Set("Content-Type", "application/json")

	DeleteChannelBatch(ctx)

	var response struct {
		Success bool  `json:"success"`
		Data    int64 `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, int64(1), response.Data)

	var auditLog model.AuditLog
	require.NoError(t, db.Order("id desc").First(&auditLog).Error)
	var auditData struct {
		Operation struct {
			Params map[string]any `json:"params"`
		} `json:"op"`
	}
	encodedAudit, err := common.Marshal(auditLog.Other)
	require.NoError(t, err)
	require.NoError(t, common.Unmarshal(encodedAudit, &auditData))
	assert.Equal(t, float64(1), auditData.Operation.Params["count"])
}

func TestSettleTestQuotaUsesTieredBilling(t *testing.T) {
	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode:   "tiered_expr",
			ExprString:    `param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`,
			ExprHash:      billingexpr.ExprHashString(`param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`),
			GroupRatio:    1,
			EstimatedTier: "stream",
			QuotaPerUnit:  common.QuotaPerUnit,
			ExprVersion:   1,
		},
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{"stream":true}`),
		},
	}

	quota, result := settleTestQuota(info, types.PriceData{
		ModelRatio:      1,
		CompletionRatio: 2,
	}, &dto.Usage{
		PromptTokens: 1000,
	})

	require.Equal(t, 1500, quota)
	require.NotNil(t, result)
	require.Equal(t, "stream", result.MatchedTier)
}

func TestBuildTestLogOtherInjectsTieredInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode: "tiered_expr",
			ExprString:  `tier("base", p * 2)`,
		},
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
	priceData := types.PriceData{
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
	}
	usage := &dto.Usage{
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 12,
		},
	}

	requestRules := []billingexpr.RequestRuleTrace{{
		Cond:       `param("service_tier") == "fast"`,
		Multiplier: 2,
		Matched:    true,
	}}
	other := buildTestLogOther(ctx, info, priceData, usage, &billingexpr.TieredResult{
		MatchedTier:  "base",
		RequestRules: requestRules,
	})

	fields := other.Snapshot()
	require.Equal(t, "tiered_expr", fields["billing_mode"])
	require.Equal(t, "base", fields["matched_tier"])
	require.Equal(t, requestRules, fields["request_rules"])
	require.NotEmpty(t, fields["expr_b64"])
}

func TestResolveChannelTestUserIDUsesRequestUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("id", 2)

	userID, err := resolveChannelTestUserID(ctx)

	require.NoError(t, err)
	require.Equal(t, 2, userID)
}

func TestSelectChannelsForAutomaticTestPassiveRecoveryOnlyUsesAutoDisabled(t *testing.T) {
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusAutoDisabled},
		{Id: 3, Status: common.ChannelStatusManuallyDisabled},
		{Id: 4, Status: common.ChannelStatusEnabled, Key: "a\nb", ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeyStatusList: map[int]int{0: common.ChannelStatusAutoDisabled}}},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModePassiveRecovery)

	require.Len(t, selected, 2)
	require.Equal(t, 2, selected[0].Id)
	assert.Equal(t, 4, selected[1].Id)
}

func TestMultiKeyAutomaticRecoveryDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		for _, cache := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/cache_%t", dialect, cache), func(t *testing.T) {
				if dialect != "sqlite" && os.Getenv("TEST_"+strings.ToUpper(dialect)+"_DSN") == "" {
					t.Skip("isolated database DSN not configured")
				}
				t.Setenv("TEST_MANAGE_USER_DIALECT", dialect)
				initModelListColumnNames(t)
				db := setupManageUserTestDB(t)
				if dialect == "postgres" {
					// Initialize dialect-specific reserved-column quoting without migrations.
					t.Setenv("SQL_DSN", os.Getenv("TEST_POSTGRES_DSN"))
					previousMaster := common.IsMasterNode
					common.IsMasterNode = false
					require.NoError(t, model.InitDB())
					common.IsMasterNode = previousMaster
					connection, err := model.DB.DB()
					require.NoError(t, err)
					require.NoError(t, connection.Close())
					model.DB, model.LOG_DB = db, db
				}
				require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelOverride{}, &model.ModelRoutingPool{}, &model.UserChannelRoutingOverride{}, &model.UserGroupChannelRoutingOverride{}))
				withSelfUseModeEnabled(t)
				previousCache, previousEnable, previousConsumeLog := common.MemoryCacheEnabled, common.AutomaticEnableChannelEnabled, common.LogConsumeEnabled
				common.MemoryCacheEnabled, common.AutomaticEnableChannelEnabled = cache, true
				common.LogConsumeEnabled = false
				t.Cleanup(func() {
					common.MemoryCacheEnabled, common.AutomaticEnableChannelEnabled = previousCache, previousEnable
					common.LogConsumeEnabled = previousConsumeLog
				})
				root := model.User{Username: "key-recovery-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default", Quota: 1000000}
				require.NoError(t, db.Create(&root).Error)
				var requests []string
				var requestsMu sync.Mutex
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					key := r.Header.Get("Authorization")
					requestsMu.Lock()
					requests = append(requests, key)
					requestsMu.Unlock()
					w.Header().Set("Content-Type", "application/json")
					if key != "Bearer recovered" {
						w.WriteHeader(http.StatusUnauthorized)
						_, _ = io.WriteString(w, `{"error":{"message":"invalid key","type":"invalid_api_key"}}`)
						return
					}
					_, _ = io.WriteString(w, `{"id":"recovery","object":"chat.completion","model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
				}))
				t.Cleanup(upstream.Close)
				baseURL := upstream.URL
				channel := model.Channel{Type: constant.ChannelTypeOpenAI, Key: "recovered\nstill-bad\nmanual", BaseURL: &baseURL, Models: "gpt-4o-mini", Group: "default", Status: common.ChannelStatusAutoDisabled, ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 3, MultiKeyMode: constant.MultiKeyModePolling, MultiKeyStatusList: map[int]int{0: common.ChannelStatusAutoDisabled, 1: common.ChannelStatusAutoDisabled, 2: common.ChannelStatusManuallyDisabled}}}
				require.NoError(t, db.Create(&channel).Error)
				require.NoError(t, channel.AddAbilities(nil))
				if cache {
					model.InitChannelCache()
				}
				summary := testChannelForHealthCheck(context.Background(), &channel, root.Id, false, 10000000)
				assert.Equal(t, 2, summary.Tested)
				assert.Equal(t, 1, summary.Enabled)
				requestsMu.Lock()
				assert.Equal(t, []string{"Bearer recovered", "Bearer still-bad"}, requests)
				requests = nil
				requestsMu.Unlock()
				persisted, err := model.GetChannelById(channel.Id, true)
				require.NoError(t, err)
				assert.Equal(t, common.ChannelStatusEnabled, persisted.Status)
				assert.NotContains(t, persisted.ChannelInfo.MultiKeyStatusList, 0)
				assert.Equal(t, common.ChannelStatusAutoDisabled, persisted.ChannelInfo.MultiKeyStatusList[1])
				assert.Equal(t, common.ChannelStatusManuallyDisabled, persisted.ChannelInfo.MultiKeyStatusList[2])
				var ability model.Ability
				require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&ability).Error)
				assert.True(t, ability.Enabled)
				common.AutomaticEnableChannelEnabled = false
				testChannelForHealthCheck(context.Background(), persisted, root.Id, false, 10000000)
				requestsMu.Lock()
				assert.Empty(t, requests, "disabled passive recovery does not dispatch probes")
				requestsMu.Unlock()
				common.AutomaticEnableChannelEnabled = true
				summary = testChannelForHealthCheck(context.Background(), persisted, root.Id, false, 10000000)
				assert.Equal(t, 1, summary.Tested, "partially available channels still retry their disabled key")
				requestsMu.Lock()
				assert.Equal(t, []string{"Bearer still-bad"}, requests)
				requestsMu.Unlock()
				assert.False(t, model.EnableAutoDisabledChannel(channel.Id, "manual"))
				require.True(t, model.UpdateChannelStatus(channel.Id, "", common.ChannelStatusManuallyDisabled, "operator"))
				assert.False(t, model.EnableAutoDisabledChannel(channel.Id, "still-bad"), "late probe cannot override manual channel disable")
			})
		}
	}
}

func TestSelectChannelsForAutomaticTestScheduledSkipsManualDisabled(t *testing.T) {
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusAutoDisabled},
		{Id: 3, Status: common.ChannelStatusManuallyDisabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModeScheduledAll)

	require.Len(t, selected, 2)
	require.Equal(t, 1, selected[0].Id)
	require.Equal(t, 2, selected[1].Id)
}

func TestSelectChannelsForAutomaticTestAutoBanOnlyUsesEligibleChannels(t *testing.T) {
	autoBanEnabled := 1
	autoBanDisabled := 0
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled, AutoBan: &autoBanEnabled},
		{Id: 2, Status: common.ChannelStatusEnabled, AutoBan: &autoBanDisabled},
		{Id: 3, Status: common.ChannelStatusAutoDisabled, AutoBan: &autoBanEnabled},
		{Id: 4, Status: common.ChannelStatusManuallyDisabled, AutoBan: &autoBanEnabled},
		{Id: 5, Status: common.ChannelStatusEnabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModeAutoBanOnly)

	require.Len(t, selected, 2)
	require.Equal(t, 1, selected[0].Id)
	require.Equal(t, 3, selected[1].Id)
}

func TestRunChannelTestWorkersHonorsConfiguredConcurrency(t *testing.T) {
	originalInterval := common.RequestInterval
	common.RequestInterval = 0
	t.Cleanup(func() { common.RequestInterval = originalInterval })

	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusEnabled},
		{Id: 3, Status: common.ChannelStatusEnabled},
		{Id: 4, Status: common.ChannelStatusEnabled},
	}
	started := make(chan struct{}, len(channels))
	release := make(chan struct{})
	var active atomic.Int32
	var maxActive atomic.Int32
	progress := make([]int, 0, len(channels)+1)
	summaryResult := make(chan channelTestSummary, 1)

	go func() {
		summaryResult <- runChannelTestWorkers(
			context.Background(),
			channels,
			2,
			func(_ context.Context, _ *model.Channel) channelTestSummary {
				current := active.Add(1)
				defer active.Add(-1)
				for {
					observed := maxActive.Load()
					if current <= observed || maxActive.CompareAndSwap(observed, current) {
						break
					}
				}
				started <- struct{}{}
				<-release
				return channelTestSummary{Tested: 1, Succeeded: 1}
			},
			func(processed, _ int) {
				progress = append(progress, processed)
			},
		)
	}()

	<-started
	<-started
	select {
	case <-started:
		t.Fatal("started more channel tests than the configured concurrency")
	default:
	}
	close(release)

	summary := <-summaryResult

	assert.Equal(t, int32(2), maxActive.Load())
	assert.Equal(t, channelTestSummary{Tested: 4, Succeeded: 4}, summary)
	assert.Equal(t, []int{0, 1, 2, 3, 4}, progress)
}

func TestRunChannelTestWorkersStopsAfterCancellation(t *testing.T) {
	originalInterval := common.RequestInterval
	common.RequestInterval = 0
	t.Cleanup(func() { common.RequestInterval = originalInterval })

	ctx, cancel := context.WithCancel(context.Background())
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusEnabled},
		{Id: 3, Status: common.ChannelStatusEnabled},
		{Id: 4, Status: common.ChannelStatusEnabled},
	}
	started := make(chan struct{}, len(channels))
	progress := make([]int, 0, 1)
	summaryResult := make(chan channelTestSummary, 1)

	go func() {
		summaryResult <- runChannelTestWorkers(
			ctx,
			channels,
			2,
			func(ctx context.Context, _ *model.Channel) channelTestSummary {
				started <- struct{}{}
				<-ctx.Done()
				return channelTestSummary{Tested: 1, Succeeded: 1}
			},
			func(processed, _ int) {
				progress = append(progress, processed)
			},
		)
	}()

	<-started
	<-started
	cancel()

	summary := <-summaryResult

	select {
	case <-started:
		t.Fatal("started another channel test after cancellation")
	default:
	}
	assert.Equal(t, channelTestSummary{Tested: 2, Succeeded: 2}, summary)
	assert.Equal(t, []int{0}, progress)
}

func TestTestAllChannelsRejectsExistingActiveTask(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))

	existing, err := model.CreateSystemTask(model.SystemTaskTypeChannelTest, nil, nil)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/test", nil)

	TestAllChannels(ctx)

	require.Equal(t, http.StatusConflict, recorder.Code)
	require.Contains(t, recorder.Body.String(), existing.TaskID)
	require.Contains(t, recorder.Body.String(), "已有通道测试任务正在运行或等待中")
}
