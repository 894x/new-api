package router

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageURLParameterCapabilityRoutesToSupportingChannelE2E(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.Channel{},
		&model.ChannelModelOverride{},
		&model.Log{},
		&model.UserSubscription{},
	))
	ratio_setting.InitRatioSettings()
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = originalMemoryCacheEnabled })

	var unsupportedRequests atomic.Int32
	unsupportedUpstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		unsupportedRequests.Add(1)
		writeOpenAIChatE2EResponse(writer)
	}))
	t.Cleanup(unsupportedUpstream.Close)

	var supportedRequests atomic.Int32
	supportedBodies := make(chan string, 1)
	supportedUpstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		supportedRequests.Add(1)
		body, _ := io.ReadAll(request.Body)
		supportedBodies <- string(body)
		writeOpenAIChatE2EResponse(writer)
	}))
	t.Cleanup(supportedUpstream.Close)

	user := model.User{Username: "parameter-capability-e2e", Status: common.UserStatusEnabled, Group: "default", Quota: 1_000_000}
	require.NoError(t, model.DB.Create(&user).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		UserId:         user.Id,
		Key:            "parametercapabilitye2ekey",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		UnlimitedQuota: true,
	}).Error)

	disabled := false
	participates := true
	highPriority := int64(100)
	lowPriority := int64(10)
	channels := []*model.Channel{
		{
			Type: constant.ChannelTypeOpenAI, Name: "image-url-unsupported", Key: "test-key",
			Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(unsupportedUpstream.URL),
			Models: "gpt-4o-mini", Group: "default", Priority: &highPriority,
		},
		{
			Type: constant.ChannelTypeOpenAI, Name: "image-url-supported", Key: "test-key",
			Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(supportedUpstream.URL),
			Models: "gpt-4o-mini", Group: "default", Priority: &lowPriority,
		},
	}
	channels[0].SetOtherSettings(dto.ChannelOtherSettings{ParameterCapabilities: &dto.ParameterCapabilityConfig{
		Defaults: map[string]dto.ParameterCapability{
			"messages.*.content.*.image_url": {
				Supported:              &disabled,
				ParticipateInSelection: &participates,
			},
		},
	}})
	for _, channel := range channels {
		require.NoError(t, model.DB.Create(channel).Error)
		require.NoError(t, channel.AddAbilities(nil))
	}
	model.InitChannelCache()

	engine := gin.New()
	SetRelayRouter(engine)
	textBody := []byte(`{
		"model":"gpt-4o-mini",
		"messages":[{"role":"user","content":"Hello"}]
	}`)
	textRecorder := serveParameterCapabilityE2ERequest(engine, textBody)
	require.Equal(t, http.StatusOK, textRecorder.Code, textRecorder.Body.String())

	imageBody := []byte(`{
		"model":"gpt-4o-mini",
		"messages":[{"role":"user","content":[
			{"type":"text","text":"What is in this image?"},
			{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="}}
		]}]
	}`)
	imageRecorder := serveParameterCapabilityE2ERequest(engine, imageBody)

	require.Equal(t, http.StatusOK, imageRecorder.Code, imageRecorder.Body.String())
	assert.Equal(t, int32(1), unsupportedRequests.Load())
	assert.Equal(t, int32(1), supportedRequests.Load())
	assert.Contains(t, <-supportedBodies, `"image_url"`)
}

