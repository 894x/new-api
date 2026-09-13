package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingAssetStore struct {
	uploads int
	fail    bool
	files   map[string][]byte
}

func (s *recordingAssetStore) Put(_ context.Context, object *model.AssetStoredObject, body io.Reader) error {
	s.uploads++
	if s.fail {
		return errors.New("object storage unavailable")
	}
	data, err := io.ReadAll(body)
	s.files[object.ObjectKey] = data
	return err
}
func (s *recordingAssetStore) Sign(_ context.Context, object *model.AssetStoredObject) (string, error) {
	return "https://" + object.Bucket + ".cos." + object.Region + ".myqcloud.com/" + object.ObjectKey + "?q-signature=test", nil
}
func (s *recordingAssetStore) Get(_ context.Context, object *model.AssetStoredObject, _ string) (*http.Response, error) {
	data := s.files[object.ObjectKey]
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{object.ContentType}}, ContentLength: int64(len(data)), Body: io.NopCloser(bytes.NewReader(data))}, nil
}

func (s *recordingAssetStore) Delete(_ context.Context, object *model.AssetStoredObject) error {
	if s.fail {
		return errors.New("deletion unavailable")
	}
	delete(s.files, object.ObjectKey)
	return nil
}

func TestStoredAssetCleanupRespectsGraceRetriesAndReimport(t *testing.T) {
	store := setupStoredAssetTest(t)
	asset := &model.UserAsset{Id: "asset-na-cleanup", UserId: 1, AssetType: "Image", SourceURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(storedAssetPNG(t, color.RGBA{R: 120, A: 255}))}
	require.NoError(t, ImportAssetContent(context.Background(), asset))
	require.NoError(t, model.CreateUserAsset(asset))
	require.NoError(t, model.DeleteUserAsset(1, asset.Id))
	require.NoError(t, DeleteUnusedAssetContent(context.Background()))
	assert.Len(t, store.files, 1, "recent task download URLs must remain usable")
	require.NoError(t, model.DB.Model(&model.AssetStoredObject{}).Where("id = ?", asset.StoredObjectId).UpdateColumn("updated_time", time.Now().Unix()-8*86400).Error)
	store.fail = true
	require.Error(t, DeleteUnusedAssetContent(context.Background()))
	object, err := model.GetAssetStoredObject(1, asset.StoredObjectId)
	require.NoError(t, err)
	assert.Equal(t, "deleting", object.State)
	store.fail = false
	require.NoError(t, model.DB.Model(&model.AssetStoredObject{}).Where("id = ?", object.Id).UpdateColumn("lease_until", time.Now().Unix()-1).Error)
	require.NoError(t, DeleteUnusedAssetContent(context.Background()))
	assert.Empty(t, store.files)
	asset.StoredObjectId = ""
	require.NoError(t, ImportAssetContent(context.Background(), asset))
	assert.Equal(t, 2, store.uploads)
}

func TestStoredAssetRefreshDoesNotRequireUpstreamReplica(t *testing.T) {
	setupStoredAssetTest(t)
	asset := &model.UserAsset{Id: "asset-na-original-only", UserId: 1, AssetType: "Image"}
	require.NoError(t, ImportAssetFile(t.Context(), asset, bytes.NewReader(storedAssetPNG(t, color.RGBA{R: 120, A: 255}))))
	require.NoError(t, model.CreateUserAsset(asset))
	details, err := RefreshAssetLibraryAsset(t.Context(), asset.Id)
	require.NoError(t, err)
	assert.Nil(t, details, "original-only assets have no supplier details to refresh")
	asset.StoredObjectId = ""
	require.NoError(t, model.DB.Model(asset).Update("stored_object_id", "").Error)
	_, err = RefreshAssetLibraryAsset(t.Context(), asset.Id)
	require.ErrorContains(t, err, "no available upstream replica")
}

