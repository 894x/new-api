package wecomaibot

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/perf_metrics"

	"github.com/samber/lo"
)

// Tool 是暴露给大模型的查询工具：Parameters 为 JSON Schema，Handler 全部只读，
// 返回结果不得包含渠道 Key 等凭据。AdminOnly 工具仅对管理员身份开放（含用户
// 隐私数据或会真实触达上游的能力），由 handler 按配置的管理员名单过滤。
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]any
	AdminOnly   bool
	Handler     func(ctx context.Context, args map[string]any) (any, error)
}

// extraTools 是宿主注入的扩展工具（如需要 relay 依赖的余额实时刷新）。
var extraTools []Tool

// RegisterTool 注册宿主提供的扩展工具；与现有工具重名时忽略。
func RegisterTool(tool Tool) {
	for _, existing := range currentTools() {
		if existing.Name == tool.Name {
			common.SysLog("wecom ai bot duplicate tool ignored: " + tool.Name)
			return
		}
	}
	extraTools = append(extraTools, tool)
}

func currentTools() []Tool {
	return append(builtinTools(), extraTools...)
}

func argString(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return value
}

// argInt JSON 数字统一解码为 float64，转 int 并提供缺省值。
func argInt(args map[string]any, key string, def int) int {
	value, ok := args[key].(float64)
	if !ok {
		return def
	}
	return int(value)
}

func truncateRunes(text string, max int) string {
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return string(runes[:max]) + "…"
}

func formatUnixTime(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04:05")
}

// channelView 渠道摘要视图（永不包含 key 等凭据）。
func channelView(channel *model.Channel) map[string]any {
	return map[string]any{
		"id":                 channel.Id,
		"name":               channel.Name,
		"type":               channel.Type,
		"type_name":          constant.GetChannelTypeName(channel.Type),
		"status":             channel.Status,
		"group":              channel.Group,
		"priority":           lo.FromPtrOr(channel.Priority, 0),
		"weight":             lo.FromPtrOr(channel.Weight, 0),
		"balance":            channel.Balance,
		"balance_updated_at": formatUnixTime(channel.BalanceUpdatedTime),
		"model_count":        len(channel.GetModels()),
	}
}

// fetchAllChannels 分页拉取全部渠道（不含 key）。GetAllChannels 的 num 会被
// 原样写成 SQL LIMIT，不能传 0，否则全量查询会得到 LIMIT 0 的空结果。
func fetchAllChannels() ([]*model.Channel, error) {
	const pageSize = 100
	var all []*model.Channel
	for start := 0; ; start += pageSize {
		page, err := model.GetAllChannels(start, pageSize, false, true)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < pageSize {
			return all, nil
		}
	}
}

// roundPercent 把已是百分数刻度（0-100）的成功率四舍五入到一位小数。
func roundPercent(value float64) float64 {
	return math.Round(value*10) / 10
}

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}

// quotaToUSD 把内部 quota 换算为美元。
func quotaToUSD(quota int64) float64 {
	return math.Round(float64(quota)/float64(common.QuotaPerUnit)*10000) / 10000
}

// perfSeriesMaxPoints 限制回传给模型的序列点数，保证结果不超工具结果预算。
const perfSeriesMaxPoints = 32

// perfSeriesViews 把性能桶序列降采样为紧凑视图，供模型阅读与画趋势图。
func perfSeriesViews(series []perfmetrics.BucketPoint) []map[string]any {
	if len(series) > perfSeriesMaxPoints {
		step := (len(series) + perfSeriesMaxPoints - 1) / perfSeriesMaxPoints
		sampled := make([]perfmetrics.BucketPoint, 0, perfSeriesMaxPoints+1)
		for i := 0; i < len(series); i += step {
			sampled = append(sampled, series[i])
		}
		if last := series[len(series)-1]; sampled[len(sampled)-1].Ts != last.Ts {
			sampled = append(sampled, last)
		}
		series = sampled
	}
	views := make([]map[string]any, 0, len(series))
	for _, point := range series {
		view := map[string]any{
			"time":           time.Unix(point.Ts, 0).Format("15:04"),
			"success_rate":   roundPercent(point.SuccessRate),
			"avg_latency_ms": point.AvgLatencyMs,
		}
		if point.AvgTtftMs > 0 {
			view["avg_ttft_ms"] = point.AvgTtftMs
		}
		if point.AvgTps > 0 {
			view["avg_tps"] = round2(point.AvgTps)
		}
		views = append(views, view)
	}
	return views
}

