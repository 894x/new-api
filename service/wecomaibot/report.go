package wecomaibot

import (
	"context"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/go-redis/redis/v8"
)

// 每日运营日报：按自然天汇总用量与性能，经 aibot_send_msg 主动推送到最近
// 活跃的会话目标（企微要求用户先给机器人发过消息，故只推 7 天内活跃目标）。

const (
	reportTargetKey     = "wecom_aibot:report:targets"
	reportTargetMaxAge  = 7 * 24 * time.Hour
	reportSentKeyPrefix = "wecom_aibot:report:sent:"
)

// reportTarget 一个可主动推送的会话目标。
type reportTarget struct {
	Kind string // group / single
	ID   string
}

var (
	memReportTargetsMu sync.Mutex
	memReportTargets   = map[string]time.Time{}
	memReportSentMu    sync.Mutex
	memReportSent      = map[string]time.Time{}
)

// recordChatTarget 记录最近活跃的会话目标（群 chatid / 单聊 userid），供日报推送。
func recordChatTarget(chat *msgCallback) {
	var kind, id string
	switch chat.ChatType {
	case chatTypeGroup:
		kind, id = chatTypeGroup, chat.ChatID
	case chatTypeSingle:
		kind, id = chatTypeSingle, chat.From.UserID
	default:
		return
	}
	if id == "" {
		return
	}
	member := kind + ":" + id
	if common.RedisEnabled {
		if err := common.RDB.ZAdd(context.Background(), reportTargetKey,
			&redis.Z{Score: float64(time.Now().Unix()), Member: member}).Err(); err != nil {
			common.SysLog("wecom ai bot record report target failed: " + err.Error())
		}
		return
	}
	memReportTargetsMu.Lock()
	memReportTargets[member] = time.Now()
	memReportTargetsMu.Unlock()
}

// resolveReportTargets 按配置解析推送目标（优先级：显式指定 > 管理员单聊 > 全部活跃）。
func resolveReportTargets(maxAge time.Duration) []reportTarget {
	return resolveReportTargetsFor(operation_setting.GetWecomAIBotSetting(), maxAge)
}

func resolveReportTargetsFor(setting *operation_setting.WecomAIBotSetting, maxAge time.Duration) []reportTarget {
	if configured := strings.TrimSpace(setting.ReportTargets); configured != "" {
		targets := make([]reportTarget, 0)
		for _, token := range strings.FieldsFunc(configured, func(r rune) bool {
			return r == ',' || r == '，' || r == ' ' || r == '\n'
		}) {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			// 群 chatid 通常较长/含特殊字符，短 token 视为 userid 单聊
			targets = append(targets, reportTarget{Kind: chatTypeSingle, ID: token})
		}
		return targets
	}
	if admins := operation_setting.ParseWecomAIBotAdminUserids(setting.AdminUserIDs); len(admins) > 0 {
		targets := make([]reportTarget, 0, len(admins))
		for _, userid := range admins {
			targets = append(targets, reportTarget{Kind: chatTypeSingle, ID: userid})
		}
		return targets
	}
	return listReportTargets(maxAge)
}

// listReportTargets 返回 maxAge 内活跃过的推送目标。
func listReportTargets(maxAge time.Duration) []reportTarget {
	var members []string
	if common.RedisEnabled {
		deadline := strconv.FormatInt(time.Now().Add(-maxAge).Unix(), 10)
		result, err := common.RDB.ZRangeByScore(context.Background(), reportTargetKey,
			&redis.ZRangeBy{Min: deadline, Max: "+inf"}).Result()
		if err != nil {
			common.SysLog("wecom ai bot list report targets failed: " + err.Error())
			return nil
		}
		members = result
	} else {
		now := time.Now()
		memReportTargetsMu.Lock()
		for member, seen := range memReportTargets {
			if now.Sub(seen) > maxAge {
				delete(memReportTargets, member)
				continue
			}
			members = append(members, member)
		}
		memReportTargetsMu.Unlock()
	}
	targets := make([]reportTarget, 0, len(members))
	for _, member := range members {
		kind, id, ok := strings.Cut(member, ":")
		if !ok || id == "" {
			continue
		}
		targets = append(targets, reportTarget{Kind: kind, ID: id})
	}
	return targets
}