func TestKimiK3VideoURLParameterCapabilityRoutesToSupportingChannelE2E(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.Channel{},
		&model.ChannelModelOverride{},
		&model.Log{},
		&model.UserSubscription{},
	))
	ratio_setting.InitRatioSettings()
	previousModelRatios := ratio_setting.ModelRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"kimi-k3":1}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousModelRatios)) })
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = originalMemoryCacheEnabled })

	var textOnlyRequests atomic.Int32
	textOnlyBodies := make(chan string, 1)
	textOnlyUpstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		textOnlyRequests.Add(1)
		body, _ := io.ReadAll(request.Body)
		textOnlyBodies <- string(body)
		writeKimiK3ChatE2EResponse(writer, "chatcmpl-text-only-channel")
	}))
	t.Cleanup(textOnlyUpstream.Close)

	var videoRequests atomic.Int32
	videoBodies := make(chan string, 1)
	videoUpstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		videoRequests.Add(1)
		body, _ := io.ReadAll(request.Body)
		videoBodies <- string(body)
		writeKimiK3ChatE2EResponse(writer, "chatcmpl-kimi-k3-video-channel")
	}))
	t.Cleanup(videoUpstream.Close)

	user := model.User{Username: "kimi-k3-video-capability-e2e", Status: common.UserStatusEnabled, Group: "default", Quota: 1_000_000}
	require.NoError(t, model.DB.Create(&user).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		UserId:         user.Id,
		Key:            "parametercapabilitye2ekey",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		UnlimitedQuota: true,
	}).Error)

	unsupported := false
	supported := true
	participates := true
	highPriority := int64(100)
	lowPriority := int64(10)
	channels := []*model.Channel{
		{
			Type: constant.ChannelTypeOpenAI, Name: "kimi-k3-text-only", Key: "test-key",
			Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(textOnlyUpstream.URL),
			Models: "kimi-k3", Group: "default", Priority: &highPriority,
		},
		{
			Type: constant.ChannelTypeOpenAI, Name: "kimi-k3-official-video-compatible", Key: "test-key",
			Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(videoUpstream.URL),
			Models: "kimi-k3", Group: "default", Priority: &lowPriority,
		},
	}
	channels[0].SetOtherSettings(dto.ChannelOtherSettings{ParameterCapabilities: &dto.ParameterCapabilityConfig{
		Defaults: map[string]dto.ParameterCapability{
			"messages.*.content.*.video_url": {
				Supported:              &unsupported,
				ParticipateInSelection: &participates,
			},
		},
	}})
	channels[1].SetOtherSettings(dto.ChannelOtherSettings{ParameterCapabilities: &dto.ParameterCapabilityConfig{
		Defaults: map[string]dto.ParameterCapability{
			"messages.*.content.*.video_url": {
				Supported:              &supported,
				ParticipateInSelection: &participates,
			},
		},
	}})
	for _, channel := range channels {
		require.NoError(t, model.DB.Create(channel).Error)
		require.NoError(t, channel.AddAbilities(nil))
	}
	model.InitChannelCache()

	engine := gin.New()
	SetRelayRouter(engine)
	gateway := httptest.NewServer(engine)
	t.Cleanup(gateway.Close)

	textResponse := postParameterCapabilityE2ERequest(t, gateway.URL, []byte(`{
		"model":"kimi-k3",
		"messages":[{"role":"user","content":"Hello"}]
	}`))
	require.Equal(t, http.StatusOK, textResponse.StatusCode)
	textResponseBody, err := io.ReadAll(textResponse.Body)
	require.NoError(t, err)
	require.NoError(t, textResponse.Body.Close())
	assert.Contains(t, string(textResponseBody), `"model":"kimi-k3"`)

	videoResponse := postParameterCapabilityE2ERequest(t, gateway.URL, []byte(`{
		"model":"kimi-k3",
		"messages":[{"role":"user","content":[
			{"type":"video_url","video_url":{"url":"ms://file-kimi-k3-video-e2e"}},
			{"type":"text","text":"Summarize this video."}
		]}]
	}`))
	require.Equal(t, http.StatusOK, videoResponse.StatusCode)
	videoResponseBody, err := io.ReadAll(videoResponse.Body)
	require.NoError(t, err)
	require.NoError(t, videoResponse.Body.Close())
	assert.Contains(t, string(videoResponseBody), `"model":"kimi-k3"`)

	assert.Equal(t, int32(1), textOnlyRequests.Load())
	assert.Equal(t, int32(1), videoRequests.Load())
	assert.NotContains(t, <-textOnlyBodies, `"video_url"`)
	videoBody := <-videoBodies
	assert.Contains(t, videoBody, `"type":"video_url"`)
	assert.Contains(t, videoBody, `"url":"ms://file-kimi-k3-video-e2e"`)
}

