package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type perfAnalyticsMetricResponse struct {
	P50Ms       int64 `json:"p50_ms"`
	P90Ms       int64 `json:"p90_ms"`
	P99Ms       int64 `json:"p99_ms"`
	SampleCount int64 `json:"sample_count"`
}

type perfAnalyticsResponse struct {
	Success bool `json:"success"`
	Data    struct {
		EffectiveStartTs int64 `json:"effective_start_timestamp"`
		EffectiveEndTs   int64 `json:"effective_end_timestamp"`
		Summary          struct {
			RequestCount int64                       `json:"request_count"`
			SuccessRate  float64                     `json:"success_rate"`
			Rpm          float64                     `json:"rpm"`
			Tpm          float64                     `json:"tpm"`
			CacheHitRate float64                     `json:"cache_hit_rate"`
			Ttft         perfAnalyticsMetricResponse `json:"ttft"`
			Tpot         perfAnalyticsMetricResponse `json:"tpot"`
		} `json:"summary"`
		Series []struct {
			Ts           int64   `json:"ts"`
			RequestCount int64   `json:"request_count"`
			Rpm          float64 `json:"rpm"`
			Tpm          float64 `json:"tpm"`
			CacheHitRate float64 `json:"cache_hit_rate"`
		} `json:"series"`
	} `json:"data"`
}

type perfAnalyticsOptionsResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Models []string `json:"models"`
		Users  []struct {
			Id       int    `json:"id"`
			Username string `json:"username"`
		} `json:"users"`
		Tokens []struct {
			Id     int    `json:"id"`
			UserId int    `json:"user_id"`
			Name   string `json:"name"`
		} `json:"tokens"`
	} `json:"data"`
}

