package wecomaibot

import (
	"bytes"
	"context"
	pngformat "image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/glebarez/sqlite"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func withAIBotSetting(t *testing.T, mutate func(*operation_setting.WecomAIBotSetting)) {
	t.Helper()
	setting := operation_setting.GetWecomAIBotSetting()
	original := *setting
	mutate(setting)
	t.Cleanup(func() { *setting = original })
}

func withoutRedis(t *testing.T) {
	t.Helper()
	original := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = original })
}

func TestNewStreamFrameJSON(t *testing.T) {
	frame := newStreamFrame("req-cb-1", "stream-1", "正在查询", false)
	data, err := common.Marshal(frame)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, common.Unmarshal(data, &decoded))
	assert.Equal(t, "aibot_respond_msg", decoded["cmd"])
	headers, ok := decoded["headers"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "req-cb-1", headers["req_id"])
	body, ok := decoded["body"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "stream", body["msgtype"])
	stream, ok := body["stream"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "stream-1", stream["id"])
	assert.Equal(t, false, stream["finish"])
	assert.Equal(t, "正在查询", stream["content"])

	welcome := newTextFrame(cmdRespondWelcome, "req-evt-1", "你好，我是运维助手")
	data, err = common.Marshal(welcome)
	require.NoError(t, err)
	decoded = nil
	require.NoError(t, common.Unmarshal(data, &decoded))
	assert.Equal(t, "aibot_respond_welcome_msg", decoded["cmd"])
	body, ok = decoded["body"].(map[string]any)
	require.True(t, ok)
	text, ok := body["text"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "你好，我是运维助手", text["content"])
}

func TestTruncateStreamContent(t *testing.T) {
	short := "余额 5 美元"
	assert.Equal(t, short, truncateStreamContent(short))

	long := strings.Repeat("中", 15000) // 45000 字节，超过单帧上限
	truncated := truncateStreamContent(long)
	assert.LessOrEqual(t, len(truncated), maxStreamContentBytes+utf8.RuneLen('…'))
	assert.True(t, utf8.ValidString(truncated))
	assert.True(t, strings.HasSuffix(truncated, "…"))
	assert.Greater(t, len(truncated), maxStreamContentBytes-utf8.UTFMax)
}

func TestExtractUserText(t *testing.T) {
	tests := []struct {
		name      string
		chat      msgCallback
		wantText  string
		supported bool
	}{
		{
			name:      "单聊文本",
			chat:      msgCallback{ChatType: chatTypeSingle, MsgType: "text", Text: &textContent{Content: "查一下渠道余额"}},
			wantText:  "查一下渠道余额",
			supported: true,
		},
		{
			name:      "单聊语音已转文本",
			chat:      msgCallback{ChatType: chatTypeSingle, MsgType: "voice", Voice: &textContent{Content: "渠道状态"}},
			wantText:  "渠道状态",
			supported: true,
		},
		{
			name: "群聊剥离@机器人前缀",
			chat: msgCallback{ChatType: chatTypeGroup, MsgType: "text",
				Text: &textContent{Content: "@运维机器人 帮我查渠道余额"}},
			wantText:  "帮我查渠道余额",
			supported: true,
		},
		{
			name: "群聊仅@机器人",
			chat: msgCallback{ChatType: chatTypeGroup, MsgType: "text",
				Text: &textContent{Content: "@运维机器人"}},
			wantText:  "",
			supported: true,
		},
		{
			name: "图文混排拼接文本",
			chat: msgCallback{ChatType: chatTypeSingle, MsgType: "mixed", Mixed: &mixedContent{MsgItem: []mixedItem{
				{MsgType: "text", Text: &textContent{Content: "第一条"}},
				{MsgType: "text", Text: &textContent{Content: "第二条"}},
			}}},
			wantText:  "第一条第二条",
			supported: true,
		},
		{
			name: "引用消息拼进输入",
			chat: msgCallback{ChatType: chatTypeSingle, MsgType: "text",
				Text:  &textContent{Content: "它现在正常吗"},
				Quote: &quoteContent{MsgType: "text", Text: &textContent{Content: "渠道A 余额 5 美元"}}},
			wantText:  "它现在正常吗\n（引用消息：渠道A 余额 5 美元）",
			supported: true,
		},
		{
			name:      "单聊图片暂不支持",
			chat:      msgCallback{ChatType: chatTypeSingle, MsgType: "image"},
			supported: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, supported := extractUserText(&tt.chat)
			assert.Equal(t, tt.supported, supported)
			if tt.supported {
				assert.Equal(t, tt.wantText, text)
			}
		})
	}
}

