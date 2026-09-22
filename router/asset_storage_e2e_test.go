package router

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash/crc64"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	projecti18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests use the production routers, auth, SDK, persistence and video
// adaptor. Only the external source, COS and video/asset providers are local
// HTTP simulators. No storage service factory is replaced.
type assetE2EObject struct {
	data                []byte
	digest, contentType string
}
type assetE2ERequest struct{ path, source, digest string }
type assetE2EState struct {
	mu                     sync.Mutex
	objects                map[string]assetE2EObject
	puts, sourceGets       int
	source                 []byte
	sourceOffline, failPut bool
	requests               []assetE2ERequest
}

type assetE2ETransport struct {
	base   http.RoundTripper
	cosURL *url.URL
}

func (s assetE2ETransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Hostname() == "asset-e2e-123456.cos.ap-guangzhou.myqcloud.com" {
		copy := r.Clone(r.Context())
		copy.URL.Scheme, copy.URL.Host = s.cosURL.Scheme, s.cosURL.Host
		copy.Host = r.URL.Host
		return s.base.RoundTrip(copy)
	}
	if r.URL.Hostname() != "127.0.0.1" && r.URL.Hostname() != "localhost" {
		return nil, fmt.Errorf("E2E prohibits external network: %s", r.URL.Hostname())
	}
	return s.base.RoundTrip(r)
}

func (s *assetE2EState) serveCOS(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !strings.Contains(r.Header.Get("Authorization"), "q-signature=") && r.URL.Query().Get("q-signature") == "" {
		http.Error(w, "signature required", 403)
		return
	}
	if r.Method == http.MethodPut {
		if s.failPut {
			http.Error(w, "simulated upload failure", 503)
			return
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		digest := sha256.Sum256(data)
		if hex.EncodeToString(digest[:]) != r.Header.Get("x-cos-meta-sha256") {
			http.Error(w, "content digest mismatch", 400)
			return
		}
		s.objects[r.URL.Path] = assetE2EObject{data: data, digest: hex.EncodeToString(digest[:]), contentType: r.Header.Get("Content-Type")}
		s.puts++
		w.Header().Set("x-cos-hash-crc64ecma", strconv.FormatUint(crc64.Checksum(data, crc64.MakeTable(crc64.ECMA)), 10))
		return
	}
	if r.Method == http.MethodDelete {
		delete(s.objects, r.URL.Path)
		w.WriteHeader(204)
		return
	}
	object, exists := s.objects[r.URL.Path]
	if !exists {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", object.contentType)
	w.Header().Set("x-cos-meta-sha256", object.digest)
	http.ServeContent(w, r, "original", time.Time{}, bytes.NewReader(object.data))
}

func assetE2EPNG(t *testing.T, c color.RGBA) []byte {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, 512, 512))
	for y := 0; y < 512; y++ {
		for x := 0; x < 512; x++ {
			picture.SetRGBA(x, y, c)
		}
	}
	var data bytes.Buffer
	require.NoError(t, png.Encode(&data, picture))
	return data.Bytes()
}

type assetE2EFixture struct {
	gateway, source, upstream *httptest.Server
	state                     *assetE2EState
	user                      model.User
	adminToken                string
}

