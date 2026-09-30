package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestProcessChannelErrorUsesSnapshotWithoutLeakingChannelMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	previousErrorLogEnabled := constant.ErrorLogEnabled

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.Log{}))
	model.DB, model.LOG_DB = database, database
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	constant.ErrorLogEnabled = true
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedisEnabled
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		constant.ErrorLogEnabled = previousErrorLogEnabled
		require.NoError(t, sqlDB.Close())
	})

	require.NoError(t, database.Create(&model.User{Id: 7, Username: "log-owner", Group: "default"}).Error)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set("id", 7)
	ctx.Set("username", "log-owner")
	ctx.Set("token_name", "test-token")
	ctx.Set("token_id", 11)
	ctx.Set("original_model", "gpt-test")
	ctx.Set("group", "default")
	ctx.Set("channel_id", 202)
	ctx.Set("channel_name", "mutable-context-channel")
	ctx.Set("channel_type", 9)
	ctx.Set("use_channel", []string{"101"})
	common.SetContextKey(ctx, constant.ContextKeyRequestStartTime, time.Now().Add(-time.Second))

	channelSnapshot := types.ChannelError{
		ChannelId:   101,
		ChannelType: 1,
		ChannelName: "snapshot-channel",
		AutoBan:     false,
	}
	apiErr := types.NewOpenAIError(errors.New("upstream failed"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)

	processChannelError(ctx, channelSnapshot, apiErr, nil)

	var stored model.Log
	require.NoError(t, database.First(&stored).Error)
	assert.Equal(t, channelSnapshot.ChannelId, stored.ChannelId)
	storedOther, err := common.StrToMap(stored.Other)
	require.NoError(t, err)
	assert.Equal(t, float64(http.StatusBadGateway), storedOther["status_code"])
	for _, key := range []string{"channel_id", "channel_name", "channel_type"} {
		assert.NotContains(t, storedOther, key)
	}
	adminInfo, ok := storedOther["admin_info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []any{"101"}, adminInfo["use_channel"])

	logs, total, err := model.GetUserLogs(7, model.LogTypeError, 0, 0, "", "", 0, 10, "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	assert.Equal(t, channelSnapshot.ChannelId, logs[0].ChannelId)
	assert.Empty(t, logs[0].ChannelName)
	userOther, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	assert.NotContains(t, userOther, "admin_info")
	for _, key := range []string{"channel_id", "channel_name", "channel_type"} {
		assert.NotContains(t, userOther, key)
	}
}

// Uses independent main/log databases. The existing dialect fixture also
// supports TEST_TASK_DB_DIALECT=mysql/postgres with dedicated test DSNs.
func setupRelayErrorLogDatabases(t *testing.T) (*gorm.DB, *gorm.DB) {
	t.Helper()
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	oldRedis, oldMemory := common.RedisEnabled, common.MemoryCacheEnabled
	oldErrorLog := constant.ErrorLogEnabled
	mainDB, dialect := openTaskDialectDatabase(t, &model.User{}, &model.Token{}, &model.Channel{}, &model.ChannelModelOverride{}, &model.UserChannelRoutingOverride{}, &model.UserSubscription{})
	logDB, logDialect := openTaskDialectDatabase(t, &model.Log{})
	model.DB, model.LOG_DB = mainDB, logDB
	common.SetDatabaseTypes(dialect, logDialect)
	common.RedisEnabled, common.MemoryCacheEnabled = false, false
	constant.ErrorLogEnabled = true
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLog)
		common.RedisEnabled, common.MemoryCacheEnabled = oldRedis, oldMemory
		constant.ErrorLogEnabled = oldErrorLog
	})
	require.NoError(t, mainDB.Create(&model.User{Id: 7, Username: "relay-log-owner", AffCode: "relay7", Group: "default", Quota: 100000}).Error)
	require.NoError(t, mainDB.Create(&model.Token{Id: 11, UserId: 7, Name: "test-token", Key: "relay-log-test-key", RemainQuota: 100000}).Error)
	return mainDB, logDB
}

