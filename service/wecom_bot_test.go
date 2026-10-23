package service

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func withWecomBotSetting(t *testing.T, mutate func(*operation_setting.WecomBotSetting)) {
	t.Helper()
	setting := operation_setting.GetWecomBotSetting()
	original := *setting
	mutate(setting)
	t.Cleanup(func() { *setting = original })
}

// enableTestWecomBot 为用例构造一台启用的测试机器人，可选追加覆盖其他全局字段。
func enableTestWecomBot(t *testing.T, webhook string, mentions string, mutate ...func(*operation_setting.WecomBotSetting)) {
	t.Helper()
	withWecomBotSetting(t, func(setting *operation_setting.WecomBotSetting) {
		setting.Enabled = true
		setting.Bots = []operation_setting.WecomBotConfig{{
			ID:             "test-bot",
			Name:           "测试机器人",
			WebhookApiKey:  webhook,
			MentionUserids: mentions,
			Enabled:        true,
		}}
		for _, fn := range mutate {
			fn(setting)
		}
	})
}

func captureWecomBotAlerts(t *testing.T) <-chan wecomBotAlert {
	t.Helper()
	captured := make(chan wecomBotAlert, 16)
	original := enqueueWecomBotAlert
	enqueueWecomBotAlert = func(alert wecomBotAlert) { captured <- alert }
	t.Cleanup(func() { enqueueWecomBotAlert = original })
	return captured
}

func waitForWecomBotAlert(t *testing.T, captured <-chan wecomBotAlert) (wecomBotAlert, bool) {
	t.Helper()
	select {
	case alert := <-captured:
		return alert, true
	case <-time.After(2 * time.Second):
		return wecomBotAlert{}, false
	}
}

func assertNoWecomBotAlert(t *testing.T, captured <-chan wecomBotAlert) {
	t.Helper()
	select {
	case alert := <-captured:
		t.Fatalf("unexpected wecom bot alert: %+v", alert)
	case <-time.After(300 * time.Millisecond):
	}
}

func resetWecomBotAlertSlots(t *testing.T) {
	t.Helper()
	wecomBotAlertSlots.Range(func(key, value any) bool {
		wecomBotAlertSlots.Delete(key)
		return true
	})
}

func withWecomBotMemoryCooldown(t *testing.T) {
	t.Helper()
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	resetWecomBotAlertSlots(t)
	t.Cleanup(func() { common.RedisEnabled = previousRedis })
}

type wecomRequestMirror struct {
	MsgType  string `json:"msgtype"`
	Markdown struct {
		Content string `json:"content"`
	} `json:"markdown"`
	Text struct {
		Content       string   `json:"content"`
		MentionedList []string `json:"mentioned_list"`
	} `json:"text"`
}

func TestBuildWecomBotPayloadMarkdown(t *testing.T) {
	alert := wecomBotAlert{
		title: "渠道异常告警",
		color: "warning",
		lines: []string{"渠道：OpenAI-01（#12）", "状态码：500"},
	}
	payload, err := buildWecomBotPayload(alert, nil)
	require.NoError(t, err)
	var mirror wecomRequestMirror
	require.NoError(t, common.Unmarshal(payload, &mirror))
	assert.Equal(t, "markdown", mirror.MsgType)
	assert.Contains(t, mirror.Markdown.Content, "渠道异常告警")
	assert.Contains(t, mirror.Markdown.Content, `<font color="warning">`)
	assert.Contains(t, mirror.Markdown.Content, "> 渠道：OpenAI-01（#12）")
	assert.NotContains(t, mirror.Markdown.Content, "<@")
}

func TestBuildWecomBotPayloadMarkdownWithMentions(t *testing.T) {
	alert := wecomBotAlert{title: "渠道异常告警", color: "warning", lines: []string{"渠道：A（#1）"}}
	payload, err := buildWecomBotPayload(alert, []string{"zhangsan", "lisi"})
	require.NoError(t, err)
	var mirror wecomRequestMirror
	require.NoError(t, common.Unmarshal(payload, &mirror))
	assert.Equal(t, "markdown", mirror.MsgType)
	assert.Contains(t, mirror.Markdown.Content, "\n<@zhangsan> <@lisi>")
	assert.Empty(t, mirror.Text.MentionedList)
}

