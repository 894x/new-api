package service

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	// 企微群机器人接口限频 20 条/分钟，单发送协程按更保守的间隔发送。
	wecomBotSendInterval = 3500 * time.Millisecond
	wecomBotQueueSize    = 128
	// 企微 markdown 消息上限 4096 字节，预留格式与 @ 提示的开销。
	wecomBotMaxContentBytes = 3800
	wecomBotMaxMessageRunes = 300
)

// wecomBotErrorAlertInfo 携带从请求路径预提取的脱敏错误上下文，供异步告警使用。
type wecomBotErrorAlertInfo struct {
	ChannelId   int
	ChannelName string
	StatusCode  int
	ErrorCode   string
	Message     string
	ModelName   string
	Attempt     int
}

type wecomBotAlert struct {
	title    string
	color    string
	lines    []string
	category string
	// scope 告警针对的对象："channel:<id>" 或空（用户巡检等无渠道维度）。
	// 发送前据此检查静默：scope 命中或全局静默（"all"）时丢弃。
	scope string
}

// enqueueWecomBotAlert 是告警入队入口，测试中可替换以拦截。
var enqueueWecomBotAlert = enqueueWecomBotAlertToQueue

var (
	wecomBotQueueOnce sync.Once
	wecomBotQueue     chan wecomBotAlert
)

func enqueueWecomBotAlertToQueue(alert wecomBotAlert) {
	wecomBotQueueOnce.Do(func() {
		wecomBotQueue = make(chan wecomBotAlert, wecomBotQueueSize)
		gopool.Go(runWecomBotQueueWorker)
	})
	select {
	case wecomBotQueue <- alert:
	default:
		common.SysLog("wecom bot alert queue is full, dropping alert")
	}
}

func runWecomBotQueueWorker() {
	var lastSend time.Time
	for alert := range wecomBotQueue {
		if isWecomBotAlertSilenced(alert.scope) {
			continue
		}
		if wait := wecomBotSendInterval - time.Since(lastSend); wait > 0 {
			time.Sleep(wait)
		}
		lastSend = time.Now()
		if err := sendWecomBotAlert(alert); err != nil {
			common.SysLog(fmt.Sprintf("failed to send wecom bot alert: %s", err.Error()))
		}
	}
}

// ============================================================================
// 告警静默（免打扰）
// ============================================================================

const (
	wecomBotSilenceAll      = "all"
	wecomBotSilenceRedisKey = "wecom_bot_silence"
)

// WecomBotAlertSilence 一条生效中的静默规则。
type WecomBotAlertSilence struct {
	Scope  string    `json:"scope"` // "all" 或 "channel:<id>"
	Reason string    `json:"reason"`
	By     string    `json:"by"`
	Until  time.Time `json:"until"`
}

type wecomBotSilenceRecord struct {
	Reason string `json:"reason"`
	By     string `json:"by"`
	Until  int64  `json:"until"`
}

var memWecomBotSilences sync.Map // scope -> wecomBotSilenceRecord（Redis 不可用时的降级存储）

// SetWecomBotAlertSilence 设置静默：scope 为 "all" 或 "channel:<id>"，until 之前丢弃匹配的告警。
func SetWecomBotAlertSilence(scope string, until time.Time, reason string, by string) error {
	scope = normalizeWecomBotSilenceScope(scope)
	if scope == "" {
		return fmt.Errorf("invalid silence scope")
	}
	if !until.After(time.Now()) {
		return fmt.Errorf("silence until must be in the future")
	}
	record := wecomBotSilenceRecord{Reason: reason, By: by, Until: until.Unix()}
	if common.RedisEnabled {
		encoded, err := common.Marshal(record)
		if err != nil {
			return err
		}
		return common.RDB.HSet(context.Background(), wecomBotSilenceRedisKey, scope, string(encoded)).Err()
	}
	memWecomBotSilences.Store(scope, record)
	return nil
}

// ClearWecomBotAlertSilence 解除静默，返回是否确实存在该条。
func ClearWecomBotAlertSilence(scope string) bool {
	scope = normalizeWecomBotSilenceScope(scope)
	if scope == "" {
		return false
	}
	if common.RedisEnabled {
		removed, err := common.RDB.HDel(context.Background(), wecomBotSilenceRedisKey, scope).Result()
		if err != nil {
			common.SysLog(fmt.Sprintf("failed to clear wecom bot silence: %s", err.Error()))
			return false
		}
		return removed > 0
	}
	_, existed := memWecomBotSilences.LoadAndDelete(scope)
	return existed
}

