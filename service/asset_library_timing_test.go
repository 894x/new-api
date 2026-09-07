package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssetLibraryTimelineCapturesProviderErrorsAndRequestIDs(t *testing.T) {
	for _, tc := range []struct {
		name, response, expectedID, code string
		call                             func(context.Context, *model.ChannelAssetConfig) error
	}{
		{"action", `{"ResponseMetadata":{"RequestId":"action-id","Error":{"Code":"Busy","Message":"secret https://signed.example/?token=private"}}}`, "action-id", "Busy", func(ctx context.Context, config *model.ChannelAssetConfig) error {
			return CallAssetLibraryUpstream(ctx, config, "CreateAsset", nil, nil)
		}},
		{"openapi", `{"code":429,"message":"secret https://signed.example/?token=private","trace_id":"trace-id"}`, "trace-id", "429", func(ctx context.Context, config *model.ChannelAssetConfig) error {
			return callOpenAPIAssetLibrary(ctx, config, "/asset/create", nil, nil)
		}},
		{"sls", `{"success":false,"code":"Busy","message":"secret https://signed.example/?token=private"}`, "header-id", "Busy", func(ctx context.Context, config *model.ChannelAssetConfig) error {
			return callSeedanceSLSAssetLibrary(ctx, config, http.MethodPost, "", nil, nil)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Request-ID", "header-id")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()
			ctx, _ := BeginAssetLibraryOperation(context.WithValue(t.Context(), common.RequestIdKey, "local-id"), 1, "CreateAsset", "asset-na-1")
			err := tc.call(ctx, &model.ChannelAssetConfig{ChannelId: 7, Enabled: true, AuthType: AssetLibraryAuthBearer, APIKey: "credential-secret", BaseURL: server.URL})
			require.Error(t, err)
			trace := assetTrace(ctx)
			require.Len(t, trace.timing.Stages, 1)
			stage := trace.timing.Stages[0]
			assert.Equal(t, "failed", stage.Outcome)
			assert.Equal(t, http.StatusTooManyRequests, stage.HTTPStatus)
			assert.Equal(t, tc.expectedID, stage.UpstreamRequestID)
			assert.Equal(t, tc.code, stage.ErrorCode)
			assert.Equal(t, 7, stage.ChannelID)
			assert.Equal(t, "asset-na-1", stage.AssetID)
			data, err := common.Marshal(trace.timing)
			require.NoError(t, err)
			assert.NotContains(t, string(data), "secret")
			assert.NotContains(t, string(data), "https://")
		})
	}
}

func TestAssetLibraryTimelineAuditPreservesRequestCorrelationAndFailures(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}))
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "asset-owner"}).Error)
	previous := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() { model.LOG_DB = previous })
	ctx, finish := BeginAssetLibraryOperation(context.WithValue(t.Context(), common.RequestIdKey, "request-1"), 1, "CreateAsset", "asset-na-1")
	span := startAssetLibraryStage(ctx, "source_download", "", "", 0)
	span.finish(context.DeadlineExceeded)
	SetAssetLibraryAudit(ctx, "Created asset", "", "asset_library.asset.create", map[string]interface{}{"id": "asset-na-1"})
	finish(errors.New("secret details must not escape"))
	var logs []model.Log
	require.NoError(t, db.Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, "request-1", logs[0].RequestId)
	assert.Contains(t, logs[0].Other, `"error_kind":"timeout"`)
	assert.Contains(t, logs[0].Other, `"asset_timing"`)
	assert.NotContains(t, logs[0].Other, "secret")
	assert.Zero(t, logs[0].Quota)
}

