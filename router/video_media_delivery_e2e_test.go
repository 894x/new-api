package router

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	projecti18n "github.com/QuantumNous/new-api/i18n"
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

// Real HTTP gateway + authentication + database + COS SDK + provider adaptor.
// Only external providers are simulators. No routing/conversion functions are mocked.
func TestVideoMediaDeliveryE2E(t *testing.T) {
	require.NoError(t, projecti18n.Init())
	type channelSpec struct{ urlLimit, base64Limit int64 }
	for _, memoryCache := range []bool{false, true} {
		for _, tc := range []struct {
			name, input, output                            string
			channels                                       []channelSpec // descending priority
			wantChannel, status, heads, gets, puts         int
			retry, rangeProbe, unknownSize, inaccurateSize bool
		}{
			{name: "URL direct beats higher priority conversion", input: "url", output: "url", channels: []channelSpec{{0, 64}, {64, 0}}, wantChannel: 1, status: 200, heads: 1},
			{name: "Base64 direct beats higher priority upload", input: "base64", output: "base64", channels: []channelSpec{{64, 0}, {0, 64}}, wantChannel: 1, status: 200},
			{name: "URL to Base64 downloads identical videos once", input: "url", output: "base64", channels: []channelSpec{{0, 64}}, status: 200, heads: 1, gets: 1},
			{name: "Base64 to URL uploads identical videos once", input: "base64", output: "url", channels: []channelSpec{{64, 0}}, status: 200, puts: 1},
			{name: "Base64 limit switches to larger URL allowance", input: "base64", output: "url", channels: []channelSpec{{64, 8}}, status: 200, puts: 1},
			{name: "URL limit switches to larger Base64 allowance", input: "url", output: "base64", channels: []channelSpec{{8, 64}}, status: 200, heads: 1, gets: 1},
			{name: "Both limits reject before upstream", input: "url", channels: []channelSpec{{8, 8}, {8, 0}}, status: 400, heads: 1},
			{name: "Malformed Base64 rejected", input: "invalid", channels: []channelSpec{{64, 64}}, status: 400},
			{name: "Range probe is shared across channels", input: "url", output: "url", channels: []channelSpec{{0, 64}, {64, 0}}, wantChannel: 1, status: 200, heads: 1, gets: 1, rangeProbe: true},
			{name: "Unknown URL size can pass through", input: "url", output: "url", channels: []channelSpec{{64, 0}}, status: 200, heads: 1, gets: 1, unknownSize: true},
			{name: "Converted download reused after upstream retry", input: "url", output: "base64", channels: []channelSpec{{0, 64}, {0, 64}}, wantChannel: 1, status: 200, heads: 1, gets: 1, retry: true},
			{name: "Temporary upload reused after upstream retry", input: "base64", output: "url", channels: []channelSpec{{64, 0}, {64, 0}}, wantChannel: 1, status: 200, puts: 1, retry: true},
			{name: "Actual size reroutes without repeated download", input: "url", output: "base64", channels: []channelSpec{{0, 8}, {0, 64}}, wantChannel: 1, status: 200, heads: 1, gets: 1, inaccurateSize: true},
		} {
			t.Run(fmt.Sprintf("cache=%v/%s", memoryCache, tc.name), func(t *testing.T) {
				setupRelayRouterTestDB(t)
				require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelModelOverride{}, &model.Log{}, &model.UserSubscription{}, &model.RelayMediaObject{}))
				ratio_setting.InitRatioSettings()
				previousRatio := ratio_setting.ModelRatio2JSONString()
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"kimi-k3":1}`))
				fetch := system_setting.GetFetchSetting()
				settings := model_setting.GetGlobalSettings()
				previousFetch, previousMemory, previousPass, previousRetry, previousLimit := *fetch, common.MemoryCacheEnabled, settings.PassThroughRequestEnabled, common.RetryTimes, constant.MaxFileDownloadMB
				t.Cleanup(func() {
					*fetch = previousFetch
					common.MemoryCacheEnabled = previousMemory
					settings.PassThroughRequestEnabled = previousPass
					common.RetryTimes = previousRetry
					constant.MaxFileDownloadMB = previousLimit
					require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousRatio))
				})
				fetch.EnableSSRFProtection = false
				settings.PassThroughRequestEnabled = false
				common.MemoryCacheEnabled = memoryCache
				common.RetryTimes = 2
				constant.MaxFileDownloadMB = 64
				service.InitHttpClient()
				t.Setenv("ASSET_STORAGE_ENABLED", "true")
				t.Setenv("COS_BUCKET", "asset-e2e-123456")
				t.Setenv("COS_REGION", "ap-guangzhou")
				t.Setenv("COS_SECRET_ID", "e2e-id")
				t.Setenv("COS_SECRET_KEY", "e2e-secret")
				t.Setenv("COS_SESSION_TOKEN", "")
				state := &assetE2EState{objects: make(map[string]assetE2EObject)}
				cosServer := httptest.NewServer(http.HandlerFunc(state.serveCOS))
				t.Cleanup(cosServer.Close)
				cosURL, err := url.Parse(cosServer.URL)
				require.NoError(t, err)
				previousTransport := http.DefaultTransport
				http.DefaultTransport = assetE2ETransport{base: previousTransport, cosURL: cosURL}
				t.Cleanup(func() { http.DefaultTransport = previousTransport })
				video := []byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'm', 'p', '4', '2', 0, 0, 0, 0, 'm', 'p', '4', '2', 'i', 's', 'o', 'm'}
				encoded := "data:video/mp4;base64," + base64.StdEncoding.EncodeToString(video)
				var heads, gets atomic.Int32
				source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Empty(t, r.Header.Get("Authorization"))
					if r.Method == http.MethodHead {
						heads.Add(1)
						if tc.rangeProbe || tc.unknownSize {
							w.WriteHeader(405)
							return
						}
						size := len(video)
						if tc.inaccurateSize {
							size = 4
						}
						w.Header().Set("Content-Length", strconv.Itoa(size))
						return
					}
					gets.Add(1)
					if tc.rangeProbe && r.Header.Get("Range") != "" {
						w.Header().Set("Content-Range", "bytes 0-0/24")
						w.WriteHeader(206)
						_, _ = w.Write(video[:1])
						return
					}
					if tc.unknownSize {
						w.(http.Flusher).Flush()
					}
					_, _ = w.Write(video)
				}))
				t.Cleanup(source.Close)
				user := model.User{Username: "video-media-e2e", Status: common.UserStatusEnabled, Group: "default", Quota: 1_000_000}
				require.NoError(t, model.DB.Create(&user).Error)
				require.NoError(t, model.DB.Create(&model.Token{UserId: user.Id, Key: "parametercapabilitye2ekey", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}).Error)
				hits := make(chan int, 4)
				for index, spec := range tc.channels {
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						hits <- index
						if tc.retry && index == 0 {
							w.WriteHeader(500)
							_, _ = w.Write([]byte(`{"error":{"message":"retry","type":"server_error"}}`))
							return
						}
						body, err := io.ReadAll(r.Body)
						assert.NoError(t, err)
						for _, item := range gjson.GetBytes(body, "messages.0.content").Array() {
							value := item.Get("video_url.url").String()
							assert.Equal(t, 2.0, item.Get("video_url.fps").Float())
							if tc.output == "base64" {
								assert.Equal(t, encoded, value)
							} else if tc.input == "url" {
								assert.Equal(t, source.URL, value)
							} else {
								assert.Contains(t, value, "/relay-media/")
								assert.Contains(t, value, "q-signature=")
								response, err := http.Get(value)
								if assert.NoError(t, err) {
									data, err := io.ReadAll(response.Body)
									response.Body.Close()
									assert.NoError(t, err)
									assert.Equal(t, video, data)
								}
							}
						}
						writeKimiK3ChatE2EResponse(w, "media-e2e")
					}))
					t.Cleanup(upstream.Close)
					formats := map[string]dto.MediaFormatCapability{}
					for name, limit := range map[string]int64{"url": spec.urlLimit, "base64": spec.base64Limit} {
						formats[name] = dto.MediaFormatCapability{Supported: common.GetPointer(limit > 0)}
						if limit > 0 {
							formats[name] = dto.MediaFormatCapability{Supported: common.GetPointer(true), MaxMediaBytes: common.GetPointer(limit)}
						}
					}
					channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: fmt.Sprintf("video-%d", index), Key: "upstream-test-key", Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(upstream.URL), Models: "kimi-k3", Group: "default", Priority: common.GetPointer(int64(100 - index*10))}
					channel.SetOtherSettings(dto.ChannelOtherSettings{ParameterCapabilities: &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{"messages.*.content.*.video_url": {ParticipateInSelection: common.GetPointer(true), Media: &dto.MediaCapability{Kind: "video", Formats: formats, Conversions: dto.MediaConversions{URLToBase64: common.GetPointer(true), Base64ToURL: common.GetPointer(true)}}}}}})
					require.NoError(t, model.DB.Create(channel).Error)
					require.NoError(t, channel.AddAbilities(nil))
				}
				model.InitChannelCache()
				engine := gin.New()
				SetRelayRouter(engine)
				gateway := httptest.NewServer(engine)
				t.Cleanup(gateway.Close)
				input := source.URL
				if tc.input == "base64" {
					input = encoded
				}
				if tc.input == "invalid" {
					input = "data:video/mp4;base64,A!=="
				}
				body := []byte(`{"model":"kimi-k3","messages":[{"role":"user","content":[{"type":"video_url","video_url":{"url":"` + input + `","fps":2}},{"type":"video_url","video_url":{"url":"` + input + `","fps":2}}]}]}`)
				response := postParameterCapabilityE2ERequest(t, gateway.URL, body)
				result, err := io.ReadAll(response.Body)
				response.Body.Close()
				require.NoError(t, err)
				require.Equal(t, tc.status, response.StatusCode, string(result))
				assert.Equal(t, int32(tc.heads), heads.Load())
				assert.Equal(t, int32(tc.gets), gets.Load())
				if tc.status == 200 {
					if tc.retry {
						assert.Equal(t, 0, <-hits)
					}
					select {
					case got := <-hits:
						assert.Equal(t, tc.wantChannel, got)
					default:
						t.Error("upstream not called")
					}
				}
				assert.Empty(t, hits)
				state.mu.Lock()
				assert.Equal(t, tc.puts, state.puts)
				state.mu.Unlock()
				var ledgers []model.RelayMediaObject
				require.NoError(t, model.DB.Find(&ledgers).Error)
				assert.Len(t, ledgers, tc.puts)
				if tc.puts > 0 {
					require.NoError(t, service.DeleteExpiredRelayMedia(context.Background()))
					state.mu.Lock()
					assert.Len(t, state.objects, 1)
					state.mu.Unlock()
					require.NoError(t, model.DB.Model(&model.RelayMediaObject{}).Where("id = ?", ledgers[0].ID).Update("expires_at", 1).Error)
					require.NoError(t, service.DeleteExpiredRelayMedia(context.Background()))
					state.mu.Lock()
					assert.Empty(t, state.objects)
					state.mu.Unlock()
					var count int64
					require.NoError(t, model.DB.Model(&model.RelayMediaObject{}).Count(&count).Error)
					assert.Zero(t, count)
				}
				assert.False(t, bytes.Contains(result, []byte(encoded)), "response must not echo Base64")
				assert.False(t, strings.Contains(string(result), "e2e-secret"))
			})
		}
	}
}