func TestBuildWecomBotPayloadMentionAllFallsBackToText(t *testing.T) {
	alert := wecomBotAlert{title: "渠道已被自动禁用", color: "warning", lines: []string{"渠道：A（#1）"}}
	payload, err := buildWecomBotPayload(alert, []string{"@all"})
	require.NoError(t, err)
	var mirror wecomRequestMirror
	require.NoError(t, common.Unmarshal(payload, &mirror))
	assert.Equal(t, "text", mirror.MsgType)
	assert.Equal(t, []string{"@all"}, mirror.Text.MentionedList)
	assert.Equal(t, "渠道已被自动禁用\n渠道：A（#1）", mirror.Text.Content)
	assert.NotContains(t, mirror.Text.Content, "**")
}

func TestWecomBotWebhookURL(t *testing.T) {
	assert.Equal(t,
		"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abc-123",
		wecomBotWebhookURL("abc-123"))
	assert.Equal(t,
		"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abc+123",
		wecomBotWebhookURL("abc 123"))
	assert.Equal(t,
		"https://example.test/webhook/send?key=x",
		wecomBotWebhookURL("https://example.test/webhook/send?key=x"))
}

func TestSendWecomBotAlertDirect(t *testing.T) {
	InitHttpClient()
	fetchSetting := system_setting.GetFetchSetting()
	originalFetch := *fetchSetting
	fetchSetting.EnableSSRFProtection = false
	t.Cleanup(func() { *fetchSetting = originalFetch })

	var lastBody []byte
	var lastHeader http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		lastBody = body
		lastHeader = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	t.Cleanup(server.Close)

	withWecomBotSetting(t, func(setting *operation_setting.WecomBotSetting) {
		setting.Bots = []operation_setting.WecomBotConfig{{
			ID: "test-bot", Name: "测试机器人", WebhookApiKey: server.URL, Enabled: true,
		}}
	})

	alert := wecomBotAlert{title: "渠道异常告警", color: "warning", lines: []string{"渠道：A（#1）", "状态码：500"}}
	require.NoError(t, sendWecomBotAlert(alert))

	var mirror wecomRequestMirror
	require.NoError(t, common.Unmarshal(lastBody, &mirror))
	assert.Equal(t, "markdown", mirror.MsgType)
	assert.Contains(t, mirror.Markdown.Content, "渠道异常告警")
	assert.Equal(t, "application/json; charset=utf-8", lastHeader.Get("Content-Type"))
}

func TestSendWecomBotAlertApiError(t *testing.T) {
	InitHttpClient()
	fetchSetting := system_setting.GetFetchSetting()
	originalFetch := *fetchSetting
	fetchSetting.EnableSSRFProtection = false
	t.Cleanup(func() { *fetchSetting = originalFetch })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errcode":93000,"errmsg":"rate limited"}`))
	}))
	t.Cleanup(server.Close)

	withWecomBotSetting(t, func(setting *operation_setting.WecomBotSetting) {
		setting.Bots = []operation_setting.WecomBotConfig{{ID: "test-bot", Name: "测试机器人", WebhookApiKey: server.URL, Enabled: true}}
	})

	err := sendWecomBotAlert(wecomBotAlert{title: "告警", color: "warning"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "93000")
	assert.Contains(t, err.Error(), "rate limited")
}

