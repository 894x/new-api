package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSLSAssetFailureSurvivesRefreshAndRecordsTransitionOnce(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}))
	require.NoError(t, db.Create(&model.User{Id: 7, Username: "asset-owner"}).Error)
	previous := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() { model.LOG_DB = previous })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/volcengine/assets/la_failed", r.URL.Path)
		_, _ = w.Write([]byte(`{"success":true,"data":{"logical_id":"la_failed","status":"Failed","fail_reason":"InputImageSensitiveContentDetected.PolicyViolation: The input image may be related to copyright restrictions."}}`))
	}))
	t.Cleanup(server.Close)
	require.NoError(t, db.Create(&model.Channel{Id: 6, Type: constant.ChannelTypeSeedanceSLS, Key: "secret"}).Error)
	require.NoError(t, db.Create(&model.ChannelAssetConfig{ChannelId: 6, Enabled: true, Backend: AssetLibraryBackendSeedanceSLS, BaseURL: server.URL, AuthType: AssetLibraryAuthBearer, APIKey: "secret"}).Error)
	require.NoError(t, db.Create(&model.UserAsset{Id: "asset-na-failed", UserId: 7, GroupId: "group-na-1", AssetType: "Image", SourceURL: "http://example.com/image.jpg"}).Error)
	require.NoError(t, db.Create(&model.UserAssetReplica{AssetId: "asset-na-failed", ChannelId: 6, UpstreamAssetId: "la_failed", State: model.AssetReplicaStateProcessing, UpstreamStatus: "Processing"}).Error)
	for range 2 {
		_, err := RefreshAssetLibraryAsset(t.Context(), "asset-na-failed")
		require.NoError(t, err)
	}
	replica, err := model.GetUserAssetReplica("asset-na-failed", 6)
	require.NoError(t, err)
	assert.Equal(t, model.AssetReplicaStateFailed, replica.State)
	assert.Contains(t, replica.LastError, "copyright restrictions")
	assert.Equal(t, "InputImageSensitiveContentDetected.PolicyViolation", replica.LastErrorCode)
	var logs []model.Log
	require.NoError(t, db.Find(&logs).Error)
	require.Len(t, logs, 1, "only the transition should create an asset operation log")
	assert.Equal(t, model.LogTypeAssetUpdate, logs[0].Type)
	assert.Equal(t, 7, logs[0].UserId)
	assert.Contains(t, logs[0].Other, `"status":"failed"`)
	assert.Contains(t, logs[0].Other, "copyright restrictions")
	assert.Zero(t, logs[0].Quota)
}

func TestSLSAssetCreatePreservesImmediateFailureDetails(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"logical_id":"la_failed","logical_group_id":"lg_1","status":"Failed","fail_reason":"ContentRejected: image rejected"}}`))
	}))
	t.Cleanup(server.Close)
	require.NoError(t, db.Create(&model.Channel{Id: 6, Type: constant.ChannelTypeSeedanceSLS, Key: "secret"}).Error)
	require.NoError(t, db.Create(&model.ChannelAssetConfig{ChannelId: 6, Enabled: true, Backend: AssetLibraryBackendSeedanceSLS, BaseURL: server.URL, AuthType: AssetLibraryAuthBearer, APIKey: "secret"}).Error)
	require.NoError(t, db.Create(&model.UserAssetGroup{Id: "group-na-1", UserId: 7}).Error)
	asset := &model.UserAsset{Id: "asset-na-failed", UserId: 7, GroupId: "group-na-1", AssetType: "Image", SourceURL: "http://example.com/image.jpg"}
	require.NoError(t, db.Create(asset).Error)
	_, err := ReplicateAsset(t.Context(), asset)
	require.NoError(t, err)
	replica, err := model.GetUserAssetReplica(asset.Id, 6)
	require.NoError(t, err)
	assert.Equal(t, "ContentRejected", replica.LastErrorCode)
	assert.Equal(t, "image rejected", replica.LastError)
}

func TestAssetFailureDiagnosticsRemainBoundedWhenDebugLoggingIsEnabled(t *testing.T) {
	previous := common.DebugEnabled
	common.DebugEnabled = true
	t.Cleanup(func() { common.DebugEnabled = previous })
	ctx, _ := BeginAssetLibraryOperation(t.Context(), 1, "GetAsset", "asset-na-1")
	observeAssetLibraryReplica(ctx, &model.UserAssetReplica{AssetId: "asset-na-1", State: model.AssetReplicaStateFailed,
		LastError: "Rejected https://private.example/file?key=secret api_key:credential " + strings.Repeat("x", 6000)})
	data, err := common.Marshal(assetTrace(ctx).timing)
	require.NoError(t, err)
	assert.Less(t, len(data), 3000)
	assert.NotContains(t, string(data), "private.example")
	assert.NotContains(t, string(data), "credential")
	assert.NotContains(t, string(data), "key=secret")
}

func TestAssetFailureSummaryPrefersKnownReasonOverEarlierEmptyReplica(t *testing.T) {
	db := setupAssetLibraryServiceTestDB(t)
	for _, id := range []int{1, 2} {
		require.NoError(t, db.Create(&model.ChannelAssetConfig{ChannelId: id, Enabled: true}).Error)
		replica := &model.UserAssetReplica{AssetId: "asset-na-failed", ChannelId: id, State: model.AssetReplicaStateFailed, UpstreamStatus: "Failed"}
		if id == 2 {
			replica.LastErrorCode = "ContentRejected"
			replica.LastError = "Copyright restrictions"
		}
		require.NoError(t, db.Create(replica).Error)
	}
	status, details, _, err := GetAssetLibraryAggregateState("asset-na-failed", true)
	require.NoError(t, err)
	assert.Equal(t, "Failed", status)
	require.NotNil(t, details)
	assert.Equal(t, "Copyright restrictions", details.Message)
	_, public, _, err := GetAssetLibraryAggregateState("asset-na-failed", false)
	require.NoError(t, err)
	require.NotNil(t, public)
	assert.Equal(t, "Asset processing failed", public.Message)
}

func TestSLSAssetGetRetainsFailureReasonWithoutTreatingHTTP200AsTransportFailure(t *testing.T) {
	for _, tc := range []struct {
		status, reason string
		hasError       bool
	}{
		{"Failed", "ContentRejected: input image rejected", true},
		{"Failed", "image rejected without an error code", true},
		{"Failed", "", true},
		{"Active", "", false},
		{"Processing", "", false},
	} {
		t.Run(tc.status+tc.reason, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := common.Marshal(map[string]any{"success": true, "data": map[string]any{"logical_id": "la_1", "status": tc.status, "fail_reason": tc.reason}})
				require.NoError(t, err)
				_, _ = w.Write(body)
			}))
			t.Cleanup(server.Close)
			details, err := (seedanceSLSAssetLibraryBackend{}).GetAsset(t.Context(), &model.ChannelAssetConfig{Enabled: true, BaseURL: server.URL, AuthType: AssetLibraryAuthBearer, APIKey: "secret"}, "la_1")
			require.NoError(t, err)
			assert.Equal(t, tc.status, details.Status)
			if !tc.hasError {
				assert.Nil(t, details.Error)
				return
			}
			require.NotNil(t, details.Error)
			assert.NotEmpty(t, details.Error.Message)
			if tc.reason != "" {
				assert.Contains(t, tc.reason, details.Error.Message)
			}
		})
	}
}
