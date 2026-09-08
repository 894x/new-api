package common

import (
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDynamicRoutingAttemptRetryKeepsPerAttemptIdentityAndTiming(t *testing.T) {
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	times := []time.Time{
		base,
		base.Add(100 * time.Millisecond),
		base.Add(120 * time.Millisecond),
		base.Add(200 * time.Millisecond),
		base.Add(time.Second),
		base.Add(1100 * time.Millisecond),
		base.Add(1250 * time.Millisecond),
		base.Add(2 * time.Second),
		base.Add(3 * time.Second),
	}
	next := 0
	info := &RelayInfo{
		StartTime:         base.Add(-time.Minute),
		FirstResponseTime: base.Add(-time.Minute - time.Second),
		isFirstResponse:   true,
		IsStream:          true,
	}
	info.setAttemptNowForTest(func() time.Time {
		at := times[next]
		next++
		return at
	})

	info.BeginDynamicRoutingAttempt(11, info.GetChannelType(), "public-model", true)
	info.MarkAttemptUpstreamStarted()
	info.SetFirstResponseTime()
	info.RecordAttemptVisibleText("x")
	info.SetAttemptHTTPStatus(http.StatusServiceUnavailable)
	info.StreamStatus = NewStreamStatus()
	info.StreamStatus.SetEndReason(StreamEndReasonDone, nil)
	first, ok := info.FinishDynamicRoutingAttempt(types.NewOpenAIError(
		errors.New("upstream unavailable"),
		types.ErrorCodeBadResponse,
		http.StatusServiceUnavailable,
	))
	require.True(t, ok)
	assert.Equal(t, 11, first.ChannelID)
	assert.Equal(t, "public-model", first.Model)
	assert.True(t, first.HardFailure)
	assert.False(t, first.Success)
	assert.False(t, first.HasTTFT)
	assert.Equal(t, base.Add(100*time.Millisecond), info.FirstResponseTime)

	info.BeginDynamicRoutingAttempt(22, info.GetChannelType(), "public-model", true)
	info.MarkAttemptUpstreamStarted()
	info.SetFirstResponseTime()
	info.RecordAttemptVisibleText("x")
	info.RecordAttemptVisibleText("x")
	info.SetAttemptCompletionTokens(4)
	info.StreamStatus = NewStreamStatus()
	info.StreamStatus.SetEndReason(StreamEndReasonDone, nil)
	second, ok := info.FinishDynamicRoutingAttempt(nil)
	require.True(t, ok)
	assert.Equal(t, 22, second.ChannelID)
	assert.Equal(t, "public-model", second.Model)
	assert.True(t, second.Success)
	assert.False(t, second.HardFailure)
	assert.True(t, second.HasTTFT)
	assert.Equal(t, 250*time.Millisecond, second.TTFT)
	assert.True(t, second.HasTPOT)
	assert.Equal(t, 250*time.Millisecond, second.TPOT)
	assert.Equal(t, 4, second.CompletionTokens)
	assert.Equal(t, base.Add(100*time.Millisecond), info.FirstResponseTime,
		"retry must not reset the request-level first response time")
	assert.Equal(t, len(times), next)
}

func TestDynamicRoutingAttemptCapturesSelectedChannelTypeAcrossRetry(t *testing.T) {
	info := &RelayInfo{
		IsStream:    true,
		ChannelMeta: &ChannelMeta{ChannelType: constant.ChannelTypeOpenAI},
	}

	info.BeginDynamicRoutingAttempt(2, constant.ChannelTypeAzure, "public-model", true)
	info.MarkAttemptUpstreamStarted()
	info.SetAttemptHTTPStatus(http.StatusNotFound)
	info.StreamStatus = NewStreamStatus()
	info.StreamStatus.SetEndReason(StreamEndReasonHandlerStop, nil)
	azureMissing := types.WithOpenAIError(types.OpenAIError{Message: "deployment missing", Code: "DeploymentNotFound"}, http.StatusNotFound)
	azureSample, observed := info.FinishDynamicRoutingAttempt(azureMissing)
	require.True(t, observed)
	assert.True(t, azureSample.HardFailure, "the selected Azure attempt must not inherit the prior OpenAI channel type")

	info.ChannelMeta.ChannelType = constant.ChannelTypeAzure
	info.BeginDynamicRoutingAttempt(3, constant.ChannelTypeOpenAI, "public-model", true)
	info.MarkAttemptUpstreamStarted()
	info.SetAttemptHTTPStatus(http.StatusNotFound)
	info.StreamStatus = NewStreamStatus()
	info.StreamStatus.SetEndReason(StreamEndReasonHandlerStop, nil)
	ordinaryMissing := types.WithOpenAIError(types.OpenAIError{Message: "resource missing", Code: "resource_not_found"}, http.StatusNotFound)
	openAISample, observed := info.FinishDynamicRoutingAttempt(ordinaryMissing)
	require.True(t, observed)
	assert.False(t, openAISample.HardFailure, "a current OpenAI 404 must not inherit the stale Azure channel type")
}

func TestDynamicRoutingAttemptVisibleContentDefinesTTFTWhileRequestKeepsFirstEvent(t *testing.T) {
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	times := []time.Time{
		base,
		base.Add(40 * time.Millisecond),  // metadata-only first SSE event
		base.Add(100 * time.Millisecond), // first visible-content SSE event
		base.Add(300 * time.Millisecond), // last visible-content SSE event
		base.Add(time.Second),            // handler return after billing work
	}
	next := 0
	info := &RelayInfo{StartTime: base.Add(-time.Second), isFirstResponse: true, IsStream: true}
	info.setAttemptNowForTest(func() time.Time {
		at := times[next]
		next++
		return at
	})

	info.BeginDynamicRoutingAttempt(7, info.GetChannelType(), "model", true)
	info.MarkAttemptUpstreamStarted()
	info.SetFirstResponseTime()
	info.RecordAttemptVisibleText("x")
	info.RecordAttemptVisibleText("x")
	info.SetAttemptCompletionTokens(3)
	info.StreamStatus = NewStreamStatus()
	info.StreamStatus.SetEndReason(StreamEndReasonDone, nil)
	sample, ok := info.FinishDynamicRoutingAttempt(nil)

	require.True(t, ok)
	assert.Equal(t, base.Add(40*time.Millisecond), sample.FirstResponseAt)
	assert.Equal(t, base.Add(100*time.Millisecond), sample.FirstContentAt)
	assert.Equal(t, 100*time.Millisecond, sample.TTFT)
	assert.Equal(t, 100*time.Millisecond, sample.TPOT)
	assert.Equal(t, base.Add(40*time.Millisecond), info.FirstResponseTime,
		"request-level timing retains the historical first non-DONE event boundary")
	assert.Equal(t, len(times), next)
}

func TestDynamicRoutingAttemptTPOTStopsBeforeBillingPostProcessing(t *testing.T) {
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	now := base
	info := &RelayInfo{IsStream: true}
	info.setAttemptNowForTest(func() time.Time { return now })

	info.BeginDynamicRoutingAttempt(7, info.GetChannelType(), "model", true)
	info.MarkAttemptUpstreamStarted()
	now = base.Add(100 * time.Millisecond)
	info.RecordAttemptVisibleText("x")
	now = base.Add(300 * time.Millisecond)
	info.RecordAttemptVisibleText("x")
	now = base.Add(800 * time.Millisecond) // slow billing/post-processing starts later
	info.SetAttemptCompletionTokens(3)
	info.StreamStatus = NewStreamStatus()
	info.StreamStatus.SetEndReason(StreamEndReasonDone, nil)
	now = base.Add(time.Second) // billing, DB, and Redis work completed later

	sample, ok := info.FinishDynamicRoutingAttempt(nil)

	require.True(t, ok)
	assert.Equal(t, base.Add(time.Second), sample.ObservedAt)
	assert.True(t, sample.HasTPOT)
	assert.Equal(t, 100*time.Millisecond, sample.TPOT,
		"TPOT must end at the last visible output event, not after billing post-processing")
}

func TestDynamicRoutingAttemptKeepsFirstReliableTokenBoundaryImmutable(t *testing.T) {
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	now := base
	info := &RelayInfo{IsStream: true}
	info.setAttemptNowForTest(func() time.Time { return now })
	info.BeginDynamicRoutingAttempt(7, info.GetChannelType(), "model", true)
	info.MarkAttemptUpstreamStarted()
	now = base.Add(100 * time.Millisecond)
	info.RecordAttemptVisibleText("x")
	now = base.Add(300 * time.Millisecond)
	info.RecordAttemptVisibleText("x")
	info.SetAttemptCompletionTokens(3)
	now = base.Add(800 * time.Millisecond)
	info.SetAttemptCompletionTokens(10)
	info.StreamStatus = NewStreamStatus()
	info.StreamStatus.SetEndReason(StreamEndReasonDone, nil)
	now = base.Add(time.Second)

	sample, ok := info.FinishDynamicRoutingAttempt(nil)

	require.True(t, ok)
	assert.Equal(t, 3, sample.CompletionTokens)
	assert.Equal(t, 100*time.Millisecond, sample.TPOT)
}

func TestDynamicRoutingAttemptTPOTUsesFirstAndLastVisibleContent(t *testing.T) {
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	now := base
	info := &RelayInfo{IsStream: true}
	info.setAttemptNowForTest(func() time.Time { return now })
	info.BeginDynamicRoutingAttempt(7, info.GetChannelType(), "model", true)
	info.MarkAttemptUpstreamStarted()
	now = base.Add(100 * time.Millisecond)
	info.RecordAttemptVisibleText("x")
	now = base.Add(180 * time.Millisecond)
	info.RecordAttemptVisibleText("x")
	now = base.Add(300 * time.Millisecond)
	info.RecordAttemptVisibleText("x")
	now = base.Add(900 * time.Millisecond)
	info.SetAttemptCompletionTokens(3)
	info.StreamStatus = NewStreamStatus()
	info.StreamStatus.SetEndReason(StreamEndReasonDone, nil)
	now = base.Add(2 * time.Second)

	sample, ok := info.FinishDynamicRoutingAttempt(nil)

	require.True(t, ok)
	assert.Equal(t, base.Add(100*time.Millisecond), sample.FirstContentAt)
	assert.Equal(t, base.Add(300*time.Millisecond), sample.LastContentAt)
	assert.Equal(t, 100*time.Millisecond, sample.TTFT)
	assert.Equal(t, 100*time.Millisecond, sample.TPOT)
	assert.Equal(t, base.Add(2*time.Second), sample.ObservedAt)
}

func TestDynamicRoutingAttemptBackpressureKeepsTTFTButInvalidatesTPOT(t *testing.T) {
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	now := base
	info := &RelayInfo{IsStream: true}
	info.setAttemptNowForTest(func() time.Time { return now })
	info.BeginDynamicRoutingAttempt(7, info.GetChannelType(), "model", true)
	info.MarkAttemptUpstreamStarted()
	now = base.Add(100 * time.Millisecond)
	info.RecordAttemptVisibleText("first")
	now = base.Add(300 * time.Millisecond)
	info.RecordAttemptVisibleText("last")
	info.SetAttemptCompletionTokens(3)
	info.InvalidateDynamicRoutingAttemptTPOT()
	info.StreamStatus = NewStreamStatus()
	info.StreamStatus.SetEndReason(StreamEndReasonDone, nil)
	now = base.Add(time.Second)

	sample, ok := info.FinishDynamicRoutingAttempt(nil)

	require.True(t, ok)
	assert.True(t, sample.HasTTFT)
	assert.Equal(t, 100*time.Millisecond, sample.TTFT)
	assert.False(t, sample.TTFTInvalidated)
	assert.True(t, sample.TPOTInvalidated)
	assert.False(t, sample.HasTPOT)
	assert.Zero(t, sample.TPOT)
}

func TestDynamicRoutingAttemptBackpressureBeforeVisibleContentInvalidatesTTFT(t *testing.T) {
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	now := base
	info := &RelayInfo{IsStream: true}
	info.setAttemptNowForTest(func() time.Time { return now })
	info.BeginDynamicRoutingAttempt(7, info.GetChannelType(), "model", true)
	info.MarkAttemptUpstreamStarted()
	info.MarkDynamicRoutingAttemptBackpressure()
	now = base.Add(100 * time.Millisecond)
	info.RecordAttemptVisibleText("first")
	now = base.Add(300 * time.Millisecond)
	info.RecordAttemptVisibleText("last")
	info.SetAttemptCompletionTokens(3)
	info.StreamStatus = NewStreamStatus()
	info.StreamStatus.SetEndReason(StreamEndReasonDone, nil)

	sample, ok := info.FinishDynamicRoutingAttempt(nil)

	require.True(t, ok)
	assert.True(t, sample.TTFTInvalidated)
	assert.True(t, sample.TPOTInvalidated)
	assert.False(t, sample.HasTTFT)
	assert.Zero(t, sample.TTFT)
	assert.False(t, sample.HasTPOT)
	assert.Zero(t, sample.TPOT)
}

func TestDynamicRoutingAttemptAccumulatesOnlySuppliedVisibleTextAndResetsOnRetry(t *testing.T) {
	info := &RelayInfo{}
	info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), "public-model", true)
	info.MarkAttemptUpstreamStarted()
	info.RecordAttemptVisibleText("visible ")
	info.RecordAttemptVisibleText("answer")

	assert.Equal(t, "visible answer", info.DynamicRoutingAttemptVisibleText())

	info.BeginDynamicRoutingAttempt(2, info.GetChannelType(), "public-model", true)
	assert.Empty(t, info.DynamicRoutingAttemptVisibleText())
}

