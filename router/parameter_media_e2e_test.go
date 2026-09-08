package router

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestParameterMediaConversionsReachUpstreamE2E(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelOverride{}, &model.Log{}, &model.UserSubscription{}))
	ratio_setting.InitRatioSettings()
	fetch := system_setting.GetFetchSetting()
	settings := model_setting.GetGlobalSettings()
	originalFetch, originalPass, originalLimit, originalMemory := *fetch, settings.PassThroughRequestEnabled, constant.MaxFileDownloadMB, common.MemoryCacheEnabled
	t.Cleanup(func() {
		*fetch = originalFetch
		settings.PassThroughRequestEnabled = originalPass
		constant.MaxFileDownloadMB = originalLimit
		common.MemoryCacheEnabled = originalMemory
	})
	fetch.EnableSSRFProtection = false
	settings.PassThroughRequestEnabled = false
	constant.MaxFileDownloadMB = 64
	common.MemoryCacheEnabled = true
	service.InitHttpClient()
	var pngData bytes.Buffer
	require.NoError(t, png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 1, 1))))
	wavData := []byte{'R', 'I', 'F', 'F', 38, 0, 0, 0, 'W', 'A', 'V', 'E', 'f', 'm', 't', ' ', 16, 0, 0, 0, 1, 0, 1, 0, 0x40, 0x1f, 0, 0, 0x80, 0x3e, 0, 0, 2, 0, 16, 0, 'd', 'a', 't', 'a', 2, 0, 0, 0, 0, 0}
	videoData := []byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'm', 'p', '4', '2', 0, 0, 0, 0, 'm', 'p', '4', '2', 'i', 's', 'o', 'm'}
	media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/image":
			_, _ = w.Write(pngData.Bytes())
		case "/audio":
			_, _ = w.Write(wavData)
		case "/video":
			_, _ = w.Write(videoData)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(media.Close)
	bodies := make(chan []byte, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies <- body
		writeOpenAIChatE2EResponse(w)
	}))
	t.Cleanup(upstream.Close)
	user := model.User{Username: "media-e2e", Status: common.UserStatusEnabled, Group: "default", Quota: 1_000_000}
	require.NoError(t, model.DB.Create(&user).Error)
	require.NoError(t, model.DB.Create(&model.Token{UserId: user.Id, Key: "parametercapabilitye2ekey", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}).Error)
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "media-conversions", Key: "test-key", Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(upstream.URL), Models: "gpt-4o-mini", Group: "default"}
	channel.SetOtherSettings(dto.ChannelOtherSettings{ParameterCapabilities: &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{
		"messages.*.content.*.image_url":   {Transform: dto.ParameterTransformImage},
		"messages.*.content.*.input_audio": {Transform: dto.ParameterTransformAudio},
		"messages.*.content.*.video_url":   {Transform: dto.ParameterTransformVideo},
	}}})
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	model.InitChannelCache()
	engine := gin.New()
	SetRelayRouter(engine)
	body := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"` + media.URL + `/image","detail":"low"}},{"type":"input_audio","input_audio":{"data":"` + media.URL + `/audio","format":"wav"}},{"type":"video_url","video_url":{"url":"` + media.URL + `/video"}}]}]}`)
	recorder := serveParameterCapabilityE2ERequest(engine, body)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	select {
	case sent := <-bodies:
		assert.Equal(t, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(pngData.Bytes()), gjson.GetBytes(sent, "messages.0.content.0.image_url.url").String())
		assert.Equal(t, "low", gjson.GetBytes(sent, "messages.0.content.0.image_url.detail").String())
		assert.Equal(t, base64.StdEncoding.EncodeToString(wavData), gjson.GetBytes(sent, "messages.0.content.1.input_audio.data").String())
		assert.Equal(t, "wav", gjson.GetBytes(sent, "messages.0.content.1.input_audio.format").String())
		assert.Equal(t, "data:video/mp4;base64,"+base64.StdEncoding.EncodeToString(videoData), gjson.GetBytes(sent, "messages.0.content.2.video_url.url").String())
	default:
		t.Fatal("upstream did not receive request")
	}
	settings.PassThroughRequestEnabled = true
	conflict := serveParameterCapabilityE2ERequest(engine, body)
	assert.Equal(t, http.StatusBadRequest, conflict.Code, conflict.Body.String())
	// Public error details follow the gateway's existing channel-error redaction.
	assert.Empty(t, bodies, "conflicting request must not reach upstream")
}
