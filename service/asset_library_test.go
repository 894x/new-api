package service

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAssetLibraryServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.ChannelAssetConfig{},
		&model.UserAssetGroup{},
		&model.UserAsset{},
		&model.UserAssetGroupReplica{},
		&model.UserAssetReplica{},
	))
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, err := db.DB()
		if err == nil {
			require.NoError(t, sqlDB.Close())
		}
	})
	return db
}

func TestRewriteAssetReferencesRewritesNestedPayloadWithoutMutation(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	assetId := "asset-na-0123456789abcdef0123456789abcdef"
	otherAssetId := "asset-na-abcdef0123456789abcdef0123456789"
	require.NoError(t, db.Create(&model.UserAsset{
		Id: assetId, UserId: 7, GroupId: "group-na-0123456789abcdef0123456789abcdef",
		SourceURL: "https://example.com/a.png", AssetType: "Image", ProjectName: "default",
	}).Error)
	require.NoError(t, db.Create(&model.UserAssetReplica{
		AssetId: assetId, ChannelId: 11, UpstreamAssetId: "Asset-upstream", State: model.AssetReplicaStateReady,
	}).Error)
	payload := map[string]any{
		"content": []any{
			map[string]any{"image_url": "asset://" + assetId},
			"asset://" + otherAssetId,
		},
		"similar": "prefix asset://" + assetId,
	}

	_, err := RewriteAssetReferences(7, 11, payload)
	require.ErrorContains(t, err, otherAssetId)
	assert.Equal(t, "asset://"+assetId, payload["content"].([]any)[0].(map[string]any)["image_url"])

	payload["content"] = payload["content"].([]any)[:1]
	rewritten, err := RewriteAssetReferences(7, 11, payload)
	require.NoError(t, err)
	assert.Equal(t, "asset://Asset-upstream", rewritten["content"].([]any)[0].(map[string]any)["image_url"])
	assert.Equal(t, "asset://"+assetId, payload["content"].([]any)[0].(map[string]any)["image_url"])
	assert.Equal(t, payload["similar"], rewritten["similar"])
}

func TestRewriteAssetReferencesRejectsAnotherUsersReplica(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	assetId := "asset-na-0123456789abcdef0123456789abcdef"
	require.NoError(t, db.Create(&model.UserAsset{
		Id: assetId, UserId: 8, GroupId: "group-na-0123456789abcdef0123456789abcdef",
		SourceURL: "https://example.com/a.png", AssetType: "Image", ProjectName: "default",
	}).Error)
	require.NoError(t, db.Create(&model.UserAssetReplica{
		AssetId: assetId, ChannelId: 11, UpstreamAssetId: "Asset-upstream", State: model.AssetReplicaStateReady,
	}).Error)

	_, err := RewriteAssetReferences(7, 11, map[string]any{"url": "asset://" + assetId})
	require.ErrorContains(t, err, "unavailable")
}

func TestRewriteAssetReferencesRejectsRawUpstreamAssetURI(t *testing.T) {
	setupAssetLibraryServiceTestDB(t)

	for _, uri := range []string{
		"asset://Asset-upstream-owned-by-another-user",
		"ASSET://Asset-upstream-owned-by-another-user",
		" asset://Asset-upstream-owned-by-another-user ",
	} {
		_, err := RewriteAssetReferences(7, 11, map[string]any{"url": uri})
		require.ErrorContains(t, err, "use an account asset ID")
	}
}

func TestSaveAssetLibraryChannelConfigClearsReplicasOnlyWhenIdentityChanges(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	existing := &model.ChannelAssetConfig{
		ChannelId: 11, Enabled: true, BaseURL: "https://assets.example.com", AuthType: AssetLibraryAuthBearer,
		APIKey: "old-key", Region: DefaultAssetLibraryRegion, ProjectName: "project-a",
	}
	require.NoError(t, db.Create(existing).Error)
	require.NoError(t, db.Create(&model.UserAssetGroupReplica{
		GroupId: "group-na-0123456789abcdef0123456789abcdef", ChannelId: 11,
		UpstreamGroupId: "group-upstream", State: model.AssetReplicaStateReady,
	}).Error)
	require.NoError(t, db.Create(&model.UserAssetReplica{
		AssetId: "asset-na-0123456789abcdef0123456789abcdef", ChannelId: 11,
		UpstreamAssetId: "asset-upstream", State: model.AssetReplicaStateReady,
	}).Error)

	changed, err := SaveAssetLibraryChannelConfig(&model.ChannelAssetConfig{
		ChannelId: 11, Enabled: false, BaseURL: existing.BaseURL, AuthType: existing.AuthType,
		APIKey: existing.APIKey, Region: existing.Region, ProjectName: existing.ProjectName,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"enabled"}, changed)
	count, err := model.CountChannelAssetReplicas(11)
	require.NoError(t, err)
	assert.EqualValues(t, 2, count)

	changed, err = SaveAssetLibraryChannelConfig(&model.ChannelAssetConfig{
		ChannelId: 11, Enabled: true, BaseURL: existing.BaseURL, AuthType: existing.AuthType,
		APIKey: "new-key", Region: existing.Region, ProjectName: existing.ProjectName,
	})
	require.NoError(t, err)
	assert.Contains(t, changed, "credentials")
	count, err = model.CountChannelAssetReplicas(11)
	require.NoError(t, err)
	assert.Zero(t, count)
}

func TestReplicateAssetGroupSerializesConcurrentCreatesPerChannel(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	var upstreamCalls atomic.Int32
	firstCallEntered := make(chan struct{})
	releaseFirstCall := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "CreateAssetGroup", request.URL.Query().Get("Action"))
		if upstreamCalls.Add(1) == 1 {
			close(firstCallEntered)
			<-releaseFirstCall
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ResponseMetadata":{},"Result":{"Id":"group-upstream"}}`))
	}))
	t.Cleanup(server.Close)
	require.NoError(t, db.Create(&model.ChannelAssetConfig{
		ChannelId: 9, Enabled: true, BaseURL: server.URL, AuthType: AssetLibraryAuthBearer,
		APIKey: "test-key", Region: DefaultAssetLibraryRegion, ProjectName: "channel-project",
	}).Error)
	group := &model.UserAssetGroup{
		Id: "group-na-0123456789abcdef0123456789abcdef", UserId: 7, Name: "character",
		GroupType: "AIGC", ProjectName: "logical-project",
	}
	require.NoError(t, db.Create(group).Error)

	var waitGroup sync.WaitGroup
	errorsByCall := make(chan error, 2)
	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		_, err := ReplicateAssetGroup(t.Context(), group)
		errorsByCall <- err
	}()
	<-firstCallEntered
	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		_, err := ReplicateAssetGroup(t.Context(), group)
		errorsByCall <- err
	}()
	close(releaseFirstCall)
	waitGroup.Wait()
	close(errorsByCall)
	for err := range errorsByCall {
		require.NoError(t, err)
	}
	assert.Equal(t, int32(1), upstreamCalls.Load())
	replica, err := model.GetUserAssetGroupReplica(group.Id, 9)
	require.NoError(t, err)
	assert.Equal(t, "group-upstream", replica.UpstreamGroupId)
}