func TestDynamicRoutingAttemptRetryKeepsImmutablePublicModelAfterBillingMutation(t *testing.T) {
	info := &RelayInfo{OriginModelName: "gemini-2.5-pro", IsStream: true}
	publicModel := info.OriginModelName

	info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), publicModel, true)
	info.MarkAttemptUpstreamStarted()
	info.OriginModelName = "gemini-2.5-pro-nothinking"
	first, ok := info.FinishDynamicRoutingAttempt(types.NewOpenAIError(
		errors.New("first channel failed"),
		types.ErrorCodeBadResponse,
		http.StatusServiceUnavailable,
	))
	require.True(t, ok)
	assert.Equal(t, publicModel, first.Model)

	info.BeginDynamicRoutingAttempt(2, info.GetChannelType(), publicModel, true)
	info.MarkAttemptUpstreamStarted()
	assert.Equal(t, publicModel, info.DynamicRoutingAttemptModel())
	info.StreamStatus = NewStreamStatus()
	info.StreamStatus.SetEndReason(StreamEndReasonDone, nil)
	second, ok := info.FinishDynamicRoutingAttempt(nil)

	require.True(t, ok)
	assert.Equal(t, publicModel, second.Model)
}

func TestDynamicRoutingAttemptOmitsZeroDurationSamplesWhenClockResolutionCoalescesEvents(t *testing.T) {
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	info := &RelayInfo{IsStream: true}
	info.setAttemptNowForTest(func() time.Time { return base })
	info.BeginDynamicRoutingAttempt(7, info.GetChannelType(), "model", true)
	info.MarkAttemptUpstreamStarted()
	info.RecordAttemptVisibleText("x")
	info.RecordAttemptVisibleText("x")
	info.SetAttemptCompletionTokens(3)
	info.StreamStatus = NewStreamStatus()
	info.StreamStatus.SetEndReason(StreamEndReasonDone, nil)

	sample, ok := info.FinishDynamicRoutingAttempt(nil)

	require.True(t, ok)
	assert.False(t, sample.HasTTFT)
	assert.Zero(t, sample.TTFT)
	assert.False(t, sample.HasTPOT)
	assert.Zero(t, sample.TPOT)
}