func TestSendWecomBotAlertUndecodableResponse(t *testing.T) {
	InitHttpClient()
	fetchSetting := system_setting.GetFetchSetting()
	originalFetch := *fetchSetting
	fetchSetting.EnableSSRFProtection = false
	t.Cleanup(func() { *fetchSetting = originalFetch })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	t.Cleanup(server.Close)

	withWecomBotSetting(t, func(setting *operation_setting.WecomBotSetting) {
		setting.Bots = []operation_setting.WecomBotConfig{{ID: "test-bot", Name: "测试机器人", WebhookApiKey: server.URL, Enabled: true}}
	})

	err := sendWecomBotAlert(wecomBotAlert{title: "告警", color: "warning"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode")
}

func TestSendWecomBotAlertTruncatesContent(t *testing.T) {
	InitHttpClient()
	fetchSetting := system_setting.GetFetchSetting()
	originalFetch := *fetchSetting
	fetchSetting.EnableSSRFProtection = false
	t.Cleanup(func() { *fetchSetting = originalFetch })

	var lastBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		lastBody = body
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	t.Cleanup(server.Close)

	withWecomBotSetting(t, func(setting *operation_setting.WecomBotSetting) {
		setting.Bots = []operation_setting.WecomBotConfig{{ID: "test-bot", Name: "测试机器人", WebhookApiKey: server.URL, Enabled: true}}
	})

	longLine := strings.Repeat("超", 3000)
	alert := wecomBotAlert{title: "渠道异常告警", color: "warning", lines: []string{longLine}}
	require.NoError(t, sendWecomBotAlert(alert))

	var mirror wecomRequestMirror
	require.NoError(t, common.Unmarshal(lastBody, &mirror))
	assert.LessOrEqual(t, len(mirror.Markdown.Content), wecomBotMaxContentBytes+3)
	assert.True(t, strings.HasSuffix(mirror.Markdown.Content, "…"))
}

func TestAcquireMemoryWecomBotAlertSlot(t *testing.T) {
	withWecomBotMemoryCooldown(t)

	assert.True(t, acquireMemoryWecomBotAlertSlot("k1", time.Minute))
	assert.False(t, acquireMemoryWecomBotAlertSlot("k1", time.Minute))
	assert.True(t, acquireMemoryWecomBotAlertSlot("k2", time.Minute))

	assert.True(t, acquireMemoryWecomBotAlertSlot("k3", 50*time.Millisecond))
	time.Sleep(60 * time.Millisecond)
	assert.True(t, acquireMemoryWecomBotAlertSlot("k3", time.Minute))
}

func TestNotifyWecomBotChannelErrorGating(t *testing.T) {
	withWecomBotMemoryCooldown(t)
	captured := captureWecomBotAlerts(t)

	t.Run("disabled setting skips alert", func(t *testing.T) {
		withWecomBotSetting(t, func(setting *operation_setting.WecomBotSetting) {
			setting.Enabled = false
			setting.Bots = []operation_setting.WecomBotConfig{{ID: "test-bot", Name: "测试机器人", WebhookApiKey: "key", Enabled: true}}
		})
		notifyWecomBotChannelError(wecomBotErrorAlertInfo{ChannelId: 1, ChannelName: "A", StatusCode: 500})
		assertNoWecomBotAlert(t, captured)
	})

	t.Run("error notify disabled skips alert", func(t *testing.T) {
		withWecomBotSetting(t, func(setting *operation_setting.WecomBotSetting) {
			setting.Enabled = true
			setting.ErrorNotifyEnabled = false
			setting.Bots = []operation_setting.WecomBotConfig{{ID: "test-bot", Name: "测试机器人", WebhookApiKey: "key", Enabled: true}}
		})
		notifyWecomBotChannelError(wecomBotErrorAlertInfo{ChannelId: 1, ChannelName: "A", StatusCode: 500})
		assertNoWecomBotAlert(t, captured)
	})

	t.Run("sends alert once per channel and status within cooldown", func(t *testing.T) {
		withWecomBotSetting(t, func(setting *operation_setting.WecomBotSetting) {
			setting.Enabled = true
			setting.ErrorNotifyEnabled = true
			setting.Bots = []operation_setting.WecomBotConfig{{ID: "test-bot", Name: "测试机器人", WebhookApiKey: "key", Enabled: true}}
			setting.ErrorCooldownMinutes = 10
		})
		info := wecomBotErrorAlertInfo{
			ChannelId:   12,
			ChannelName: "OpenAI-01",
			StatusCode:  500,
			ErrorCode:   "bad_response_status_code",
			Message:     "upstream boom",
			ModelName:   "gpt-4o",
			Attempt:     2,
		}
		notifyWecomBotChannelError(info)

		alert, ok := waitForWecomBotAlert(t, captured)
		require.True(t, ok)
		assert.Equal(t, "渠道异常告警", alert.title)
		joined := strings.Join(alert.lines, "\n")
		assert.Contains(t, joined, "渠道：OpenAI-01（#12）")
		assert.Contains(t, joined, "状态码：500")
		assert.Contains(t, joined, "错误码：bad_response_status_code")
		assert.Contains(t, joined, "模型：gpt-4o")
		assert.Contains(t, joined, "重试序号：2")
		assert.Contains(t, joined, "错误信息：upstream boom")
		assert.Contains(t, joined, "时间：")

		notifyWecomBotChannelError(info)
		assertNoWecomBotAlert(t, captured)

		info.StatusCode = 429
		notifyWecomBotChannelError(info)
		_, ok = waitForWecomBotAlert(t, captured)
		require.True(t, ok)
	})
}

func TestNotifyWecomBotChannelStatusAlerts(t *testing.T) {
	withWecomBotMemoryCooldown(t)
	captured := captureWecomBotAlerts(t)

	t.Run("disable alert respects switch", func(t *testing.T) {
		withWecomBotSetting(t, func(setting *operation_setting.WecomBotSetting) {
			setting.Enabled = true
			setting.DisableNotifyEnabled = true
			setting.Bots = []operation_setting.WecomBotConfig{{ID: "test-bot", Name: "测试机器人", WebhookApiKey: "key", Enabled: true}}
		})
		notifyWecomBotChannelDisabled("OpenAI-01", 12, "invalid key")
		alert, ok := waitForWecomBotAlert(t, captured)
		require.True(t, ok)
		assert.Equal(t, "渠道已被自动禁用", alert.title)
		joined := strings.Join(alert.lines, "\n")
		assert.Contains(t, joined, "渠道：OpenAI-01（#12）")
		assert.Contains(t, joined, "原因：invalid key")

		withWecomBotSetting(t, func(setting *operation_setting.WecomBotSetting) {
			setting.Enabled = true
			setting.DisableNotifyEnabled = false
			setting.Bots = []operation_setting.WecomBotConfig{{ID: "test-bot", Name: "测试机器人", WebhookApiKey: "key", Enabled: true}}
		})
		notifyWecomBotChannelDisabled("OpenAI-01", 12, "invalid key")
		assertNoWecomBotAlert(t, captured)
	})

	t.Run("enable alert respects switch", func(t *testing.T) {
		withWecomBotSetting(t, func(setting *operation_setting.WecomBotSetting) {
			setting.Enabled = true
			setting.EnableNotifyEnabled = true
			setting.Bots = []operation_setting.WecomBotConfig{{ID: "test-bot", Name: "测试机器人", WebhookApiKey: "key", Enabled: true}}
		})
		notifyWecomBotChannelEnabled("OpenAI-01", 12)
		alert, ok := waitForWecomBotAlert(t, captured)
		require.True(t, ok)
		assert.Equal(t, "渠道已恢复启用", alert.title)
		assert.Equal(t, "info", alert.color)

		withWecomBotSetting(t, func(setting *operation_setting.WecomBotSetting) {
			setting.Enabled = true
			setting.EnableNotifyEnabled = false
			setting.Bots = []operation_setting.WecomBotConfig{{ID: "test-bot", Name: "测试机器人", WebhookApiKey: "key", Enabled: true}}
		})
		notifyWecomBotChannelEnabled("OpenAI-01", 12)
		assertNoWecomBotAlert(t, captured)
	})
}

func setupWecomBotProcessChannelErrorTest(t *testing.T) (*gin.Context, <-chan wecomBotAlert) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	withWecomBotMemoryCooldown(t)

	previousErrorLogEnabled := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = false
	t.Cleanup(func() { constant.ErrorLogEnabled = previousErrorLogEnabled })

	captured := captureWecomBotAlerts(t)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("original_model", "gpt-4o")
	return ctx, captured
}

func TestProcessChannelErrorTriggersWecomBotAlert(t *testing.T) {
	ctx, captured := setupWecomBotProcessChannelErrorTest(t)

	previousAutoDisable := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = false
	t.Cleanup(func() { common.AutomaticDisableChannelEnabled = previousAutoDisable })

	withWecomBotSetting(t, func(setting *operation_setting.WecomBotSetting) {
		setting.Enabled = true
		setting.ErrorNotifyEnabled = true
		setting.Bots = []operation_setting.WecomBotConfig{{ID: "test-bot", Name: "测试机器人", WebhookApiKey: "key", Enabled: true}}
		setting.ErrorCooldownMinutes = 10
	})

	channelError := types.ChannelError{ChannelId: 12, ChannelName: "OpenAI-01", AutoBan: true}
	err := types.NewErrorWithStatusCode(errors.New("upstream boom"), types.ErrorCodeBadResponseStatusCode, 500)
	ProcessChannelError(ctx, channelError, err, nil)

	alert, ok := waitForWecomBotAlert(t, captured)
	require.True(t, ok)
	joined := strings.Join(alert.lines, "\n")
	assert.Contains(t, joined, "渠道：OpenAI-01（#12）")
	assert.Contains(t, joined, "状态码：500")
	assert.Contains(t, joined, "模型：gpt-4o")
}

func TestProcessChannelErrorSkipsWecomBotAlertForClientGone(t *testing.T) {
	ctx, captured := setupWecomBotProcessChannelErrorTest(t)

	withWecomBotSetting(t, func(setting *operation_setting.WecomBotSetting) {
		setting.Enabled = true
		setting.ErrorNotifyEnabled = true
		setting.Bots = []operation_setting.WecomBotConfig{{ID: "test-bot", Name: "测试机器人", WebhookApiKey: "key", Enabled: true}}
	})

	channelError := types.ChannelError{ChannelId: 12, ChannelName: "OpenAI-01", AutoBan: true}
	ProcessChannelError(ctx, channelError, types.NewClientGoneError(errors.New("canceled")), nil)
	assertNoWecomBotAlert(t, captured)
}

func TestProcessChannelErrorSkipsWecomBotAlertWhenAutoDisabling(t *testing.T) {
	ctx, captured := setupWecomBotProcessChannelErrorTest(t)

	// 自动禁用路径会触发 DisableChannel，需要真实的数据库表。
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.User{}))
	previousDB := model.DB
	model.DB = database
	t.Cleanup(func() {
		model.DB = previousDB
		_ = sqlDB.Close()
	})

	previousAutoDisable := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() { common.AutomaticDisableChannelEnabled = previousAutoDisable })

	withWecomBotSetting(t, func(setting *operation_setting.WecomBotSetting) {
		setting.Enabled = true
		setting.ErrorNotifyEnabled = true
		setting.DisableNotifyEnabled = true
		setting.Bots = []operation_setting.WecomBotConfig{{ID: "test-bot", Name: "测试机器人", WebhookApiKey: "key", Enabled: true}}
	})

	channelError := types.ChannelError{ChannelId: 12, ChannelName: "OpenAI-01", AutoBan: true}
	channelErr := types.NewError(errors.New("invalid key"), types.ErrorCodeChannelInvalidKey)
	ProcessChannelError(ctx, channelError, channelErr, nil)
	assertNoWecomBotAlert(t, captured)

	// 等待异步 DisableChannel 完成，避免清理数据库时产生竞态。
	time.Sleep(300 * time.Millisecond)
}

