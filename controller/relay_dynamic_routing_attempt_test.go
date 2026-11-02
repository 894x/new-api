package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/dynamicrouting"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldObserveDynamicRoutingAttemptHonorsBypasses(t *testing.T) {
	tests := []struct {
		name     string
		prepare  func(*gin.Context, *relaycommon.RelayInfo)
		expected bool
	}{
		{name: "ordinary relay", expected: true},
		{
			name: "non-stream relay",
			prepare: func(c *gin.Context, _ *relaycommon.RelayInfo) {
				common.SetContextKey(c, constant.ContextKeyDynamicRoutingEligible, false)
			},
		},
		{
			name: "forced specific channel",
			prepare: func(c *gin.Context, _ *relaycommon.RelayInfo) {
				c.Set("specific_channel_id", 42)
			},
		},
		{
			name: "channel test defense in depth",
			prepare: func(_ *gin.Context, info *relaycommon.RelayInfo) {
				info.IsChannelTest = true
			},
		},
		{
			name: "task locked channel defense in depth",
			prepare: func(_ *gin.Context, info *relaycommon.RelayInfo) {
				info.TaskRelayInfo = &relaycommon.TaskRelayInfo{LockedChannel: struct{}{}}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			info := &relaycommon.RelayInfo{}
			common.SetContextKey(c, constant.ContextKeyDynamicRoutingEligible, true)
			if tt.prepare != nil {
				tt.prepare(c, info)
			}
			assert.Equal(t, tt.expected, shouldObserveDynamicRoutingAttempt(c, info))
		})
	}
}

func TestGetChannelRestoresPublicModelBeforeRetrySetup(t *testing.T) {
	const publicModel = "gemini-2.5-pro"
	database, dialect := openTaskDialectDatabase(t, &model.Ability{}, &model.ModelRoutingPool{}, &model.UserGroupChannelRoutingOverride{})
	previousDB, previousMemoryCache := model.DB, common.MemoryCacheEnabled
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, common.MemoryCacheEnabled = database, false
	common.SetDatabaseTypes(dialect, previousLog)
	t.Cleanup(func() {
		model.DB, common.MemoryCacheEnabled = previousDB, previousMemoryCache
		common.SetDatabaseTypes(previousMain, previousLog)
	})
	require.NoError(t, database.Create(&[]model.Ability{
		{Group: "default", Model: publicModel, ChannelId: 1, Enabled: true},
		{Group: "default", Model: publicModel, ChannelId: 2, Enabled: true},
	}).Error)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:streamGenerateContent", nil)
	common.SetContextKey(c, constant.ContextKeyUserId, 1)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	c.Set("channel_id", 1)
	c.Set("channel_type", constant.ChannelTypeGemini)
	c.Set("channel_name", "first")

	info := &relaycommon.RelayInfo{OriginModelName: publicModel + "-nothinking"}
	_, getErr := getChannel(c, info, &service.RetryParam{ModelName: publicModel})
	require.Nil(t, getErr)
	assert.Equal(t, publicModel, info.OriginModelName)

	second := &model.Channel{Id: 2, Type: constant.ChannelTypeGemini, Name: "second", Key: "test-key", Models: publicModel, MonitorConfig: `{"targets":[{"model":"gemini-2.5-pro","endpoint_type":"gemini"}]}`}
	require.Nil(t, middleware.SetupContextForSelectedChannel(c, second, info.OriginModelName))
	assert.Equal(t, publicModel, c.GetString("original_model"))
	selected, getErr := getChannel(c, info, &service.RetryParam{ModelName: publicModel})
	require.Nil(t, getErr)
	assert.Equal(t, second.MonitorConfig, selected.MonitorConfig, "first-attempt context must retain the channel monitoring switch and targets")
	assert.Equal(t, second.Models, selected.Models)
	info.RelayFormat = types.RelayFormatGemini
	assert.Equal(t, "gemini", service.ChannelTrafficMonitoringEndpoint(selected, info, publicModel))
	info.InitChannelMeta(c)
	assert.Equal(t, publicModel, info.UpstreamModelName)
}

func TestDynamicRoutingSampleFromAttemptPreservesIdentityAndMetrics(t *testing.T) {
	at := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	startedAt := at.Add(-500 * time.Millisecond)
	attempt := relaycommon.DynamicRoutingAttemptSample{
		ChannelID:         17,
		Model:             "public-model",
		ObservedAt:        at,
		UpstreamStartedAt: startedAt,
		TTFT:              120 * time.Millisecond,
		TPOT:              25 * time.Millisecond,
		HasTTFT:           true,
		HasTPOT:           true,
		Success:           true,
		HardFailure:       false,
	}

	key, sample := dynamicRoutingSampleFromAttempt(attempt)

	assert.Equal(t, dynamicrouting.ObservationKey{ChannelID: 17, Model: "public-model"}, key)
	assert.Equal(t, dynamicrouting.Sample{
		ObservedAt:        at,
		UpstreamStartedAt: startedAt,
		TTFT:              120 * time.Millisecond,
		TPOT:              25 * time.Millisecond,
		HasTTFT:           true,
		HasTPOT:           true,
		Success:           true,
	}, sample)

	_, zeroStart := dynamicRoutingSampleFromAttempt(relaycommon.DynamicRoutingAttemptSample{ObservedAt: at})
	assert.True(t, zeroStart.UpstreamStartedAt.IsZero(), "a missing dispatch boundary must not become a recovery probe")
}

func TestFinishDynamicRoutingAttemptDiscardsCanceledRequest(t *testing.T) {
	requestContext, cancel := context.WithCancel(context.Background())
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(requestContext)
	info := &relaycommon.RelayInfo{IsStream: true}
	info.BeginDynamicRoutingAttempt(17, info.GetChannelType(), "public-model", true)
	info.MarkAttemptUpstreamStarted()
	cancel()
	hardErr := types.NewOpenAIError(
		errors.New("transport canceled"),
		types.ErrorCodeDoRequestFailed,
		http.StatusInternalServerError,
	)

	_, observed := finishDynamicRoutingAttempt(c, info, hardErr)

	assert.False(t, observed)
	_, observed = info.FinishDynamicRoutingAttempt(hardErr)
	assert.False(t, observed, "cancellation must clear the attempt before a later finish")
}

func TestShouldPublishDynamicRoutingAttemptKeepsHealthAndPerformanceSignals(t *testing.T) {
	tests := []struct {
		name    string
		sample  relaycommon.DynamicRoutingAttemptSample
		publish bool
	}{
		{name: "metadata-only success is health-only", sample: relaycommon.DynamicRoutingAttemptSample{Success: true}, publish: true},
		{name: "neutral attempt without health or performance", sample: relaycommon.DynamicRoutingAttemptSample{}},
		{name: "hard failure without timing", sample: relaycommon.DynamicRoutingAttemptSample{HardFailure: true}, publish: true},
		{name: "TTFT sample", sample: relaycommon.DynamicRoutingAttemptSample{Success: true, HasTTFT: true}, publish: true},
		{name: "TPOT sample", sample: relaycommon.DynamicRoutingAttemptSample{Success: true, HasTPOT: true}, publish: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.publish, shouldPublishDynamicRoutingAttempt(tt.sample))
		})
	}
}

func TestFinishDynamicRoutingAttemptReturnsCompletedAttempt(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{}
	info.BeginDynamicRoutingAttempt(17, info.GetChannelType(), "public-model", true)
	info.MarkAttemptUpstreamStarted()

	sample, observed := finishDynamicRoutingAttempt(c, info, nil)

	require.True(t, observed)
	assert.True(t, sample.Success)
}