func TestVideoResolutionParameterCapabilitiesIgnoreRemovedLegacyConfigE2E(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.Channel{},
		&model.ChannelModelOverride{},
		&model.Log{},
		&model.Task{},
		&model.UserSubscription{},
	))
	ratio_setting.InitRatioSettings()
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = originalMemoryCacheEnabled })

	var highPriorityRequests atomic.Int32
	highPriorityUpstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		highPriorityRequests.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"task-high-priority"}`))
	}))
	t.Cleanup(highPriorityUpstream.Close)

	var compatibleRequests atomic.Int32
	compatibleBodies := make(chan string, 3)
	compatibleUpstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		compatibleRequests.Add(1)
		body, _ := io.ReadAll(request.Body)
		compatibleBodies <- string(body)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"task-parameter-compatible"}`))
	}))
	t.Cleanup(compatibleUpstream.Close)

	user := model.User{Username: "video-resolution-parameter-e2e", Status: common.UserStatusEnabled, Group: "default", Quota: 10_000_000}
	require.NoError(t, model.DB.Create(&user).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		UserId:         user.Id,
		Key:            "videoresolutionparametere2ekey",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		UnlimitedQuota: true,
	}).Error)

	highPriority := int64(100)
	lowPriority := int64(10)
	channels := []*model.Channel{
		{
			Type: constant.ChannelTypeDoubaoVideo, Name: "legacy-says-1080-parameter-says-720", Key: "test-key",
			Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(highPriorityUpstream.URL),
			Models: "doubao-seedance-2-0-260128", Group: "default", Priority: &highPriority,
			OtherSettings: `{
				"video_capabilities":{"models":{"doubao-seedance-2-0-260128":{"resolutions":["1080p"]}}},
				"parameter_capabilities":{"defaults":{
					"size":{"allowed_values":["1280x720"],"participate_in_selection":true},
					"resolution":{"allowed_values":["720p"],"participate_in_selection":true},
					"metadata.resolution":{"allowed_values":["720p"],"participate_in_selection":true}
				}}
			}`,
		},
		{
			Type: constant.ChannelTypeDoubaoVideo, Name: "legacy-says-720-parameter-says-1080", Key: "test-key",
			Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(compatibleUpstream.URL),
			Models: "doubao-seedance-2-0-260128", Group: "default", Priority: &lowPriority,
			OtherSettings: `{
				"video_capabilities":{"models":{"doubao-seedance-2-0-260128":{"resolutions":["720p"]}}},
				"parameter_capabilities":{"defaults":{
					"size":{"allowed_values":["1920x1080"],"participate_in_selection":true},
					"resolution":{"allowed_values":["1080p"],"participate_in_selection":true},
					"metadata.resolution":{"allowed_values":["1080p"],"participate_in_selection":true}
				}}
			}`,
		},
	}
	for _, channel := range channels {
		require.NoError(t, model.DB.Create(channel).Error)
		require.NoError(t, channel.AddAbilities(nil))
	}
	model.InitChannelCache()

	engine := gin.New()
	SetVideoRouter(engine)
	gateway := httptest.NewServer(engine)
	t.Cleanup(gateway.Close)

	requests := []string{
		`{"model":"doubao-seedance-2-0-260128","prompt":"size path","size":"1920x1080","duration":4}`,
		`{"model":"doubao-seedance-2-0-260128","prompt":"resolution path","resolution":"1080p","duration":4}`,
		`{"model":"doubao-seedance-2-0-260128","prompt":"metadata path","metadata":{"resolution":"1080p"},"duration":4}`,
	}
	for _, body := range requests {
		request, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/video/generations", bytes.NewBufferString(body))
		require.NoError(t, err)
		request.Header.Set("Authorization", "Bearer videoresolutionparametere2ekey")
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		require.NoError(t, err)
		responseBody, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		require.Equal(t, http.StatusOK, response.StatusCode, string(responseBody))
	}

	assert.Zero(t, highPriorityRequests.Load())
	assert.Equal(t, int32(3), compatibleRequests.Load())
	for range requests {
		assert.NotEmpty(t, <-compatibleBodies)
	}
}

func serveParameterCapabilityE2ERequest(engine *gin.Engine, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer parametercapabilitye2ekey")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder
}

func postParameterCapabilityE2ERequest(t *testing.T, gatewayURL string, body []byte) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, gatewayURL+"/v1/chat/completions", bytes.NewReader(body))
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer parametercapabilitye2ekey")
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	return response
}

func writeOpenAIChatE2EResponse(writer http.ResponseWriter) {
	writer.Header().Set("Content-Type", "application/json")
	_, _ = writer.Write([]byte(`{
		"id":"chatcmpl-parameter-capability-e2e",
		"object":"chat.completion",
		"created":1,
		"model":"gpt-4o-mini",
		"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
	}`))
}

func writeKimiK3ChatE2EResponse(writer http.ResponseWriter, id string) {
	writer.Header().Set("Content-Type", "application/json")
	_, _ = writer.Write([]byte(`{
		"id":"` + id + `",
		"object":"chat.completion",
		"created":1,
		"model":"kimi-k3",
		"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
	}`))
}