func setupAssetStorageE2E(t *testing.T) *assetE2EFixture {
	t.Helper()
	setupRelayRouterTestDB(t)
	require.NoError(t, projecti18n.Init())
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelAssetConfig{}, &model.UserAsset{}, &model.UserAssetGroup{}, &model.UserAssetReplica{}, &model.UserAssetGroupReplica{}, &model.AssetStoredObject{}, &model.AssetStorageAccount{}, &model.ChannelModelOverride{}, &model.Log{}, &model.Task{}, &model.UserSubscription{}, &model.Option{}, &model.UserSession{}, &model.AuthFlow{}, &model.TwoFA{}, &model.CustomOAuthProvider{}, &model.UserOAuthBinding{}, &model.Setup{}))
	sqlDB, err := model.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousCache, previousSetup := common.MemoryCacheEnabled, constant.Setup
	previousFetch := *system_setting.GetFetchSetting()
	previousQuota := system_setting.GetAssetStorageSetting().DefaultQuotaMB
	previousOptions := common.OptionMap
	common.MemoryCacheEnabled, constant.Setup = true, true
	system_setting.GetFetchSetting().AllowPrivateIp = true
	system_setting.GetFetchSetting().AllowedPorts = []string{"1-65535"}
	system_setting.GetAssetStorageSetting().DefaultQuotaMB = 1
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousCache
		constant.Setup = previousSetup
		*system_setting.GetFetchSetting() = previousFetch
		system_setting.GetAssetStorageSetting().DefaultQuotaMB = previousQuota
		common.OptionMap = previousOptions
	})
	ratio_setting.InitRatioSettings()
	model.InitOptionMap()
	service.InitHttpClient()
	t.Setenv("ASSET_STORAGE_ENABLED", "true")
	t.Setenv("COS_BUCKET", "asset-e2e-123456")
	t.Setenv("COS_REGION", "ap-guangzhou")
	t.Setenv("COS_SECRET_ID", "e2e-id")
	t.Setenv("COS_SECRET_KEY", "e2e-secret")
	t.Setenv("COS_SESSION_TOKEN", "")
	f := &assetE2EFixture{state: &assetE2EState{objects: make(map[string]assetE2EObject), source: assetE2EPNG(t, color.RGBA{R: 70, G: 140, B: 220, A: 255})}}
	cosServer := httptest.NewServer(http.HandlerFunc(f.state.serveCOS))
	t.Cleanup(cosServer.Close)
	cosURL, err := url.Parse(cosServer.URL)
	require.NoError(t, err)
	previousTransport := http.DefaultTransport
	http.DefaultTransport = assetE2ETransport{base: previousTransport, cosURL: cosURL}
	t.Cleanup(func() { http.DefaultTransport = previousTransport })
	f.source = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.state.mu.Lock()
		defer f.state.mu.Unlock()
		f.state.sourceGets++
		if f.state.sourceOffline {
			http.Error(w, "source has expired", 410)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(f.state.source)
	}))
	t.Cleanup(f.source.Close)
	f.upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		var digest string
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			if err := r.ParseMultipartForm(1024 * 1024); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			defer r.MultipartForm.RemoveAll()
			file, _, err := r.FormFile("input_reference")
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			defer file.Close()
			data, err := io.ReadAll(file)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			sum := sha256.Sum256(data)
			digest = hex.EncodeToString(sum[:])
		} else if r.Method == http.MethodPost {
			if err := common.DecodeJson(r.Body, &payload); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
		}
		var source string
		if content, ok := payload["content"].([]any); ok {
			for _, item := range content {
				part, _ := item.(map[string]any)
				imageURL, _ := part["image_url"].(map[string]any)
				if value, ok := imageURL["url"].(string); ok {
					source = value
				}
			}
		}
		if value, ok := payload["source_url"].(string); ok {
			source = value
		}
		if source != "" {
			if !strings.HasPrefix(source, "https://asset-e2e-123456.cos.ap-guangzhou.myqcloud.com/") {
				http.Error(w, "unmanaged reference reached upstream", 400)
				return
			}
			response, err := http.Get(source)
			if err != nil {
				http.Error(w, err.Error(), 502)
				return
			}
			data, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil || response.StatusCode != 200 {
				http.Error(w, "upstream cannot read original", 502)
				return
			}
			sum := sha256.Sum256(data)
			digest = hex.EncodeToString(sum[:])
		}
		f.state.mu.Lock()
		f.state.requests = append(f.state.requests, assetE2ERequest{path: r.URL.Path, source: source, digest: digest})
		n := len(f.state.requests)
		f.state.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/assets") {
			_, _ = io.WriteString(w, `{"success":true,"data":{"logical_id":"sls-e2e-original","logical_group_id":"sls-e2e-group","status":"Active"}}`)
			return
		}
		_, _ = fmt.Fprintf(w, `{"id":"upstream-e2e-%d"}`, n)
	}))
	t.Cleanup(f.upstream.Close)
	password, err := common.Password2Hash("Asset-E2E-Password-2026!")
	require.NoError(t, err)
	f.user = model.User{Username: "asset-e2e-admin", AffCode: "e2eadmin", Password: password, DisplayName: "Asset E2E", Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default", Quota: 10000000, AuthVersion: 1}
	require.NoError(t, model.DB.Create(&f.user).Error)
	require.NoError(t, model.DB.Create(&model.Token{UserId: f.user.Id, Key: "assete2euserkey", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}).Error)
	other := model.User{Username: "asset-e2e-other", AffCode: "e2eother", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", Quota: 1000000, AuthVersion: 1}
	require.NoError(t, model.DB.Create(&other).Error)
	require.NoError(t, model.DB.Create(&model.Token{UserId: other.Id, Key: "assete2eotherkey", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}).Error)
	bundle, err := service.CreateLoginSession(f.user.Id, "password", "127.0.0.1", "asset-e2e")
	require.NoError(t, err)
	f.adminToken = bundle.AccessToken
	channel := model.Channel{Type: constant.ChannelTypeDoubaoVideo, Name: "asset-e2e-video", Key: "e2e-upstream-key", Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(f.upstream.URL), Models: "doubao-seedance-2-0-260128", Group: "default"}
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	model.InitChannelCache()
	engine := gin.New()
	engine.Use(gin.Recovery(), middleware.BodyStorageCleanup())
	SetApiRouter(engine)
	SetVideoRouter(engine)
	SetTaskPluginProtocolRouter(engine)
	previousRegistry := pluginruntime.DefaultRegistry
	pluginruntime.DefaultRegistry = pluginruntime.NewRegistry()
	t.Cleanup(func() { pluginruntime.DefaultRegistry = previousRegistry })
	for _, key := range []string{"doubao", "sora"} {
		source, err := builtinplugins.Source(key)
		require.NoError(t, err)
		_, err = pluginruntime.DefaultRegistry.RegisterFactory(source, pluginruntime.Options{Key: key})
		require.NoError(t, err)
	}
	pluginDispatcher := SetPluginRouter(engine)
	// Serve the already built dashboard for the optional browser acceptance run.
	static := http.FileServer(http.Dir(filepath.Join("..", "web", "dist")))
	engine.NoRoute(pluginDispatcher, func(c *gin.Context) {
		if filepath.Ext(c.Request.URL.Path) != "" {
			static.ServeHTTP(c.Writer, c.Request)
			return
		}
		c.File(filepath.Join("..", "web", "dist", "index.html"))
	})
	f.gateway = httptest.NewServer(engine)
	t.Cleanup(f.gateway.Close)
	return f
}

func (f *assetE2EFixture) request(t *testing.T, method, path, token string, payload any) (int, http.Header, []byte) {
	t.Helper()
	var body io.Reader
	if payload != nil {
		data, err := common.Marshal(payload)
		require.NoError(t, err)
		body = bytes.NewReader(data)
	}
	r, err := http.NewRequest(method, f.gateway.URL+path, body)
	require.NoError(t, err)
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(r)
	require.NoError(t, err)
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, response.Header, data
}

func (f *assetE2EFixture) submit(t *testing.T, source, token string, expected int) (string, []byte) {
	t.Helper()
	status, headers, body := f.request(t, "POST", "/api/v3/contents/generations/tasks", token, map[string]any{"model": "doubao-seedance-2-0-260128", "content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": source}}, map[string]any{"type": "text", "text": "Animate retained original"}}, "duration": 4, "resolution": "480p", "ratio": "16:9"})
	require.Equal(t, expected, status, string(body))
	return headers.Get("X-New-Api-Asset-Ids"), body
}