func TestAssetLibraryQueriesNeverRecordUsageLogs(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}))
	previous := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() { model.LOG_DB = previous })
	for _, action := range []string{"GetAsset", "GetAssetGroup", "ListAssets", "ListAssetGroups"} {
		for _, failed := range []bool{false, true} {
			ctx, finish := BeginAssetLibraryOperation(t.Context(), 1, action, "asset-na-1")
			assetTrace(ctx).started = time.Now().Add(-3 * time.Second)
			observeAssetLibraryReplica(ctx, &model.UserAssetReplica{AssetId: "asset-na-1", State: model.AssetReplicaStateReady})
			var err error
			if failed {
				err = errors.New("query failed")
			}
			finish(err)
		}
	}
	var count int64
	require.NoError(t, db.Model(&model.Log{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestAssetReplicaFirstActiveSurvivesStaleReplicaWrite(t *testing.T) {
	setupAssetLibraryServiceTestDB(t)
	replica := &model.UserAssetReplica{AssetId: "asset-na-1", ChannelId: 7, State: model.AssetReplicaStateProcessing, SubmittedAtMS: 1000}
	require.NoError(t, model.SaveUserAssetReplica(replica))
	stale := *replica
	replica.FirstActiveAtMS = 2000
	replica.State = model.AssetReplicaStateReady
	require.NoError(t, model.SaveUserAssetReplica(replica))
	require.NoError(t, model.SaveUserAssetReplica(&stale))
	saved, err := model.GetUserAssetReplica(replica.AssetId, 7)
	require.NoError(t, err)
	assert.EqualValues(t, 2000, saved.FirstActiveAtMS)
}

func TestBatchAssetTimelineUsesPerAssetStartAndAttribution(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"logical_id":"upstream-asset","logical_group_id":"upstream-group","status":"Processing"}}`))
	}))
	defer server.Close()
	require.NoError(t, db.Create(&model.Channel{Id: 7, Type: constant.ChannelTypeSeedanceSLS}).Error)
	require.NoError(t, db.Create(&model.ChannelAssetConfig{ChannelId: 7, Enabled: true, AuthType: AssetLibraryAuthBearer, APIKey: "key", BaseURL: server.URL}).Error)
	group := &model.UserAssetGroup{Id: "group-na-1", UserId: 1, Name: "group"}
	require.NoError(t, db.Create(group).Error)
	ctx, _ := BeginAssetLibraryOperation(t.Context(), 1, "AutoImport", "")
	assetTrace(ctx).timing.StartedAtMS = 1000
	for _, tc := range []struct {
		id    string
		start int64
	}{{"asset-na-1", 2000}, {"asset-na-2", 5000}} {
		asset := &model.UserAsset{Id: tc.id, UserId: 1, GroupId: group.Id, AssetType: "Image"}
		require.NoError(t, db.Create(asset).Error)
		assetCtx := BeginAssetLibraryUpload(ctx, asset.Id)
		// Explicit timestamps make this a deterministic boundary test, without
		// sleeps or timing comparisons against the scheduler.
		assetCtx = context.WithValue(assetCtx, assetLibraryUploadStartKey{}, tc.start)
		span := startAssetLibraryStage(assetCtx, "source_download", "", "", 0)
		span.finish(nil)
		assert.Equal(t, tc.id, span.stage.AssetID)
		report, err := ReplicateAsset(assetCtx, asset)
		require.NoError(t, err)
		require.Empty(t, report.Errors)
		replica, err := model.GetUserAssetReplica(asset.Id, 7)
		require.NoError(t, err)
		assert.Equal(t, tc.start, replica.UploadStartedAtMS)
	}
}

func TestAssetOperationCategoriesSupportUserLogFiltering(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}))
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "asset-owner"}).Error)
	previous := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() { model.LOG_DB = previous })
	for _, tc := range []struct {
		action, requestAction string
		logType               int
	}{
		{"asset_library.asset.create", "", model.LogTypeAssetUpload},
		{"asset_library.group.delete", "", model.LogTypeAssetDelete},
		{"asset_library.asset.update", "", model.LogTypeAssetUpdate},
		{"asset_library.group.create", "", model.LogTypeAssetGroupCreate},
		{"asset_library.asset.sync", "", model.LogTypeAssetSync},
		{"asset_library.group.sync", "", model.LogTypeAssetSync},
		{"channel.asset_library.sync", "", model.LogTypeAssetSync},
		{"asset_library.request", "AutoImport", model.LogTypeAssetUpload},
		{"asset_library.request", "ReplicateAsset", model.LogTypeAssetSync},
		{"asset_library.request", "SyncAssetReplicas", model.LogTypeAssetSync},
		{"asset_library.request", "SyncAssetGroupReplicas", model.LogTypeAssetSync},
		{"asset_library.request", "SyncAssetLibraryChannel", model.LogTypeAssetSync},
		{"channel.asset_library.update", "", model.LogTypeManage},
		{"user.update", "", model.LogTypeManage},
	} {
		requestID := tc.action + tc.requestAction
		model.RecordOperationAuditLog(1, "operation", "", tc.action, map[string]interface{}{"action": tc.requestAction}, map[string]interface{}{"private": "hidden"}, nil, requestID)
		logs, total, err := model.GetUserLogs(1, tc.logType, 0, 0, "", "", 0, 20, "", requestID)
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
		require.Len(t, logs, 1)
		assert.Equal(t, tc.logType, logs[0].Type)
		assert.NotContains(t, logs[0].Other, "private")
		assert.Contains(t, logs[0].Other, tc.action)
	}
}