func TestRelayPreflightFailuresRecordUsageLogs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mainDB, logDB := setupRelayErrorLogDatabases(t)
	oldCount, oldMedia, oldNonStream := constant.CountToken, constant.GetMediaToken, constant.GetMediaTokenNotStream
	oldWorker := system_setting.WorkerUrl
	oldFetch := *system_setting.GetFetchSetting()
	constant.CountToken, constant.GetMediaToken, constant.GetMediaTokenNotStream = true, true, true
	system_setting.WorkerUrl = ""
	system_setting.GetFetchSetting().EnableSSRFProtection = false
	service.InitHttpClient()
	t.Cleanup(func() {
		constant.CountToken, constant.GetMediaToken, constant.GetMediaTokenNotStream = oldCount, oldMedia, oldNonStream
		system_setting.WorkerUrl = oldWorker
		*system_setting.GetFetchSetting() = oldFetch
	})
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer upstream.Close()
	channel := &model.Channel{Id: 101, Type: constant.ChannelTypeOpenAI, Name: "preflight-channel", Key: "test-key", Models: "gpt-4o", Group: "default", Status: common.ChannelStatusEnabled, BaseURL: &upstream.URL, AutoBan: common.GetPointer(1)}
	require.NoError(t, mainDB.Create(channel).Error)

	for _, tc := range []struct {
		name       string
		mediaCode  int
		body       string
		wantStatus int
		wantCode   types.ErrorCode
	}{
		{name: "missing media", mediaCode: 404, wantStatus: 400, wantCode: types.ErrorCodeInvalidRequest},
		{name: "inaccessible media", mediaCode: 403, wantStatus: 400, wantCode: types.ErrorCodeInvalidRequest},
		{name: "expired media", mediaCode: 410, wantStatus: 400, wantCode: types.ErrorCodeInvalidRequest},
		{name: "unauthorized media", mediaCode: 401, wantStatus: 400, wantCode: types.ErrorCodeInvalidRequest},
		{name: "bad media request", mediaCode: 400, wantStatus: 400, wantCode: types.ErrorCodeInvalidRequest},
		{name: "media server failed", mediaCode: 503, wantStatus: 500, wantCode: types.ErrorCodeCountTokenFailed},
		{name: "media rate limited", mediaCode: 429, wantStatus: 500, wantCode: types.ErrorCodeCountTokenFailed},
		{name: "media network failed", mediaCode: -1, wantStatus: 500, wantCode: types.ErrorCodeCountTokenFailed},
		{name: "empty messages", body: `{"model":"gpt-4o","messages":[]}`, wantStatus: 400, wantCode: types.ErrorCodeInvalidRequest},
		{name: "malformed JSON", body: `{"model":`, wantStatus: 400, wantCode: types.ErrorCodeInvalidRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.body
			if tc.mediaCode != 0 {
				media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.mediaCode) }))
				t.Cleanup(media.Close)
				if tc.mediaCode == -1 {
					media.Close()
				}
				body = fmt.Sprintf(`{"model":"gpt-4o","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":%q}}]}]}`, media.URL+"/missing.png?signature=private-test-signature")
			}
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			ctx.Request.Header.Set("Content-Type", "application/json")
			t.Cleanup(func() { common.CleanupBodyStorage(ctx); service.CleanupFileSources(ctx) })
			requestID := "preflight-" + strings.ReplaceAll(tc.name, " ", "-")
			ctx.Set(common.RequestIdKey, requestID)
			ctx.Set("id", 7)
			ctx.Set("username", "relay-log-owner")
			ctx.Set("role", common.RoleAdminUser)
			ctx.Set("group", "default")
			ctx.Set("token_group", "default")
			ctx.Set("using_group", "default")
			ctx.Set("token_id", 11)
			ctx.Set("token_name", "test-token")
			ctx.Set("specific_channel_id", "101")
			require.Nil(t, middleware.SetupContextForSelectedChannel(ctx, channel, "gpt-4o"))
			common.StartRequestTiming(ctx, time.Now())
			Relay(ctx, types.RelayFormatOpenAI)
			assert.Equal(t, tc.wantStatus, recorder.Code, recorder.Body.String())
			var response struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.Equal(t, string(tc.wantCode), response.Error.Code)
			assert.NotContains(t, recorder.Body.String(), "private-test-signature")
			var rows []model.Log
			require.NoError(t, logDB.Where("request_id = ?", requestID).Find(&rows).Error)
			require.Len(t, rows, 1)
			row := rows[0]
			assert.Equal(t, model.LogTypeError, row.Type)
			assert.Equal(t, 7, row.UserId)
			assert.Equal(t, 11, row.TokenId)
			assert.Equal(t, "gpt-4o", row.ModelName)
			assert.Zero(t, row.Quota)
			assert.Zero(t, row.PromptTokens)
			assert.Zero(t, row.CompletionTokens)
			assert.Empty(t, row.UpstreamRequestId)
			assert.NotContains(t, row.Content, "private-test-signature")
			other, err := common.StrToMap(row.Other)
			require.NoError(t, err)
			assert.Equal(t, float64(tc.wantStatus), other["status_code"])
			assert.Equal(t, string(tc.wantCode), other["error_code"])
			admin, ok := other["admin_info"].(map[string]any)
			require.True(t, ok)
			if tc.mediaCode != 0 {
				assert.Equal(t, "token_count", admin["failure_stage"])
				if tc.mediaCode > 0 {
					assert.Equal(t, float64(tc.mediaCode), admin["media_download_status_code"])
				}
			} else {
				assert.Equal(t, "request_validation", admin["failure_stage"])
			}
		})
	}
	assert.Zero(t, upstreamCalls.Load())
	var user model.User
	var token model.Token
	var storedChannel model.Channel
	require.NoError(t, mainDB.First(&user, 7).Error)
	require.NoError(t, mainDB.First(&token, 11).Error)
	require.NoError(t, mainDB.First(&storedChannel, 101).Error)
	assert.Equal(t, 100000, user.Quota)
	assert.Equal(t, 100000, token.RemainQuota)
	assert.Equal(t, common.ChannelStatusEnabled, storedChannel.Status)
}

