package model

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm/clause"
)

const ChannelMonitorRetention = 7 * 24 * time.Hour

type ChannelMonitorTarget struct {
	Model        string `json:"model"`
	EndpointType string `json:"endpoint_type"`
}

type ChannelMonitorConfig struct {
	Enabled *bool                  `json:"enabled,omitempty"`
	Targets []ChannelMonitorTarget `json:"targets,omitempty"`
}

func (channel *Channel) GetMonitorConfig() (ChannelMonitorConfig, error) {
	var config ChannelMonitorConfig
	if strings.TrimSpace(channel.MonitorConfig) != "" {
		if err := common.UnmarshalJsonStr(channel.MonitorConfig, &config); err != nil {
			return config, fmt.Errorf("invalid channel monitoring configuration")
		}
	}
	if len(config.Targets) > 16 {
		return config, fmt.Errorf("channel monitoring supports at most 16 targets")
	}
	seen := make(map[ChannelMonitorTarget]bool)
	models := channel.GetModels()
	for i := range config.Targets {
		target := &config.Targets[i]
		target.Model = strings.TrimSpace(target.Model)
		target.EndpointType = strings.TrimSpace(target.EndpointType)
		if target.Model == "" || len(target.Model) > 128 || !slices.Contains(models, target.Model) {
			return config, fmt.Errorf("monitoring model must be a configured channel model of at most 128 bytes")
		}
		switch constant.EndpointType(target.EndpointType) {
		case "", constant.EndpointTypeOpenAI, constant.EndpointTypeOpenAIResponse, constant.EndpointTypeAnthropic, constant.EndpointTypeGemini, constant.EndpointTypeEmbeddings, constant.EndpointTypeJinaRerank:
		default:
			return config, fmt.Errorf("unsupported channel monitoring protocol")
		}
		if seen[*target] {
			return config, fmt.Errorf("duplicate channel monitoring target")
		}
		seen[*target] = true
	}
	return config, nil
}

func (channel *Channel) MonitorTargets() (bool, []ChannelMonitorTarget, error) {
	config, err := channel.GetMonitorConfig()
	if err != nil {
		return false, nil, err
	}
	if config.Enabled != nil && !*config.Enabled {
		return false, nil, nil
	}
	if len(config.Targets) > 0 {
		return true, config.Targets, nil
	}
	modelName := ""
	if channel.TestModel != nil {
		modelName = strings.TrimSpace(*channel.TestModel)
	}
	if modelName == "" {
		models := channel.GetModels()
		if len(models) > 0 {
			modelName = strings.TrimSpace(models[0])
		}
	}
	if modelName == "" || len(modelName) > 128 {
		return true, nil, nil
	}
	return true, []ChannelMonitorTarget{{Model: modelName}}, nil
}

// ChannelMonitorRecord stores observations independently of consume-log settings.
// SampleKey makes a retried system-task write idempotent. Error summaries never
// contain upstream response bodies, credentials, or request headers.
type ChannelMonitorRecord struct {
	ID              int64    `json:"id" gorm:"primaryKey"`
	SampleKey       string   `json:"-" gorm:"size:64;uniqueIndex"`
	ChannelID       int      `json:"channel_id" gorm:"index:idx_channel_monitor_window,priority:1"`
	Model           string   `json:"model" gorm:"size:128;index:idx_channel_monitor_window,priority:2"`
	EndpointType    string   `json:"endpoint_type" gorm:"size:32"`
	CreatedAt       int64    `json:"created_at" gorm:"index:idx_channel_monitor_window,priority:3;index"`
	State           string   `json:"state" gorm:"size:24"`
	LatencyMs       int64    `json:"latency_ms"`
	TTFTMs          *int64   `json:"ttft_ms"`
	TokensPerSecond *float64 `json:"tokens_per_second"`
	StatusCode      int      `json:"status_code"`
	ErrorCode       string   `json:"error_code" gorm:"size:64"`
}

func SaveChannelMonitorRecord(ctx context.Context, record *ChannelMonitorRecord) error {
	return DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(record).Error
}

func PruneChannelMonitorRecords(ctx context.Context, now time.Time) error {
	var ids []int64
	if err := DB.WithContext(ctx).Model(&ChannelMonitorRecord{}).Where("created_at < ?", now.Add(-ChannelMonitorRetention).Unix()).Order("id").Limit(2000).Pluck("id", &ids).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	return DB.WithContext(ctx).Where("id IN ?", ids).Delete(&ChannelMonitorRecord{}).Error
}

type ChannelMonitorBucket struct {
	Timestamp    int64  `json:"timestamp"`
	State        string `json:"state"`
	Count        int    `json:"count"`
	SuccessCount int    `json:"success_count"`
	LatencyMs    *int64 `json:"latency_ms"`
	ErrorCode    string `json:"error_code,omitempty"`
}