func TestDoubaoPluginManagedAssetRoutesEndToEnd(t *testing.T) {
	for _, path := range []string{"/doubao/api/v3/contents/generations/tasks", "/v1/videos"} {
		t.Run(path, func(t *testing.T) {
			f := setupAssetStorageE2E(t)
			payload := map[string]any{
				"model": "doubao-seedance-2-0-260128", "duration": 4, "resolution": "480p",
				"content": []any{
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": f.source.URL}},
					map[string]any{"type": "text", "text": "Animate retained original"},
				},
			}
			status, headers, body := f.request(t, "POST", path, "assete2euserkey", payload)
			require.Equal(t, http.StatusOK, status, string(body))
			require.Contains(t, headers.Get("Content-Type"), "application/json")
			assetID := headers.Get("X-New-Api-Asset-Ids")
			require.NotEmpty(t, assetID)
			var response struct {
				ID string `json:"id"`
			}
			require.NoError(t, common.Unmarshal(body, &response))
			var task model.Task
			require.NoError(t, model.DB.Where("task_id = ?", response.ID).First(&task).Error)
			assert.Equal(t, constant.TaskPlatform("doubao"), task.Platform)
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
			assert.Equal(t, "/api/v3/contents/generations/tasks", requests[0].path)

			payload["content"].([]any)[0].(map[string]any)["image_url"].(map[string]any)["url"] = "asset://" + assetID
			status, headers, body = f.request(t, "POST", path, "assete2euserkey", payload)
			require.Equal(t, http.StatusOK, status, string(body))
			assert.Equal(t, assetID, headers.Get("X-New-Api-Asset-Ids"))
			status, _, body = f.request(t, "POST", path, "assete2eotherkey", payload)
			assert.Equal(t, http.StatusForbidden, status, string(body))
			f.state.mu.Lock()
			assert.Len(t, f.state.requests, 2)
			assert.Equal(t, 1, f.state.puts)
			f.state.mu.Unlock()
		})
	}
}