func TestDynamicRoutingAttemptStreamClassification(t *testing.T) {
	channelErr := types.NewError(errors.New("invalid key"), types.ErrorCodeChannelInvalidKey)
	tests := []struct {
		name       string
		stream     bool
		reason     StreamEndReason
		handlerErr *types.NewAPIError
		httpStatus int
		softError  bool
		streamErr  error
		observed   bool
		success    bool
		hard       bool
	}{
		{name: "nonstream success", observed: true, success: true},
		{name: "done", stream: true, reason: StreamEndReasonDone, observed: true, success: true},
		{name: "eof", stream: true, reason: StreamEndReasonEOF, observed: true, success: true},
		{name: "handler stop without error", stream: true, reason: StreamEndReasonHandlerStop, observed: true, success: true},
		{name: "done with soft stream error", stream: true, reason: StreamEndReasonDone, softError: true, observed: true, success: true},
		{name: "timeout", stream: true, reason: StreamEndReasonTimeout, observed: true, hard: true},
		{name: "scanner error", stream: true, reason: StreamEndReasonScannerErr, observed: true, hard: true},
		{name: "client gone", stream: true, reason: StreamEndReasonClientGone},
		{name: "ping failure", stream: true, reason: StreamEndReasonPingFail},
		{name: "panic", stream: true, reason: StreamEndReasonPanic},
		{name: "missing stream status", stream: true},
		{
			name: "immediate upstream 503 precedes stream status", stream: true,
			handlerErr: types.NewOpenAIError(errors.New("unavailable"), types.ErrorCodeBadResponse, http.StatusServiceUnavailable),
			observed:   true, hard: true,
		},
		{
			name: "immediate channel error precedes stream status", stream: true,
			handlerErr: channelErr, observed: true, hard: true,
		},
		{
			name: "handler stop follows soft handler error", stream: true, reason: StreamEndReasonHandlerStop,
			handlerErr: types.NewOpenAIError(errors.New("bad request"), types.ErrorCodeBadResponse, http.StatusBadRequest),
			observed:   true,
		},
		{
			name: "ordinary upstream resource not found remains soft", stream: true, reason: StreamEndReasonHandlerStop,
			handlerErr: types.WithOpenAIError(types.OpenAIError{Message: "resource missing", Code: "resource_not_found"}, http.StatusNotFound),
			observed:   true,
		},
		{
			name: "OpenAI model not found is channel model hard", stream: true, reason: StreamEndReasonHandlerStop,
			handlerErr: types.WithOpenAIError(types.OpenAIError{Message: "model missing", Code: "model_not_found"}, http.StatusNotFound),
			observed:   true, hard: true,
		},
		{
			name: "Claude not found is channel model hard", stream: true, reason: StreamEndReasonHandlerStop,
			handlerErr: types.WithClaudeError(types.ClaudeError{Type: "not_found_error", Message: "model missing"}, http.StatusNotFound),
			observed:   true, hard: true,
		},
		{
			name: "handler stop follows channel error", stream: true, reason: StreamEndReasonHandlerStop,
			handlerErr: channelErr, observed: true, hard: true,
		},
		{
			name: "handler stop uses recorded channel error", stream: true, reason: StreamEndReasonHandlerStop,
			streamErr: channelErr, observed: true, hard: true,
		},
		{
			name: "handler stop uses recorded local error", stream: true, reason: StreamEndReasonHandlerStop,
			streamErr: errors.New("local stream conversion failed"), observed: true,
		},
		{
			name: "429 is hard", stream: true, reason: StreamEndReasonDone,
			handlerErr: types.NewOpenAIError(errors.New("rate limited"), types.ErrorCodeBadResponse, http.StatusTooManyRequests),
			observed:   true, hard: true,
		},
		{
			name: "embedded 408 is hard", stream: true, reason: StreamEndReasonDone,
			handlerErr: types.NewOpenAIError(errors.New("upstream timed out"), types.ErrorCodeBadResponse, http.StatusRequestTimeout),
			observed:   true, hard: true,
		},
		{
			name: "embedded upstream payment required is hard", stream: true, reason: StreamEndReasonDone,
			handlerErr: types.NewOpenAIError(errors.New("upstream credits exhausted"), types.ErrorCodeBadResponse, http.StatusPaymentRequired),
			observed:   true, hard: true,
		},
		{
			name: "embedded upstream permanent redirect is hard", stream: true, reason: StreamEndReasonDone,
			handlerErr: types.NewOpenAIError(errors.New("upstream redirected"), types.ErrorCodeBadResponse, http.StatusMovedPermanently),
			observed:   true, hard: true,
		},
		{
			name: "raw upstream permanent redirect is hard", stream: true, reason: StreamEndReasonDone,
			httpStatus: http.StatusMovedPermanently, observed: true, hard: true,
		},
		{
			name: "embedded upstream temporary redirect is hard", stream: true, reason: StreamEndReasonDone,
			handlerErr: types.NewOpenAIError(errors.New("upstream redirected"), types.ErrorCodeBadResponse, http.StatusTemporaryRedirect),
			observed:   true, hard: true,
		},
		{
			name: "raw upstream temporary redirect is hard", stream: true, reason: StreamEndReasonDone,
			httpStatus: http.StatusTemporaryRedirect, observed: true, hard: true,
		},
		{
			name: "raw upstream no content is protocol hard", stream: true, reason: StreamEndReasonDone,
			httpStatus: http.StatusNoContent, observed: true, hard: true,
		},
		{
			name: "raw unexpected protocol switch is hard", stream: true, reason: StreamEndReasonDone,
			httpStatus: http.StatusSwitchingProtocols, observed: true, hard: true,
		},
		{
			name: "raw upstream OK remains success", stream: true, reason: StreamEndReasonDone,
			httpStatus: http.StatusOK, observed: true, success: true,
		},
		{
			name: "raw upstream payment required is hard", stream: true, reason: StreamEndReasonDone,
			httpStatus: http.StatusPaymentRequired, observed: true, hard: true,
		},
		{
			name: "raw upstream 408 is hard", stream: true, reason: StreamEndReasonDone,
			httpStatus: http.StatusRequestTimeout, observed: true, hard: true,
		},
		{
			name: "embedded proxy authentication is hard", stream: true, reason: StreamEndReasonDone,
			handlerErr: types.NewOpenAIError(errors.New("proxy authentication required"), types.ErrorCodeBadResponse, http.StatusProxyAuthRequired),
			observed:   true, hard: true,
		},
		{
			name: "raw proxy authentication is hard", stream: true, reason: StreamEndReasonDone,
			httpStatus: http.StatusProxyAuthRequired, observed: true, hard: true,
		},
		{
			name: "embedded Cohere invalid token is hard", stream: true, reason: StreamEndReasonDone,
			handlerErr: types.NewOpenAIError(errors.New("invalid token"), types.ErrorCodeBadResponse, 498),
			observed:   true, hard: true,
		},
		{
			name: "raw Cohere invalid token is hard", stream: true, reason: StreamEndReasonDone,
			httpStatus: 498, observed: true, hard: true,
		},
		{
			name: "embedded upstream client closed status is hard", stream: true, reason: StreamEndReasonDone,
			handlerErr: types.NewOpenAIError(errors.New("upstream canceled"), types.ErrorCodeBadResponse, 499),
			observed:   true, hard: true,
		},
		{
			name: "raw upstream client closed status is hard", stream: true, reason: StreamEndReasonDone,
			httpStatus: 499, observed: true, hard: true,
		},
		{
			name: "raw upstream 503 remains hard after status mapping", stream: true, reason: StreamEndReasonDone,
			handlerErr: types.NewOpenAIError(errors.New("mapped"), types.ErrorCodeBadResponse, http.StatusBadRequest),
			httpStatus: http.StatusServiceUnavailable, observed: true, hard: true,
		},
		{
			name: "raw upstream 400 remains soft after status mapping to 500", stream: true, reason: StreamEndReasonHandlerStop,
			handlerErr: types.NewOpenAIError(errors.New("mapped"), types.ErrorCodeBadResponse, http.StatusInternalServerError),
			httpStatus: http.StatusBadRequest, observed: true,
		},
		{
			name:       "nonstream raw upstream 400 remains soft after status mapping to 500",
			handlerErr: types.NewOpenAIError(errors.New("mapped"), types.ErrorCodeBadResponse, http.StatusInternalServerError),
			httpStatus: http.StatusBadRequest, observed: true,
		},
		{
			name: "HTTP 200 embedded upstream 500 remains hard", stream: true, reason: StreamEndReasonHandlerStop,
			handlerErr: types.NewOpenAIError(errors.New("embedded upstream failure"), types.ErrorCodeBadResponse, http.StatusInternalServerError),
			httpStatus: http.StatusOK, observed: true, hard: true,
		},
		{
			name: "exact channel error remains hard even with raw upstream 400", stream: true, reason: StreamEndReasonHandlerStop,
			handlerErr: channelErr, httpStatus: http.StatusBadRequest, observed: true, hard: true,
		},
		{
			name: "stream raw upstream 503 is hard even without handler error", stream: true, reason: StreamEndReasonDone,
			httpStatus: http.StatusServiceUnavailable, observed: true, hard: true,
		},
		{
			name:       "nonstream raw upstream 503 is hard even without handler error",
			httpStatus: http.StatusServiceUnavailable, observed: true, hard: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
			info := &RelayInfo{IsStream: tt.stream}
			info.setAttemptNowForTest(func() time.Time { return base })
			info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), "model", true)
			info.MarkAttemptUpstreamStarted()
			if tt.stream && tt.success {
				info.SetFirstResponseTime()
			}
			if tt.httpStatus != 0 {
				info.SetAttemptHTTPStatus(tt.httpStatus)
			}
			if tt.reason != StreamEndReasonNone {
				info.StreamStatus = NewStreamStatus()
				if tt.softError {
					info.StreamStatus.RecordError("recoverable event")
				}
				info.StreamStatus.SetEndReason(tt.reason, tt.streamErr)
			}

			sample, ok := info.FinishDynamicRoutingAttempt(tt.handlerErr)
			assert.Equal(t, tt.observed, ok)
			if ok {
				assert.Equal(t, tt.success, sample.Success)
				assert.Equal(t, tt.hard, sample.HardFailure)
			}
		})
	}
}