// ListWecomBotAlertSilences 列出所有生效中的静默规则（顺带清理已过期的）。
func ListWecomBotAlertSilences() []WecomBotAlertSilence {
	var records map[string]string
	if common.RedisEnabled {
		var err error
		records, err = common.RDB.HGetAll(context.Background(), wecomBotSilenceRedisKey).Result()
		if err != nil {
			common.SysLog(fmt.Sprintf("failed to list wecom bot silences: %s", err.Error()))
			return nil
		}
	} else {
		records = make(map[string]string)
		memWecomBotSilences.Range(func(key, value any) bool {
			if scope, ok := key.(string); ok {
				if record, ok := value.(wecomBotSilenceRecord); ok {
					encoded, err := common.Marshal(record)
					if err == nil {
						records[scope] = string(encoded)
					}
				}
			}
			return true
		})
	}
	now := time.Now().Unix()
	silences := make([]WecomBotAlertSilence, 0, len(records))
	for scope, encoded := range records {
		var record wecomBotSilenceRecord
		if err := common.Unmarshal([]byte(encoded), &record); err != nil {
			continue
		}
		if record.Until <= now {
			// Redis 路径顺手清理过期项；内存路径按 scope 清理
			if common.RedisEnabled {
				_ = common.RDB.HDel(context.Background(), wecomBotSilenceRedisKey, scope).Err()
			} else {
				if stored, ok := memWecomBotSilences.Load(scope); ok {
					if current, ok := stored.(wecomBotSilenceRecord); ok && current.Until == record.Until {
						memWecomBotSilences.Delete(scope)
					}
				}
			}
			continue
		}
		silences = append(silences, WecomBotAlertSilence{
			Scope:  scope,
			Reason: record.Reason,
			By:     record.By,
			Until:  time.Unix(record.Until, 0),
		})
	}
	sort.Slice(silences, func(i, j int) bool {
		return silences[i].Until.Before(silences[j].Until)
	})
	return silences
}

// isWecomBotAlertSilenced 判断告警是否被静默：scope 命中或全局（"all"）静默即生效。
// Redis 异常时放行告警，与冷却闸门一致，宁发不漏。
func isWecomBotAlertSilenced(scope string) bool {
	scopes := []string{wecomBotSilenceAll}
	if scope != "" && scope != wecomBotSilenceAll {
		scopes = append(scopes, scope)
	}
	now := time.Now().Unix()
	for _, check := range scopes {
		if common.RedisEnabled {
			encoded, err := common.RDB.HGet(context.Background(), wecomBotSilenceRedisKey, check).Result()
			if err != nil {
				continue
			}
			var record wecomBotSilenceRecord
			if common.UnmarshalJsonStr(encoded, &record) == nil && record.Until > now {
				return true
			}
			continue
		}
		if value, ok := memWecomBotSilences.Load(check); ok {
			if record, ok := value.(wecomBotSilenceRecord); ok && record.Until > now {
				return true
			}
		}
	}
	return false
}

func normalizeWecomBotSilenceScope(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == wecomBotSilenceAll {
		return scope
	}
	if channelID, err := strconv.Atoi(strings.TrimPrefix(scope, "channel:")); err == nil && channelID > 0 && scope == fmt.Sprintf("channel:%d", channelID) {
		return scope
	}
	return ""
}

var (
	wecomBotAlertGates    sync.Map
	wecomBotGateCleanOnce sync.Once
)

// wecomBotGatePrefix / wecomBotCountPrefix 闸门与抑制计数的 Redis 键前缀。
const (
	wecomBotGatePrefix  = "wecom_bot_alert:gate:"
	wecomBotCountPrefix = "wecom_bot_alert:count:"
)

// memAlertGate 内存降级用的告警闸门：until 为冷却截止，count 为窗口内抑制计数。
type memAlertGate struct {
	mu    sync.Mutex
	until time.Time
	count int
}