func TestSessionStoreAndDedup(t *testing.T) {
	withoutRedis(t)

	turns := []chatTurn{
		{Role: "user", Content: "渠道A正常吗"},
		{Role: "assistant", Content: "正常"},
		{Role: "user", Content: "余额呢"},
		{Role: "assistant", Content: "5 美元"},
	}
	key := sessionKeyPrefix + t.Name()
	saveSession(key, turns, 10*time.Minute, 2)
	loaded := loadSession(key)
	// 超过 2 轮只保留最近 2 轮
	require.Len(t, loaded, 4) // 4 条消息 = 2 轮，不应被裁剪
	assert.Equal(t, "余额呢", loaded[2].Content)

	long := make([]chatTurn, 0, 30)
	for range 30 {
		long = append(long, chatTurn{Role: "user", Content: "x"})
	}
	saveSession(key, long, 10*time.Minute, 10)
	trimmed := loadSession(key)
	require.Len(t, trimmed, 20)
	assert.Equal(t, "x", trimmed[0].Content)

	// msgid 排重：同一消息只放行一次
	msgID := "msgid-" + t.Name()
	assert.True(t, acquireMsgID(msgID))
	assert.False(t, acquireMsgID(msgID))
	assert.True(t, acquireMsgID(""))
	// 工具冷却槽
	slot := "slot-" + t.Name()
	assert.True(t, AcquireToolSlot(slot, time.Minute))
	assert.False(t, AcquireToolSlot(slot, time.Minute))
}

func TestIsUserAllowed(t *testing.T) {
	setting := &operation_setting.WecomAIBotSetting{UserWhitelist: "alice, bob"}
	assert.False(t, isUserAllowed(setting, "carol"))
	assert.True(t, isUserAllowed(setting, "alice"))
	setting.AllowAllUsers = true
	assert.True(t, isUserAllowed(setting, "carol"))
}

type stubLLMDriver struct {
	mu     sync.Mutex
	calls  []*ChatRequest
	script []ChatResponse
}

func (s *stubLLMDriver) ChatCompletion(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	copied := *req
	s.calls = append(s.calls, &copied)
	if len(s.calls) > len(s.script) {
		return nil, context.DeadlineExceeded
	}
	resp := s.script[len(s.calls)-1]
	return &resp, nil
}

func TestRunAgentLoopWithToolCalling(t *testing.T) {
	originalDriver := llmDriver
	t.Cleanup(func() { llmDriver = originalDriver })

	echoTool := Tool{
		Name:        "get_answer",
		Description: "测试用查询工具",
		Parameters:  map[string]any{"type": "object"},
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			return map[string]any{"ok": true, "echo": argString(args, "keyword")}, nil
		},
	}
	driver := &stubLLMDriver{script: []ChatResponse{
		{ToolCalls: []ToolCall{{ID: "call_1", Name: "get_answer", Arguments: `{"keyword":"渠道A"}`}}},
		{Content: "渠道A 余额 5 美元"},
	}}
	llmDriver = driver

	var progress []string
	answer, turns, err := runAgentLoop(context.Background(), []Tool{echoTool}, nil, "帮我查渠道A的余额", func(text string) {
		progress = append(progress, text)
	})
	require.NoError(t, err)
	assert.Equal(t, "渠道A 余额 5 美元", answer)
	require.Len(t, progress, 1)
	assert.Equal(t, "正在查询：get_answer（渠道A）", progress[0])

	// 会话更新为 user + assistant 两轮
	require.Len(t, turns, 2)
	assert.Equal(t, "user", turns[0].Role)
	assert.Equal(t, "帮我查渠道A的余额", turns[0].Content)
	assert.Equal(t, "assistant", turns[1].Role)

	// 第二次请求应包含 assistant 工具调用回放与工具结果
	require.Len(t, driver.calls, 2)
	second := driver.calls[1]
	require.Len(t, second.Messages, 4)
	assert.Equal(t, "system", second.Messages[0].Role)
	assert.Equal(t, "user", second.Messages[1].Role)
	assistant := second.Messages[2]
	assert.Equal(t, "assistant", assistant.Role)
	assert.Contains(t, string(assistant.ToolCalls), "get_answer")
	toolMsg := second.Messages[3]
	assert.Equal(t, "tool", toolMsg.Role)
	assert.Equal(t, "call_1", toolMsg.ToolCallID)
	assert.Equal(t, "get_answer", toolMsg.Name)
	assert.Contains(t, toolMsg.Content, `"ok":true`)
	assert.Contains(t, toolMsg.Content, "渠道A")
	require.Len(t, second.Tools, 1)
	assert.Equal(t, "get_answer", second.Tools[0].Name)
}