// topUsage 按指定维度聚合用量行并按消费降序取前 N。
func topUsage(rows []*model.QuotaData, key func(*model.QuotaData) string, top int) []map[string]any {
	type usageTotals struct {
		requests int64
		tokens   int64
		quota    int64
	}
	byKey := map[string]*usageTotals{}
	for _, row := range rows {
		name := key(row)
		if name == "" {
			continue
		}
		agg, ok := byKey[name]
		if !ok {
			agg = &usageTotals{}
			byKey[name] = agg
		}
		agg.requests += int64(row.Count)
		agg.tokens += int64(row.TokenUsed)
		agg.quota += int64(row.Quota)
	}
	names := make([]string, 0, len(byKey))
	for name := range byKey {
		names = append(names, name)
	}
	slices.SortFunc(names, func(a, b string) int {
		return cmp.Compare(byKey[b].quota, byKey[a].quota)
	})
	if len(names) > top {
		names = names[:top]
	}
	views := make([]map[string]any, 0, len(names))
	for _, name := range names {
		agg := byKey[name]
		views = append(views, map[string]any{
			"name":     name,
			"requests": agg.requests,
			"tokens":   agg.tokens,
			"usd":      quotaToUSD(agg.quota),
		})
	}
	return views
}

func findChannelByArgs(args map[string]any) (*model.Channel, error) {
	channelID := argInt(args, "channel_id", 0)
	name := strings.TrimSpace(argString(args, "name"))
	if channelID > 0 {
		return model.GetChannelById(channelID, false)
	}
	if name != "" {
		channels, err := model.SearchChannels(name, "", "", true)
		if err != nil {
			return nil, err
		}
		for _, channel := range channels {
			if channel.Name == name {
				return channel, nil
			}
		}
		if len(channels) > 0 {
			return channels[0], nil
		}
		return nil, fmt.Errorf("no channel named %q", name)
	}
	return nil, errors.New("channel_id or name is required")
}