func TestWecomBotSettingValidation(t *testing.T) {
	t.Run("webhook api key", func(t *testing.T) {
		require.NoError(t, operation_setting.ValidateWecomBotWebhookApiKey(""))
		require.NoError(t, operation_setting.ValidateWecomBotWebhookApiKey("abc-123_XYZ"))
		require.NoError(t, operation_setting.ValidateWecomBotWebhookApiKey("https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abc"))
		require.Error(t, operation_setting.ValidateWecomBotWebhookApiKey("http://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abc"))
		require.Error(t, operation_setting.ValidateWecomBotWebhookApiKey("https://evil.example/send?key=abc"))
		require.Error(t, operation_setting.ValidateWecomBotWebhookApiKey("https://qyapi.weixin.qq.com/cgi-bin/webhook/send"))
		require.Error(t, operation_setting.ValidateWecomBotWebhookApiKey("bad key!"))
	})

	t.Run("mention userids", func(t *testing.T) {
		require.NoError(t, operation_setting.ValidateWecomBotMentionUserids(""))
		require.NoError(t, operation_setting.ValidateWecomBotMentionUserids("zhangsan, lisi,@all"))
		require.Error(t, operation_setting.ValidateWecomBotMentionUserids("bad userid!"))
		require.Error(t, operation_setting.ValidateWecomBotMentionUserids(strings.Repeat("a", 600)))
	})

	t.Run("parse mention userids", func(t *testing.T) {
		assert.Equal(t, []string{"zhangsan", "lisi"}, operation_setting.ParseWecomMentionUserids("zhangsan，lisi, zhangsan"))
		assert.Equal(t, []string{"@all"}, operation_setting.ParseWecomMentionUserids("@all, @all"))
		assert.Empty(t, operation_setting.ParseWecomMentionUserids(" , ,"))
	})

	t.Run("cooldown minutes", func(t *testing.T) {
		require.NoError(t, operation_setting.ValidateWecomErrorCooldownMinutes("10"))
		require.Error(t, operation_setting.ValidateWecomErrorCooldownMinutes("0"))
		require.Error(t, operation_setting.ValidateWecomErrorCooldownMinutes("1441"))
		require.Error(t, operation_setting.ValidateWecomErrorCooldownMinutes("abc"))
	})

	t.Run("normalize cooldown minutes", func(t *testing.T) {
		assert.Equal(t, operation_setting.DefaultWecomErrorCooldownMinutes, operation_setting.NormalizeWecomErrorCooldownMinutes(0))
		assert.Equal(t, operation_setting.DefaultWecomErrorCooldownMinutes, operation_setting.NormalizeWecomErrorCooldownMinutes(-5))
		assert.Equal(t, 30, operation_setting.NormalizeWecomErrorCooldownMinutes(30))
		assert.Equal(t, operation_setting.MaxWecomErrorCooldownMinutes, operation_setting.NormalizeWecomErrorCooldownMinutes(99999))
	})
}

