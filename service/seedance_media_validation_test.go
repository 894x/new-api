package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"image/color"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedanceTestMedia(field, source, role string) map[string]any {
	item := map[string]any{"type": field, field: map[string]any{"url": source}}
	if role != "" {
		item["role"] = role
	}
	return item
}

func TestSeedanceBase64VideoConvertsBeforeAssetImportAndPrunesOldFiles(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("SEEDANCE_VIDEO_DIR", directory)
	previousAddress := system_setting.TaskPublicAddress
	previousLimit := system_setting.GetAssetStorageSetting().SeedanceVideoMaxMB
	system_setting.TaskPublicAddress = "https://media.example/gateway"
	system_setting.GetAssetStorageSetting().SeedanceVideoMaxMB = 1
	t.Cleanup(func() {
		system_setting.TaskPublicAddress = previousAddress
		system_setting.GetAssetStorageSetting().SeedanceVideoMaxMB = previousLimit
	})
	oldName := strings.Repeat("a", 32) + ".mp4"
	require.NoError(t, os.WriteFile(filepath.Join(directory, oldName), make([]byte, 1_000_000), 0600))
	oldTime := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(filepath.Join(directory, oldName), oldTime, oldTime))
	encoded := base64.StdEncoding.EncodeToString(buildAssetLibraryTestMP4("isom", 854, 480, 1000, 2000, 48))
	source := "data:video/mp4;base64," + encoded
	managed, err := StoreVideoAssetReferences(t.Context(), 7, map[string]any{
		"content": []any{seedanceTestMedia("video_url", source, "reference_video")},
	}, nil, true)
	require.NoError(t, err)
	assert.Equal(t, source, managed["content"].([]any)[0].(map[string]any)["video_url"].(map[string]any)["url"])
	payload := map[string]any{"content": []any{
		seedanceTestMedia("video_url", source, "reference_video"),
		seedanceTestMedia("video_url", source, "reference_video"),
		seedanceTestMedia("image_url", "https://example.com/image.png", ""),
	}}
	prepared, err := ConvertSeedanceBase64Videos(t.Context(), payload, true)
	require.NoError(t, err)
	content := prepared["content"].([]any)
	first := content[0].(map[string]any)["video_url"].(map[string]any)["url"].(string)
	second := content[1].(map[string]any)["video_url"].(map[string]any)["url"].(string)
	assert.Equal(t, first, second)
	assert.Contains(t, first, "https://media.example/gateway/v1/seedance-media/")
	assert.Equal(t, source, payload["content"].([]any)[0].(map[string]any)["video_url"].(map[string]any)["url"])
	assert.Equal(t, "https://example.com/image.png", content[2].(map[string]any)["image_url"].(map[string]any)["url"])
	_, statErr := os.Stat(filepath.Join(directory, oldName))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
	entries, err := os.ReadDir(directory)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.True(t, validSeedanceVideoName(entries[0].Name()))
	_, err = ConvertSeedanceBase64Videos(t.Context(), payload, false)
	require.ErrorContains(t, err, "disabled")
	_, err = StoreSeedanceBase64Video(t.Context(), "data:video/mp4;base64,not-base64")
	require.ErrorContains(t, err, "invalid video base64")
	system_setting.GetAssetStorageSetting().SeedanceVideoMaxMB = 2
	rawURL, err := StoreSeedanceBase64Video(t.Context(), encoded)
	require.NoError(t, err)
	assert.Contains(t, rawURL, "/v1/seedance-media/")
	system_setting.TaskPublicAddress = "http://localhost:3000"
	_, err = StoreSeedanceBase64Video(t.Context(), source)
	require.ErrorContains(t, err, "publicly reachable")
}

