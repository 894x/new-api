package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessChannelErrorRecordsRequestTiming(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))

	originalErrorLogEnabled := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = true
	t.Cleanup(func() {
		constant.ErrorLogEnabled = originalErrorLogEnabled
	})

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set("id", 1)
	ctx.Set("token_name", "test-token")
	ctx.Set("original_model", "kimi-k3")
	ctx.Set("token_id", 2)
	ctx.Set("group", "default")
	ctx.Set("channel_id", 14)
	ctx.Set("channel_name", "from waninter")
	ctx.Set("channel_type", 1)
	common.SetContextKey(ctx, constant.ContextKeyIsStream, true)

	receivedAt := time.Now().Add(-2 * time.Second).Truncate(time.Millisecond)
	common.SetContextKey(ctx, constant.ContextKeyRequestStartTime, receivedAt)
	timing := common.StartRequestTiming(ctx, receivedAt)
	timing.Mark(common.RequestTimingBodyRead, receivedAt.Add(10*time.Millisecond))
	timing.Mark(common.RequestTimingUpstreamRequestStarted, receivedAt.Add(20*time.Millisecond))
	timing.Mark(common.RequestTimingUpstreamRequestWritten, receivedAt.Add(30*time.Millisecond))
	timing.Mark(common.RequestTimingUpstreamResponseHeaders, receivedAt.Add(40*time.Millisecond))

	processChannelError(
		ctx,
		types.ChannelError{ChannelId: 14, ChannelName: "from waninter"},
		"mapped-kimi-k3",
		types.NewErrorWithStatusCode(errors.New("upstream unavailable"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway),
	)

	var log model.Log
	require.NoError(t, db.Where("type = ?", model.LogTypeError).First(&log).Error)

	var other struct {
		UpstreamModelName string `json:"upstream_model_name"`
		AdminInfo         struct {
			RequestTiming common.RequestTimingSnapshot `json:"request_timing"`
		} `json:"admin_info"`
	}
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	assert.Equal(t, "mapped-kimi-k3", other.UpstreamModelName)
	assert.Equal(t, receivedAt.UnixMilli(), other.AdminInfo.RequestTiming.RequestReceivedAtMs)
	assert.Equal(t, receivedAt.Add(40*time.Millisecond).UnixMilli(), other.AdminInfo.RequestTiming.UpstreamResponseHeadersAtMs)
	assert.GreaterOrEqual(t, other.AdminInfo.RequestTiming.RequestCompletedAtMs, receivedAt.Add(40*time.Millisecond).UnixMilli())
	assert.Zero(t, timing.Snapshot().RequestCompletedAtMs, "an attempt error must not complete the shared request timeline before retrying")
}