func builtinTools() []Tool {
	return []Tool{
		{
			Name:        "search_channels",
			Description: "按关键字搜索渠道（支持渠道 ID、名称、模型名匹配），返回渠道基础信息列表；关键字留空返回全部渠道。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"keyword": map[string]any{"type": "string", "description": "渠道 ID、名称或模型名关键字"},
					"status":  map[string]any{"type": "integer", "description": "按状态过滤：1=启用，2=手动禁用，3=自动禁用；缺省为全部"},
				},
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				keyword := strings.TrimSpace(argString(args, "keyword"))
				statusFilter := argInt(args, "status", 0)
				var channels []*model.Channel
				var err error
				if keyword == "" {
					channels, err = fetchAllChannels()
				} else {
					channels, err = model.SearchChannels(keyword, "", "", true)
				}
				if err != nil {
					return nil, err
				}
				const maxResults = 20
				views := make([]map[string]any, 0, min(len(channels), maxResults))
				matched := 0
				for _, channel := range channels {
					if statusFilter != 0 && channel.Status != statusFilter {
						continue
					}
					matched++
					if len(views) < maxResults {
						views = append(views, channelView(channel))
					}
				}
				return map[string]any{"matched": matched, "returned": len(views), "channels": views}, nil
			},
		},
		{
			Name:        "get_channel_detail",
			Description: "查询单个渠道的详细状态：启用状态、缓存余额、支持模型、分组、优先级、响应时间等。余额来自平台缓存（可能滞后于上游实时值）。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"channel_id": map[string]any{"type": "integer", "description": "渠道 ID"},
					"name":       map[string]any{"type": "string", "description": "渠道名称（没有 ID 时按名称查找）"},
				},
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				channel, err := findChannelByArgs(args)
				if err != nil {
					return nil, err
				}
				models := channel.GetModels()
				const maxModels = 50
				modelNames := models
				if len(modelNames) > maxModels {
					modelNames = modelNames[:maxModels]
				}
				view := channelView(channel)
				view["models"] = modelNames
				view["model_total"] = len(models)
				view["response_time_ms"] = channel.ResponseTime
				view["test_time"] = formatUnixTime(channel.TestTime)
				view["used_quota"] = channel.UsedQuota
				view["auto_ban"] = lo.FromPtrOr(channel.AutoBan, 1) == 1
				view["tag"] = lo.FromPtrOr(channel.Tag, "")
				view["remark"] = channel.Remark
				view["created_at"] = formatUnixTime(channel.CreatedTime)
				view["is_multi_key"] = channel.ChannelInfo.IsMultiKey
				if channel.TestModel != nil && *channel.TestModel != "" {
					view["test_model"] = *channel.TestModel
				}
				return view, nil
			},
		},
		{
			Name:        "get_recent_channel_errors",
			Description: "查询最近的渠道转发错误日志，可按渠道过滤。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"minutes":    map[string]any{"type": "integer", "description": "查询最近多少分钟内的错误，缺省 60，上限 1440"},
					"channel_id": map[string]any{"type": "integer", "description": "只看该渠道；缺省为全部渠道"},
					"limit":      map[string]any{"type": "integer", "description": "返回条数，缺省 10，上限 20"},
				},
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				minutes := min(max(argInt(args, "minutes", 60), 1), 1440)
				limit := min(max(argInt(args, "limit", 10), 1), 20)
				channelID := argInt(args, "channel_id", 0)
				now := time.Now().Unix()
				logs, _, err := model.GetAllLogs(model.LogTypeError, now-int64(minutes)*60, now, "", "", "", 0, limit, channelID, "", "", "")
				if err != nil {
					return nil, err
				}
				views := make([]map[string]any, 0, len(logs))
				for _, entry := range logs {
					views = append(views, map[string]any{
						"time":       formatUnixTime(entry.CreatedAt),
						"channel_id": entry.ChannelId,
						"model_name": entry.ModelName,
						"content":    truncateRunes(entry.Content, 200),
					})
				}
				return map[string]any{"total": len(views), "logs": views}, nil
			},
		},
		{
			Name:        "get_last_balance_patrol",
			Description: "查询最近一次渠道余额巡检系统任务的执行结果（检查渠道数、更新/禁用/失败计数）。",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				task, err := model.GetLatestSystemTask(model.SystemTaskTypeChannelBalance)
				if err != nil {
					return nil, err
				}
				if task == nil {
					return map[string]any{"message": "尚未执行过余额巡检"}, nil
				}
				var result any
				if task.Result != "" {
					var parsed any
					if err := common.UnmarshalJsonStr(task.Result, &parsed); err == nil {
						result = parsed
					} else {
						result = task.Result
					}
				}
				return map[string]any{
					"status":      string(task.Status),
					"finished_at": formatUnixTime(task.UpdatedAt),
					"error":       task.Error,
					"result":      result,
				}, nil
			},
		},
		{
			Name:        "get_system_status",
			Description: "查询平台整体状态：版本、运行时长、数据库类型、Redis、渠道启用/禁用计数。",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				channels, err := fetchAllChannels()
				if err != nil {
					return nil, err
				}
				enabled := 0
				for _, channel := range channels {
					if channel.Status == common.ChannelStatusEnabled {
						enabled++
					}
				}
				return map[string]any{
					"version":          common.Version,
					"uptime_seconds":   time.Now().Unix() - common.StartTime,
					"main_database":    string(common.MainDatabaseType()),
					"log_database":     string(common.LogDatabaseType()),
					"redis_enabled":    common.RedisEnabled,
					"channel_total":    len(channels),
					"channel_enabled":  enabled,
					"channel_disabled": len(channels) - enabled,
				}, nil
			},
		},
		{
			Name:        "get_model_performance",
			Description: "查询模型转发性能统计：请求量、成功率、平均延迟、平均首字延迟、吞吐 TPS。指定模型时返回按时间桶的序列（适合用 render_chart 画趋势图）；留空返回全部模型概览。数据来自性能指标桶聚合，最多滞后一个桶周期。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"model": map[string]any{"type": "string", "description": "模型名；留空返回全部模型概览"},
					"hours": map[string]any{"type": "integer", "description": "统计最近多少小时，缺省 24，范围 1-168"},
					"group": map[string]any{"type": "string", "description": "只统计该分组；留空为全部分组"},
				},
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				hours := min(max(argInt(args, "hours", 24), 1), 168)
				modelName := strings.TrimSpace(argString(args, "model"))
				group := strings.TrimSpace(argString(args, "group"))
				if modelName == "" {
					summary, err := perfmetrics.QuerySummaryAll(hours, nil)
					if err != nil {
						return nil, err
					}
					models := append([]perfmetrics.ModelSummary{}, summary.Models...)
					slices.SortFunc(models, func(a, b perfmetrics.ModelSummary) int {
						return cmp.Compare(b.RequestCount, a.RequestCount)
					})
					if len(models) > 10 {
						models = models[:10]
					}
					views := make([]map[string]any, 0, len(models))
					for _, entry := range models {
						views = append(views, map[string]any{
							"model":          entry.ModelName,
							"requests":       entry.RequestCount,
							"success_rate":   roundPercent(entry.SuccessRate),
							"avg_latency_ms": entry.AvgLatencyMs,
							"avg_tps":        round2(entry.AvgTps),
						})
					}
					overview := map[string]any{"hours": hours, "models": views}
					if summary.Summary != nil {
						overview["overall_success_rate"] = roundPercent(summary.Summary.SuccessRate)
						overview["overall_avg_latency_ms"] = summary.Summary.AvgLatencyMs
					}
					if len(views) == 0 {
						overview["message"] = "窗口内没有性能数据"
					}
					return overview, nil
				}
				result, err := perfmetrics.Query(perfmetrics.QueryParams{Model: modelName, Group: group, Hours: hours})
				if err != nil {
					return nil, err
				}
				detail := map[string]any{"model": modelName, "hours": hours}
				if result.Summary != nil {
					detail["success_rate"] = roundPercent(result.Summary.SuccessRate)
					detail["avg_latency_ms"] = result.Summary.AvgLatencyMs
					detail["avg_tps"] = round2(result.Summary.AvgTps)
				}
				detail["series"] = perfSeriesViews(result.Series)
				if len(result.Groups) > 0 {
					groups := make([]map[string]any, 0, len(result.Groups))
					for _, entry := range result.Groups {
						groups = append(groups, map[string]any{
							"group":          entry.Group,
							"success_rate":   roundPercent(entry.SuccessRate),
							"avg_latency_ms": entry.AvgLatencyMs,
						})
					}
					detail["groups"] = groups
				}
				if len(result.Series) == 0 {
					detail["message"] = "窗口内没有该模型的性能数据"
				}
				return detail, nil
			},
		},
		{
			Name:        "get_usage_overview",
			AdminOnly:   true,
			Description: "查询用量统计（仅管理员）：按模型的调用量、Token 与消费金额，以及按用户的调用量与消费金额（数据来自数据看板聚合，可能有几分钟滞后，适合用 render_chart 画对比或占比图）。金额为美元。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"hours": map[string]any{"type": "integer", "description": "统计最近多少小时，缺省 24，范围 1-168"},
					"top":   map[string]any{"type": "integer", "description": "每类返回条数，缺省 10，范围 1-20"},
				},
			},
			Handler: func(ctx context.Context, args map[string]any) (any, error) {
				hours := min(max(argInt(args, "hours", 24), 1), 168)
				top := min(max(argInt(args, "top", 10), 1), 20)
				now := time.Now().Unix()
				start := now - int64(hours)*3600
				modelRows, err := model.GetAllQuotaDates(start, now, "")
				if err != nil {
					return nil, err
				}
				userRows, err := model.GetQuotaDataGroupByUser(start, now)
				if err != nil {
					return nil, err
				}
				byModel := topUsage(modelRows, func(row *model.QuotaData) string { return row.ModelName }, top)
				byUser := topUsage(userRows, func(row *model.QuotaData) string { return row.Username }, top)
				overview := map[string]any{
					"hours":    hours,
					"by_model": byModel,
					"by_user":  byUser,
				}
				if len(byModel) == 0 && len(byUser) == 0 {
					overview["message"] = "窗口内没有用量数据"
				}
				return overview, nil
			},
		},
		{
			Name:        "render_chart",
			Description: "把数据渲染成图表，作为图片消息跟在文字回复之后（单次回复最多 3 张，只渲染真实查到的数据）。kind：line=时间序列趋势折线；bar=单系列对比柱状图；grouped_bar=多系列分组柱状图；donut=占比环形图。",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind":   map[string]any{"type": "string", "description": "图表类型：line、bar、grouped_bar 或 donut"},
					"title":  map[string]any{"type": "string", "description": "图表标题（必填）"},
					"labels": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "x 轴标签或类别名，1-60 个"},
					"series": map[string]any{
						"type":        "array",
						"description": "数据系列，1-6 组；每组 {name, data}，data 为与 labels 等长的数字数组；donut 只允许一组",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"name": map[string]any{"type": "string", "description": "系列名"},
								"data": map[string]any{"type": "array", "items": map[string]any{"type": "number"}, "description": "数值数组"},
							},
						},
					},
					"subtitle": map[string]any{"type": "string", "description": "副标题（可选）"},
					"y_label":  map[string]any{"type": "string", "description": "y 轴名称（可选）"},
					"unit":     map[string]any{"type": "string", "description": "数值单位（可选），如 %、$、ms"},
				},
				"required": []string{"kind", "title", "labels", "series"},
			},
			Handler: renderChartToolHandler,
		},
	}
}