func TestSendWecomBotAlertFanOut(t *testing.T) {
	InitHttpClient()
	fetchSetting := system_setting.GetFetchSetting()
	originalFetch := *fetchSetting
	fetchSetting.EnableSSRFProtection = false
	t.Cleanup(func() { *fetchSetting = originalFetch })

	var goodHits, badHits atomic.Int32
	goodServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		goodHits.Add(1)
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	t.Cleanup(goodServer.Close)
	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		badHits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(badServer.Close)

	alert := wecomBotAlert{title: "渠道异常告警", color: "warning", lines: []string{"渠道：A（#1）"}}

	t.Run("broadcasts to enabled bots and reports failures", func(t *testing.T) {
		goodHits.Store(0)
		badHits.Store(0)
		withWecomBotSetting(t, func(setting *operation_setting.WecomBotSetting) {
			setting.Bots = []operation_setting.WecomBotConfig{
				{ID: "b1", Name: "机器人一", WebhookApiKey: goodServer.URL, Enabled: true},
				{ID: "b2", Name: "机器人二", WebhookApiKey: badServer.URL, Enabled: true},
				{ID: "b3", Name: "机器人三", WebhookApiKey: goodServer.URL, Enabled: false},
			}
		})
		err := sendWecomBotAlert(alert)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "机器人二")
		assert.Equal(t, int32(1), goodHits.Load())
		assert.Equal(t, int32(1), badHits.Load())
	})

	t.Run("no enabled bot returns error", func(t *testing.T) {
		withWecomBotSetting(t, func(setting *operation_setting.WecomBotSetting) {
			setting.Bots = nil
		})
		err := sendWecomBotAlert(alert)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no enabled wecom bot")
	})
}