func TestDynamicRoutingAttemptDiscardsBeforeUpstreamAndWhenDisabled(t *testing.T) {
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	err := types.NewOpenAIError(errors.New("local conversion failed"), types.ErrorCodeConvertRequestFailed, http.StatusInternalServerError)

	info := &RelayInfo{}
	info.setAttemptNowForTest(func() time.Time { return base })
	info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), "model", true)
	_, ok := info.FinishDynamicRoutingAttempt(err)
	assert.False(t, ok)

	info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), "model", false)
	info.MarkAttemptUpstreamStarted()
	_, ok = info.FinishDynamicRoutingAttempt(nil)
	assert.False(t, ok)
}

func TestDynamicRoutingAttemptPublishesOnlyExplicitPreUpstreamChannelFailures(t *testing.T) {
	tests := []struct {
		name       string
		handlerErr *types.NewAPIError
		markHard   bool
		observed   bool
		hard       bool
	}{
		{
			name:       "invalid channel key",
			handlerErr: types.NewError(errors.New("invalid key"), types.ErrorCodeChannelInvalidKey),
			observed:   true,
			hard:       true,
		},
		{
			name:       "AWS client construction channel error",
			handlerErr: types.NewError(errors.New("AWS client failed"), types.ErrorCodeChannelAwsClientError),
			observed:   true,
			hard:       true,
		},
		{
			name:       "ordinary local construction error remains discarded",
			handlerErr: types.NewOpenAIError(errors.New("local marshal failed"), types.ErrorCodeConvertRequestFailed, http.StatusInternalServerError),
		},
		{
			name:       "marked transient channel preflight is hard without channel error",
			handlerErr: types.NewOpenAIError(errors.New("token endpoint temporarily unavailable"), types.ErrorCodeDoRequestFailed, http.StatusBadGateway),
			markHard:   true,
			observed:   true,
			hard:       true,
		},
		{
			name:       "client request error remains discarded",
			handlerErr: types.NewOpenAIError(errors.New("invalid request"), types.ErrorCodeInvalidRequest, http.StatusBadRequest),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &RelayInfo{}
			info.BeginDynamicRoutingAttempt(1, constant.ChannelTypeOpenAI, "model", true)
			if tt.markHard {
				info.MarkDynamicRoutingAttemptPreUpstreamHard()
			}

			sample, observed := info.FinishDynamicRoutingAttempt(tt.handlerErr)

			assert.Equal(t, tt.observed, observed)
			assert.Equal(t, tt.hard, sample.HardFailure)
			assert.False(t, sample.Success)
			assert.True(t, sample.UpstreamStartedAt.IsZero())
			assert.False(t, sample.HasTTFT)
			assert.False(t, sample.HasTPOT)
		})
	}
}