func TestAssetStorageUnifiedCustodyEndToEnd(t *testing.T) {
	f := setupAssetStorageE2E(t)
	id, body := f.submit(t, f.source.URL, "assete2euserkey", 200)
	require.NotEmpty(t, id, string(body))
	var public struct {
		ID string `json:"id"`
	}
	require.NoError(t, common.Unmarshal(body, &public))
	var task model.Task
	require.NoError(t, model.DB.Where("task_id = ?", public.ID).First(&task).Error)
	require.NotNil(t, task.PrivateData.AssetReferences)
	assert.Equal(t, id, task.PrivateData.AssetReferences.Items[0].AssetID)
	asset, err := model.GetUserAsset(f.user.Id, id)
	require.NoError(t, err)
	object, err := model.GetAssetStoredObject(f.user.Id, asset.StoredObjectId)
	require.NoError(t, err)
	f.state.mu.Lock()
	assert.Equal(t, 1, f.state.puts)
	require.Len(t, f.state.requests, 1)
	assert.Equal(t, object.SHA256, f.state.requests[0].digest)
	f.state.mu.Unlock()
	t.Log("PASS URL -> real gateway auth/routing -> COS SDK HTTP PUT -> upstream signed GET -> durable task reference")
	duplicate, _ := f.submit(t, f.source.URL, "assete2euserkey", 200)
	assert.Equal(t, id, duplicate)
	f.state.mu.Lock()
	assert.Equal(t, 1, f.state.puts)
	assert.Equal(t, 2, f.state.sourceGets)
	f.state.sourceOffline = true
	f.state.mu.Unlock()
	_, _ = f.submit(t, "asset://"+id, "assete2euserkey", 200)
	status, _, preview := f.request(t, "GET", "/api/asset-library/assets/"+id+"/content", "assete2euserkey", nil)
	require.Equal(t, 200, status, string(preview))
	assert.Equal(t, f.state.source, preview)
	f.state.mu.Lock()
	assert.Equal(t, 1, f.state.puts)
	assert.Equal(t, 2, f.state.sourceGets)
	f.state.mu.Unlock()
	t.Log("PASS repeated URL uploads once; asset URI still generates and previews after source expiry")
	status, _, _ = f.request(t, "GET", "/api/asset-library/assets/"+id+"/content", "assete2eotherkey", nil)
	assert.Equal(t, 404, status)
	_, _ = f.submit(t, "asset://"+id, "assete2eotherkey", 403)
	status, _, _ = f.request(t, "GET", "/api/asset-library/storage", "", nil)
	assert.Equal(t, 401, status)
	t.Log("PASS cross-account asset use/download and anonymous usage access rejected")
	status, _, body = f.request(t, "PUT", "/api/option/", f.adminToken, map[string]any{"key": "asset_storage_setting.default_quota_mb", "value": "0"})
	require.Equal(t, 200, status, string(body))
	require.Contains(t, string(body), `"success":true`)
	_, _ = f.submit(t, "asset://"+id, "assete2euserkey", 200)
	f.state.mu.Lock()
	f.state.sourceOffline = false
	f.state.source = assetE2EPNG(t, color.RGBA{R: 180, G: 50, B: 70, A: 255})
	beforeRequests := len(f.state.requests)
	f.state.mu.Unlock()
	_, body = f.submit(t, f.source.URL, "assete2euserkey", 400)
	assert.Contains(t, string(body), "quota")
	f.state.mu.Lock()
	assert.Equal(t, 1, f.state.puts)
	assert.Len(t, f.state.requests, beforeRequests)
	f.state.mu.Unlock()
	used, err := model.GetAssetStorageUsedBytes(f.user.Id)
	require.NoError(t, err)
	assert.Equal(t, object.FileSize, used)
	t.Log("PASS MB setting saved through admin API; zero quota keeps existing assets and blocks new upstream dispatch")
	_, _, body = f.request(t, "PUT", "/api/option/", f.adminToken, map[string]any{"key": "asset_storage_setting.default_quota_mb", "value": "1"})
	require.Contains(t, string(body), `"success":true`)
	f.state.mu.Lock()
	f.state.failPut = true
	f.state.mu.Unlock()
	_, _ = f.submit(t, f.source.URL, "assete2euserkey", 400)
	used, err = model.GetAssetStorageUsedBytes(f.user.Id)
	require.NoError(t, err)
	assert.Equal(t, object.FileSize, used)
	f.state.mu.Lock()
	f.state.failPut = false
	f.state.mu.Unlock()
	changed, _ := f.submit(t, f.source.URL, "assete2euserkey", 200)
	assert.NotEqual(t, id, changed)
	f.state.mu.Lock()
	assert.Equal(t, 2, f.state.puts)
	f.state.sourceOffline = true
	f.state.mu.Unlock()
	t.Log("PASS failed COS upload releases reservation; retry succeeds; changed content creates a new original")
	sls := model.Channel{Type: constant.ChannelTypeSeedanceSLS, Name: "asset-e2e-sync", Key: "key", Status: common.ChannelStatusEnabled}
	require.NoError(t, model.DB.Create(&sls).Error)
	require.NoError(t, model.DB.Create(&model.ChannelAssetConfig{ChannelId: sls.Id, Enabled: true, Backend: service.AssetLibraryBackendSeedanceSLS, BaseURL: f.upstream.URL, AuthType: service.AssetLibraryAuthBearer, APIKey: "key"}).Error)
	status, _, body = f.request(t, "POST", "/api/asset-library/admin/assets/"+id+"/sync", f.adminToken, map[string]any{"channel_ids": []int{sls.Id}})
	require.Equal(t, 200, status, string(body))
	require.Contains(t, string(body), `"success":true`)
	replica, err := model.GetUserAssetReplica(id, sls.Id)
	require.NoError(t, err)
	assert.Equal(t, model.AssetReplicaStateReady, replica.State)
	f.state.mu.Lock()
	last := f.state.requests[len(f.state.requests)-1]
	assert.Equal(t, object.SHA256, last.digest)
	assert.Equal(t, 2, f.state.puts)
	f.state.mu.Unlock()
	t.Log("PASS existing asset-library sync downloads the same COS original after external source expiry")
	if script := os.Getenv("ASSET_STORAGE_E2E_BROWSER_SCRIPT"); script != "" {
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
		defer cancel()
		command := exec.CommandContext(ctx, os.Getenv("ASSET_STORAGE_E2E_NODE"), script, f.gateway.URL, id)
		output, err := command.CombinedOutput()
		t.Log(string(output))
		require.NoError(t, err, "browser acceptance failed")
	}
}