func TestNormalizeWecomBots(t *testing.T) {
	t.Run("valid list passes and backfills ids", func(t *testing.T) {
		bots, err := operation_setting.NormalizeWecomBots([]operation_setting.WecomBotConfig{
			{Name: " 运维群 ", WebhookApiKey: "key-1", Enabled: true},
			{Name: "备用群", WebhookApiKey: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abc", MentionUserids: "@all"},
		})
		require.NoError(t, err)
		require.Len(t, bots, 2)
		assert.Equal(t, "运维群", bots[0].Name)
		assert.NotEmpty(t, bots[0].ID)
		assert.NotEmpty(t, bots[1].ID)
	})

	t.Run("list rules", func(t *testing.T) {
		many := make([]operation_setting.WecomBotConfig, operation_setting.MaxWecomBotCount+1)
		for i := range many {
			many[i] = operation_setting.WecomBotConfig{Name: fmt.Sprintf("bot-%d", i), WebhookApiKey: "k", Enabled: true}
		}
		_, err := operation_setting.NormalizeWecomBots(many)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot exceed")

		_, err = operation_setting.NormalizeWecomBots([]operation_setting.WecomBotConfig{{Name: "", WebhookApiKey: "k"}})
		require.Error(t, err)

		_, err = operation_setting.NormalizeWecomBots([]operation_setting.WecomBotConfig{
			{Name: "a", WebhookApiKey: "k", Enabled: true},
			{Name: "A", WebhookApiKey: "k", Enabled: true},
		})
		require.Error(t, err)

		_, err = operation_setting.NormalizeWecomBots([]operation_setting.WecomBotConfig{{Name: "a", Enabled: true}})
		require.Error(t, err)

		_, err = operation_setting.NormalizeWecomBots([]operation_setting.WecomBotConfig{{Name: "a", WebhookApiKey: "bad key!"}})
		require.Error(t, err)

		_, err = operation_setting.NormalizeWecomBots([]operation_setting.WecomBotConfig{{Name: "a", WebhookApiKey: "k", MentionUserids: "bad userid!"}})
		require.Error(t, err)
	})

	t.Run("validate bots json", func(t *testing.T) {
		bots, err := operation_setting.ValidateWecomBotsJSON(`[{"id":"x","name":"群一","webhook_api_key":"k","enabled":true}]`)
		require.NoError(t, err)
		require.Len(t, bots, 1)

		_, err = operation_setting.ValidateWecomBotsJSON(`{"bad":`)
		require.Error(t, err)

		bots, err = operation_setting.ValidateWecomBotsJSON("")
		require.NoError(t, err)
		assert.Empty(t, bots)
	})
}

func TestWecomBotAfterConfigUpdateMigration(t *testing.T) {
	setting := operation_setting.GetWecomBotSetting()
	original := *setting
	t.Cleanup(func() { *setting = original })

	t.Run("legacy single bot migrates to list idempotently", func(t *testing.T) {
		setting.Bots = nil
		setting.WebhookApiKey = "legacy-key"
		setting.MentionUserids = "zhangsan"
		require.NoError(t, setting.AfterConfigUpdate())
		require.Len(t, setting.Bots, 1)
		assert.Equal(t, "legacy-key", setting.Bots[0].WebhookApiKey)
		assert.Equal(t, "zhangsan", setting.Bots[0].MentionUserids)
		assert.True(t, setting.Bots[0].Enabled)
		assert.NotEmpty(t, setting.Bots[0].ID)

		require.NoError(t, setting.AfterConfigUpdate())
		assert.Len(t, setting.Bots, 1)
	})

	t.Run("invalid list keeps entry state", func(t *testing.T) {
		setting.Bots = []operation_setting.WecomBotConfig{{Name: "", WebhookApiKey: "k", Enabled: true}}
		require.Error(t, setting.AfterConfigUpdate())
		require.Len(t, setting.Bots, 1)
		assert.Empty(t, setting.Bots[0].Name)
	})
}