func TestRunAgentLoopWithoutDriver(t *testing.T) {
	originalDriver := llmDriver
	llmDriver = nil
	t.Cleanup(func() { llmDriver = originalDriver })

	_, _, err := runAgentLoop(context.Background(), nil, nil, "你好", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "对话模型未接入")
}

// newFakeWecomAIBotServer 启动一个假企微长连接服务器：对所有请求帧回执
// errcode=0，把收到的帧原样投递到 captured；onFrame 在每帧回执后回调，
// 供测试主动下发服务端帧（如 disconnected 事件）。
func newFakeWecomAIBotServer(t *testing.T, onFrame func(conn *websocket.Conn, frame *inFrame) error) (url string, captured chan []byte) {
	t.Helper()
	captured = make(chan []byte, 32)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var frame inFrame
			if common.Unmarshal(data, &frame) != nil || frame.Cmd == "" {
				continue
			}
			select {
			case captured <- data:
			default:
			}
			// 上传各步回执需要带 body，其余命令回执保持空 body
			ackPayload := map[string]any{
				"headers": map[string]any{"req_id": frame.Headers.ReqID},
				"errcode": 0,
				"errmsg":  "ok",
			}
			switch frame.Cmd {
			case cmdUploadMediaInit:
				ackPayload["body"] = map[string]any{"upload_id": "upload-test-1"}
			case cmdUploadMediaFinish:
				ackPayload["body"] = map[string]any{"media_id": "media-test-1", "type": "image", "created_at": 1}
			}
			ack, err := common.Marshal(ackPayload)
			if err != nil {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, ack); err != nil {
				return
			}
			if onFrame != nil {
				if err := onFrame(conn, &frame); err != nil {
					return
				}
			}
		}
	}))
	t.Cleanup(server.Close)
	return "ws" + strings.TrimPrefix(server.URL, "http"), captured
}

func dialTestConn(t *testing.T, url string) *wsConn {
	t.Helper()
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	w := &wsConn{ws: ws, sendCh: make(chan []byte, sendQueueSize), done: make(chan struct{})}
	t.Cleanup(w.close)
	go w.writePump()
	// 回执按 req_id 路由依赖读循环，必须随连接启动
	go func() { _ = w.readLoop(func(*inFrame) {}) }()
	return w
}