// dailyReportData 一个自然天的运营汇总。
type dailyReportData struct {
	Day          string
	TotalUSD     float64
	PrevUSD      float64
	Requests     int64
	PrevRequests int64
	Tokens       int64
	SuccessRate  float64 // 0-100；无性能数据时为 -1
	TopModels    []map[string]any
	Anomalies    []string
}

// gatherDailyReport 汇总 dayStart 起一个自然天的用量、性能与异常项。
func gatherDailyReport(dayStart int64) (*dailyReportData, error) {
	dayEnd := dayStart + 86400
	data := &dailyReportData{
		Day:         time.Unix(dayStart, 0).Format("2006-01-02"),
		SuccessRate: -1,
	}
	rows, err := model.GetAllQuotaDates(dayStart, dayEnd, "")
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		data.Requests += int64(row.Count)
		data.Tokens += int64(row.TokenUsed)
		data.TotalUSD += float64(row.Quota) / common.QuotaPerUnit
	}
	data.TopModels = topUsage(rows, func(row *model.QuotaData) string { return row.ModelName }, 5)

	prevRows, err := model.GetAllQuotaDates(dayStart-86400, dayStart, "")
	if err != nil {
		return nil, err
	}
	for _, row := range prevRows {
		data.PrevRequests += int64(row.Count)
		data.PrevUSD += float64(row.Quota) / common.QuotaPerUnit
	}

	summaries, err := model.GetPerfMetricsSummaryAll(dayStart, dayEnd, nil)
	if err != nil {
		common.SysLog("wecom ai bot daily report perf query failed: " + err.Error())
	} else {
		var requests, success int64
		modelStats := map[string]*[2]int64{}
		for _, summary := range summaries {
			requests += summary.RequestCount
			success += summary.SuccessCount
			stats, ok := modelStats[summary.ModelName]
			if !ok {
				stats = &[2]int64{}
				modelStats[summary.ModelName] = stats
			}
			stats[0] += summary.RequestCount
			stats[1] += summary.SuccessCount
		}
		if requests > 0 {
			data.SuccessRate = roundPercent(float64(success) / float64(requests) * 100)
			for _, name := range slices.Sorted(maps.Keys(modelStats)) {
				stats := modelStats[name]
				if stats[0] >= 50 && float64(stats[1])/float64(stats[0]) < 0.95 {
					data.Anomalies = append(data.Anomalies, fmt.Sprintf(
						"模型 %s 错误率 %.1f%%（%d 次请求）",
						name, (1-float64(stats[1])/float64(stats[0]))*100, stats[0]))
				}
			}
		}
	}
	if data.PrevRequests >= 200 && data.Requests*10 < data.PrevRequests*3 {
		data.Anomalies = append(data.Anomalies, fmt.Sprintf("请求量环比骤降：%d → %d 次", data.PrevRequests, data.Requests))
	}
	data.TotalUSD = math.Round(data.TotalUSD*100) / 100
	data.PrevUSD = math.Round(data.PrevUSD*100) / 100
	return data, nil
}

// renderDailyReportMarkdown 生成日报正文：核心数字带环比、异常置顶、Top 模型。
func renderDailyReportMarkdown(data *dailyReportData) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "**new-api 运营日报 · %s**\n\n", data.Day)
	fmt.Fprintf(&builder, "**消费 $%.2f**（前日 $%.2f，环比 %s）\n", data.TotalUSD, data.PrevUSD, changePercent(data.TotalUSD, data.PrevUSD))
	fmt.Fprintf(&builder, "**请求 %d 次**（前日 %d 次，环比 %s）\n", data.Requests, data.PrevRequests, changePercent(float64(data.Requests), float64(data.PrevRequests)))
	if data.SuccessRate >= 0 {
		fmt.Fprintf(&builder, "**成功率 %.1f%%**\n", data.SuccessRate)
	}
	if len(data.Anomalies) > 0 {
		builder.WriteString("\n**异常关注**\n")
		for _, item := range data.Anomalies {
			fmt.Fprintf(&builder, "- %s\n", item)
		}
	}
	if len(data.TopModels) > 0 {
		builder.WriteString("\n**模型消费 Top**\n")
		for _, entry := range data.TopModels {
			fmt.Fprintf(&builder, "- %s：$%.2f · %d 次\n", entry["name"], entry["usd"], entry["requests"])
		}
	}
	return truncateStreamContent(builder.String())
}

