package controller

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// WecomBotResponse 管理页展示结构：Webhook 掩码显示，不返回完整凭据。
type WecomBotResponse struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	WebhookMasked  string `json:"webhook_masked"`
	MentionUserids string `json:"mention_userids"`
	Enabled        bool   `json:"enabled"`
}

type wecomBotPayload struct {
	Name           string `json:"name"`
	WebhookApiKey  string `json:"webhook_api_key"`
	MentionUserids string `json:"mention_userids"`
	Enabled        bool   `json:"enabled"`
}

func toWecomBotResponse(bot operation_setting.WecomBotConfig) WecomBotResponse {
	return WecomBotResponse{
		ID:             bot.ID,
		Name:           bot.Name,
		WebhookMasked:  maskWecomBotWebhook(bot.WebhookApiKey),
		MentionUserids: bot.MentionUserids,
		Enabled:        bot.Enabled,
	}
}

// maskWecomBotWebhook 掩码机器人 Webhook：裸 Key 仅保留末 4 位，完整 URL 仅掩码 key 参数。
func maskWecomBotWebhook(webhook string) string {
	trimmed := strings.TrimSpace(webhook)
	if trimmed == "" {
		return ""
	}
	mask := func(secret string) string {
		if len(secret) <= 4 {
			return "****"
		}
		return "****" + secret[len(secret)-4:]
	}
	if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
		return mask(trimmed)
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "****"
	}
	key := parsed.Query().Get("key")
	if key == "" {
		return "****"
	}
	// 掩码值仅用于展示；key 字符集（[A-Za-z0-9_-]）URL 安全，直接拼接避免 Encode 转义 `*`
	parsed.RawQuery = "key=" + mask(key)
	return parsed.String()
}

// GetWecomBots 返回机器人列表（Webhook 掩码）。
func GetWecomBots(c *gin.Context) {
	setting := operation_setting.GetWecomBotSetting()
	views := make([]WecomBotResponse, 0, len(setting.Bots))
	for _, bot := range setting.Bots {
		views = append(views, toWecomBotResponse(bot))
	}
	common.ApiSuccess(c, views)
}

// CreateWecomBot 新增一台机器人。
func CreateWecomBot(c *gin.Context) {
	var payload wecomBotPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	setting := operation_setting.GetWecomBotSetting()
	bots := append(append([]operation_setting.WecomBotConfig{}, setting.Bots...), operation_setting.WecomBotConfig{
		Name:           payload.Name,
		WebhookApiKey:  payload.WebhookApiKey,
		MentionUserids: payload.MentionUserids,
		Enabled:        payload.Enabled,
	})
	normalized, err := saveWecomBots(bots)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	common.ApiSuccess(c, toWecomBotResponse(normalized[len(normalized)-1]))
}

// UpdateWecomBot 更新一台机器人；Webhook 留空表示保留现有凭据。
func UpdateWecomBot(c *gin.Context) {
	id := c.Param("id")
	var payload wecomBotPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	setting := operation_setting.GetWecomBotSetting()
	bots := append([]operation_setting.WecomBotConfig{}, setting.Bots...)
	found := false
	var updated operation_setting.WecomBotConfig
	for i, bot := range bots {
		if bot.ID != id {
			continue
		}
		found = true
		bot.Name = payload.Name
		bot.MentionUserids = payload.MentionUserids
		bot.Enabled = payload.Enabled
		if strings.TrimSpace(payload.WebhookApiKey) != "" {
			bot.WebhookApiKey = payload.WebhookApiKey
		}
		bots[i] = bot
		updated = bot
		break
	}
	if !found {
		common.ApiErrorMsg(c, "机器人不存在")
		return
	}
	if _, err := saveWecomBots(bots); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	common.ApiSuccess(c, toWecomBotResponse(updated))
}

// DeleteWecomBot 删除一台机器人。
func DeleteWecomBot(c *gin.Context) {
	id := c.Param("id")
	setting := operation_setting.GetWecomBotSetting()
	bots := make([]operation_setting.WecomBotConfig, 0, len(setting.Bots))
	found := false
	for _, bot := range setting.Bots {
		if bot.ID == id {
			found = true
			continue
		}
		bots = append(bots, bot)
	}
	if !found {
		common.ApiErrorMsg(c, "机器人不存在")
		return
	}
	if _, err := saveWecomBots(bots); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// saveWecomBots 校验并持久化机器人列表，返回规范化结果。
func saveWecomBots(bots []operation_setting.WecomBotConfig) ([]operation_setting.WecomBotConfig, error) {
	normalized, err := operation_setting.NormalizeWecomBots(bots)
	if err != nil {
		return nil, err
	}
	encoded, err := common.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	if err := model.UpdateOption(operation_setting.WecomBotBotsOptionKey, string(encoded)); err != nil {
		return nil, err
	}
	return normalized, nil
}