// admitWecomBotAlert 冷却闸门 + 抑制计数（digest 降噪模式）：同一 key 首次告警立即
// 放行；冷却窗口内的重复告警只计数，窗口结束后的下一次告警放行并携带累计抑制次数，
// 让消息里能体现"窗口内共发生 N 次"而不连环轰炸。Redis 异常时放行，宁重不漏；
// 多节点部署下通过 Redis 共享闸门。
func admitWecomBotAlert(key string, cooldown time.Duration) (admitted bool, suppressed int) {
	if common.RedisEnabled {
		return admitRedisWecomBotAlert(key, cooldown)
	}
	return admitMemoryWecomBotAlert(key, cooldown)
}

func admitRedisWecomBotAlert(key string, cooldown time.Duration) (bool, int) {
	ctx := context.Background()
	gated, err := common.RDB.SetNX(ctx, wecomBotGatePrefix+key, "1", cooldown).Result()
	if err != nil {
		common.SysLog(fmt.Sprintf("failed to check wecom bot alert gate: %s", err.Error()))
		return true, 0
	}
	if !gated {
		if err := common.RDB.Incr(ctx, wecomBotCountPrefix+key).Err(); err == nil {
			// 计数 TTL 比闸门略长，保证冷却结束后放行的那次仍能读到累计值
			_ = common.RDB.Expire(ctx, wecomBotCountPrefix+key, cooldown+time.Hour).Err()
		}
		return false, 0
	}
	count, err := common.RDB.Get(ctx, wecomBotCountPrefix+key).Result()
	if err != nil {
		return true, 0
	}
	_ = common.RDB.Del(ctx, wecomBotCountPrefix+key).Err()
	suppressed, err := strconv.Atoi(count)
	if err != nil || suppressed < 0 {
		return true, 0
	}
	return true, suppressed
}

func admitMemoryWecomBotAlert(key string, cooldown time.Duration) (bool, int) {
	wecomBotGateCleanOnce.Do(startWecomBotGateCleanup)
	value, _ := wecomBotAlertGates.LoadOrStore(key, &memAlertGate{})
	gate := value.(*memAlertGate)
	gate.mu.Lock()
	defer gate.mu.Unlock()
	now := time.Now()
	if now.Before(gate.until) {
		gate.count++
		return false, 0
	}
	suppressed := gate.count
	gate.until = now.Add(cooldown)
	gate.count = 0
	return true, suppressed
}

// startWecomBotGateCleanup 每小时清理已过期且无累计计数的内存闸门。
func startWecomBotGateCleanup() {
	gopool.Go(func() {
		for {
			time.Sleep(time.Hour)
			now := time.Now()
			wecomBotAlertGates.Range(func(key, value any) bool {
				gate := value.(*memAlertGate)
				gate.mu.Lock()
				expired := now.After(gate.until) && gate.count == 0
				gate.mu.Unlock()
				if expired {
					wecomBotAlertGates.Delete(key)
				}
				return true
			})
		}
	})
}

// notifyWecomBotChannelError 推送渠道转发错误告警，同一「渠道+状态码」冷却窗口内
// 合并为计数摘要（窗口后首次放行时携带累计次数）。
func notifyWecomBotChannelError(info wecomBotErrorAlertInfo) {
	setting := operation_setting.GetWecomBotSetting()
	if !setting.Enabled || !setting.ErrorNotifyEnabled || !setting.HasEnabledBot() {
		return
	}
	slotKey := fmt.Sprintf("wecom_bot_alert:error:%d:%d", info.ChannelId, info.StatusCode)
	admitted, suppressed := admitWecomBotAlert(slotKey, time.Duration(setting.ErrorCooldownMinutes)*time.Minute)
	if !admitted {
		return
	}
	lines := []string{
		fmt.Sprintf("渠道：%s（#%d）", info.ChannelName, info.ChannelId),
		fmt.Sprintf("状态码：%d", info.StatusCode),
	}
	if info.ErrorCode != "" {
		lines = append(lines, fmt.Sprintf("错误码：%s", info.ErrorCode))
	}
	if info.ModelName != "" {
		lines = append(lines, fmt.Sprintf("模型：%s", info.ModelName))
	}
	if info.Attempt > 0 {
		lines = append(lines, fmt.Sprintf("重试序号：%d", info.Attempt))
	}
	if info.Message != "" {
		lines = append(lines, fmt.Sprintf("错误信息：%s", truncateWecomBotMessage(info.Message)))
	}
	if suppressed > 0 {
		lines = append(lines, fmt.Sprintf("上轮冷却窗口内同类错误累计：%d 次", suppressed))
	}
	lines = append(lines, fmt.Sprintf("时间：%s", time.Now().Format("2006-01-02 15:04:05")))
	enqueueWecomBotAlert(wecomBotAlert{
		title:    "渠道异常告警",
		color:    "warning",
		lines:    lines,
		category: operation_setting.WecomBotCategoryChannelError,
		scope:    fmt.Sprintf("channel:%d", info.ChannelId),
	})
}