func TestConnProtocolRoundTrip(t *testing.T) {
	url, captured := newFakeWecomAIBotServer(t, nil)
	w := dialTestConn(t, url)

	// 订阅鉴权：req_id 由本地生成，回执按 req_id 关联
	subscribe := &outFrame{
		Cmd:     cmdSubscribe,
		Headers: frameHeaders{ReqID: newReqID(cmdSubscribe)},
		Body:    &subscribeBody{BotID: "bot-1", Secret: "secret-1"},
	}
	ack, err := w.sendRequest(subscribe, subscribeAckWait)
	require.NoError(t, err)
	assert.Equal(t, 0, ack.ErrCode)

	// 心跳
	ping := &outFrame{Cmd: cmdPing, Headers: frameHeaders{ReqID: newReqID(cmdPing)}}
	_, err = w.sendRequest(ping, ackWait)
	require.NoError(t, err)

	// 流式回复：两帧共用回调 req_id 与 stream.id
	replier := newStreamReplier(w, "req-cb-1")
	replier.progress("正在查询…")
	replier.finish("渠道A 余额 5 美元")

	var frames []inFrame
	deadline := time.After(5 * time.Second)
	for len(frames) < 2 {
		select {
		case data := <-captured:
			var frame inFrame
			require.NoError(t, common.Unmarshal(data, &frame))
			if frame.Cmd == cmdRespondMsg {
				frames = append(frames, frame)
			}
		case <-deadline:
			t.Fatal("fake server did not receive reply frames")
		}
	}
	require.Equal(t, cmdRespondMsg, frames[0].Cmd)
	require.Equal(t, cmdRespondMsg, frames[1].Cmd)
	assert.Equal(t, "req-cb-1", frames[0].Headers.ReqID)
	assert.Equal(t, "req-cb-1", frames[1].Headers.ReqID)

	var first, last streamBody
	require.NoError(t, common.Unmarshal(frames[0].Body, &first))
	require.NoError(t, common.Unmarshal(frames[1].Body, &last))
	assert.False(t, first.Stream.Finish)
	assert.True(t, last.Stream.Finish)
	assert.Equal(t, first.Stream.ID, last.Stream.ID)
	assert.Equal(t, "正在查询…", first.Stream.Content)
	assert.Equal(t, "渠道A 余额 5 美元", last.Stream.Content)
}

func TestClientExitsWhenKicked(t *testing.T) {
	disconnectedEvent, err := common.Marshal(&outFrame{
		Cmd:     cmdEventCallback,
		Headers: frameHeaders{ReqID: newReqID(cmdEventCallback)},
		Body: map[string]any{
			"msgid":    "evt-1",
			"chattype": chatTypeSingle,
			"event":    map[string]any{"eventtype": eventDisconnected},
		},
	})
	require.NoError(t, err)

	// 客户端完成订阅并收到回执后，服务端立即推送 disconnected 事件（模拟新连接顶替）
	url, _ := newFakeWecomAIBotServer(t, func(conn *websocket.Conn, frame *inFrame) error {
		if frame.Cmd != cmdSubscribe {
			return nil
		}
		return conn.WriteMessage(websocket.TextMessage, disconnectedEvent)
	})

	originalURL := wecomAIBotWSURL
	wecomAIBotWSURL = url
	t.Cleanup(func() { wecomAIBotWSURL = originalURL })

	cl := &client{credential: botCredential{BotID: "bot-1", Secret: "secret-1"}}
	exitCh := make(chan clientExit, 1)
	go func() { exitCh <- cl.runOnce(context.Background()) }()
	select {
	case exit := <-exitCh:
		assert.Equal(t, exitKicked, exit)
	case <-time.After(5 * time.Second):
		t.Fatal("client did not exit after disconnected event")
	}
}