// Preserve multipart file bytes through the real upload API, including quota
// and object deduplication. Video request multipart is covered separately.
func TestAssetStorageFileUploadEndToEnd(t *testing.T) {
	f := setupAssetStorageE2E(t)
	_, _, body := f.request(t, "POST", "/api/asset-library?Action=CreateAssetGroup&Version=2024-01-01", "assete2euserkey", map[string]any{"Name": "E2E uploads"})
	var group struct {
		Result struct {
			ID string `json:"Id"`
		}
	}
	require.NoError(t, common.Unmarshal(body, &group))
	require.NotEmpty(t, group.Result.ID, string(body))
	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	require.NoError(t, writer.WriteField("GroupId", group.Result.ID))
	require.NoError(t, writer.WriteField("Name", "File original"))
	require.NoError(t, writer.WriteField("AssetType", "Image"))
	file, err := writer.CreateFormFile("file", "original.png")
	require.NoError(t, err)
	_, err = file.Write(f.state.source)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	r, err := http.NewRequest("POST", f.gateway.URL+"/api/asset-library/upload", &payload)
	require.NoError(t, err)
	r.Header.Set("Authorization", "Bearer assete2euserkey")
	r.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := http.DefaultClient.Do(r)
	require.NoError(t, err)
	defer response.Body.Close()
	body, err = io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, 200, response.StatusCode, string(body))
	var result struct {
		Result struct {
			ID string `json:"Id"`
		}
	}
	require.NoError(t, common.Unmarshal(body, &result))
	require.NotEmpty(t, result.Result.ID, string(body))
	status, _, content := f.request(t, "GET", "/api/asset-library/assets/"+result.Result.ID+"/content", "assete2euserkey", nil)
	require.Equal(t, 200, status)
	assert.Equal(t, f.state.source, content)
	for _, byteRange := range []string{"bytes=3-12", "bytes=99999999-"} {
		request, err := http.NewRequest("GET", f.gateway.URL+"/api/asset-library/assets/"+result.Result.ID+"/content", nil)
		require.NoError(t, err)
		request.Header.Set("Authorization", "Bearer assete2euserkey")
		request.Header.Set("Range", byteRange)
		response, err := http.DefaultClient.Do(request)
		require.NoError(t, err)
		data, err := io.ReadAll(response.Body)
		response.Body.Close()
		require.NoError(t, err)
		if byteRange == "bytes=3-12" {
			assert.Equal(t, 206, response.StatusCode)
			assert.Equal(t, f.state.source[3:13], data)
		} else {
			assert.Equal(t, 416, response.StatusCode, string(data))
			assert.Equal(t, fmt.Sprintf("bytes */%d", len(f.state.source)), response.Header.Get("Content-Range"))
		}
	}
	t.Log("PASS authenticated multipart upload -> COS SDK -> owned content byte-for-byte")
	asset, err := model.GetUserAsset(f.user.Id, result.Result.ID)
	require.NoError(t, err)
	status, _, body = f.request(t, "POST", "/api/asset-library?Action=DeleteAsset&Version=2024-01-01", "assete2euserkey", map[string]any{"Id": asset.Id})
	require.Equal(t, 200, status, string(body))
	used, err := model.GetAssetStorageUsedBytes(f.user.Id)
	require.NoError(t, err)
	assert.Zero(t, used)
	status, _, _ = f.request(t, "GET", "/api/asset-library/assets/"+asset.Id+"/content", "assete2euserkey", nil)
	assert.Equal(t, 404, status)
	require.NoError(t, service.DeleteUnusedAssetContent(t.Context()))
	f.state.mu.Lock()
	assert.Len(t, f.state.objects, 1, "keep existing model download URLs during the grace period")
	f.state.mu.Unlock()
	require.NoError(t, model.DB.Model(&model.AssetStoredObject{}).Where("id = ?", asset.StoredObjectId).UpdateColumn("updated_time", time.Now().Unix()-8*86400).Error)
	require.NoError(t, service.DeleteUnusedAssetContent(t.Context()))
	f.state.mu.Lock()
	assert.Empty(t, f.state.objects)
	f.state.mu.Unlock()
	t.Log("PASS delete releases quota and revokes asset access; COS cleanup waits for the retention grace period")
}