type ChannelMonitorSummary struct {
	ChannelID   int                    `json:"channel_id"`
	Enabled     bool                   `json:"enabled"`
	Registered  bool                   `json:"registered"`
	State       string                 `json:"state"`
	Latest      *ChannelMonitorRecord  `json:"latest"`
	SuccessRate *float64               `json:"success_rate"`
	SampleCount int                    `json:"sample_count"`
	Buckets     []ChannelMonitorBucket `json:"buckets"`
	Truncated   bool                   `json:"truncated"`
}

func ChannelMonitorSummaries(ctx context.Context, channels []*Channel, modelName string, enabled bool, interval time.Duration, now time.Time) ([]ChannelMonitorSummary, error) {
	const bucketSeconds = int64(15 * 60)
	end := now.Unix()
	start := end - 24*60*60
	ids := make([]int, 0, len(channels))
	for _, channel := range channels {
		ids = append(ids, channel.Id)
	}
	var records []ChannelMonitorRecord
	if len(ids) > 0 {
		if err := DB.WithContext(ctx).Where("channel_id IN ? AND model = ? AND created_at >= ? AND created_at <= ?", ids, modelName, start, end).Order("created_at DESC, id DESC").Limit(100001).Find(&records).Error; err != nil {
			return nil, err
		}
	}
	truncated := len(records) > 100000
	if truncated {
		records = records[:100000]
	}
	byChannel := make(map[int][]ChannelMonitorRecord)
	for _, record := range records {
		byChannel[record.ChannelID] = append(byChannel[record.ChannelID], record)
	}
	summaries := make([]ChannelMonitorSummary, 0, len(channels))
	for _, channel := range channels {
		summary := ChannelMonitorSummary{ChannelID: channel.Id, State: "pending", Buckets: make([]ChannelMonitorBucket, 96), Truncated: truncated}
		for i := range summary.Buckets {
			summary.Buckets[i] = ChannelMonitorBucket{Timestamp: start + int64(i)*bucketSeconds, State: "empty"}
		}
		monitor, targets, err := channel.MonitorTargets()
		if err != nil {
			summary.State = "configuration"
		}
		summary.Enabled = monitor
		protocols := make(map[string]bool)
		for _, target := range targets {
			if target.Model == modelName {
				protocols[target.EndpointType] = true
				summary.Registered = true
			}
		}
		latestProtocols := make(map[string]bool)
		staleProtocol := false
		latestRank := -1
		succeeded, measured := 0, 0
		for _, record := range byChannel[channel.Id] {
			if !protocols[record.EndpointType] {
				continue
			}
			if summary.Latest == nil {
				copy := record
				summary.Latest = &copy
			}
			if !latestProtocols[record.EndpointType] {
				latestProtocols[record.EndpointType] = true
				staleProtocol = staleProtocol || end-record.CreatedAt > int64(interval.Seconds()*2)+60
				if rank := channelMonitorStateRank(record.State); rank > latestRank {
					summary.State = record.State
					latestRank = rank
					copy := record
					summary.Latest = &copy
				}
			}
			index := min(95, int((record.CreatedAt-start)/bucketSeconds))
			bucket := &summary.Buckets[index]
			if bucket.Count == 0 || channelMonitorStateRank(record.State) > channelMonitorStateRank(bucket.State) {
				bucket.State = record.State
				bucket.ErrorCode = record.ErrorCode
			}
			bucket.Count++
			summary.SampleCount++
			if record.State == "healthy" || record.State == "slow" {
				succeeded++
				measured++
				bucket.SuccessCount++
				if bucket.LatencyMs == nil {
					latency := record.LatencyMs
					bucket.LatencyMs = &latency
				}
			} else if record.State == "unavailable" {
				measured++
			}
		}
		if measured > 0 && !truncated {
			rate := float64(succeeded) * 100 / float64(measured)
			summary.SuccessRate = &rate
		}
		switch {
		case err != nil:
			summary.State = "configuration"
		case !monitor:
			summary.State = "disabled"
		case !summary.Registered:
			summary.State = "unregistered"
		case !enabled || channel.Status == common.ChannelStatusManuallyDisabled:
			summary.State = "paused"
		case staleProtocol:
			summary.State = "stale"
		case len(latestProtocols) < len(protocols):
			summary.State = "pending"
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

func channelMonitorStateRank(state string) int {
	switch state {
	case "unavailable":
		return 5
	case "configuration", "unsupported":
		return 4
	case "slow":
		return 3
	case "healthy":
		return 2
	default:
		return 0
	}
}
