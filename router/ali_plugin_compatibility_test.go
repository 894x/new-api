package router

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAlibabaPluginManagedAssetsEndToEnd(t *testing.T) {
	for _, path := range []string{"/api/v1/services/aigc/video-generation/video-synthesis", "/ali/api/v1/services/aigc/video-generation/video-synthesis", "/v1/videos"} {
		t.Run(path, func(t *testing.T) {
			f := setupAssetStorageE2E(t)
			source, err := builtinplugins.Source("alibaba")
			require.NoError(t, err)
			_, err = pluginruntime.DefaultRegistry.RegisterFactory(source, pluginruntime.Options{Key: "alibaba"})
			require.NoError(t, err)
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/v1/services/aigc/video-generation/video-synthesis", r.URL.Path)
				var payload struct {
					Model string `json:"model"`
					Input struct {
						Media []struct{ Type, URL string } `json:"media"`
					} `json:"input"`
				}
				if err := common.DecodeJson(r.Body, &payload); err != nil || len(payload.Input.Media) != 1 {
					http.Error(w, "invalid media payload", http.StatusBadRequest)
					return
				}
				assert.Equal(t, "wan3.0-video", payload.Model)
				assert.Equal(t, "first_frame", payload.Input.Media[0].Type)
				url := payload.Input.Media[0].URL
				if !strings.HasPrefix(url, "https://asset-e2e-123456.cos.ap-guangzhou.myqcloud.com/") {
					http.Error(w, "unmanaged asset reached provider", http.StatusBadRequest)
					return
				}
				response, err := http.Get(url)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadGateway)
					return
				}
				defer response.Body.Close()
				data, err := io.ReadAll(response.Body)
				if err != nil || response.StatusCode != http.StatusOK {
					http.Error(w, "provider cannot read retained original", http.StatusBadGateway)
					return
				}
				sum := sha256.Sum256(data)
				f.state.mu.Lock()
				f.state.requests = append(f.state.requests, assetE2ERequest{path: r.URL.Path, source: url, digest: hex.EncodeToString(sum[:])})
				n := len(f.state.requests)
				f.state.mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"output":{"task_id":"wan-asset-%d","task_status":"PENDING"}}`, n)
			}))
			t.Cleanup(provider.Close)
			channel := model.Channel{Type: constant.ChannelTypeAli, Name: "wan-asset-e2e", Key: "wan-key", Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(provider.URL), Models: "wan3.0-video", Group: "default"}
			require.NoError(t, model.DB.Create(&channel).Error)
			require.NoError(t, channel.AddAbilities(nil))
			model.InitChannelCache()
			media := map[string]any{"type": "first_frame", "url": f.source.URL}
			input := map[string]any{"prompt": "animate retained original", "media": []any{media}}
			parameters := map[string]any{"duration": 5, "resolution": "480P"}
			payload := map[string]any{"model": "wan3.0-video", "input": input, "parameters": parameters}
			if path == "/v1/videos" {
				payload = map[string]any{"model": "wan3.0-video", "prompt": "animate retained original", "metadata": map[string]any{"input": input, "parameters": parameters}}
			}
			status, headers, body := f.request(t, http.MethodPost, path, "assete2euserkey", payload)
			require.Equal(t, http.StatusOK, status, string(body))
			require.Contains(t, headers.Get("Content-Type"), "application/json")
			assetID := headers.Get("X-New-Api-Asset-Ids")
			require.NotEmpty(t, assetID)
			var task model.Task
			require.NoError(t, model.DB.Where("channel_id = ?", channel.Id).First(&task).Error)
			assert.Equal(t, constant.TaskPlatform("alibaba"), task.Platform)
			require.NotNil(t, task.PrivateData.AssetReferences)
			require.Len(t, task.PrivateData.AssetReferences.Items, 1)
			assert.Equal(t, assetID, task.PrivateData.AssetReferences.Items[0].AssetID)
			asset, err := model.GetUserAsset(f.user.Id, assetID)
			require.NoError(t, err)
			object, err := model.GetAssetStoredObject(f.user.Id, asset.StoredObjectId)
			require.NoError(t, err)
			f.state.mu.Lock()
			requests := append([]assetE2ERequest(nil), f.state.requests...)
			f.state.sourceOffline = true
			f.state.mu.Unlock()
			require.Len(t, requests, 1)
			assert.Equal(t, object.SHA256, requests[0].digest)
			media["url"] = "asset://" + assetID
			status, headers, body = f.request(t, http.MethodPost, path, "assete2euserkey", payload)
			require.Equal(t, http.StatusOK, status, string(body))
			assert.Equal(t, assetID, headers.Get("X-New-Api-Asset-Ids"))
			status, _, body = f.request(t, http.MethodPost, path, "assete2eotherkey", payload)
			assert.Equal(t, http.StatusForbidden, status, string(body))
			f.state.mu.Lock()
			assert.Len(t, f.state.requests, 2)
			assert.Equal(t, 1, f.state.puts)
			f.state.mu.Unlock()
		})
	}
}