func TestAssetStorageConcurrentMBQuotaEndToEnd(t *testing.T) {
	f := setupAssetStorageE2E(t)
	sources := make([]string, 0, 2)
	for _, pixel := range []color.RGBA{{R: 200, A: 255}, {B: 200, A: 255}} {
		data := assetE2EPNG(t, pixel)
		picture, err := png.Decode(bytes.NewReader(data))
		require.NoError(t, err)
		var uncompressed bytes.Buffer
		encoder := png.Encoder{CompressionLevel: png.NoCompression}
		require.NoError(t, encoder.Encode(&uncompressed, picture))
		require.Greater(t, uncompressed.Len(), 500000)
		require.Less(t, uncompressed.Len(), 1000000)
		sources = append(sources, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(uncompressed.Bytes()))
	}
	type outcome struct {
		status int
		body   []byte
		err    error
	}
	results := make(chan outcome, 2)
	start := make(chan struct{})
	for _, source := range sources {
		body, err := common.Marshal(map[string]any{"model": "doubao-seedance-2-0-260128", "content": []any{map[string]any{"type": "text", "text": "Animate this image"}, map[string]any{"type": "image_url", "image_url": map[string]any{"url": source}}}, "duration": 4, "resolution": "480p"})
		require.NoError(t, err)
		go func() {
			<-start
			request, err := http.NewRequest("POST", f.gateway.URL+"/api/v3/contents/generations/tasks", bytes.NewReader(body))
			if err != nil {
				results <- outcome{err: err}
				return
			}
			request.Header.Set("Authorization", "Bearer assete2euserkey")
			request.Header.Set("Content-Type", "application/json")
			response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
			if err != nil {
				results <- outcome{err: err}
				return
			}
			data, err := io.ReadAll(response.Body)
			response.Body.Close()
			results <- outcome{status: response.StatusCode, body: data, err: err}
		}()
	}
	close(start)
	succeeded, rejected := 0, 0
	for range sources {
		result := <-results
		require.NoError(t, result.err)
		if result.status == 200 {
			succeeded++
		} else {
			require.Equal(t, 400, result.status, string(result.body))
			require.Contains(t, string(result.body), "quota")
			rejected++
		}
	}
	assert.Equal(t, 1, succeeded)
	assert.Equal(t, 1, rejected)
	used, err := model.GetAssetStorageUsedBytes(f.user.Id)
	require.NoError(t, err)
	assert.Greater(t, used, int64(500000))
	assert.LessOrEqual(t, used, int64(1000000))
	f.state.mu.Lock()
	defer f.state.mu.Unlock()
	assert.Equal(t, 1, f.state.puts)
	assert.Len(t, f.state.requests, 1)
	t.Log("PASS two concurrent >0.5 MB imports with 1 MB quota admit exactly one upload and one video task")
}