func TestChannelToolsQueryRealDatabaseRows(t *testing.T) {
	// 回归：search_channels 空关键字与 get_system_status 曾走
	// GetAllChannels(0, 0, ...)，num=0 被写成 SQL LIMIT 0，恒返回空列表。
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.Channel{}))
	previousDB := model.DB
	model.DB = database
	t.Cleanup(func() {
		model.DB = previousDB
		_ = sqlDB.Close()
	})

	require.NoError(t, database.Create(&model.Channel{
		Id: 1, Name: "alpha", Type: 1, Key: "sk-a",
		Status: common.ChannelStatusEnabled, Models: "gpt-4o", Group: "default",
	}).Error)
	require.NoError(t, database.Create(&model.Channel{
		Id: 2, Name: "beta", Type: 1, Key: "sk-b",
		Status: common.ChannelStatusManuallyDisabled, Models: "glm-5.3", Group: "default",
	}).Error)

	tools := builtinTools()
	search, ok := findTool(tools, "search_channels")
	require.True(t, ok)

	result, err := search.Handler(context.Background(), map[string]any{})
	require.NoError(t, err)
	view := result.(map[string]any)
	assert.Equal(t, 2, view["matched"])
	channels := view["channels"].([]map[string]any)
	require.Len(t, channels, 2)
	for _, channel := range channels {
		assert.NotContains(t, channel, "key")
	}

	result, err = search.Handler(context.Background(), map[string]any{"status": float64(1)})
	require.NoError(t, err)
	assert.Equal(t, 1, result.(map[string]any)["matched"])
	// 关键字路径依赖 model 包内 initCol 的方言列名（仅生产 InitDB 初始化），
	// 跨包测试无法构造，该分支由管理页渠道搜索共用覆盖。

	status, ok := findTool(tools, "get_system_status")
	require.True(t, ok)
	result, err = status.Handler(context.Background(), map[string]any{})
	require.NoError(t, err)
	statusView := result.(map[string]any)
	assert.Equal(t, 2, statusView["channel_total"])
	assert.Equal(t, 1, statusView["channel_enabled"])
	assert.Equal(t, 1, statusView["channel_disabled"])
}

func TestFinishWithImagesSendsMediaReplies(t *testing.T) {
	url, captured := newFakeWecomAIBotServer(t, nil)
	w := dialTestConn(t, url)

	replier := newStreamReplier(w, "req-cb-9")
	png := []byte{0x89, 0x50, 0x4E, 0x47, 1, 2, 3}
	replier.finishWithImages("结论见上图", [][]byte{png})

	// 帧序列：文本最终帧 → 上传 init/chunk/finish → 独立图片消息回复
	deadline := time.After(5 * time.Second)
	var frames []inFrame
	for len(frames) < 5 {
		select {
		case data := <-captured:
			var frame inFrame
			require.NoError(t, common.Unmarshal(data, &frame))
			frames = append(frames, frame)
		case <-deadline:
			t.Fatalf("fake server received only %d frames, want 5", len(frames))
		}
	}

	require.Equal(t, cmdRespondMsg, frames[0].Cmd)
	var body streamBody
	require.NoError(t, common.Unmarshal(frames[0].Body, &body))
	require.True(t, body.Stream.Finish)
	assert.Equal(t, "结论见上图", body.Stream.Content)
	assert.Equal(t, "req-cb-9", frames[0].Headers.ReqID)

	require.Equal(t, cmdUploadMediaInit, frames[1].Cmd)
	require.Equal(t, cmdUploadMediaChunk, frames[2].Cmd)
	require.Equal(t, cmdUploadMediaFinish, frames[3].Cmd)

	require.Equal(t, cmdRespondMsg, frames[4].Cmd)
	var mediaBody mediaReplyBody
	require.NoError(t, common.Unmarshal(frames[4].Body, &mediaBody))
	assert.Equal(t, "image", mediaBody.MsgType)
	assert.Equal(t, "media-test-1", mediaBody.Image.MediaID)
	assert.Equal(t, "req-cb-9", frames[4].Headers.ReqID)
}

func TestBuildChartSpecValidation(t *testing.T) {
	valid := map[string]any{
		"kind":   "line",
		"title":  "成功率趋势",
		"labels": []any{"08:00", "09:00"},
		"series": []any{map[string]any{
			"name": "成功率",
			"data": []any{float64(98.5), float64(99.1)},
		}},
	}
	spec, err := buildChartSpec(valid)
	require.NoError(t, err)
	assert.Equal(t, "line", spec.Kind)
	assert.Equal(t, "成功率趋势", spec.Title)

	for name, mutate := range map[string]func(map[string]any){
		"非法 kind":     func(m map[string]any) { m["kind"] = "pie" },
		"缺 title":     func(m map[string]any) { delete(m, "title") },
		"labels 非字符串": func(m map[string]any) { m["labels"] = []any{float64(1)} },
		"系列数据长度与标签不一致": func(m map[string]any) {
			m["series"] = []any{map[string]any{"name": "a", "data": []any{float64(1)}}}
		},
		"数据非数字": func(m map[string]any) {
			m["series"] = []any{map[string]any{"name": "a", "data": []any{"x", "y"}}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			args := map[string]any{}
			for key, value := range valid {
				args[key] = value
			}
			mutate(args)
			_, err := buildChartSpec(args)
			require.Error(t, err)
		})
	}

	donut := map[string]any{
		"kind": "donut", "title": "占比", "labels": []any{"a"},
		"series": []any{map[string]any{"name": "x", "data": []any{float64(0)}}},
	}
	_, err = buildChartSpec(donut)
	require.Error(t, err)
}