// notifyWecomBotChannelDisabled 推送渠道自动禁用告警，仅在状态真正变更时被调用。
func notifyWecomBotChannelDisabled(channelName string, channelId int, reason string) {
	setting := operation_setting.GetWecomBotSetting()
	if !setting.Enabled || !setting.DisableNotifyEnabled || !setting.HasEnabledBot() {
		return
	}
	lines := []string{
		fmt.Sprintf("渠道：%s（#%d）", channelName, channelId),
	}
	if reason != "" {
		lines = append(lines, fmt.Sprintf("原因：%s", truncateWecomBotMessage(reason)))
	}
	lines = append(lines, fmt.Sprintf("时间：%s", time.Now().Format("2006-01-02 15:04:05")))
	enqueueWecomBotAlert(wecomBotAlert{
		title:    "渠道已被自动禁用",
		color:    "warning",
		lines:    lines,
		category: operation_setting.WecomBotCategoryChannelDisabled,
		scope:    fmt.Sprintf("channel:%d", channelId),
	})
}

// notifyWecomBotChannelEnabled 推送渠道恢复启用告警。
func notifyWecomBotChannelEnabled(channelName string, channelId int) {
	setting := operation_setting.GetWecomBotSetting()
	if !setting.Enabled || !setting.EnableNotifyEnabled || !setting.HasEnabledBot() {
		return
	}
	lines := []string{
		fmt.Sprintf("渠道：%s（#%d）", channelName, channelId),
		fmt.Sprintf("时间：%s", time.Now().Format("2006-01-02 15:04:05")),
	}
	enqueueWecomBotAlert(wecomBotAlert{
		title:    "渠道已恢复启用",
		color:    "info",
		lines:    lines,
		category: operation_setting.WecomBotCategoryChannelEnabled,
		scope:    fmt.Sprintf("channel:%d", channelId),
	})
}

// SendWecomBotTest 向单台机器人发送连通性测试消息，供管理页验证 Webhook 配置。
func SendWecomBotTest(bot operation_setting.WecomBotConfig) error {
	alert := wecomBotAlert{
		title: "测试消息",
		color: "info",
		lines: []string{
			fmt.Sprintf("机器人：%s", bot.Name),
			fmt.Sprintf("时间：%s", time.Now().Format("2006-01-02 15:04:05")),
		},
	}
	return sendWecomBotAlertToBot(alert, bot)
}

// sendWecomBotAlert 把一条告警发送到订阅了该类型的机器人；未订阅的机器人跳过，
// 没有任何机器人订阅该类型属合法状态（返回 nil），便于按需分流。
func sendWecomBotAlert(alert wecomBotAlert) error {
	setting := operation_setting.GetWecomBotSetting()
	sent := 0
	enabled := 0
	var failed []string
	for _, bot := range setting.Bots {
		if !bot.Enabled || bot.WebhookApiKey == "" {
			continue
		}
		enabled++
		if !wecomBotSubscribed(bot, alert.category) {
			continue
		}
		if err := sendWecomBotAlertToBot(alert, bot); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", bot.Name, err))
			continue
		}
		sent++
	}
	if len(failed) > 0 {
		return fmt.Errorf("wecom bot send failures: %s", strings.Join(failed, "; "))
	}
	if enabled == 0 {
		return fmt.Errorf("no enabled wecom bot")
	}
	return nil
}