func TestSeedanceBase64VideoIsFetchedByAssetLibraryAfterConversion(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	t.Setenv("ASSET_STORAGE_ENABLED", "false")
	t.Setenv("SEEDANCE_VIDEO_DIR", t.TempDir())
	previousAddress := system_setting.TaskPublicAddress
	previousFetch := *system_setting.GetFetchSetting()
	system_setting.TaskPublicAddress = "http://media.example"
	system_setting.GetFetchSetting().EnableSSRFProtection = false
	InitHttpClient()
	t.Cleanup(func() {
		system_setting.TaskPublicAddress = previousAddress
		*system_setting.GetFetchSetting() = previousFetch
		InitHttpClient()
	})
	mediaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file, contentType, err := OpenSeedanceMedia(strings.TrimPrefix(r.URL.Path, seedanceVideoURLPath))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		w.Header().Set("Content-Type", contentType)
		_, _ = io.Copy(w, file)
	}))
	defer mediaServer.Close()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	dialer := (&net.Dialer{}).DialContext
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address == "media.example:80" {
			address = mediaServer.Listener.Addr().String()
		}
		return dialer(ctx, network, address)
	}
	GetHttpClient().Transport = transport

	video := buildAssetLibraryTestMP4("isom", 854, 480, 1000, 2000, 48)
	image := storedAssetPNG(t, color.RGBA{R: 80, G: 140, B: 220, A: 255})
	var imports atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/volcengine/assets":
			var input map[string]any
			if err := common.DecodeJson(r.Body, &input); err != nil {
				http.Error(w, "invalid asset request", http.StatusBadRequest)
				return
			}
			url, _ := input["source_url"].(string)
			if !strings.HasPrefix(url, system_setting.TaskPublicAddress+seedanceVideoURLPath) {
				http.Error(w, "video was not converted before import", http.StatusBadRequest)
				return
			}
			response, err := GetHttpClient().Get(url)
			if err != nil {
				http.Error(w, "public video is not readable", http.StatusBadGateway)
				return
			}
			defer response.Body.Close()
			data, err := io.ReadAll(response.Body)
			want := video
			logicalID := "lass_video"
			if strings.HasSuffix(url, ".png") {
				want = image
				logicalID = "lass_image"
			}
			if err != nil || response.StatusCode != http.StatusOK || !assert.Equal(t, want, data) {
				http.Error(w, "public video bytes differ", http.StatusBadGateway)
				return
			}
			imports.Add(1)
			_, _ = fmt.Fprintf(w, `{"success":true,"data":{"logical_id":"%s","logical_group_id":"lasg_%s","status":"Processing"}}`, logicalID, strings.TrimPrefix(logicalID, "lass_"))
		case "GET /v1/volcengine/assets/lass_video", "GET /v1/volcengine/assets/lass_image":
			logicalID := strings.TrimPrefix(r.URL.Path, "/v1/volcengine/assets/")
			assetType := "Video"
			if logicalID == "lass_image" {
				assetType = "Image"
			}
			_, _ = fmt.Fprintf(w, `{"success":true,"data":{"logical_id":"%s","logical_group_id":"lasg_%s","status":"Active","asset_type":"%s"}}`, logicalID, strings.TrimPrefix(logicalID, "lass_"), assetType)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	require.NoError(t, db.Create(&model.Channel{Id: 11, Type: constant.ChannelTypeSeedanceSLS, Key: "test", Name: "SLS"}).Error)
	require.NoError(t, db.Create(&model.ChannelAssetConfig{ChannelId: 11, Enabled: true, Backend: AssetLibraryBackendSeedanceSLS, BaseURL: upstream.URL, AuthType: AssetLibraryAuthBearer, APIKey: "test"}).Error)
	videoSource := "data:video/mp4;base64," + base64.StdEncoding.EncodeToString(video)
	imageSource := "data:image/png;base64," + base64.StdEncoding.EncodeToString(image)
	payload := map[string]any{"model": "doubao-seedance-2-5", "content": []any{
		seedanceTestMedia("video_url", videoSource, "reference_video"),
		seedanceTestMedia("image_url", imageSource, "reference_image"),
	}}
	converted, err := ConvertSeedanceBase64Media(t.Context(), payload, true)
	require.NoError(t, err)
	ctx, err := ValidateSeedanceMedia(t.Context(), 7, "doubao-seedance-2-5", converted)
	require.NoError(t, err)
	prepared, err := PrepareAssetReferences(ctx, 7, 11, converted)
	require.NoError(t, err)
	assert.Equal(t, "asset://lass_video", prepared["content"].([]any)[0].(map[string]any)["video_url"].(map[string]any)["url"])
	assert.Equal(t, "asset://lass_image", prepared["content"].([]any)[1].(map[string]any)["image_url"].(map[string]any)["url"])
	assert.Equal(t, int32(2), imports.Load())
}

func TestSeedanceAudioDurationBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, model     string
		seconds, copies int
		wantError       string
	}{
		{"minimum", "2-0", 2, 1, ""},
		{"too short", "2-0", 1, 1, "duration"},
		{"maximum 2.0", "2-0", 15, 1, ""},
		{"too long 2.0", "2-0", 16, 1, "content[1].audio_url"},
		{"duplicate references count toward total", "2-0", 8, 2, "total reference audio duration 16.000"},
		{"exact total", "2-0", 5, 3, ""},
		{"maximum 2.5", "2-5", 30, 1, ""},
		{"too long 2.5", "2-5", 31, 1, "duration"},
		{"total 2.5 exceeded", "2-5", 16, 2, "total reference audio duration 32.000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uri := "data:audio/wav;base64," + base64.StdEncoding.EncodeToString(buildAssetLibraryTestWAV(8000, uint32(tc.seconds)))
			content := []any{seedanceTestMedia("image_url", "https://example.com/image.png", "reference_image")}
			for i := 0; i < tc.copies; i++ {
				content = append(content, seedanceTestMedia("audio_url", uri, "reference_audio"))
			}
			_, err := ValidateSeedanceMedia(t.Context(), 7, "doubao-seedance-"+tc.model, map[string]any{"content": content})
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSeedanceCompositionRejectsBeforeFetching(t *testing.T) {
	image := seedanceTestMedia("image_url", "https://example.com/image.png", "reference_image")
	audio := seedanceTestMedia("audio_url", "https://example.com/audio.wav", "reference_audio")
	video := seedanceTestMedia("video_url", "https://example.com/video.mp4", "reference_video")
	first := seedanceTestMedia("image_url", "https://example.com/first.png", "first_frame")
	last := seedanceTestMedia("image_url", "https://example.com/last.png", "last_frame")
	for _, tc := range []struct {
		name, model string
		content     []any
		ratio       any
		wantError   string
	}{
		{"audio only 2.0", "2-0", []any{audio}, nil, "requires an image or video"},
		{"audio count", "2-0", []any{image, audio, audio, audio, audio}, nil, "audio count 4"},
		{"video count", "2-0", []any{video, video, video, video}, nil, "video count 4"},
		{"duplicate first frame", "2-0", []any{first, first}, nil, "content.role"},
		{"last without first", "2-0", []any{last}, nil, "content.role"},
		{"mixed references", "2-0", []any{first, image}, nil, "cannot be mixed"},
		{"first and last", "2-0", []any{first, last}, nil, ""},
		{"first frame ratio 2.5", "2-5", []any{first}, "16:9", "ratio must be adaptive"},
		{"adaptive ratio", "2-5", []any{first, last}, "adaptive", ""},
		{"video base64", "2-5", []any{seedanceTestMedia("video_url", "data:video/mp4;base64,YQ==", "")}, nil, "video requires"},
		{"malformed audio", "2-5", []any{map[string]any{"type": "audio_url", "audio_url": "bad"}}, nil, "content[0].audio_url"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{"content": tc.content}
			if tc.ratio != nil {
				payload["ratio"] = tc.ratio
			}
			_, err := ValidateSeedanceMedia(t.Context(), 7, "doubao-seedance-"+tc.model, payload)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
			} else {
				require.NoError(t, err)
			}
		})
	}
	for _, tc := range []struct {
		model   string
		maximum int
	}{{"2-0", 9}, {"2-5", 30}} {
		t.Run("image count "+tc.model, func(t *testing.T) {
			content := make([]any, tc.maximum)
			for i := range content {
				content[i] = image
			}
			_, err := ValidateSeedanceMedia(t.Context(), 7, "doubao-seedance-"+tc.model, map[string]any{"content": content})
			require.NoError(t, err)
			_, err = ValidateSeedanceMedia(t.Context(), 7, "doubao-seedance-"+tc.model, map[string]any{"content": append(content, image)})
			require.ErrorContains(t, err, "image count")
		})
	}
}