func TestDynamicRoutingAttemptRejectsEmptyStreamSuccessButKeepsPayloadHealth(t *testing.T) {
	tests := []struct {
		name       string
		reason     StreamEndReason
		hasPayload bool
		success    bool
		hard       bool
	}{
		{name: "empty EOF is protocol hard", reason: StreamEndReasonEOF, hard: true},
		{name: "DONE-only is protocol hard", reason: StreamEndReasonDone, hard: true},
		{name: "empty handler stop is protocol hard", reason: StreamEndReasonHandlerStop, hard: true},
		{name: "metadata payload is health success", reason: StreamEndReasonDone, hasPayload: true, success: true},
		{name: "tool payload is health success", reason: StreamEndReasonEOF, hasPayload: true, success: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &RelayInfo{IsStream: true}
			info.BeginDynamicRoutingAttempt(1, constant.ChannelTypeOpenAI, "model", true)
			info.MarkAttemptUpstreamStarted()
			info.StreamStatus = NewStreamStatus()
			if tt.hasPayload {
				info.SetFirstResponseTime()
			}
			info.StreamStatus.SetEndReason(tt.reason, nil)

			sample, observed := info.FinishDynamicRoutingAttempt(nil)

			require.True(t, observed)
			assert.Equal(t, tt.success, sample.Success)
			assert.Equal(t, tt.hard, sample.HardFailure)
			assert.False(t, sample.HasTTFT)
			assert.False(t, sample.HasTPOT)
		})
	}
}