func TestAssetStorageVideoMultipartEndToEnd(t *testing.T) {
	f := setupAssetStorageE2E(t)
	channel := model.Channel{Type: constant.ChannelTypeSora, Name: "asset-e2e-sora", Key: "e2e-sora", Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(f.upstream.URL), Models: "sora-2", Group: "default"}
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	model.InitChannelCache()
	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	for key, value := range map[string]string{"model": "sora-2", "prompt": "Animate this image", "seconds": "4"} {
		require.NoError(t, writer.WriteField(key, value))
	}
	file, err := writer.CreateFormFile("input_reference", "original.png")
	require.NoError(t, err)
	_, err = file.Write(f.state.source)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	request, err := http.NewRequest("POST", f.gateway.URL+"/v1/videos", &payload)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer assete2euserkey")
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, 200, response.StatusCode, string(body))
	assetID := response.Header.Get("X-New-Api-Asset-Ids")
	assert.Contains(t, response.Header.Get("Content-Type"), "application/json")
	require.NotEmpty(t, assetID)
	asset, err := model.GetUserAsset(f.user.Id, assetID)
	require.NoError(t, err)
	object, err := model.GetAssetStoredObject(f.user.Id, asset.StoredObjectId)
	require.NoError(t, err)
	var task model.Task
	require.NoError(t, model.DB.First(&task).Error)
	require.NotNil(t, task.PrivateData.AssetReferences)
	require.Len(t, task.PrivateData.AssetReferences.Items, 1)
	assert.Equal(t, assetID, task.PrivateData.AssetReferences.Items[0].AssetID)
	f.state.mu.Lock()
	defer f.state.mu.Unlock()
	assert.Equal(t, 1, f.state.puts)
	require.Len(t, f.state.requests, 1)
	assert.Equal(t, "/v1/videos", f.state.requests[0].path)
	assert.Equal(t, object.SHA256, f.state.requests[0].digest)
	t.Log("PASS multipart video retains original in COS and delivers identical bytes to Sora with durable task reference")
}

