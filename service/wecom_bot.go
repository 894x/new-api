package service

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"slices"
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
	title string
	color string
	lines []string
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
		if wait := wecomBotSendInterval - time.Since(lastSend); wait > 0 {
			time.Sleep(wait)
		}
		lastSend = time.Now()
		if err := sendWecomBotAlert(alert); err != nil {
			common.SysLog(fmt.Sprintf("failed to send wecom bot alert: %s", err.Error()))
		}
	}
}

var (
	wecomBotAlertSlots    sync.Map
	wecomBotSlotCleanOnce sync.Once
)

// acquireWecomBotAlertSlot 在冷却窗口内对同一 key 只放行一次。Redis 异常时放行，
// 宁可偶发重复告警也不漏报；多节点部署下通过 Redis 共享去重。
func acquireWecomBotAlertSlot(key string, cooldown time.Duration) bool {
	if common.RedisEnabled {
		return acquireRedisWecomBotAlertSlot(key, cooldown)
	}
	return acquireMemoryWecomBotAlertSlot(key, cooldown)
}

func acquireRedisWecomBotAlertSlot(key string, cooldown time.Duration) bool {
	existing, err := common.RedisGet(key)
	if err != nil && err.Error() != "redis: nil" {
		common.SysLog(fmt.Sprintf("failed to check wecom bot alert slot: %s", err.Error()))
		return true
	}
	if existing != "" {
		return false
	}
	if err := common.RedisSet(key, "1", cooldown); err != nil {
		common.SysLog(fmt.Sprintf("failed to mark wecom bot alert slot: %s", err.Error()))
	}
	return true
}

func acquireMemoryWecomBotAlertSlot(key string, cooldown time.Duration) bool {
	wecomBotSlotCleanOnce.Do(startWecomBotSlotCleanup)
	now := time.Now()
	if value, ok := wecomBotAlertSlots.Load(key); ok {
		if until, ok := value.(time.Time); ok && now.Before(until) {
			return false
		}
	}
	wecomBotAlertSlots.Store(key, now.Add(cooldown))
	return true
}

func startWecomBotSlotCleanup() {
	gopool.Go(func() {
		for {
			time.Sleep(time.Hour)
			now := time.Now()
			wecomBotAlertSlots.Range(func(key, value any) bool {
				if until, ok := value.(time.Time); ok && now.After(until) {
					wecomBotAlertSlots.Delete(key)
				}
				return true
			})
		}
	})
}

// notifyWecomBotChannelError 推送渠道转发错误告警，同一「渠道+状态码」在冷却窗口内只提醒一次。
func notifyWecomBotChannelError(info wecomBotErrorAlertInfo) {
	setting := operation_setting.GetWecomBotSetting()
	if !setting.Enabled || !setting.ErrorNotifyEnabled || !setting.HasEnabledBot() {
		return
	}
	slotKey := fmt.Sprintf("wecom_bot_alert:error:%d:%d", info.ChannelId, info.StatusCode)
	if !acquireWecomBotAlertSlot(slotKey, time.Duration(setting.ErrorCooldownMinutes)*time.Minute) {
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
	lines = append(lines, fmt.Sprintf("时间：%s", time.Now().Format("2006-01-02 15:04:05")))
	enqueueWecomBotAlert(wecomBotAlert{title: "渠道异常告警", color: "warning", lines: lines})
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
	enqueueWecomBotAlert(wecomBotAlert{title: "渠道已被自动禁用", color: "warning", lines: lines})
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
	enqueueWecomBotAlert(wecomBotAlert{title: "渠道已恢复启用", color: "info", lines: lines})
}

// sendWecomBotAlert 把一条告警广播到所有启用的机器人；全部失败或无可用机器人时返回错误。
func sendWecomBotAlert(alert wecomBotAlert) error {
	setting := operation_setting.GetWecomBotSetting()
	sent := 0
	var failed []string
	for _, bot := range setting.Bots {
		if !bot.Enabled || bot.WebhookApiKey == "" {
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
	if sent == 0 {
		return fmt.Errorf("no enabled wecom bot")
	}
	return nil
}

func sendWecomBotAlertToBot(alert wecomBotAlert, bot operation_setting.WecomBotConfig) error {
	payload, err := buildWecomBotPayload(alert, operation_setting.ParseWecomMentionUserids(bot.MentionUserids))
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

func buildWecomBotPayload(alert wecomBotAlert, mentions []string) ([]byte, error) {
	// @all 只有 text 消息的 mentioned_list 官方支持。
	if slices.Contains(mentions, "@all") {
		payload := wecomBotTextPayload{MsgType: "text"}
		payload.Text.Content = renderWecomBotText(alert)
		payload.Text.MentionedList = []string{"@all"}
		return common.Marshal(payload)
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