func setupStoredAssetTest(t *testing.T) *recordingAssetStore {
	t.Helper()
	db := setupAssetLibraryServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.AssetStoredObject{}, &model.AssetStorageAccount{}))
	InitHttpClient()
	t.Setenv("ASSET_STORAGE_ENABLED", "true")
	t.Setenv("COS_BUCKET", "references-123456")
	t.Setenv("COS_REGION", "ap-guangzhou")
	t.Setenv("COS_SECRET_ID", "test-id")
	t.Setenv("COS_SECRET_KEY", "test-key")
	t.Setenv("COS_SESSION_TOKEN", "")
	t.Setenv("ASSET_STORAGE_URL_TTL_SECONDS", "86400")
	store := &recordingAssetStore{files: map[string][]byte{}}
	previous := assetObjectStoreFactory
	assetObjectStoreFactory = func(system_setting.AssetStorageConfig) assetObjectStore { return store }
	t.Cleanup(func() { assetObjectStoreFactory = previous })
	return store
}

func storedAssetPNG(t *testing.T, pixel color.RGBA) []byte {
	t.Helper()
	frame := image.NewRGBA(image.Rect(0, 0, 512, 512))
	frame.SetRGBA(0, 0, pixel)
	var buffer bytes.Buffer
	require.NoError(t, png.Encode(&buffer, frame))
	return buffer.Bytes()
}

func TestStoredAssetContentDeduplicatesWithinAccountAndPreservesBytes(t *testing.T) {
	store := setupStoredAssetTest(t)
	data := storedAssetPNG(t, color.RGBA{R: 255, A: 255})
	first := &model.UserAsset{UserId: 7, AssetType: "Image"}
	second := &model.UserAsset{UserId: 7, AssetType: "Image"}
	other := &model.UserAsset{UserId: 8, AssetType: "Image"}
	require.NoError(t, ImportAssetFile(t.Context(), first, bytes.NewReader(data)))
	require.NoError(t, ImportAssetFile(t.Context(), second, bytes.NewReader(data)))
	assert.Equal(t, first.StoredObjectId, second.StoredObjectId)
	assert.Equal(t, 1, store.uploads)
	require.NoError(t, ImportAssetFile(t.Context(), other, bytes.NewReader(data)))
	assert.NotEqual(t, first.StoredObjectId, other.StoredObjectId)
	assert.Equal(t, 2, store.uploads)
	response, err := ReadAssetContent(t.Context(), first, "")
	require.NoError(t, err)
	defer response.Body.Close()
	actual, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Equal(t, data, actual)
	assert.Equal(t, 512, first.Width)
}

func TestStoredAssetFailureIsRetryableAndNeverReady(t *testing.T) {
	store := setupStoredAssetTest(t)
	data := storedAssetPNG(t, color.RGBA{B: 255, A: 255})
	asset := &model.UserAsset{UserId: 7, AssetType: "Image"}
	store.fail = true
	require.Error(t, ImportAssetFile(t.Context(), asset, bytes.NewReader(data)))
	assert.Empty(t, asset.StoredObjectId)
	var object model.AssetStoredObject
	require.NoError(t, model.DB.First(&object).Error)
	assert.Equal(t, "failed", object.State)
	store.fail = false
	require.NoError(t, ImportAssetFile(t.Context(), asset, bytes.NewReader(data)))
	ready, err := model.GetAssetStoredObject(7, asset.StoredObjectId)
	require.NoError(t, err)
	assert.Equal(t, "ready", ready.State)
	assert.Equal(t, object.Id, ready.Id)
	assert.Equal(t, 2, store.uploads)
}