// wecomBotSubscribed 判断机器人是否订阅了指定告警类型；未配置订阅 = 接收全部。
func wecomBotSubscribed(bot operation_setting.WecomBotConfig, category string) bool {
	if len(bot.Subscriptions) == 0 {
		return true
	}
	return slices.Contains(bot.Subscriptions, category)
}

func sendWecomBotAlertToBot(alert wecomBotAlert, bot operation_setting.WecomBotConfig) error {
	mentions := operation_setting.ParseWecomMentionUserids(bot.MentionUserids)
	payload, err := buildWecomBotPayload(alert, mentions, bot.MessageStyle)
	if err != nil {
		return err
	}
	webhookURL := wecomBotWebhookURL(bot.WebhookApiKey)

	if system_setting.EnableWorker() {
		workerReq := &WorkerRequest{
			URL:    webhookURL,
			Key:    system_setting.WorkerValidKey,
			Method: http.MethodPost,
			Headers: map[string]string{
				"Content-Type": "application/json; charset=utf-8",
				"User-Agent":   "NewAPI-WecomBot-Notify/1.0",
			},
			Body: payload,
		}
		resp, err := DoWorkerRequest(workerReq)
		if err != nil {
			return fmt.Errorf("failed to send wecom bot request through worker: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("wecom bot request failed with status code: %d", resp.StatusCode)
		}
		return nil
	}

	if err := ValidateSSRFProtectedFetchURL(webhookURL); err != nil {
		return fmt.Errorf("request reject: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create wecom bot request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "NewAPI-WecomBot-Notify/1.0")
	resp, err := GetSSRFProtectedHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("failed to send wecom bot request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("wecom bot request failed with status code: %d", resp.StatusCode)
	}
	// 企微接口以 HTTP 200 + errcode 表达失败，必须解析应答体。
	var result wecomBotWebResponse
	if err := common.DecodeJson(resp.Body, &result); err != nil {
		return fmt.Errorf("failed to decode wecom bot response: %v", err)
	}
	if result.ErrCode != 0 {
		return fmt.Errorf("wecom bot api error %d: %s", result.ErrCode, result.ErrMsg)
	}
	return nil
}

type wecomBotWebResponse struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

type wecomBotTextBody struct {
	Content             string   `json:"content"`
	MentionedList       []string `json:"mentioned_list,omitempty"`
	MentionedMobileList []string `json:"mentioned_mobile_list,omitempty"`
}

type wecomBotTextPayload struct {
	MsgType string           `json:"msgtype"`
	Text    wecomBotTextBody `json:"text"`
}

type wecomBotMarkdownPayload struct {
	MsgType  string `json:"msgtype"`
	Markdown struct {
		Content string `json:"content"`
	} `json:"markdown"`
}

func buildWecomBotPayload(alert wecomBotAlert, mentions []string, style string) ([]byte, error) {
	// @all 只有 text 消息的 mentioned_list 官方支持。
	if slices.Contains(mentions, "@all") {
		payload := wecomBotTextPayload{MsgType: "text"}
		payload.Text.Content = renderWecomBotText(alert)
		payload.Text.MentionedList = []string{"@all"}
		return common.Marshal(payload)
	}
	// 模板卡片不支持 @ 提醒，配置了具体 userid 提醒时退回 markdown。
	if operation_setting.NormalizeWecomBotMessageStyle(style) == operation_setting.WecomBotStyleCard && len(mentions) == 0 {
		return buildWecomBotCardPayload(alert)
	}
	payload := wecomBotMarkdownPayload{MsgType: "markdown"}
	payload.Markdown.Content = renderWecomBotMarkdown(alert)
	if len(mentions) > 0 {
		var mentionBuilder strings.Builder
		for _, userid := range mentions {
			fmt.Fprintf(&mentionBuilder, " <@%s>", userid)
		}
		payload.Markdown.Content += "\n" + strings.TrimSpace(mentionBuilder.String())
	}
	return common.Marshal(payload)
}

// 企微模板卡片（text_notice）各字段的官方长度限制。
const (
	wecomBotCardMaxRows          = 6
	wecomBotCardKeyMaxRunes      = 10
	wecomBotCardValueMaxRunes    = 27
	wecomBotCardTitleMaxRunes    = 26
	wecomBotCardSubtitleMaxRunes = 100
)

type wecomBotCardRow struct {
	KeyName string `json:"keyname"`
	Value   string `json:"value"`
}

type wecomBotCardPayload struct {
	MsgType      string `json:"msgtype"`
	TemplateCard struct {
		CardType  string `json:"card_type"`
		MainTitle struct {
			Title string `json:"title"`
		} `json:"main_title"`
		SubTitleText          string            `json:"sub_title_text,omitempty"`
		HorizontalContentList []wecomBotCardRow `json:"horizontal_content_list,omitempty"`
		// card_action 为企微必填字段：type 1 跳转链接，点击卡片时打开。
		CardAction struct {
			Type int    `json:"type"`
			URL  string `json:"url"`
		} `json:"card_action"`
	} `json:"template_card"`
}

// wecomBotCardActionURL 返回模板卡片整卡跳转地址：优先平台站点地址，
// 未配置时回退企微官网，保证 card_action 始终合法。
func wecomBotCardActionURL() string {
	if server := strings.TrimSpace(system_setting.ServerAddress); server != "" {
		return server
	}
	return "https://work.weixin.qq.com/"
}

// buildWecomBotCardPayload 把告警渲染成 text_notice 模板卡片：
// "键：值" 行进 horizontal_content_list（超出 6 行的部分并入副标题），
// 无法按冒号拆分的行同样并入副标题。
func buildWecomBotCardPayload(alert wecomBotAlert) ([]byte, error) {
	payload := wecomBotCardPayload{MsgType: "template_card"}
	payload.TemplateCard.CardType = "text_notice"
	payload.TemplateCard.MainTitle.Title = truncateRunesWecomBot(alert.title, wecomBotCardTitleMaxRunes)
	payload.TemplateCard.CardAction.Type = 1
	payload.TemplateCard.CardAction.URL = wecomBotCardActionURL()
	rows := make([]wecomBotCardRow, 0, wecomBotCardMaxRows)
	var overflow []string
	for _, line := range alert.lines {
		if key, value, found := strings.Cut(line, "："); found {
			if len(rows) < wecomBotCardMaxRows {
				rows = append(rows, wecomBotCardRow{
					KeyName: truncateRunesWecomBot(key, wecomBotCardKeyMaxRunes),
					Value:   truncateRunesWecomBot(value, wecomBotCardValueMaxRunes),
				})
				continue
			}
		}
		overflow = append(overflow, line)
	}
	payload.TemplateCard.HorizontalContentList = rows
	if len(overflow) > 0 {
		payload.TemplateCard.SubTitleText = truncateRunesWecomBot(strings.Join(overflow, "\n"), wecomBotCardSubtitleMaxRunes)
	}
	return common.Marshal(payload)
}

func truncateRunesWecomBot(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
}

func renderWecomBotMarkdown(alert wecomBotAlert) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "**<font color=\"%s\">%s</font>**\n", alert.color, alert.title)
	for _, line := range alert.lines {
		fmt.Fprintf(&builder, "> %s\n", line)
	}
	return truncateWecomBotContent(strings.TrimSuffix(builder.String(), "\n"))
}

func renderWecomBotText(alert wecomBotAlert) string {
	lines := append([]string{alert.title}, alert.lines...)
	return truncateWecomBotContent(strings.Join(lines, "\n"))
}

func truncateWecomBotContent(content string) string {
	if len(content) <= wecomBotMaxContentBytes {
		return content
	}
	cut := wecomBotMaxContentBytes
	for cut > 0 && !utf8.RuneStart(content[cut]) {
		cut--
	}
	return content[:cut] + "…"
}

func truncateWecomBotMessage(message string) string {
	runes := []rune(message)
	if len(runes) <= wecomBotMaxMessageRunes {
		return message
	}
	return string(runes[:wecomBotMaxMessageRunes]) + "…"
}

// wecomBotWebhookURL 存储值以 http 开头时视为完整 Webhook 地址（供测试与自定义代理），
// 否则视为裸机器人 Key 拼接官方地址。
func wecomBotWebhookURL(stored string) string {
	trimmed := strings.TrimSpace(stored)
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return trimmed
	}
	return "https://" + operation_setting.WecomBotWebhookHost + "/cgi-bin/webhook/send?key=" + url.QueryEscape(trimmed)
}
