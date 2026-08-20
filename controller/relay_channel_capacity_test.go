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
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var relayCapacityTestSequence atomic.Int64

func setupRelayChannelCapacityTest(t *testing.T) *gorm.DB {
	t.Helper()
	originalDB := model.DB
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalRedisEnabled := common.RedisEnabled
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelModelOverride{}))
	model.DB = db
	common.MemoryCacheEnabled = true
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = originalDB
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		common.RedisEnabled = originalRedisEnabled
		if originalMemoryCacheEnabled && originalDB != nil && originalDB.Migrator().HasTable(&model.Channel{}) {
			model.InitChannelCache()
		}
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})
	return db
}

func createRelayCapacityChannel(t *testing.T, id int, modelName string, priority int64, rpm int64) *model.Channel {
	t.Helper()
	weight := uint(0)
	channel := &model.Channel{
		Id: id, Type: constant.ChannelTypeOpenAI, Key: "test-key", Status: common.ChannelStatusEnabled,
		Name: fmt.Sprintf("capacity-%d", id), Models: modelName, Group: "default",
		Priority: &priority, Weight: &weight, RPM: &rpm,
	}
	require.NoError(t, channel.Insert())
	return channel
}

func newRelayCapacityContext(t *testing.T, channel *model.Channel, modelName string) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	require.Nil(t, middleware.SetupContextForSelectedChannel(c, channel, modelName))
	return c
}

func TestCapacityAdmissionFailureSpillsWithoutRetryAndForcesLocal429(t *testing.T) {
	const modelName = "controller-capacity-model"
	newAttemptError := func() *types.NewAPIError {
		return types.NewErrorWithStatusCode(
			&service.ChannelModelCapacityAdmissionError{ChannelID: 6601, Model: modelName, RetryAfter: 55 * time.Second},
			types.ErrorCodeChannelModelCapacityExhausted,
			http.StatusTooManyRequests,
			types.ErrOptionWithSkipRetry(),
		)
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	retry := 0
	param := &service.RetryParam{Ctx: c, TokenGroup: "default", ModelName: modelName, Retry: &retry}
	handled, finalErr := handleChannelModelCapacityAdmissionFailure(c, param, newAttemptError())
	require.True(t, handled)
	assert.Nil(t, finalErr)
	param.IncreaseRetry()
	assert.Zero(t, retry, "capacity spillover must not consume an upstream retry")

	forcedRecorder := httptest.NewRecorder()
	forcedContext, _ := gin.CreateTestContext(forcedRecorder)
	forcedContext.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	forcedContext.Set("specific_channel_id", 6601)
	forcedContext.Set("channel_id", 6601)
	forcedRetry := 0
	forcedParam := &service.RetryParam{Ctx: forcedContext, TokenGroup: "default", ModelName: modelName, Retry: &forcedRetry}
	handled, finalErr = handleChannelModelCapacityAdmissionFailure(forcedContext, forcedParam, newAttemptError())
	require.True(t, handled)
	require.NotNil(t, finalErr)
	assert.Equal(t, http.StatusTooManyRequests, finalErr.StatusCode)
	assert.Equal(t, types.ErrorCodeChannelModelCapacityExhausted, finalErr.GetErrorCode())
	assert.Equal(t, "55", forcedRecorder.Header().Get("Retry-After"))
	assert.Equal(t, 6601, forcedContext.GetInt("channel_id"), "a forced channel must not silently spill")
	assert.Zero(t, forcedRetry)
}

func TestCapacityFallbackClearsOnlyCurrentAffinitySelectionMarkers(t *testing.T) {
	setupRelayChannelCapacityTest(t)
	sequence := relayCapacityTestSequence.Add(1)
	modelName := fmt.Sprintf("affinity-capacity-model-%d", sequence)
	highID := 7600 + int(sequence)*10
	high := createRelayCapacityChannel(t, highID, modelName, 100, 1)
	model.InitChannelCache()

	affinitySetting := operation_setting.GetChannelAffinitySetting()
	require.NotNil(t, affinitySetting)
	originalSetting := *affinitySetting
	affinitySetting.Enabled = true
	affinitySetting.SwitchOnSuccess = false
	affinitySetting.Rules = append([]operation_setting.ChannelAffinityRule{{
		Name:               "capacity-fallback-test",
		ModelRegex:         []string{"^" + modelName + "$"},
		PathRegex:          []string{"^/v1/chat/completions$"},
		KeySources:         []operation_setting.ChannelAffinityKeySource{{Type: "request_header", Key: "X-Test-Affinity"}},
		SkipRetryOnFailure: true,
		IncludeModelName:   true,
		IncludeRuleName:    true,
	}}, affinitySetting.Rules...)
	t.Cleanup(func() { *affinitySetting = originalSetting })

	affinityContext := newRelayCapacityContext(t, high, modelName)
	affinityContext.Request.Header.Set("X-Test-Affinity", fmt.Sprintf("request-%d", sequence))
	_, found := service.GetPreferredChannelByAffinity(affinityContext, modelName, "default")
	require.False(t, found)
	service.RecordChannelAffinity(affinityContext, high.Id)
	preferredID, found := service.GetPreferredChannelByAffinity(affinityContext, modelName, "default")
	require.True(t, found)
	require.Equal(t, high.Id, preferredID)
	service.MarkChannelAffinityUsed(affinityContext, "default", high.Id)
	require.True(t, service.ShouldSkipRetryAfterChannelAffinityFailure(affinityContext))
	t.Cleanup(func() { service.ClearCurrentChannelAffinityCache(affinityContext) })

	retry := 0
	param := &service.RetryParam{Ctx: affinityContext, TokenGroup: "default", ModelName: modelName, Retry: &retry}
	attemptErr := types.NewErrorWithStatusCode(
		&service.ChannelModelCapacityAdmissionError{ChannelID: high.Id, Model: modelName, RetryAfter: time.Second},
		types.ErrorCodeChannelModelCapacityExhausted,
		http.StatusTooManyRequests,
		types.ErrOptionWithSkipRetry(),
	)
	handled, finalErr := handleChannelModelCapacityAdmissionFailure(affinityContext, param, attemptErr)
	require.True(t, handled)
	assert.Nil(t, finalErr)
	param.IncreaseRetry()
	assert.Zero(t, retry)
	assert.False(t, service.ShouldSkipRetryAfterChannelAffinityFailure(affinityContext))
	adminInfo := map[string]interface{}{}
	service.AppendChannelAffinityAdminInfo(affinityContext, adminInfo)
	assert.NotContains(t, adminInfo, "channel_affinity")
	preferredID, found = service.GetPreferredChannelByAffinity(affinityContext, modelName, "default")
	require.True(t, found)
	assert.Equal(t, high.Id, preferredID, "capacity spillover must retain the long-lived affinity pin")
	upstreamErr := types.NewErrorWithStatusCode(errors.New("fallback failed"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	assert.True(t, shouldRetry(affinityContext, upstreamErr, 1), "the fallback channel must use ordinary retry semantics")
}