func setupPerfAnalyticsControllerTestDB(t *testing.T) {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))
	require.NoError(t, db.Exec(`
		CREATE TABLE perf_metric_details (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			token_id INTEGER NOT NULL,
			model_name TEXT NOT NULL,
			bucket_ts INTEGER NOT NULL,
			request_count INTEGER NOT NULL DEFAULT 0,
			success_count INTEGER NOT NULL DEFAULT 0,
			ttft_count INTEGER NOT NULL DEFAULT 0,
			tpot_count INTEGER NOT NULL DEFAULT 0,
			input_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			cache_read_tokens INTEGER NOT NULL DEFAULT 0
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE perf_metric_histograms (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			token_id INTEGER NOT NULL,
			model_name TEXT NOT NULL,
			bucket_ts INTEGER NOT NULL,
			metric TEXT NOT NULL,
			upper_bound_ms INTEGER NOT NULL,
			count INTEGER NOT NULL DEFAULT 0
		)
	`).Error)

	require.NoError(t, db.Create(&[]model.Token{
		{Id: 11, UserId: 1, Key: "self-key", Name: "Self key"},
		{Id: 22, UserId: 2, Key: "other-key", Name: "Other key"},
	}).Error)
	require.NoError(t, db.Create(&[]model.User{
		{Id: 1, Username: "alice", Password: "password", Status: 1, Role: 1, AffCode: "alice-code"},
		{Id: 2, Username: "bob", Password: "password", Status: 1, Role: 1, AffCode: "bob-code"},
	}).Error)

	require.NoError(t, db.Exec(`
		INSERT INTO perf_metric_details
			(user_id, token_id, model_name, bucket_ts, request_count, success_count, ttft_count, tpot_count, input_tokens, output_tokens, total_tokens, cache_read_tokens)
		VALUES
			(1, 11, 'gpt-test', 1000, 10, 9, 9, 8, 10000, 5000, 15000, 2500),
			(2, 22, 'gpt-test', 1000, 4, 4, 4, 4, 4000, 2000, 6000, 1000),
			(2, 22, 'claude-test', 1000, 2, 2, 2, 2, 2000, 800, 2800, 300)
	`).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO perf_metric_histograms
			(user_id, token_id, model_name, bucket_ts, metric, upper_bound_ms, count)
		VALUES
			(1, 11, 'gpt-test', 1000, 'ttft', 100, 5),
			(1, 11, 'gpt-test', 1000, 'ttft', 500, 3),
			(1, 11, 'gpt-test', 1000, 'ttft', 1000, 1),
			(1, 11, 'gpt-test', 1000, 'tpot', 20, 4),
			(1, 11, 'gpt-test', 1000, 'tpot', 40, 3),
			(1, 11, 'gpt-test', 1000, 'tpot', 80, 1),
			(2, 22, 'gpt-test', 1000, 'ttft', 2000, 4),
			(2, 22, 'gpt-test', 1000, 'tpot', 160, 4)
	`).Error)
}

func performPerfAnalyticsOptionsRequest(
	t *testing.T,
	handler gin.HandlerFunc,
	userID int,
	rawQuery string,
) (*httptest.ResponseRecorder, perfAnalyticsOptionsResponse) {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/?"+rawQuery, nil)
	ctx.Set("id", userID)
	handler(ctx)

	var response perfAnalyticsOptionsResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return recorder, response
}

func performPerfAnalyticsRequest(
	t *testing.T,
	handler gin.HandlerFunc,
	userID int,
	rawQuery string,
) (*httptest.ResponseRecorder, perfAnalyticsResponse) {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/?"+rawQuery, nil)
	ctx.Set("id", userID)
	handler(ctx)

	var response perfAnalyticsResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return recorder, response
}

func TestGetPerfAnalyticsSelfIgnoresRequestedUserAndScopesToAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPerfAnalyticsControllerTestDB(t)

	recorder, response := performPerfAnalyticsRequest(
		t,
		GetPerfAnalyticsSelf,
		1,
		"model=gpt-test&user_id=2&start_timestamp=900&end_timestamp=2000",
	)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.True(t, response.Success)
	assert.Equal(t, int64(0), response.Data.EffectiveStartTs)
	assert.Equal(t, int64(3599), response.Data.EffectiveEndTs)
	assert.Equal(t, int64(10), response.Data.Summary.RequestCount)
	assert.InDelta(t, 90, response.Data.Summary.SuccessRate, 0.001)
	assert.Equal(t, 0.17, response.Data.Summary.Rpm)
	assert.Equal(t, 250.0, response.Data.Summary.Tpm)
	assert.Equal(t, 25.0, response.Data.Summary.CacheHitRate)
	assert.Equal(t, perfAnalyticsMetricResponse{P50Ms: 100, P90Ms: 1000, P99Ms: 1000, SampleCount: 9}, response.Data.Summary.Ttft)
	assert.Equal(t, perfAnalyticsMetricResponse{P50Ms: 20, P90Ms: 80, P99Ms: 80, SampleCount: 8}, response.Data.Summary.Tpot)
	require.Len(t, response.Data.Series, 1)
	assert.Equal(t, int64(10), response.Data.Series[0].RequestCount)
	assert.Equal(t, 0.17, response.Data.Series[0].Rpm)
	assert.Equal(t, 250.0, response.Data.Series[0].Tpm)
	assert.Equal(t, 25.0, response.Data.Series[0].CacheHitRate)
}

func TestGetPerfAnalyticsSelfRejectsTokenOwnedByAnotherUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPerfAnalyticsControllerTestDB(t)

	recorder, _ := performPerfAnalyticsRequest(
		t,
		GetPerfAnalyticsSelf,
		1,
		"model=gpt-test&token_id=22&start_timestamp=900&end_timestamp=2000",
	)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestGetPerfAnalyticsAdminCanAggregateEveryUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPerfAnalyticsControllerTestDB(t)

	recorder, response := performPerfAnalyticsRequest(
		t,
		GetPerfAnalyticsAdmin,
		99,
		"model=gpt-test&start_timestamp=900&end_timestamp=2000",
	)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, int64(14), response.Data.Summary.RequestCount)
	assert.Equal(t, int64(13), response.Data.Summary.Ttft.SampleCount)
	assert.Equal(t, int64(12), response.Data.Summary.Tpot.SampleCount)
	assert.Equal(t, 0.23, response.Data.Summary.Rpm)
	assert.Equal(t, 350.0, response.Data.Summary.Tpm)
	assert.Equal(t, 25.0, response.Data.Summary.CacheHitRate)
}

func TestGetPerfAnalyticsAdminFiltersUserAndValidatesTokenOwnership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPerfAnalyticsControllerTestDB(t)

	recorder, response := performPerfAnalyticsRequest(
		t,
		GetPerfAnalyticsAdmin,
		99,
		"model=gpt-test&user_id=2&token_id=22&start_timestamp=900&end_timestamp=2000",
	)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, int64(4), response.Data.Summary.RequestCount)
	assert.Equal(t, int64(2000), response.Data.Summary.Ttft.P50Ms)
	assert.Equal(t, int64(160), response.Data.Summary.Tpot.P50Ms)

	invalidRecorder, _ := performPerfAnalyticsRequest(
		t,
		GetPerfAnalyticsAdmin,
		99,
		"model=gpt-test&user_id=1&token_id=22&start_timestamp=900&end_timestamp=2000",
	)
	assert.Equal(t, http.StatusBadRequest, invalidRecorder.Code)
}

func TestGetPerfAnalyticsSelfOptionsOnlyExposeAuthenticatedUsersModelsAndKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPerfAnalyticsControllerTestDB(t)

	recorder, response := performPerfAnalyticsOptionsRequest(
		t,
		GetPerfAnalyticsSelfOptions,
		1,
		"user_id=2",
	)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, []string{"gpt-test"}, response.Data.Models)
	assert.Empty(t, response.Data.Users)
	require.Len(t, response.Data.Tokens, 1)
	assert.Equal(t, 11, response.Data.Tokens[0].Id)
	assert.Equal(t, 1, response.Data.Tokens[0].UserId)
}

func TestGetPerfAnalyticsAdminOptionsExposeAllUsersAndCascadeBySelectedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPerfAnalyticsControllerTestDB(t)

	allRecorder, allResponse := performPerfAnalyticsOptionsRequest(
		t,
		GetPerfAnalyticsAdminOptions,
		99,
		"",
	)
	require.Equal(t, http.StatusOK, allRecorder.Code)
	assert.Equal(t, []string{"claude-test", "gpt-test"}, allResponse.Data.Models)
	require.Len(t, allResponse.Data.Users, 2)
	assert.Equal(t, "alice", allResponse.Data.Users[0].Username)
	assert.Equal(t, "bob", allResponse.Data.Users[1].Username)
	assert.Empty(t, allResponse.Data.Tokens)

	userRecorder, userResponse := performPerfAnalyticsOptionsRequest(
		t,
		GetPerfAnalyticsAdminOptions,
		99,
		"user_id=2",
	)
	require.Equal(t, http.StatusOK, userRecorder.Code)
	assert.Equal(t, []string{"claude-test", "gpt-test"}, userResponse.Data.Models)
	require.Len(t, userResponse.Data.Tokens, 1)
	assert.Equal(t, 22, userResponse.Data.Tokens[0].Id)
	assert.Equal(t, 2, userResponse.Data.Tokens[0].UserId)
}