func TestRenderChartToolWithoutCollectorFails(t *testing.T) {
	// 规格合法但上下文没有收集器（如直接调用工具）时必须拒绝，不启动渲染
	_, err := renderChartToolHandler(context.Background(), map[string]any{
		"kind":   "bar",
		"title":  "用量",
		"labels": []any{"a"},
		"series": []any{map[string]any{"name": "x", "data": []any{float64(1)}}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chart output is unavailable")
}

func TestStatsToolsAggregateSeededRows(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.QuotaData{}, &model.PerfMetric{}))
	previousDB := model.DB
	model.DB = database
	t.Cleanup(func() {
		model.DB = previousDB
		_ = sqlDB.Close()
	})

	now := time.Now().Unix()
	require.NoError(t, database.Create(&model.QuotaData{
		ModelName: "gpt-4o", Username: "alice", CreatedAt: now - 60,
		Count: 4, TokenUsed: 800, Quota: int(2 * common.QuotaPerUnit),
	}).Error)
	require.NoError(t, database.Create(&model.QuotaData{
		ModelName: "glm-5.3", Username: "bob", CreatedAt: now - 50,
		Count: 6, TokenUsed: 1200, Quota: int(1 * common.QuotaPerUnit),
	}).Error)

	tools := builtinTools()
	usage, ok := findTool(tools, "get_usage_overview")
	require.True(t, ok)
	result, err := usage.Handler(context.Background(), map[string]any{})
	require.NoError(t, err)
	view := result.(map[string]any)
	byModel := view["by_model"].([]map[string]any)
	require.Len(t, byModel, 2)
	assert.Equal(t, "gpt-4o", byModel[0]["name"])
	assert.Equal(t, int64(4), byModel[0]["requests"])
	assert.Equal(t, float64(2), byModel[0]["usd"])
	byUser := view["by_user"].([]map[string]any)
	assert.Equal(t, "alice", byUser[0]["name"])
	assert.Equal(t, float64(1), byUser[1]["usd"])

	// 性能桶：gpt-4o 10 次请求 8 次成功，平均延迟 2000ms
	require.NoError(t, database.Create(&model.PerfMetric{
		ModelName: "gpt-4o", Group: "default", BucketTs: now - 3600,
		RequestCount: 10, SuccessCount: 8, TotalLatencyMs: 20000,
	}).Error)
	perf, ok := findTool(tools, "get_model_performance")
	require.True(t, ok)
	result, err = perf.Handler(context.Background(), map[string]any{})
	require.NoError(t, err)
	perfView := result.(map[string]any)
	perfModels := perfView["models"].([]map[string]any)
	require.Len(t, perfModels, 1)
	assert.Equal(t, "gpt-4o", perfModels[0]["model"])
	assert.Equal(t, int64(10), perfModels[0]["requests"])
	assert.Equal(t, float64(80), perfModels[0]["success_rate"])
	assert.Equal(t, int64(2000), perfModels[0]["avg_latency_ms"])

	result, err = perf.Handler(context.Background(), map[string]any{"model": "gpt-4o"})
	require.NoError(t, err)
	detail := result.(map[string]any)
	assert.Equal(t, int64(2000), detail["avg_latency_ms"])
	series := detail["series"].([]map[string]any)
	require.Len(t, series, 1)
	assert.Equal(t, float64(80), series[0]["success_rate"])
}

func TestNiceAxis(t *testing.T) {
	tests := []struct {
		name             string
		lo, hi           float64
		wantMin, wantMax float64
		wantStep         float64
	}{
		{"0 到 100", 0, 100, 0, 100, 25},
		{"零上下界取整", -3, 57, -20, 60, 20},
		{"小数区间", 97.1, 99.8, 97, 100, 1},
		{"退化区间上抬", 5, 5, 5, 6, 0.25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			min, max, step := niceAxis(tt.lo, tt.hi)
			assert.Equal(t, tt.wantMin, min)
			assert.Equal(t, tt.wantMax, max)
			assert.Equal(t, tt.wantStep, step)
		})
	}
}