func TestRelayErrorLogDeduplicationAndVisibility(t *testing.T) {
	_, logDB := setupRelayErrorLogDatabases(t)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set("id", 7)
	ctx.Set("username", "relay-log-owner")
	ctx.Set("token_id", 11)
	ctx.Set(common.RequestIdKey, "attempt-error-logs")
	info := &relaycommon.RelayInfo{RetryIndex: 0}
	apiErr := types.NewErrorWithStatusCode(errors.New("upstream unavailable"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)
	processChannelError(ctx, types.ChannelError{ChannelId: 101}, apiErr, info)
	service.RecordRelayErrorLog(ctx, 101, apiErr, info, "relay_attempt")
	info.RetryIndex = 1
	processChannelError(ctx, types.ChannelError{ChannelId: 101}, apiErr, info)
	localErr := service.TokenCountAPIError(fmt.Errorf("media input: %w", &service.FileDownloadError{StatusCode: 404}))
	service.RecordRelayErrorLog(ctx, 101, localErr, info, "billing_preparation")
	service.RecordRelayErrorLog(ctx, 101, localErr, info, "billing_preparation")
	violationErr := service.NormalizeViolationFeeError(types.NewError(errors.New(service.CSAMViolationMarker), types.ErrorCodeBadResponse))
	processChannelError(ctx, types.ChannelError{ChannelId: 101}, violationErr, info)
	service.RecordRelayErrorLog(ctx, 101, service.NormalizeViolationFeeError(violationErr), info, "relay_attempt")
	ctx.Writer.WriteHeaderNow()
	committedErr := types.MarkResponseCommitted(types.NewError(errors.New("stream terminated"), types.ErrorCodeBadResponse))
	processChannelError(ctx, types.ChannelError{ChannelId: 101}, committedErr, info)
	service.RecordRelayErrorLog(ctx, 101, committedErr, info, "relay_attempt")
	service.RecordRelayErrorLog(ctx, 101, nil, info, "relay_attempt")
	constant.ErrorLogEnabled = false
	service.RecordRelayErrorLog(ctx, 101, types.NewError(errors.New("disabled"), types.ErrorCodeInvalidRequest), info, "request_validation")
	constant.ErrorLogEnabled = true
	service.RecordRelayErrorLog(ctx, 101, types.NewError(errors.New("excluded"), types.ErrorCodeInvalidRequest, types.ErrOptionWithNoRecordErrorLog()), info, "request_validation")
	var count int64
	require.NoError(t, logDB.Model(&model.Log{}).Count(&count).Error)
	assert.Equal(t, int64(5), count, "retain separate attempts and terminal failures without duplicate fallback rows, including normalized and committed errors")
	logs, err := model.GetLogByTokenId(11)
	require.NoError(t, err)
	require.Len(t, logs, 5)
	oldHide := operation_setting.GetErrorSetting().HideErrorDetails
	operation_setting.UpdateHideErrorDetails(true)
	t.Cleanup(func() { operation_setting.UpdateHideErrorDetails(oldHide) })
	sanitizeUserErrorLogs(ctx, logs)
	for _, row := range logs {
		assert.Equal(t, service.PublicErrorMessage(row.RequestId), row.Content)
		other, err := common.StrToMap(row.Other)
		require.NoError(t, err)
		assert.NotContains(t, other, "admin_info")
		assert.NotContains(t, other, "error_code")
	}
	var downloadErr *service.FileDownloadError
	require.ErrorAs(t, localErr, &downloadErr)
	assert.True(t, types.IsSkipRetryError(localErr))
	internal := service.TokenCountAPIError(errors.New("tokenizer initialization failed"))
	assert.Equal(t, http.StatusInternalServerError, internal.StatusCode)
}

func TestRelayTerminalErrorLogsDoNotDuplicateChannelErrors(t *testing.T) {
	mainDB, logDB := setupRelayErrorLogDatabases(t)
	oldCount, oldConsume := constant.CountToken, common.LogConsumeEnabled
	oldRatios := ratio_setting.ModelRatio2JSONString()
	constant.CountToken, common.LogConsumeEnabled = false, false
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"relay-error-log-test":0}`))
	t.Cleanup(func() {
		constant.CountToken, common.LogConsumeEnabled = oldCount, oldConsume
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatios))
	})
	service.InitHttpClient()
	for _, tc := range []struct {
		name       string
		status     int
		body       string
		wantErrors int
	}{
		{name: "channel failure", status: 502, body: `{"error":{"message":"upstream unavailable","type":"server_error"}}`, wantErrors: 1},
		{name: "successful request", status: 200, body: `{"id":"test-response","object":"chat.completion","model":"relay-error-log-test","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`, wantErrors: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, err := w.Write([]byte(tc.body))
				assert.NoError(t, err)
			}))
			t.Cleanup(upstream.Close)
			channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: tc.name, Key: "test-key", Models: "relay-error-log-test", Group: "default", Status: common.ChannelStatusEnabled, BaseURL: &upstream.URL, AutoBan: common.GetPointer(0)}
			require.NoError(t, mainDB.Create(channel).Error)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"relay-error-log-test","messages":[{"role":"user","content":"hello"}]}`))
			ctx.Request.Header.Set("Content-Type", "application/json")
			t.Cleanup(func() { common.CleanupBodyStorage(ctx) })
			ctx.Set("id", 7)
			ctx.Set("role", common.RoleAdminUser)
			ctx.Set("username", "relay-log-owner")
			ctx.Set("group", "default")
			ctx.Set("user_group", "default")
			ctx.Set("token_group", "default")
			ctx.Set("token_id", 11)
			ctx.Set("token_name", "test-token")
			ctx.Set("specific_channel_id", fmt.Sprint(channel.Id))
			ctx.Set(common.RequestIdKey, tc.name)
			require.Nil(t, middleware.SetupContextForSelectedChannel(ctx, channel, "relay-error-log-test"))
			Relay(ctx, types.RelayFormatOpenAI)
			assert.Equal(t, tc.status, recorder.Code, recorder.Body.String())
			var count int64
			require.NoError(t, logDB.Model(&model.Log{}).Where("request_id = ?", tc.name).Count(&count).Error)
			assert.Equal(t, int64(tc.wantErrors), count)
		})
	}
}

func TestRelayErrorLogFailedInsertCanBeRetried(t *testing.T) {
	_, logDB := setupRelayErrorLogDatabases(t)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("id", 7)
	ctx.Set(common.RequestIdKey, "failed-error-insert")
	apiErr := types.NewErrorWithStatusCode(errors.New("invalid input"), types.ErrorCodeInvalidRequest, http.StatusBadRequest)
	const callback = "test:reject_relay_error_insert"
	require.NoError(t, logDB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) { tx.AddError(errors.New("log storage unavailable")) }))
	t.Cleanup(func() { _ = logDB.Callback().Create().Remove(callback) })
	service.RecordRelayErrorLog(ctx, 0, apiErr, nil, "request_validation")
	require.NoError(t, logDB.Callback().Create().Remove(callback))
	service.RecordRelayErrorLog(ctx, 0, apiErr, nil, "request_validation")
	var rows []model.Log
	require.NoError(t, logDB.Where("request_id = ?", "failed-error-insert").Find(&rows).Error)
	require.Len(t, rows, 1, "failed inserts must not suppress later recording of the same event")
}