func changePercent(current, previous float64) string {
	if previous <= 0 {
		if current <= 0 {
			return "持平"
		}
		return "新增"
	}
	return fmt.Sprintf("%+.1f%%", (current-previous)/previous*100)
}

// runDailyReport 生成并推送一次日报到全部活跃目标，返回推送摘要。
func runDailyReport(ctx context.Context, day time.Time) (map[string]any, error) {
	dayStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location()).Unix()
	data, err := gatherDailyReport(dayStart)
	if err != nil {
		return nil, err
	}
	markdown := renderDailyReportMarkdown(data)

	var chartPNG []byte
	if len(data.TopModels) > 0 {
		labels := make([]string, 0, len(data.TopModels))
		values := make([]float64, 0, len(data.TopModels))
		for _, entry := range data.TopModels {
			name, _ := entry["name"].(string)
			usd, _ := entry["usd"].(float64)
			labels = append(labels, name)
			values = append(values, usd)
		}
		spec := &chartSpec{
			Kind: "bar", Title: "昨日模型消费 Top " + strconv.Itoa(len(labels)), Unit: "$",
			YLabel: "消费（美元）", Labels: labels,
			Series: []chartSeries{{Name: "消费", Data: values}},
		}
		if png, err := renderChartPNG(spec); err != nil {
			common.SysLog("wecom ai bot daily report chart failed: " + err.Error())
		} else {
			chartPNG = png
		}
	}

	targets := resolveReportTargets(reportTargetMaxAge)
	sent := 0
	var failures []string
	for _, target := range targets {
		if ctx.Err() != nil {
			break
		}
		chatCode := 1
		if target.Kind == chatTypeGroup {
			chatCode = 2
		}
		markdownBody := sendMarkdownBody{ChatID: target.ID, ChatType: chatCode, MsgType: "markdown"}
		markdownBody.Markdown.Content = markdown
		if err := sendProactiveFrame(markdownBody); err != nil {
			failures = append(failures, fmt.Sprintf("%s:%s: %v", target.Kind, target.ID, err))
			continue
		}
		if chartPNG != nil {
			if w := activeConn(); w != nil {
				if mediaID, err := w.uploadMedia(chartPNG, "daily-report.png"); err == nil {
					imageBody := sendImageBody{ChatID: target.ID, ChatType: chatCode, MsgType: "image", Image: mediaImage{MediaID: mediaID}}
					if err := sendProactiveFrame(imageBody); err != nil {
						common.SysLog("wecom ai bot daily report image push failed: " + err.Error())
					}
				} else {
					common.SysLog("wecom ai bot daily report image upload failed: " + err.Error())
				}
			}
		}
		sent++
	}
	return map[string]any{"sent": sent, "targets": len(targets), "failures": failures}, nil
}

// RunDailyReportTask 系统任务入口：到点且当日未发送时生成并推送日报。
func RunDailyReportTask(ctx context.Context, reportHour int) (map[string]any, error) {
	now := time.Now()
	today := now.Format("2006-01-02")
	if reportAlreadySent(today) {
		return map[string]any{"skipped": "already_sent_today"}, nil
	}
	if now.Hour() != reportHour {
		return map[string]any{"skipped": "not_report_hour"}, nil
	}
	summary, err := runDailyReport(ctx, now)
	if err != nil {
		return nil, err
	}
	markReportSent(today)
	return summary, nil
}

// reportAlreadySent / markReportSent 按日期幂等，跨调度行去重。
func reportAlreadySent(day string) bool {
	if common.RedisEnabled {
		exists, err := common.RDB.Exists(context.Background(), reportSentKeyPrefix+day).Result()
		return err == nil && exists > 0
	}
	memReportSentMu.Lock()
	defer memReportSentMu.Unlock()
	_, ok := memReportSent[day]
	return ok
}

func markReportSent(day string) {
	if common.RedisEnabled {
		if err := common.RDB.Set(context.Background(), reportSentKeyPrefix+day, "1", 25*time.Hour).Err(); err != nil {
			common.SysLog("wecom ai bot mark report sent failed: " + err.Error())
		}
		return
	}
	memReportSentMu.Lock()
	defer memReportSentMu.Unlock()
	now := time.Now()
	for key, seen := range memReportSent {
		if now.Sub(seen) > 7*24*time.Hour {
			delete(memReportSent, key)
		}
	}
	memReportSent[day] = now
}
