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
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

type channelMonitoringRequestKey struct{}

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

func channelMonitoringEndpoint(channel *model.Channel, target model.ChannelMonitorTarget) string {
	endpoint := normalizeChannelTestEndpoint(channel, target.Model, target.EndpointType)
	if endpoint != "" {
		return endpoint
	}
	name := strings.ToLower(target.Model)
	switch {
	case strings.Contains(name, "codex"):
		return string(constant.EndpointTypeOpenAIResponse)
	case strings.Contains(name, "rerank"):
		return string(constant.EndpointTypeJinaRerank)
	case channel.Type == constant.ChannelTypeMokaAI || strings.Contains(name, "embed") || strings.HasPrefix(name, "bge-") || strings.HasPrefix(name, "m3e"):
		return string(constant.EndpointTypeEmbeddings)
	default:
		return string(constant.EndpointTypeOpenAI)
	}
}

func channelMonitoringObservation(result testResult, started, completed time.Time, slowMs int) model.ChannelMonitorRecord {
	record := model.ChannelMonitorRecord{CreatedAt: completed.Unix(), LatencyMs: completed.Sub(started).Milliseconds(), State: "healthy"}
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
	if latency > int64(slowMs) {
		record.State = "slow"
	}
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
			endpoint := channelMonitoringEndpoint(channel, target)
			stream := settings.ChannelMonitoringStream && endpoint != string(constant.EndpointTypeEmbeddings) && endpoint != string(constant.EndpointTypeJinaRerank)
			probeContext, cancel := context.WithTimeout(ctx, time.Duration(max(1, settings.ChannelMonitoringTimeoutSeconds))*time.Second)
			probeContext = context.WithValue(probeContext, channelMonitoringRequestKey{}, settings)
			// Auto-disabled channels can be observed without changing their stored
			// status. The executor still enforces per-key availability and policies.
			probeChannel := *channel
			probeChannel.Status = common.ChannelStatusEnabled
			started := time.Now()
			result := testChannel(probeContext, &probeChannel, testUserID, target.Model, endpoint, stream)
			record := channelMonitoringObservation(result, started, time.Now(), settings.ChannelMonitoringSlowMs)
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
			record.EndpointType = target.EndpointType
			if err := model.SaveChannelMonitorRecord(ctx, &record); err != nil {
				common.SysError(fmt.Sprintf("failed to save channel monitoring observation: channel=%d error=%v", channel.Id, err))
				summary.Failed++
				continue
			}
			summary.Tested++
			if record.State == "healthy" || record.State == "slow" {
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
	if err := model.DB.WithContext(ctx).Select("id, models, test_model, status, monitor_config").Where("id IN ?", ids).Find(&channels).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	settings := *operation_setting.GetMonitorSetting()
	data, err := model.ChannelMonitorSummaries(ctx, channels, modelName, settings.ChannelMonitoringEnabled, time.Duration(max(1, settings.ChannelMonitoringMinutes))*time.Minute, time.Now())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}