func TestHumanNumber(t *testing.T) {
	assert.Equal(t, "1.2k", humanNumber(1234))
	assert.Equal(t, "1.2M", humanNumber(1234567))
	assert.Equal(t, "1.2B", humanNumber(1234567890))
	assert.Equal(t, "100", humanNumber(100))
	assert.Equal(t, "99.50", humanNumber(99.5))
}

func TestRenderChartPNGAllKinds(t *testing.T) {
	if !chartFontAvailable() {
		t.Skip("当前环境没有中文字体，跳过渲染冒烟")
	}
	specs := map[string]*chartSpec{
		"line": {
			Kind: "line", Title: "glm-5.3 近 24 小时成功率", Subtitle: "按 40 分钟桶聚合 · 数据来自性能指标",
			YLabel: "成功率", Unit: "%",
			Labels: []string{"00:00", "02:00", "04:00", "06:00", "08:00", "10:00", "12:00", "14:00"},
			Series: []chartSeries{{Name: "成功率", Data: []float64{99.2, 98.7, 99.5, 100, 97.8, 99.1, 98.4, 99.6}}},
		},
		"bar": {
			Kind: "bar", Title: "近 24 小时各模型调用量", Unit: "次", YLabel: "请求数",
			Labels: []string{"glm-5.3", "gpt-4o", "claude-4.5", "deepseek-v4", "kimi-k3", "qwen-max"},
			Series: []chartSeries{{Name: "调用量", Data: []float64{18234, 12045, 8210, 5102, 3480, 1520}}},
		},
		"grouped_bar": {
			Kind: "grouped_bar", Title: "各模型请求量与成功请求数对比", Unit: "次",
			Labels: []string{"glm-5.3", "gpt-4o", "claude-4.5"},
			Series: []chartSeries{
				{Name: "总请求", Data: []float64{18234, 12045, 8210}},
				{Name: "成功请求", Data: []float64{18050, 11902, 8010}},
			},
		},
		"donut": {
			Kind: "donut", Title: "近 24 小时各分组流量占比", Unit: "次",
			Labels: []string{"default", "vip", "trial", "internal"},
			Series: []chartSeries{{Name: "请求量", Data: []float64{42000, 18000, 7500, 2500}}},
		},
	}
	previewDir := filepath.Join(os.TempDir(), "wecomaibot-chart-preview")
	for name, spec := range specs {
		t.Run(name, func(t *testing.T) {
			png, err := renderChartPNG(spec)
			require.NoError(t, err)
			require.True(t, bytes.HasPrefix(png, []byte{0x89, 'P', 'N', 'G'}), "应为 PNG 输出")
			assert.Less(t, len(png), 2<<20, "文件体积应 <2MB")
			// 企微展示规范：宽 1000、高 ≤ 宽×9/16、总像素 ≤150 万
			decoded, err := pngformat.Decode(bytes.NewReader(png))
			require.NoError(t, err)
			bounds := decoded.Bounds()
			assert.Equal(t, 1000, bounds.Dx())
			assert.LessOrEqual(t, bounds.Dy(), 562)
			assert.LessOrEqual(t, bounds.Dx()*bounds.Dy(), 1_500_000)
			require.NoError(t, os.MkdirAll(previewDir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(previewDir, name+".png"), png, 0o644))
		})
	}
	t.Logf("样图已写入 %s 供人工查看", previewDir)
}

func TestFilterAdminTools(t *testing.T) {
	tools := []Tool{
		{Name: "search_channels"},
		{Name: "get_usage_overview", AdminOnly: true},
		{Name: "refresh_channel_balance", AdminOnly: true},
	}
	public := filterAdminTools(tools)
	require.Len(t, public, 1)
	assert.Equal(t, "search_channels", public[0].Name)

	setting := &operation_setting.WecomAIBotSetting{AdminUserIDs: "alice, bob"}
	assert.True(t, isAdminUser(setting, "alice"))
	assert.False(t, isAdminUser(setting, "carol"))
}