func TestDynamicRoutingAttemptPhysicalDispatchExcludesLocalConstruction(t *testing.T) {
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	now := base
	info := &RelayInfo{}
	info.setAttemptNowForTest(func() time.Time { return now })

	info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), "model", true)
	now = base.Add(500 * time.Millisecond) // local AWS client/request construction
	info.MarkAttemptUpstreamStarted()      // physical InvokeModel boundary
	now = base.Add(time.Second)

	sample, observed := info.FinishDynamicRoutingAttempt(nil)

	require.True(t, observed)
	assert.Equal(t, base.Add(500*time.Millisecond), sample.UpstreamStartedAt)
}

func TestDynamicRoutingAttemptMissingOrSingleCompletionTokenOmitsTPOT(t *testing.T) {
	for _, tokens := range []int{0, 1} {
		t.Run(string(rune('0'+tokens)), func(t *testing.T) {
			base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
			times := []time.Time{
				base,
				base.Add(10 * time.Millisecond),
				base.Add(20 * time.Millisecond),
				base.Add(100 * time.Millisecond),
				base.Add(200 * time.Millisecond),
			}
			next := 0
			info := &RelayInfo{IsStream: true}
			info.setAttemptNowForTest(func() time.Time {
				at := times[next]
				next++
				return at
			})
			info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), "model", true)
			info.MarkAttemptUpstreamStarted()
			info.SetFirstResponseTime()
			info.RecordAttemptVisibleText("x")
			info.SetAttemptCompletionTokens(tokens)
			info.StreamStatus = NewStreamStatus()
			info.StreamStatus.SetEndReason(StreamEndReasonDone, nil)

			sample, ok := info.FinishDynamicRoutingAttempt(nil)
			require.True(t, ok)
			assert.True(t, sample.HasTTFT)
			assert.False(t, sample.HasTPOT)
			assert.Zero(t, sample.TPOT)
		})
	}
}