func TestSeedanceLogicalAssetsEnforceOwnershipAndType(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	id := "asset-na-0123456789abcdef0123456789abcdef"
	require.NoError(t, db.Create(&model.UserAsset{Id: id, UserId: 7, AssetType: "Audio", MediaFormat: "wav", FileSize: 1024, Duration: 15.2}).Error)
	payload := map[string]any{"content": []any{seedanceTestMedia("audio_url", "asset://"+id, "")}}
	_, err := ValidateSeedanceMedia(t.Context(), 7, "doubao-seedance-2-5", payload)
	require.NoError(t, err)
	_, err = ValidateSeedanceMedia(t.Context(), 8, "doubao-seedance-2-5", payload)
	require.ErrorContains(t, err, "unavailable for this account")
	payload["content"] = []any{seedanceTestMedia("video_url", "asset://"+id, "")}
	_, err = ValidateSeedanceMedia(t.Context(), 7, "doubao-seedance-2-5", payload)
	require.ErrorContains(t, err, "asset type must be video")
	payload["content"] = []any{seedanceTestMedia("image_url", "https://example.com/image.png", ""), seedanceTestMedia("audio_url", "asset://"+id, "")}
	_, err = ValidateSeedanceMedia(t.Context(), 7, "doubao-seedance-2-0", payload)
	require.ErrorContains(t, err, "15.200 seconds must be between 2 and 15")
}

func TestSeedanceURLVideoBoundariesAndRequestCache(t *testing.T) {
	settings := system_setting.GetFetchSetting()
	previous := *settings
	settings.EnableSSRFProtection = false
	InitHttpClient()
	t.Cleanup(func() { *settings = previous; InitHttpClient() })
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var duration uint32 = 15
		if r.URL.Path == "/long" {
			duration = 16
		}
		_, _ = w.Write(buildAssetLibraryTestMP4("isom", 1920, 1080, 1000, duration*1000, duration*30))
	}))
	defer server.Close()
	payload := map[string]any{"content": []any{seedanceTestMedia("video_url", server.URL, "")}}
	ctx, err := ValidateSeedanceMedia(t.Context(), 7, "doubao-seedance-2-0", payload)
	require.NoError(t, err)
	_, err = ValidateSeedanceMedia(ctx, 7, "doubao-seedance-2-0", payload)
	require.NoError(t, err)
	assert.Equal(t, int32(1), calls.Load())
	payload["content"] = []any{seedanceTestMedia("video_url", server.URL, ""), seedanceTestMedia("video_url", server.URL, "")}
	_, err = ValidateSeedanceMedia(ctx, 7, "doubao-seedance-2-0", payload)
	require.ErrorContains(t, err, "total reference video duration 30.000 seconds exceeds 15")
	_, err = ValidateSeedanceMedia(ctx, 7, "doubao-seedance-2-5", payload)
	require.NoError(t, err)
	payload["content"] = []any{seedanceTestMedia("video_url", server.URL+"/long", "")}
	_, err = ValidateSeedanceMedia(t.Context(), 7, "doubao-seedance-2-0", payload)
	require.ErrorContains(t, err, "16.000 seconds must be between 2 and 15")
}

func TestSeedanceVideoAndAudioBudgetsAreSeparate(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	content := []any{}
	for i, kind := range []string{"Audio", "Video"} {
		id := fmt.Sprintf("asset-na-%032x", i+1)
		format := "wav"
		if kind == "Video" {
			format = "mp4"
		}
		require.NoError(t, db.Create(&model.UserAsset{Id: id, UserId: 7, AssetType: kind, MediaFormat: format, FileSize: 1024, Duration: 15, Width: 1280, Height: 720, FPS: 30}).Error)
		field := "audio_url"
		if kind == "Video" {
			field = "video_url"
		}
		content = append(content, seedanceTestMedia(field, "asset://"+id, ""))
	}
	_, err := ValidateSeedanceMedia(context.Background(), 7, "doubao-seedance-2.0-fast-260128", map[string]any{"content": content})
	require.NoError(t, err)
}

