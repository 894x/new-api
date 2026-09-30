package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

// DecideRelayRetry is the single retry decision for relay attempts. The reason
// is recorded in the request policy decision events of the log details.
func DecideRelayRetry(c *gin.Context, err *types.NewAPIError, retryTimes int) PolicyDecision {
	if err == nil {
		return PolicyDecision{Action: "stop", Reason: "request_completed", Source: "system"}
	}
	if err.GetErrorCode() == types.ErrorCodeClientGone {
		return PolicyDecision{Action: "stop", Reason: "client_gone", Source: "client"}
	}
	if c.Writer.Written() || types.IsResponseCommittedError(err) {
		return PolicyDecision{Action: "stop", Reason: "response_committed", Source: "system"}
	}
	if _, forced := c.Get("specific_channel_id"); forced {
		return PolicyDecision{Action: "stop", Reason: "pinned_channel", Source: "channel_constraint"}
	}
	if ShouldSkipRetryAfterChannelAffinityFailure(c) {
		source := RequestPolicy(c).SessionModeSource
		if source == "" {
			source = "session_rule"
		}
		return PolicyDecision{Action: "stop", Reason: "strict_session", Source: source}
	}
	if GetChannelConstraints(c).SuppressesRetry() {
		return PolicyDecision{Action: "stop", Reason: "pinned_channel", Source: "channel_constraint"}
	}
	if types.IsChannelError(err) {
		return PolicyDecision{Action: "retry", Reason: "channel_error", Source: "system"}
	}
	if types.IsSkipRetryError(err) {
		return PolicyDecision{Action: "stop", Reason: "non_retryable_error", Source: "system"}
	}
	if retryTimes <= 0 {
		return PolicyDecision{Action: "stop", Reason: "attempt_budget_exhausted", Source: "global"}
	}
	code := err.StatusCode
	if code >= 200 && code < 300 {
		return PolicyDecision{Action: "stop", Reason: "system_retry_exclusion", Source: "system"}
	}
	if code < 100 || code > 599 {
		return PolicyDecision{Action: "retry", Reason: "unrecognized_status", Source: "system"}
	}
	if operation_setting.IsAlwaysSkipRetryCode(err.GetErrorCode()) || operation_setting.IsAlwaysSkipRetryStatusCode(code) {
		return PolicyDecision{Action: "stop", Reason: "system_retry_exclusion", Source: "system"}
	}
	if operation_setting.ShouldRetryByStatusCode(code) {
		return PolicyDecision{Action: "retry", Reason: "retry_status_matched", Source: "global"}
	}
	return PolicyDecision{Action: "stop", Reason: "status_not_retryable", Source: "global"}
}

func ShouldRetryRelayError(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
	return DecideRelayRetry(c, openaiErr, retryTimes).Action == "retry"
}

func ProcessChannelError(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError, relayInfo *relaycommon.RelayInfo) {
	if err == nil {
		return
	}
	clientGone := err.GetErrorCode() == types.ErrorCodeClientGone
	if clientGone {
		logger.LogInfo(c, fmt.Sprintf("client_gone (channel #%d): %s", channelError.ChannelId, common.LocalLogPreview(err.Error())))
	} else {
		logger.LogError(c, fmt.Sprintf("channel error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, common.LocalLogPreview(err.MaskSensitiveErrorWithStatusCode())))
	}
	if !clientGone && ShouldDisableChannel(err) && channelError.AutoBan {
		reason := err.MaskSensitiveErrorWithStatusCode()
		gopool.Go(func() {
			DisableChannel(channelError, reason)
		})
	}

	RecordRelayErrorLog(c, channelError.ChannelId, err, relayInfo, "relay_attempt")
}

type relayErrorLogEvent struct {
	ChannelID  int
	Attempt    int
	Code       types.ErrorCode
	StatusCode int
	Message    string
}

// RecordRelayErrorLog also covers failures before channel dispatch, without
// applying channel-health side effects. Deduplication is request-local and
// attempt-specific; a failed insert never marks the event as persisted.
func RecordRelayErrorLog(c *gin.Context, channelID int, err *types.NewAPIError, relayInfo *relaycommon.RelayInfo, stage string) {
	if c == nil || err == nil || !constant.ErrorLogEnabled || !types.IsRecordErrorLog(err) {
		return
	}
	const recordedErrorsKey = "relay_recorded_error_logs"
	event := relayErrorLogEvent{ChannelID: channelID, Code: err.GetErrorCode(), StatusCode: err.StatusCode, Message: err.Error()}
	if relayInfo != nil {
		event.Attempt = relayInfo.RetryIndex
	}
	recorded, _ := common.GetContextKeyType[map[relayErrorLogEvent]struct{}](c, recordedErrorsKey)
	if _, exists := recorded[event]; exists {
		return
	}
	userId := c.GetInt("id")
	tokenName := c.GetString("token_name")
	modelName := c.GetString("original_model")
	tokenId := c.GetInt("token_id")
	userGroup := c.GetString("group")
	other := model.NewLogOther()
	if c.Request != nil && c.Request.URL != nil {
		other.SetPublic("request_path", c.Request.URL.Path)
	}
	other.SetPublic("error_type", err.GetErrorType())
	other.SetPublic("error_code", err.GetErrorCode())
	other.SetPublic("status_code", err.StatusCode)
	var downloadErr *FileDownloadError
	if errors.As(err, &downloadErr) {
		if stage == "billing_preparation" || err.GetErrorCode() == types.ErrorCodeCountTokenFailed {
			stage = "token_count"
		}
		if downloadErr.StatusCode != 0 {
			other.SetAdmin("media_download_status_code", downloadErr.StatusCode)
		}
	}
	other.SetAdmin("failure_stage", stage)
	appendStreamStatus(relayInfo, other)
	if c.Writer != nil && c.Writer.Written() {
		other.SetPublic("client_status_code", c.Writer.Status())
	}
	AppendRelayLogAdminInfo(c, relayInfo, other)
	AppendUpstreamResponseAdminInfo(c, other)
	if relayInfo != nil && relayInfo.ChannelMeta != nil && relayInfo.UpstreamModelName != "" {
		other.SetPublic("upstream_model_name", relayInfo.UpstreamModelName)
	}
	if timing := common.GetRequestTiming(c); timing != nil {
		snapshot := timing.Snapshot()
		snapshot.RequestCompletedAtMs = time.Now().UnixMilli()
		other.SetAdmin("request_timing", snapshot)
		if attempts := timing.UpstreamTransports(); len(attempts) > 0 {
			other.SetAdmin("upstream_transport", attempts)
		}
	}
	AppendResponseModelLogInfo(relayInfo, other)
	AppendTaskPluginContextAuditInfo(c, other)
	startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if startTime.IsZero() {
		startTime = time.Now()
	}
	useTimeSeconds := int(time.Since(startTime).Seconds())
	if writeErr := model.RecordErrorLog(c, userId, channelID, modelName, tokenName, err.MaskSensitiveErrorWithStatusCode(), tokenId, useTimeSeconds, common.GetContextKeyBool(c, constant.ContextKeyIsStream), userGroup, other); writeErr != nil {
		return
	}
	if recorded == nil {
		recorded = make(map[relayErrorLogEvent]struct{})
	}
	recorded[event] = struct{}{}
	c.Set(recordedErrorsKey, recorded)
}
