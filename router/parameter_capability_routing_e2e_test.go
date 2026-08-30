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

func serveParameterCapabilityE2ERequest(engine *gin.Engine, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer parametercapabilitye2ekey")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder
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