func TestSeedanceAudioPreflightAndAutoImportDownloadOnce(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	settings := system_setting.GetFetchSetting()
	previous := *settings
	settings.EnableSSRFProtection = false
	InitHttpClient()
	t.Cleanup(func() { *settings = previous; InitHttpClient() })
	var downloads, uploads atomic.Int32
	wav := buildAssetLibraryTestWAV(8000, 10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/reference.wav":
			downloads.Add(1)
			_, _ = w.Write(wav)
		case "/v1/volcengine/assets":
			uploads.Add(1)
			_, _ = w.Write([]byte(`{"success":true,"data":{"logical_id":"lass_audio","logical_group_id":"lasg_audio","status":"Processing"}}`))
		case "/v1/volcengine/assets/lass_audio":
			_, _ = w.Write([]byte(`{"success":true,"data":{"logical_id":"lass_audio","logical_group_id":"lasg_audio","status":"Active","asset_type":"Audio"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	require.NoError(t, db.Create(&model.Channel{Id: 11, Type: constant.ChannelTypeSeedanceSLS, Key: "test", Name: "SLS"}).Error)
	require.NoError(t, db.Create(&model.ChannelAssetConfig{ChannelId: 11, Enabled: true, Backend: AssetLibraryBackendSeedanceSLS, BaseURL: server.URL, AuthType: AssetLibraryAuthBearer, APIKey: "test"}).Error)
	// The URL previously pointed at a longer clip; reusing that asset would
	// invalidate the duration checked against the current source bytes.
	group := model.UserAssetGroup{Id: "group-na-0123456789abcdef0123456789abcdef", UserId: 7, Name: autoImportAssetGroupName, ProjectName: autoImportAssetProjectName}
	require.NoError(t, db.Create(&group).Error)
	require.NoError(t, db.Create(&model.UserAsset{Id: "asset-na-0123456789abcdef0123456789abcdef", UserId: 7, GroupId: group.Id, SourceURL: server.URL + "/reference.wav", AssetType: "Audio", MediaFormat: "wav", FileSize: int64(len(wav)), Duration: 30}).Error)
	require.NoError(t, db.Create(&model.UserAssetReplica{AssetId: "asset-na-0123456789abcdef0123456789abcdef", ChannelId: 11, UpstreamAssetId: "lass_old_audio", State: model.AssetReplicaStateReady}).Error)
	audio := seedanceTestMedia("audio_url", server.URL+"/reference.wav", "reference_audio")
	payload := map[string]any{"model": "doubao-seedance-2-5", "content": []any{audio, audio}}
	ctx, err := ValidateSeedanceMedia(t.Context(), 7, "doubao-seedance-2-5", payload)
	require.NoError(t, err)
	assert.Equal(t, int32(0), uploads.Load())
	prepared, err := PrepareAssetReferences(ctx, 7, 11, payload)
	require.NoError(t, err)
	assert.Equal(t, int32(1), downloads.Load())
	assert.Equal(t, int32(1), uploads.Load())
	items := prepared["content"].([]any)
	assert.Equal(t, "asset://lass_audio", items[0].(map[string]any)["audio_url"].(map[string]any)["url"])
	var assets []model.UserAsset
	require.NoError(t, db.Find(&assets).Error)
	require.Len(t, assets, 2)
	var current model.UserAsset
	require.NoError(t, db.Where("duration = ?", 10).First(&current).Error)
	assert.Equal(t, 10.0, current.Duration)
}

func TestSeedanceLegacyAssetRequiresReimportAndUnknownModelUntouched(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	id := "asset-na-0123456789abcdef0123456789abcdef"
	uri := "data:audio/wav;base64," + base64.StdEncoding.EncodeToString(buildAssetLibraryTestWAV(8000, 2))
	require.NoError(t, db.Create(&model.UserAsset{Id: id, UserId: 7, AssetType: "Audio", SourceURL: uri}).Error)
	require.NoError(t, db.Create(&model.UserAssetReplica{AssetId: id, ChannelId: 11, UpstreamAssetId: "old-unknown-duration", State: model.AssetReplicaStateReady}).Error)
	payload := map[string]any{"content": []any{seedanceTestMedia("image_url", "https://example.com/image.png", ""), seedanceTestMedia("audio_url", "asset://"+id, "")}}
	_, err := ValidateSeedanceMedia(t.Context(), 7, "doubao-seedance-2-0", payload)
	require.ErrorContains(t, err, "asset metadata is incomplete; re-import")
	_, err = ValidateSeedanceMedia(t.Context(), 7, "some-other-video-model", payload)
	require.NoError(t, err)
}

func TestSeedanceVideoMetadataLimits(t *testing.T) {
	for _, tc := range []struct {
		name          string
		duration      float64
		width, height int
		fps           float64
		wantError     string
	}{
		{"minimum duration", 2, 1280, 720, 24, ""},
		{"maximum duration", 30, 1280, 720, 60, ""},
		{"below minimum", 1.9, 1280, 720, 30, "duration"},
		{"above maximum", 30.1, 1280, 720, 30, "duration"},
		{"generation accepts 4k", 10, 3840, 2160, 30, ""},
		{"pixel upper boundary", 10, 3326, 2494, 30, ""},
		{"pixels exceed upper boundary", 10, 3327, 2494, 30, "pixel count"},
		{"frame rate too low", 10, 1280, 720, 23, "frame rate"},
		{"frame rate too high", 10, 1280, 720, 61, "frame rate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupAssetLibraryServiceTestDB(t)
			id := "asset-na-0123456789abcdef0123456789abcdef"
			require.NoError(t, db.Create(&model.UserAsset{Id: id, UserId: 7, AssetType: "Video", MediaFormat: "mp4", FileSize: 1024, Duration: tc.duration, Width: tc.width, Height: tc.height, FPS: tc.fps}).Error)
			_, err := ValidateSeedanceMedia(t.Context(), 7, "doubao-seedance-2-5", map[string]any{"content": []any{seedanceTestMedia("video_url", "asset://"+id, "")}})
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSeedanceValidatedImageCountSurvivesAutomaticImport(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	require.NoError(t, db.Create(&model.Channel{Id: 11, Type: constant.ChannelTypeSeedanceSLS, Key: "test", Name: "SLS"}).Error)
	require.NoError(t, db.Create(&model.ChannelAssetConfig{ChannelId: 11, Enabled: true, Backend: AssetLibraryBackendSeedanceSLS, BaseURL: "https://example.com", AuthType: AssetLibraryAuthBearer, APIKey: "test"}).Error)
	group := model.UserAssetGroup{Id: "group-na-0123456789abcdef0123456789abcdef", UserId: 7, Name: autoImportAssetGroupName, ProjectName: autoImportAssetProjectName}
	require.NoError(t, db.Create(&group).Error)
	content := []any{}
	for i := 0; i < 9; i++ {
		id := fmt.Sprintf("asset-na-%032x", i+1)
		source := fmt.Sprintf("https://example.com/%d.png", i)
		require.NoError(t, db.Create(&model.UserAsset{Id: id, UserId: 7, GroupId: group.Id, SourceURL: source, AssetType: "Image"}).Error)
		require.NoError(t, db.Create(&model.UserAssetReplica{AssetId: id, ChannelId: 11, UpstreamAssetId: fmt.Sprintf("image-%d", i), State: model.AssetReplicaStateReady}).Error)
		content = append(content, seedanceTestMedia("image_url", source, "reference_image"))
	}
	payload := map[string]any{"model": "doubao-seedance-2-0", "content": content}
	ctx, err := ValidateSeedanceMedia(t.Context(), 7, "doubao-seedance-2-0", payload)
	require.NoError(t, err)
	prepared, err := PrepareAssetReferences(ctx, 7, 11, payload)
	require.NoError(t, err)
	assert.Len(t, prepared["content"], 9)
	// An unrelated model must retain its existing import bound on a routing retry.
	payload["model"] = "other-model"
	_, err = PrepareAssetReferences(ctx, 7, 11, payload)
	require.ErrorContains(t, err, "at most 8 direct asset references")
}
