package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/dynamicrouting"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

type channelMonitoringRequestKey struct{}

// Recovery checks carry synthetic input, use the administrator test executor,
// and publish health only; their tiny payload must not calibrate customer latency.
func probeDynamicRoutingRecovery(ctx context.Context, key dynamicrouting.ObservationKey) (dynamicrouting.Sample, bool) {
	channel, err := model.CacheGetChannel(key.ChannelID)
	if err != nil || channel.Status != common.ChannelStatusEnabled {
		return dynamicrouting.Sample{}, false
	}
	endpoint := channel.MonitoringEndpoint(model.ChannelMonitorTarget{Model: key.Model})
	switch constant.EndpointType(endpoint) {
	case constant.EndpointTypeOpenAI, constant.EndpointTypeOpenAIResponse, constant.EndpointTypeAnthropic, constant.EndpointTypeGemini:
	default:
		return dynamicrouting.Sample{}, false
	}
	testUserID, err := resolveChannelTestUserID(nil)
	if err != nil {
		return dynamicrouting.Sample{}, false
	}
	settings := *operation_setting.GetMonitorSetting()
	settings.ChannelMonitoringPrompt = "Reply with OK."
	settings.ChannelMonitoringMaxTokens = 32
	ctx = context.WithValue(ctx, channelMonitoringRequestKey{}, settings)
	started := time.Now()
	result := testChannel(ctx, channel, testUserID, key.Model, endpoint, true)
	success := ctx.Err() == nil && result.localErr == nil && result.newAPIError == nil
	return dynamicrouting.Sample{RecoveryProbe: true, UpstreamStartedAt: started, ObservedAt: time.Now(), Success: success, HardFailure: !success}, true
}

// The existing test executor owns conversion, channel policies, and accounting;
// monitoring only supplies bounded request settings and observes its outcome.
func configureChannelMonitoringRequest(request dto.Request, settings operation_setting.MonitorSetting) error {
	if settings.ChannelMonitoringMaxTokens < 1 || settings.ChannelMonitoringMaxTokens > 512 || len(strings.TrimSpace(settings.ChannelMonitoringPrompt)) == 0 || len(settings.ChannelMonitoringPrompt) > 4096 {
		return fmt.Errorf("invalid monitoring request settings")
	}
	limit := uint(settings.ChannelMonitoringMaxTokens)
	prompt := settings.ChannelMonitoringPrompt
	switch req := request.(type) {
	case *dto.GeneralOpenAIRequest:
		req.Messages = []dto.Message{{Role: "user", Content: prompt}}
		if req.MaxCompletionTokens != nil {
			req.MaxCompletionTokens = &limit
		} else {
			req.MaxTokens = &limit
		}
	case *dto.OpenAIResponsesRequest:
		input, err := common.Marshal([]dto.Message{{Role: "user", Content: prompt}})
		if err != nil {
			return err
		}
		req.Input = input
		req.MaxOutputTokens = &limit
	case *dto.ClaudeRequest:
		req.Messages = []dto.ClaudeMessage{{Role: "user", Content: prompt}}
		req.MaxTokens = &limit
	case *dto.GeminiChatRequest:
		req.Contents = []dto.GeminiChatContent{{Role: "user", Parts: []dto.GeminiPart{{Text: prompt}}}}
		req.GenerationConfig.MaxOutputTokens = &limit
	case *dto.EmbeddingRequest:
		req.Input = []any{prompt}
	case *dto.RerankRequest:
		req.Query = prompt
	default:
		return fmt.Errorf("monitoring protocol is not supported")
	}
	return nil
}

func channelMonitoringObservation(result testResult, started, completed time.Time) model.ChannelMonitorRecord {
	record := model.ChannelMonitorRecord{CreatedAt: completed.Unix(), ObservedAtMs: completed.UnixMilli(), LatencyMs: completed.Sub(started).Milliseconds(), State: "healthy"}
	if result.context != nil {
		record.StatusCode = result.context.GetInt("channel_test_upstream_status")
	}
	if result.localErr != nil || result.newAPIError != nil {
		record.State = "configuration"
		record.ErrorCode = "local_configuration"
		if result.context != nil && result.context.GetBool("channel_test_upstream_started") {
			record.State = "unavailable"
			record.ErrorCode = "upstream_request_failed"
		}
		if result.newAPIError == nil && result.localErr != nil && strings.Contains(result.localErr.Error(), "not supported") {
			record.State = "unsupported"
			record.ErrorCode = "unsupported_protocol"
		}
		return record
	}
	if result.context != nil {
		if value, exists := result.context.Get("channel_test_relay_info"); exists {
			if info, ok := value.(*relaycommon.RelayInfo); ok && info.IsStream && !info.FirstResponseTime.Before(info.StartTime) {
				ttft := info.FirstResponseTime.Sub(info.StartTime).Milliseconds()
				record.TTFTMs = &ttft
				if value, exists := result.context.Get("channel_test_usage"); exists {
					if usage, ok := value.(*dto.Usage); ok && usage.CompletionTokens > 1 {
						generationMs := completed.Sub(info.FirstResponseTime).Milliseconds()
						if generationMs > 0 {
							speed := float64(usage.CompletionTokens-1) * 1000 / float64(generationMs)
							record.TokensPerSecond = &speed
						}
					}
				}
			}
		}
	}
	latency := record.LatencyMs
	if record.TTFTMs != nil {
		latency = *record.TTFTMs
	}
	record.State = model.ChannelMonitorLatencyState(latency)
	return record
}

