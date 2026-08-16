package middleware

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestLocalAssetIdsExtractsOnlyExactLocalURIs(t *testing.T) {
	const firstAssetId = "asset-na-0123456789abcdef0123456789abcdef"
	const secondAssetId = "asset-na-fedcba9876543210fedcba9876543210"
	request := map[string]any{
		"content": []any{
			map[string]any{"image_url": map[string]any{"url": "asset://" + firstAssetId}},
			map[string]any{"video_url": map[string]any{"url": "asset://" + firstAssetId}},
			map[string]any{"audio_url": map[string]any{"url": "asset://" + secondAssetId}},
			map[string]any{"image_url": map[string]any{"url": "asset://upstream-asset"}},
			map[string]any{"image_url": map[string]any{"url": "asset://" + firstAssetId + "?query=1"}},
			map[string]any{"image_url": map[string]any{"url": "asset://asset-na-ABCDEF0123456789abcdef0123456789"}},
		},
	}

	assert.Equal(t, []string{firstAssetId, secondAssetId}, localAssetIds(request))
}

func TestAssetLibraryRoutingSetsReplicaIntersection(t *testing.T) {
	originalDB := model.DB
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ChannelAssetConfig{}, &model.UserAsset{}, &model.UserAssetReplica{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = originalDB
		sqlDB, err := db.DB()
		if err == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	const firstAssetId = "asset-na-0123456789abcdef0123456789abcdef"
	const secondAssetId = "asset-na-fedcba9876543210fedcba9876543210"
	require.NoError(t, db.Create(&model.UserAsset{Id: firstAssetId, UserId: 77, GroupId: "group-na-test", AssetType: "Image", SourceURL: "https://example.com/one.png"}).Error)
	require.NoError(t, db.Create(&model.UserAsset{Id: secondAssetId, UserId: 77, GroupId: "group-na-test", AssetType: "Image", SourceURL: "https://example.com/two.png"}).Error)
	for _, channelId := range []int{1, 2, 3} {
		require.NoError(t, db.Create(&model.ChannelAssetConfig{ChannelId: channelId, Enabled: true, BaseURL: "https://example.com", AuthType: "bearer"}).Error)
	}
	for _, fixture := range []struct {
		assetId   string
		channelId int
	}{
		{assetId: firstAssetId, channelId: 1},
		{assetId: firstAssetId, channelId: 2},
		{assetId: secondAssetId, channelId: 2},
		{assetId: secondAssetId, channelId: 3},
	} {
		require.NoError(t, db.Create(&model.UserAssetReplica{
			AssetId:         fixture.assetId,
			ChannelId:       fixture.channelId,
			UpstreamAssetId: fmt.Sprintf("upstream-%s-%d", fixture.assetId, fixture.channelId),
			State:           model.AssetReplicaStateReady,
		}).Error)
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body := fmt.Sprintf(`{"content":[{"image_url":{"url":"asset://%s"}},{"image_url":{"url":"asset://%s"}}]}`, firstAssetId, secondAssetId)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", io.NopCloser(strings.NewReader(body)))
	ctx.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(ctx, constant.ContextKeyUserId, 77)

	AssetLibraryRouting()(ctx)

	allowed, ok := common.GetContextKeyType[map[int]struct{}](ctx, constant.ContextKeyAssetAllowedChannelIds)
	require.True(t, ok)
	assert.Equal(t, map[int]struct{}{2: {}}, allowed)
	assert.False(t, ctx.IsAborted())
}

func TestAssetLibraryRoutingRejectsMalformedLocalAssetURI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body := `{"content":[{"image_url":{"url":"asset://asset-na-not-a-logical-id"}}]}`
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", io.NopCloser(strings.NewReader(body)))
	ctx.Request.Header.Set("Content-Type", "application/json")

	AssetLibraryRouting()(ctx)

	assert.True(t, ctx.IsAborted())
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"type":"new_api_error"`)
}

func TestAssetLibraryRoutingRejectsRawUpstreamAssetURI(t *testing.T) {
	for _, uri := range []string{
		"asset://Asset-upstream-owned-by-another-user",
		"ASSET://Asset-upstream-owned-by-another-user",
		" asset://Asset-upstream-owned-by-another-user ",
	} {
		t.Run(uri, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			body := fmt.Sprintf(`{"content":[{"image_url":{"url":%q}}]}`, uri)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", io.NopCloser(strings.NewReader(body)))
			ctx.Request.Header.Set("Content-Type", "application/json")

			AssetLibraryRouting()(ctx)

			assert.True(t, ctx.IsAborted())
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "use an account asset ID")
		})
	}
}

func TestChannelAllowedForAssetsPreservesOrdinaryAndConstrainsAffinityCandidates(t *testing.T) {
	allowed := map[int]struct{}{22: {}}
	assert.True(t, channelAllowedForAssets(11, nil, false))
	assert.True(t, channelAllowedForAssets(22, allowed, true))
	assert.False(t, channelAllowedForAssets(11, allowed, true))
}
