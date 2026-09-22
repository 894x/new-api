package router

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedanceSLSPluginManagedAssetsEndToEnd(t *testing.T) {
	for _, path := range []string{"/api/v3/contents/generations/tasks", "/seedance-sls/api/v3/contents/generations/tasks", "/v1/videos", "/v1/video/generations"} {
		t.Run(path, func(t *testing.T) {
			f := setupAssetStorageE2E(t)
			source, err := builtinplugins.Source("seedance-sls")
			require.NoError(t, err)
			_, err = pluginruntime.DefaultRegistry.RegisterFactory(source, pluginruntime.Options{Key: "seedance-sls"})
			require.NoError(t, err)
			var imports, polls, submits atomic.Int32
			var ready atomic.Bool
			uploadedDigests := make(chan string, 1)
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method + " " + r.URL.Path {
				case "POST /v1/volcengine/assets":
					assert.Equal(t, "Bearer library-only-key", r.Header.Get("Authorization"))
					var payload map[string]any
					if err := common.DecodeJson(r.Body, &payload); !assert.NoError(t, err) {
						http.Error(w, "invalid asset", 400)
						return
					}
					assert.Equal(t, "Image", payload["asset_type"])
					assert.NotEmpty(t, payload["group_name"], "SLS creates its first group with the asset")
					assert.NotContains(t, payload, "group_id")
					url, ok := payload["source_url"].(string)
					if !ok || !strings.HasPrefix(url, "https://asset-e2e-123456.cos.ap-guangzhou.myqcloud.com/") {
						http.Error(w, "unmanaged reference reached SLS", 400)
						return
					}
					response, err := http.Get(url)
					if !assert.NoError(t, err) {
						http.Error(w, "download failed", 502)
						return
					}
					defer response.Body.Close()
					data, err := io.ReadAll(response.Body)
					if !assert.NoError(t, err) || !assert.Equal(t, http.StatusOK, response.StatusCode) {
						http.Error(w, "invalid retained original", 502)
						return
					}
					digest := sha256.Sum256(data)
					uploadedDigests <- hex.EncodeToString(digest[:])
					imports.Add(1)
					_, _ = io.WriteString(w, `{"success":true,"data":{"logical_id":"lass_sls_image","logical_group_id":"lasg_sls_group","status":"Processing"}}`)
				case "GET /v1/volcengine/assets/lass_sls_image":
					assert.Equal(t, "Bearer library-only-key", r.Header.Get("Authorization"))
					polls.Add(1)
					ready.Store(true)
					_, _ = io.WriteString(w, `{"success":true,"data":{"logical_id":"lass_sls_image","logical_group_id":"lasg_sls_group","status":"Active","asset_type":"Image"}}`)
				case "POST /v1/video/generations":
					assert.Equal(t, "Bearer video-only-key", r.Header.Get("Authorization"))
					var payload map[string]any
					if err := common.DecodeJson(r.Body, &payload); !assert.NoError(t, err) {
						http.Error(w, "invalid video", 400)
						return
					}
					if !ready.Load() {
						http.Error(w, "asset not active", 409)
						return
					}
					content, ok := payload["content"].([]any)
					if !ok || !assert.Len(t, content, 2) {
						http.Error(w, "missing reference", 400)
						return
					}
					first := content[0].(map[string]any)
					assert.Equal(t, "asset://lass_sls_image", first["image_url"].(map[string]any)["url"])
					assert.Equal(t, "first_frame", first["role"])
					assert.Equal(t, false, payload["generate_audio"])
					_, _ = fmt.Fprintf(w, `{"task_id":"sls-asset-task-%d"}`, submits.Add(1))
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(provider.Close)
			channel := model.Channel{Type: constant.ChannelTypeSeedanceSLS, Name: "sls-assets", Key: "video-only-key", Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(provider.URL), Models: "doubao-seedance-2-0-260128", Group: "default", Priority: common.GetPointer(int64(100))}
			require.NoError(t, model.DB.Create(&channel).Error)
			require.NoError(t, channel.AddAbilities(nil))
			require.NoError(t, model.DB.Create(&model.ChannelAssetConfig{ChannelId: channel.Id, Enabled: true, Backend: service.AssetLibraryBackendSeedanceSLS, BaseURL: provider.URL, AuthType: service.AssetLibraryAuthBearer, APIKey: "library-only-key"}).Error)
			model.InitChannelCache()
			imageURL := map[string]any{"url": f.source.URL}
			payload := map[string]any{"model": "doubao-seedance-2-0-260128", "duration": 4, "resolution": "480p", "generate_audio": false, "content": []any{
				map[string]any{"type": "image_url", "image_url": imageURL, "role": "first_frame"}, map[string]any{"type": "text", "text": "Animate retained image"},
			}}
			status, headers, body := f.request(t, http.MethodPost, path, "assete2euserkey", payload)
			require.Equal(t, http.StatusOK, status, string(body))
			assetID := headers.Get("X-New-Api-Asset-Ids")
			require.NotEmpty(t, assetID)
			var task model.Task
			require.NoError(t, model.DB.Where("channel_id = ?", channel.Id).First(&task).Error)
			assert.Equal(t, constant.TaskPlatform("seedance-sls"), task.Platform)
			require.NotNil(t, task.PrivateData.AssetReferences)
			require.Len(t, task.PrivateData.AssetReferences.Items, 1)
			assert.Equal(t, assetID, task.PrivateData.AssetReferences.Items[0].AssetID)
			asset, err := model.GetUserAsset(f.user.Id, assetID)
			require.NoError(t, err)
			object, err := model.GetAssetStoredObject(f.user.Id, asset.StoredObjectId)
			require.NoError(t, err)
			assert.Equal(t, object.SHA256, <-uploadedDigests)
			replica, err := model.GetUserAssetReplica(assetID, channel.Id)
			require.NoError(t, err)
			assert.Equal(t, model.AssetReplicaStateReady, replica.State)
			assert.Equal(t, "lass_sls_image", replica.UpstreamAssetId)
			f.state.mu.Lock()
			f.state.sourceOffline = true
			f.state.mu.Unlock()
			imageURL["url"] = "asset://" + assetID
			status, headers, body = f.request(t, http.MethodPost, path, "assete2euserkey", payload)
			require.Equal(t, http.StatusOK, status, string(body))
			assert.Equal(t, assetID, headers.Get("X-New-Api-Asset-Ids"))
			status, _, body = f.request(t, http.MethodPost, path, "assete2eotherkey", payload)
			assert.Equal(t, http.StatusForbidden, status, string(body))
			assert.Equal(t, int32(1), imports.Load())
			assert.Positive(t, polls.Load())
			assert.Equal(t, int32(2), submits.Load())
			f.state.mu.Lock()
			assert.Equal(t, 1, f.state.puts)
			f.state.mu.Unlock()
		})
	}
}