type channelMonitoringHandler struct{}

func (channelMonitoringHandler) Type() string { return model.SystemTaskTypeChannelMonitor }
func (channelMonitoringHandler) Enabled() bool {
	return operation_setting.GetMonitorSetting().ChannelMonitoringEnabled
}
func (channelMonitoringHandler) Interval() time.Duration {
	return time.Duration(max(1, operation_setting.GetMonitorSetting().ChannelMonitoringMinutes)) * time.Minute
}
func (channelMonitoringHandler) NewPayload() any { return nil }

func (channelMonitoringHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	settings := *operation_setting.GetMonitorSetting()
	testUserID, err := resolveChannelTestUserID(nil)
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	var channels []*model.Channel
	if err := model.DB.WithContext(ctx).Where("status <> ?", common.ChannelStatusManuallyDisabled).Find(&channels).Error; err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	progress := service.NewSystemTaskProgressReporter(task, runnerID)
	results := runChannelTestWorkers(ctx, channels, settings.ChannelTestConcurrency, func(ctx context.Context, channel *model.Channel) channelTestSummary {
		summary := channelTestSummary{}
		enabled, targets, configErr := channel.MonitorTargets()
		if configErr != nil {
			summary.Failed++
			return summary
		}
		if !enabled {
			return summary
		}
		for _, target := range targets {
			if ctx.Err() != nil {
				return summary
			}
			endpoint := channel.MonitoringEndpoint(target)
			stream := settings.ChannelMonitoringStream && endpoint != string(constant.EndpointTypeEmbeddings) && endpoint != string(constant.EndpointTypeJinaRerank)
			probeContext, cancel := context.WithTimeout(ctx, time.Duration(max(1, settings.ChannelMonitoringTimeoutSeconds))*time.Second)
			probeContext = context.WithValue(probeContext, channelMonitoringRequestKey{}, settings)
			// Auto-disabled channels can be observed without changing their stored
			// status. The executor still enforces per-key availability and policies.
			probeChannel := *channel
			probeChannel.Status = common.ChannelStatusEnabled
			started := time.Now()
			result := testChannel(probeContext, &probeChannel, testUserID, target.Model, endpoint, stream)
			record := channelMonitoringObservation(result, started, time.Now())
			record.Source, record.Count = "probe", 1
			if probeContext.Err() != nil {
				record.State = "unavailable"
				record.ErrorCode = "timeout"
			}
			cancel()
			if ctx.Err() != nil {
				return summary
			}
			key := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s:%s", task.TaskID, channel.Id, target.Model, target.EndpointType)))
			record.SampleKey = fmt.Sprintf("%x", key)
			record.ChannelID = channel.Id
			record.Model = target.Model
			record.EndpointType = endpoint
			if err := model.SaveChannelMonitorRecord(ctx, &record); err != nil {
				common.SysError(fmt.Sprintf("failed to save channel monitoring observation: channel=%d error=%v", channel.Id, err))
				summary.Failed++
				continue
			}
			summary.Tested++
			if record.State == "healthy" || record.State == "slow" || record.State == "very_slow" {
				summary.Succeeded++
			} else {
				summary.Failed++
			}
		}
		return summary
	}, progress)
	if ctx.Err() != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, results, ctx.Err())
		return
	}
	if err := model.PruneChannelMonitorRecords(ctx, time.Now()); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, results, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, results, nil)
}

func GetChannelMonitoring(c *gin.Context) {
	modelName := strings.TrimSpace(c.Query("model"))
	if modelName == "" || len(modelName) > 128 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "model is required and must not exceed 128 bytes"})
		return
	}
	parts := strings.Split(c.Query("channel_ids"), ",")
	if len(parts) > 50 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "at most 50 channels can be monitored per query"})
		return
	}
	ids := make([]int, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.Atoi(part)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid channel IDs"})
			return
		}
		ids = append(ids, id)
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	var channels []*model.Channel
	if err := model.DB.WithContext(ctx).Select("id, type, models, test_model, status, monitor_config").Where("id IN ?", ids).Find(&channels).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	settings := *operation_setting.GetMonitorSetting()
	excluded, err := operation_setting.ParseHTTPStatusCodeRanges(settings.ChannelMonitoringExcludedStatusCodes)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	now := time.Now()
	data, err := model.ChannelMonitorSummaries(ctx, channels, modelName, settings.ChannelMonitoringEnabled, time.Duration(max(1, settings.ChannelMonitoringMinutes))*time.Minute, excluded, now)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// Cache statistics may be unavailable independently of probe monitoring.
	cacheCtx, cacheCancel := context.WithTimeout(ctx, 2*time.Second)
	defer cacheCancel()
	cache, cacheErr := model.QueryChannelCacheStats(cacheCtx, ids, modelName, now.Add(-24*time.Hour).Unix(), now.Unix())
	if cacheErr == nil {
		for i := range data {
			data[i].Cache = cache[data[i].ChannelID]
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}