func TestAssetStoragePerUserQuotaEndToEnd(t *testing.T) {
	f := setupAssetStorageE2E(t)
	path := fmt.Sprintf("/api/asset-library/admin/users/%d/storage", f.user.Id)
	status, _, body := f.request(t, "GET", path, f.adminToken, nil)
	require.Equal(t, 200, status, string(body))
	require.Contains(t, string(body), `"quota_override_mb":null`)
	id, _ := f.submit(t, f.source.URL, "assete2euserkey", 200)
	status, _, body = f.request(t, "PUT", path, f.adminToken, map[string]any{"quota_mb": 0})
	require.Equal(t, 200, status, string(body))
	require.Contains(t, string(body), `"quota_mb":0`)
	_, _ = f.submit(t, "asset://"+id, "assete2euserkey", 200)
	f.state.mu.Lock()
	f.state.source = assetE2EPNG(t, color.RGBA{R: 240, A: 255})
	f.state.mu.Unlock()
	_, body = f.submit(t, f.source.URL, "assete2euserkey", 400)
	assert.Contains(t, string(body), "quota")
	status, _, body = f.request(t, "PUT", path, f.adminToken, map[string]any{"quota_mb": 2})
	require.Equal(t, 200, status, string(body))
	status, _, body = f.request(t, "PUT", "/api/option/", f.adminToken, map[string]any{"key": "asset_storage_setting.default_quota_mb", "value": "0"})
	require.Equal(t, 200, status, string(body))
	_, _ = f.submit(t, f.source.URL, "assete2euserkey", 200)
	status, _, body = f.request(t, "GET", "/api/asset-library/storage", "assete2euserkey", nil)
	require.Equal(t, 200, status)
	require.Contains(t, string(body), `"quota_mb":2`)
	_, _, otherUsage := f.request(t, "GET", "/api/asset-library/storage", "assete2eotherkey", nil)
	require.Contains(t, string(otherUsage), `"quota_mb":0`)
	for _, invalid := range []any{map[string]any{}, map[string]any{"quota_mb": -1}, map[string]any{"quota_mb": 0.5}, map[string]any{"quota_mb": 1000001}, map[string]any{"quota_mb": "2"}} {
		status, _, body = f.request(t, "PUT", path, f.adminToken, invalid)
		require.Equal(t, 400, status, string(body))
	}
	status, _, _ = f.request(t, "PUT", path, "", map[string]any{"quota_mb": 5})
	require.Equal(t, 401, status)
	var ordinary model.User
	require.NoError(t, model.DB.Where("username = ?", "asset-e2e-other").First(&ordinary).Error)
	ordinarySession, err := service.CreateLoginSession(ordinary.Id, "password", "127.0.0.1", "quota-test")
	require.NoError(t, err)
	status, _, _ = f.request(t, "PUT", path, ordinarySession.AccessToken, map[string]any{"quota_mb": 5})
	require.Equal(t, 403, status)
	admin := model.User{Username: "quota-admin", AffCode: "quotaadmin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, model.DB.Create(&admin).Error)
	adminSession, err := service.CreateLoginSession(admin.Id, "password", "127.0.0.1", "quota-admin")
	require.NoError(t, err)
	for _, protectedUser := range []int{f.user.Id, admin.Id} {
		status, _, _ = f.request(t, "PUT", fmt.Sprintf("/api/asset-library/admin/users/%d/storage", protectedUser), adminSession.AccessToken, map[string]any{"quota_mb": 5})
		require.Equal(t, 403, status)
	}
	status, _, body = f.request(t, "PUT", fmt.Sprintf("/api/asset-library/admin/users/%d/storage", ordinary.Id), adminSession.AccessToken, map[string]any{"quota_mb": 3})
	require.Equal(t, 200, status, string(body))
	status, _, _ = f.request(t, "PUT", "/api/asset-library/admin/users/999999/storage", f.adminToken, map[string]any{"quota_mb": 5})
	require.Equal(t, 404, status)
	used, err := model.GetAssetStorageUsedBytes(f.user.Id)
	require.NoError(t, err)
	status, _, body = f.request(t, "PUT", path, f.adminToken, map[string]any{"quota_mb": nil})
	require.Equal(t, 200, status, string(body))
	require.Contains(t, string(body), `"quota_override_mb":null`)
	require.Contains(t, string(body), `"quota_mb":0`)
	after, err := model.GetAssetStorageUsedBytes(f.user.Id)
	require.NoError(t, err)
	assert.Equal(t, used, after)
	t.Log("PASS per-user quota override, explicit zero, null inheritance, account isolation, validation and administrator role hierarchy")
}