func TestStoredVideoReferencesReuseSnapshotsAndEnforceOwnership(t *testing.T) {
	store := setupStoredAssetTest(t)
	source := "data:image/png;base64," + base64.StdEncoding.EncodeToString(storedAssetPNG(t, color.RGBA{G: 255, A: 255}))
	payload := map[string]any{"prompt": "keep https://prompt.example/image.png", "callback_url": "https://callback.example/hook", "metadata": map[string]any{"input": map[string]any{"media": []any{map[string]any{"type": "image", "url": source}}}}}
	snapshot := ManagedAssetReferences{}
	result, err := StoreVideoAssetReferences(t.Context(), 7, payload, snapshot)
	require.NoError(t, err)
	assert.Equal(t, payload["prompt"], result["prompt"])
	assert.Equal(t, payload["callback_url"], result["callback_url"])
	_, err = StoreVideoAssetReferences(t.Context(), 7, payload, snapshot)
	require.NoError(t, err)
	assert.Equal(t, 1, store.uploads)
	require.Len(t, snapshot, 1)
	var asset *model.UserAsset
	for _, entry := range snapshot {
		asset = entry
	}
	require.NotNil(t, asset)
	managedURL, err := AssetContentURL(t.Context(), asset)
	require.NoError(t, err)
	_, err = StoreVideoAssetReferences(t.Context(), 7, map[string]any{"image": managedURL}, ManagedAssetReferences{})
	require.NoError(t, err)
	assert.Equal(t, 1, store.uploads)
	_, err = StoreVideoAssetReferences(t.Context(), 8, map[string]any{"image": "asset://" + asset.Id}, ManagedAssetReferences{})
	require.Error(t, err)
	_, err = StoreVideoAssetReferences(t.Context(), 8, map[string]any{"image": managedURL}, ManagedAssetReferences{})
	require.Error(t, err)
	assert.Equal(t, 1, store.uploads)
}

func TestStoredReferenceURLContentChangesCreateNewOriginal(t *testing.T) {
	store := setupStoredAssetTest(t)
	data := storedAssetPNG(t, color.RGBA{R: 255, A: 255})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(data) }))
	defer server.Close()
	previous := *system_setting.GetFetchSetting()
	system_setting.GetFetchSetting().AllowPrivateIp = true
	system_setting.GetFetchSetting().AllowedPorts = []string{"1-65535"}
	t.Cleanup(func() { *system_setting.GetFetchSetting() = previous })
	first, err := importVideoReference(t.Context(), 7, server.URL, "Image")
	require.NoError(t, err)
	duplicate, err := importVideoReference(t.Context(), 7, server.URL, "Image")
	require.NoError(t, err)
	assert.Equal(t, first.Id, duplicate.Id)
	assert.Equal(t, 1, store.uploads)
	data = storedAssetPNG(t, color.RGBA{B: 255, A: 255})
	changed, err := importVideoReference(t.Context(), 7, server.URL, "Image")
	require.NoError(t, err)
	assert.NotEqual(t, first.Id, changed.Id)
	assert.Equal(t, 2, store.uploads)
}

func TestStoredOriginalSuppliesExistingChannelSynchronization(t *testing.T) {
	store := setupStoredAssetTest(t)
	source := "data:image/png;base64," + base64.StdEncoding.EncodeToString(storedAssetPNG(t, color.RGBA{R: 255, A: 255}))
	asset, err := importVideoReference(t.Context(), 7, source, "Image")
	require.NoError(t, err)
	var upstreamURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, common.DecodeJson(r.Body, &body))
		upstreamURL, _ = body["source_url"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"logical_id":"upstream-test","logical_group_id":"group-test","status":"Active"}}`))
	}))
	defer server.Close()
	require.NoError(t, model.DB.Create(&model.Channel{Id: 17, Type: constant.ChannelTypeSeedanceSLS, Key: "key"}).Error)
	require.NoError(t, model.DB.Create(&model.ChannelAssetConfig{ChannelId: 17, Enabled: true, Backend: AssetLibraryBackendSeedanceSLS, BaseURL: server.URL, AuthType: AssetLibraryAuthBearer, APIKey: "key"}).Error)
	report, err := ReplicateAsset(t.Context(), asset)
	require.NoError(t, err)
	assert.Empty(t, report.Errors)
	assert.True(t, strings.HasPrefix(upstreamURL, "https://references-123456.cos.ap-guangzhou.myqcloud.com/assets/7/"), upstreamURL)
	assert.Contains(t, upstreamURL, "q-signature=")
	assert.Equal(t, 1, store.uploads)
	report, err = ReplicateAsset(t.Context(), asset)
	require.NoError(t, err)
	assert.Empty(t, report.Errors)
	assert.Equal(t, 1, store.uploads)
}