func TestDynamicRoutingAttemptStateIsConcurrentSafe(t *testing.T) {
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	var clockNanos atomic.Int64
	clockNanos.Store(base.UnixNano())
	info := &RelayInfo{StartTime: base.Add(-time.Second), isFirstResponse: true, IsStream: true}
	info.setAttemptNowForTest(func() time.Time { return time.Unix(0, clockNanos.Load()).UTC() })
	info.BeginDynamicRoutingAttempt(1, info.GetChannelType(), "model", true)
	info.MarkAttemptUpstreamStarted()
	clockNanos.Store(base.Add(100 * time.Millisecond).UnixNano())
	info.SetFirstResponseTime()
	info.RecordAttemptVisibleText("x")
	clockNanos.Store(base.Add(200 * time.Millisecond).UnixNano())

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(tokens int) {
			defer wg.Done()
			info.SetFirstResponseTime()
			info.RecordAttemptVisibleText("x")
			info.SetAttemptCompletionTokens(tokens)
			info.SetAttemptHTTPStatus(http.StatusOK)
			_ = info.HasSendResponse()
		}(i + 2)
	}
	wg.Wait()
	clockNanos.Store(base.Add(300 * time.Millisecond).UnixNano())
	info.StreamStatus = NewStreamStatus()
	info.StreamStatus.SetEndReason(StreamEndReasonDone, nil)

	sample, ok := info.FinishDynamicRoutingAttempt(nil)

	require.True(t, ok)
	assert.True(t, sample.Success)
	assert.Equal(t, 100*time.Millisecond, sample.TTFT)
	assert.True(t, sample.HasTPOT)
	assert.Equal(t, base.Add(100*time.Millisecond), info.FirstResponseTime)
}
