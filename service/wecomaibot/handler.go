package wecomaibot

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// agentTimeout 为整轮查询预算：企微要求流式回复 10 分钟内 finish，
// 预留收尾余量后在到期前主动结题。
const agentTimeout = 8 * time.Minute

// agentConcurrencyLimit 全局同时处理的对话数上限，防止多用户并发时
// FC 循环与 LLM 调用占满 CPU/内存（每用户串行之外的总闸）。
const agentConcurrencyLimit = 8

var agentSlots = make(chan struct{}, agentConcurrencyLimit)

var (
	busyMu    sync.Mutex
	busyUsers = make(map[string]struct{})
)

// handleMessage 处理一条用户消息：排重 → 群聊开关 → 权限门禁 → 提取文本 →
// 每用户串行 → FC 查询循环 → 流式回复。企微限制同一用户与一个机器人最多
// 3 条并发交互，这里收紧为逐条串行。
func handleMessage(w *wsConn, frame *inFrame) {
	var chat msgCallback
	if err := common.Unmarshal(frame.Body, &chat); err != nil {
		return
	}
	if !acquireMsgID(chat.MsgID) {
		return
	}
	setting := operation_setting.GetWecomAIBotSetting()
	replier := newStreamReplier(w, frame.Headers.ReqID)

	if chat.ChatType == chatTypeGroup && !setting.GroupEnabled {
		return
	}
	if !isUserAllowed(setting, chat.From.UserID) {
		// userid 可能是密文 open_userid，记录原值方便管理员复制进白名单
		common.SysLog(fmt.Sprintf("wecom ai bot rejected unauthorized user: userid=%s chattype=%s", chat.From.UserID, chat.ChatType))
		replier.finish("未授权：该账号暂无使用本机器人的权限，请联系管理员将其加入白名单。")
		return
	}
	text, supported := extractUserText(&chat)
	if !supported {
		replier.finish("暂时只支持文字和语音消息，请用文字提问。")
		return
	}
	if strings.TrimSpace(text) == "" {
		// 群聊里只 @ 机器人没有内容，无需回复
		return
	}
	key := sessionKey(&chat)
	if !tryBeginUser(key) {
		replier.finish("正在处理上一条消息，请稍候再发。")
		return
	}
	defer endUser(key)

	// 全局并发闸：防止不同用户同时涌入拖垮 LLM 调用与渲染
	select {
	case agentSlots <- struct{}{}:
		defer func() { <-agentSlots }()
	default:
		replier.finish("当前查询排队已满，请稍后再试。")
		return
	}

	started := time.Now()
	replier.progress("收到，正在查询…")

	// 记录会话目标（供每日日报主动推送）并按管理员名单过滤工具（最小权限）
	recordChatTarget(&chat)
	tools := currentTools()
	if !isAdminUser(setting, chat.From.UserID) {
		tools = filterAdminTools(tools)
	}

	// 图表收集器经 context 传给工具：图片不进模型，直接附在最终回复帧
	collector := &chartCollector{}
	ctx, cancel := context.WithTimeout(
		context.WithValue(context.Background(), chartCollectorKey, collector), agentTimeout)
	defer cancel()
	history := loadSession(key)
	answer, turns, err := runAgentLoop(ctx, tools, history, text, replier.progress)
	if err != nil {
		common.SysLog("wecom ai bot agent error: " + err.Error())
		replier.finish("查询失败：" + truncateStreamContent(err.Error()))
		return
	}
	saveSession(key, turns, time.Duration(setting.SessionTTLMinutes)*time.Minute, setting.SessionRounds)
	images := collector.drain()
	common.SysLog(fmt.Sprintf("wecom ai bot answered: userid=%s chattype=%s charts=%d elapsed=%dms",
		chat.From.UserID, chat.ChatType, len(images), time.Since(started).Milliseconds()))
	replier.finishWithImages(answer, images)
}

// handleEvent 处理事件回调：进会话欢迎语；被另一条连接顶替时立即停手
// （企微单机器人单连接，重连会形成互踢循环）。
func handleEvent(cl *client, w *wsConn, frame *inFrame) {
	var ev eventCallback
	if err := common.Unmarshal(frame.Body, &ev); err != nil {
		return
	}
	switch ev.Event.EventType {
	case eventEnterChat:
		if text := strings.TrimSpace(operation_setting.GetWecomAIBotSetting().WelcomeText); text != "" {
			// 欢迎语要求收到事件后 5 秒内回复
			welcome := newTextFrame(cmdRespondWelcome, frame.Headers.ReqID, text)
			if _, err := w.sendRequest(welcome, ackWait); err != nil {
				common.SysLog("wecom ai bot welcome reply failed: " + err.Error())
			}
		}
	case eventDisconnected:
		common.SysLog("wecom ai bot connection taken over by another client, stop reconnecting")
		cl.kicked.Store(true)
		w.close()
	}
}

func isUserAllowed(setting *operation_setting.WecomAIBotSetting, userid string) bool {
	if setting.AllowAllUsers {
		return true
	}
	return slices.Contains(operation_setting.ParseWecomAIBotUserWhitelist(setting.UserWhitelist), userid)
}

func isAdminUser(setting *operation_setting.WecomAIBotSetting, userid string) bool {
	return slices.Contains(operation_setting.ParseWecomAIBotAdminUserids(setting.AdminUserIDs), userid)
}

// filterAdminTools 非管理员只保留公开工具（最小权限：管理员工具含用户隐私
// 数据或会真实触达上游的调用）。
func filterAdminTools(tools []Tool) []Tool {
	public := make([]Tool, 0, len(tools))
	for _, tool := range tools {
		if !tool.AdminOnly {
			public = append(public, tool)
		}
	}
	return public
}

func tryBeginUser(key string) bool {
	busyMu.Lock()
	defer busyMu.Unlock()
	if _, busy := busyUsers[key]; busy {
		return false
	}
	busyUsers[key] = struct{}{}
	return true
}

func endUser(key string) {
	busyMu.Lock()
	defer busyMu.Unlock()
	delete(busyUsers, key)
}

// extractUserText 从回调中提取用户输入文本：群聊文本剥离 @机器人名 前缀；
// 语音回调已由企微转成文本；引用消息拼进输入帮助模型理解上下文。
func extractUserText(chat *msgCallback) (string, bool) {
	var builder strings.Builder
	switch chat.MsgType {
	case "text":
		if chat.Text == nil {
			return "", false
		}
		builder.WriteString(chat.Text.Content)
	case "voice":
		if chat.Voice == nil {
			return "", false
		}
		builder.WriteString(chat.Voice.Content)
	case "mixed":
		if chat.Mixed == nil {
			return "", false
		}
		for _, item := range chat.Mixed.MsgItem {
			if item.Text != nil {
				builder.WriteString(item.Text.Content)
			}
		}
	default:
		return "", false
	}
	text := strings.TrimSpace(builder.String())
	if chat.ChatType == chatTypeGroup && strings.HasPrefix(text, "@") {
		if idx := strings.IndexAny(text, " \t\u00A0"); idx > 0 {
			text = strings.TrimSpace(text[idx:])
		} else {
			text = ""
		}
	}
	if chat.Quote != nil && chat.Quote.Text != nil {
		if quoted := strings.TrimSpace(chat.Quote.Text.Content); quoted != "" {
			if text == "" {
				text = "（引用消息：" + quoted + "）"
			} else {
				text += "\n（引用消息：" + quoted + "）"
			}
		}
	}
	return text, true
}
