package controller

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWecomAIBotConfigPreservesReportAndAdminSettings(t *testing.T) {
	setting := operation_setting.GetWecomAIBotSetting()
	original := *setting
	t.Cleanup(func() { *setting = original })
	setting.AdminUserIDs = "ops-admin"
	setting.ReportEnabled = true
	setting.ReportHour = 16
	setting.ReportTargets = "group:ops-chat"
	setting.Secret = "private-bot-secret"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	GetWecomAIBotConfig(c)
	var response struct {
		Data wecomAIBotConfigPayload `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, setting.AdminUserIDs, response.Data.AdminUserIDs)
	assert.True(t, response.Data.ReportEnabled)
	assert.Equal(t, 16, response.Data.ReportHour)
	assert.Equal(t, setting.ReportTargets, response.Data.ReportTargets)
	assert.NotContains(t, w.Body.String(), setting.Secret)
	values, err := normalizeWecomAIBotConfig(response.Data)
	require.NoError(t, err)
	assert.Equal(t, "ops-admin", values["wecom_aibot_setting.admin_userids"])
	assert.Equal(t, "true", values["wecom_aibot_setting.report_enabled"])
	assert.Equal(t, "group:ops-chat", values["wecom_aibot_setting.report_targets"])
}

func TestMaskWecomAIBotSecret(t *testing.T) {
	assert.Equal(t, "", maskWecomAIBotSecret(""))
	assert.Equal(t, "****", maskWecomAIBotSecret("abc"))
	assert.Equal(t, "****e359", maskWecomAIBotSecret("81af1357-0f73-45e5-b7cf-465d453fe359"))
}

func TestNormalizeWecomAIBotConfig(t *testing.T) {
	setting := operation_setting.GetWecomAIBotSetting()
	original := *setting
	t.Cleanup(func() { *setting = original })

	// Secret 留空表示沿用现有凭据
	setting.Secret = "existing-secret"
	values, err := normalizeWecomAIBotConfig(wecomAIBotConfigPayload{
		Enabled: false, BotID: " bot-1 ", Model: "gpt-4o-mini", ChannelID: 3,
	})
	require.NoError(t, err)
	assert.Equal(t, "existing-secret", values[operation_setting.WecomAIBotSecretOptionKey])
	assert.Equal(t, "bot-1", values["wecom_aibot_setting.bot_id"])
	assert.Equal(t, "5", values["wecom_aibot_setting.max_tool_rounds"])

	// 启用但渠道未配置 → 拒绝
	_, err = normalizeWecomAIBotConfig(wecomAIBotConfigPayload{
		Enabled: true, BotID: "bot-1", Secret: "s", Model: "gpt-4o-mini",
	})
	require.Error(t, err)

	// 启用且核心配置齐备 → 通过，数值字段自动收敛到上下限
	values, err = normalizeWecomAIBotConfig(wecomAIBotConfigPayload{
		Enabled: true, BotID: "bot-1", Secret: "s", Model: "gpt-4o-mini", ChannelID: 3,
		MaxToolRounds: 99, SessionRounds: 0, SessionTTLMinutes: 30,
		UserWhitelist: "alice, bob",
	})
	require.NoError(t, err)
	assert.Equal(t, "true", values["wecom_aibot_setting.enabled"])
	assert.Equal(t, "10", values["wecom_aibot_setting.max_tool_rounds"])
	assert.Equal(t, "10", values["wecom_aibot_setting.session_rounds"])
	assert.Equal(t, "30", values["wecom_aibot_setting.session_ttl_minutes"])
	assert.Equal(t, "alice, bob", values["wecom_aibot_setting.user_whitelist"])

	// 白名单中的超长 userid → 拒绝
	_, err = normalizeWecomAIBotConfig(wecomAIBotConfigPayload{
		Enabled: false, BotID: "b", Secret: "s", UserWhitelist: strings.Repeat("x", 100),
	})
	require.Error(t, err)
}
