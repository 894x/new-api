package wecomaibot

import (
	"context"
	"errors"
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
)

// 日报固定按北京时间统计上一完整自然日。
var reportLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

const reportSentKeyPrefix = "wecom_aibot:report:sent:"

type reportTarget struct {
	Kind string
	ID   string
}

var (
	memReportSentMu sync.Mutex
	memReportSent   = map[string]time.Time{}
)

// resolveReportTargetsFor 只接受显式授权目标或仍有访问权限的管理员，空配置不广播。
// 裸 userid 与 single:userid 均为单聊；群聊必须显式写 group:chatid。
func resolveReportTargetsFor(setting *operation_setting.WecomAIBotSetting) []reportTarget {
	configured := strings.TrimSpace(setting.ReportTargets)
	if configured == "" {
		configured = setting.AdminUserIDs
	}
	var targets []reportTarget
	for _, token := range strings.FieldsFunc(configured, func(r rune) bool {
		return r == ',' || r == '，' || r == ' ' || r == '\n'
	}) {
		target := reportTarget{Kind: chatTypeSingle, ID: token}
		if kind, id, ok := strings.Cut(token, ":"); ok {
			target = reportTarget{Kind: kind, ID: id}
		}
		if target.ID == "" {
			continue
		}
		switch target.Kind {
		case chatTypeSingle:
			if !isUserAllowed(setting, target.ID) {
				continue
			}
		case chatTypeGroup:
			if !setting.GroupEnabled {
				continue
			}
		default:
			continue
		}
		if !slices.Contains(targets, target) {
			targets = append(targets, target)
		}
	}
	return targets
}

// CanSendDailyReport guards both task claiming and execution with node-local ownership.
func CanSendDailyReport() bool {
	status := GetStatus()
	if !status.Enabled || !status.Holding {
		return false
	}
	w := activeConn()
	if w == nil {
		return false
	}
	select {
	case <-w.done:
		return false
	default:
		return true
	}
}

// DailyReportDue keeps retries open throughout the configured Beijing-time hour.
func DailyReportDue(now time.Time, hour int) bool {
	now = now.In(reportLocation)
	if now.Hour() != hour {
		return false
	}
	sent, err := reportAlreadySent(now.Format("2006-01-02"))
	return err != nil || !sent
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
	dayEnd := dayStart + 86400 - 1
	data := &dailyReportData{
		Day:         time.Unix(dayStart, 0).In(reportLocation).Format("2006-01-02"),
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

	prevRows, err := model.GetAllQuotaDates(dayStart-86400, dayStart-1, "")
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

// runDailyReport 向已授权目标推送日报，并保留各目标的投递结果供重试。
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
			return nil, fmt.Errorf("render daily report chart: %w", err)
		} else {
			chartPNG = png
		}
	}

	targets := resolveReportTargetsFor(operation_setting.GetWecomAIBotSetting())
	if len(targets) == 0 {
		return nil, errors.New("no authorized daily report targets")
	}
	sent := 0
	var deliveries []map[string]any
	var failures []error
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		key := day.Format("2006-01-02") + ":" + target.Kind + ":" + target.ID
		chatCode := 1
		if target.Kind == chatTypeGroup {
			chatCode = 2
		}
		delivered := true
		for _, part := range []string{"markdown", "image"} {
			if part == "image" && chartPNG == nil {
				continue
			}
			current := operation_setting.GetWecomAIBotSetting()
			if !current.ReportEnabled || !slices.Contains(resolveReportTargetsFor(current), target) {
				failures = append(failures, errors.New("daily report target authorization revoked"))
				deliveries = append(deliveries, map[string]any{"target": target.Kind + ":" + target.ID, "error": "authorization revoked"})
				delivered = false
				break
			}
			partKey := key + ":" + part
			done, sendErr := reportAlreadySent(partKey)
			if sendErr == nil && !done {
				if !CanSendDailyReport() {
					sendErr = errors.New("daily report connection ownership lost")
				} else if part == "markdown" {
					body := sendMarkdownBody{ChatID: target.ID, ChatType: chatCode, MsgType: "markdown"}
					body.Markdown.Content = markdown
					sendErr = sendProactiveFrame(body)
				} else {
					w := activeConn()
					var mediaID string
					if w == nil {
						sendErr = errors.New("daily report connection is offline")
					} else {
						mediaID, sendErr = w.uploadMedia(chartPNG, "daily-report.png")
					}
					if sendErr == nil {
						sendErr = sendProactiveFrame(sendImageBody{ChatID: target.ID, ChatType: chatCode, MsgType: "image", Image: mediaImage{MediaID: mediaID}})
					}
				}
				if sendErr == nil {
					sendErr = markReportSent(partKey)
				}
			}
			if sendErr != nil {
				failures = append(failures, fmt.Errorf("%s:%s: %w", target.Kind, target.ID, sendErr))
				deliveries = append(deliveries, map[string]any{"target": target.Kind + ":" + target.ID, "part": part, "error": sendErr.Error()})
				delivered = false
				break
			}
		}
		if delivered {
			sent++
			deliveries = append(deliveries, map[string]any{"target": target.Kind + ":" + target.ID, "delivered": true})
		}
	}
	summary := map[string]any{"sent": sent, "targets": len(targets), "deliveries": deliveries}
	return summary, errors.Join(failures...)
}

// RunDailyReportTask only runs on the connection owner; failed deliveries remain retryable.
func RunDailyReportTask(ctx context.Context, reportHour int) (map[string]any, error) {
	setting := operation_setting.GetWecomAIBotSetting()
	if !setting.Enabled || !setting.ReportEnabled {
		return map[string]any{"skipped": "disabled"}, nil
	}
	now := time.Now().In(reportLocation)
	today := now.Format("2006-01-02")
	sent, err := reportAlreadySent(today)
	if err != nil {
		return nil, err
	}
	if sent {
		return map[string]any{"skipped": "already_sent_today"}, nil
	}
	if now.Hour() != reportHour {
		return map[string]any{"skipped": "not_report_hour"}, nil
	}
	if !CanSendDailyReport() {
		return nil, errors.New("daily report requires the active connection owner")
	}
	summary, err := runDailyReport(ctx, now.AddDate(0, 0, -1))
	if err != nil {
		return summary, err
	}
	if err := markReportSent(today); err != nil {
		return summary, err
	}
	return summary, nil
}

// reportAlreadySent / markReportSent 按日期幂等，跨调度行去重。
func reportAlreadySent(day string) (bool, error) {
	if common.RedisEnabled {
		exists, err := common.RDB.Exists(context.Background(), reportSentKeyPrefix+day).Result()
		return exists > 0, err
	}
	memReportSentMu.Lock()
	defer memReportSentMu.Unlock()
	_, ok := memReportSent[day]
	return ok, nil
}

func markReportSent(day string) error {
	if common.RedisEnabled {
		return common.RDB.Set(context.Background(), reportSentKeyPrefix+day, "1", 49*time.Hour).Err()
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
	return nil
}