func TestGatherAndRenderDailyReport(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.QuotaData{}, &model.PerfMetric{}))
	previousDB := model.DB
	model.DB = database
	t.Cleanup(func() {
		model.DB = previousDB
		_ = sqlDB.Close()
	})

	// 报告日与前一日各 24 小时窗口；报告日内再前 86400 恰为前一日
	base := time.Now()
	dayStart := time.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, base.Location()).Add(-24 * time.Hour).Unix()
	seed := []model.QuotaData{
		{ModelName: "glm-5.3", Username: "admin", CreatedAt: dayStart + 3600, Count: 10, TokenUsed: 1000, Quota: int(2 * common.QuotaPerUnit)},
		{ModelName: "kimi-k3", Username: "admin", CreatedAt: dayStart + 7200, Count: 1, TokenUsed: 100, Quota: 1},
		{ModelName: "glm-5.3", Username: "admin", CreatedAt: dayStart - 3600, Count: 20, TokenUsed: 2000, Quota: int(3 * common.QuotaPerUnit)},
	}
	for i := range seed {
		require.NoError(t, database.Create(&seed[i]).Error)
	}
	// 性能：报告日 100 次请求 90 次成功（错误率 10%，≥50 次应产出异常项）
	require.NoError(t, database.Create(&model.PerfMetric{
		ModelName: "glm-5.3", Group: "default", BucketTs: dayStart + 600,
		RequestCount: 100, SuccessCount: 90, TotalLatencyMs: 100000,
	}).Error)

	data, err := gatherDailyReport(dayStart)
	require.NoError(t, err)
	assert.Equal(t, 11, int(data.Requests))
	assert.Equal(t, 20, int(data.PrevRequests))
	assert.InDelta(t, 2.0, data.TotalUSD, 0.001)
	assert.InDelta(t, 3.0, data.PrevUSD, 0.001)
	assert.InDelta(t, 90.0, data.SuccessRate, 0.001)
	require.NotEmpty(t, data.Anomalies)
	assert.Contains(t, data.Anomalies[0], "glm-5.3")
	require.Len(t, data.TopModels, 2)
	assert.Equal(t, "glm-5.3", data.TopModels[0]["name"])

	markdown := renderDailyReportMarkdown(data)
	assert.Contains(t, markdown, "运营日报")
	assert.Contains(t, markdown, "消费 $2.00")
	assert.Contains(t, markdown, "请求 11 次")
	assert.Contains(t, markdown, "异常关注")
}

func TestDailyReportSentIdempotent(t *testing.T) {
	withoutRedis(t)
	today := time.Now().Format("2006-01-02")
	assert.False(t, reportAlreadySent(today))
	markReportSent(today)
	assert.True(t, reportAlreadySent(today))

	// 已发送的当天任务直接跳过，不触达推送链路
	summary, err := RunDailyReportTask(context.Background(), time.Now().Hour())
	require.NoError(t, err)
	assert.Equal(t, "already_sent_today", summary["skipped"])
}

func TestResolveReportTargets(t *testing.T) {
	withoutRedis(t)
	setting := &operation_setting.WecomAIBotSetting{}
	targets := resolveReportTargetsFor(setting, time.Hour)
	assert.Empty(t, targets) // 无配置无管理员且无活跃目标时返回空

	setting.AdminUserIDs = "admin1, admin2"
	targets = resolveReportTargetsFor(setting, time.Hour)
	require.Len(t, targets, 2)
	assert.Equal(t, "admin1", targets[0].ID)
	assert.Equal(t, chatTypeSingle, targets[0].Kind)

	setting.ReportTargets = "group_chat_123, user456"
	targets = resolveReportTargetsFor(setting, time.Hour)
	require.Len(t, targets, 2)
	assert.Equal(t, "group_chat_123", targets[0].ID)
}
