package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/wecomaibot"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

// wecomAIBotLLMDriver 以 channel-test 同款的 adaptor 直驱方式跑一次对话补全：
// 走完整协议转换但标记 IsChannelTest（内部免计费），工具定义与消息原样透传。
type wecomAIBotLLMDriver struct{}

func (wecomAIBotLLMDriver) ChatCompletion(ctx context.Context, req *wecomaibot.ChatRequest) (*wecomaibot.ChatResponse, error) {
	setting := operation_setting.GetWecomAIBotSetting()
	if !setting.ReadyForEnable() {
		return nil, errors.New("wecom ai bot model channel is not configured")
	}
	channel, err := model.GetChannelById(setting.ChannelID, true)
	if err != nil {
		return nil, fmt.Errorf("failed to load wecom ai bot channel: %w", err)
	}
	testUserID, err := resolveChannelTestUserID(nil)
	if err != nil {
		return nil, err
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	cache, err := model.GetUserCache(testUserID)
	if err != nil {
		return nil, err
	}
	cache.WriteContext(c)
	c.Set("id", testUserID)
	c.Set("channel", channel.Type)
	c.Set("base_url", channel.GetBaseURL())
	group, _ := model.GetUserGroup(testUserID, false)
	c.Set("group", group)
	if apiErr := middleware.SetupContextForSelectedChannel(c, channel, setting.Model); apiErr != nil {
		return nil, apiErr
	}

	request := buildWecomAIBotRequest(setting.Model, req)
	info, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAI, request, nil)
	if err != nil {
		return nil, err
	}
	info.IsChannelTest = true
	info.InitChannelMeta(c)

	if err := helper.ModelMappedHelper(c, info, request); err != nil {
		return nil, err
	}
	if err := helper.ApplyReasoningModelSuffix(c, info, request); err != nil {
		return nil, err
	}
	request.SetModelName(info.UpstreamModelName)

	apiType, _ := common.ChannelType2APIType(channel.Type)
	adaptor := relay.GetAdaptor(apiType)
	if adaptor == nil {
		return nil, fmt.Errorf("invalid api type: %d", apiType)
	}
	adaptor.Init(info)

	convertedRequest, err := adaptor.ConvertOpenAIRequest(c, info, request)
	if err != nil {
		return nil, err
	}
	jsonData, err := common.Marshal(convertedRequest)
	if err != nil {
		return nil, err
	}
	jsonData, err = relaycommon.ApplyRequestPoliciesWithRelayInfo(jsonData, info, service.NewParameterMediaTransformer(c))
	if err != nil {
		return nil, err
	}

	c.Request.Body = io.NopCloser(bytes.NewBuffer(jsonData))
	respAny, err := adaptor.DoRequest(c, info, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	httpResp, _ := respAny.(*http.Response)
	if httpResp == nil {
		return nil, errors.New("wecom ai bot got empty upstream response")
	}
	if httpResp.StatusCode != http.StatusOK {
		upstreamErr := service.RelayErrorHandler(ctx, httpResp, true)
		if httpResp.Body != nil {
			_ = httpResp.Body.Close()
		}
		return nil, fmt.Errorf("wecom ai bot upstream error: %v", upstreamErr)
	}
	if _, respErr := adaptor.DoResponse(c, httpResp, info); respErr != nil {
		return nil, respErr
	}
	body, err := io.ReadAll(w.Result().Body)
	if err != nil {
		return nil, err
	}
	var parsed dto.OpenAITextResponse
	if err := common.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("wecom ai bot failed to parse completion: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return nil, errors.New("wecom ai bot completion has no choices")
	}
	message := parsed.Choices[0].Message
	out := &wecomaibot.ChatResponse{Content: message.StringContent()}
	for _, toolCall := range message.ParseToolCalls() {
		out.ToolCalls = append(out.ToolCalls, wecomaibot.ToolCall{
			ID:        toolCall.ID,
			Name:      toolCall.Function.Name,
			Arguments: toolCall.Function.Arguments,
		})
	}
	return out, nil
}

func buildWecomAIBotRequest(modelName string, req *wecomaibot.ChatRequest) *dto.GeneralOpenAIRequest {
	request := &dto.GeneralOpenAIRequest{
		Model:  modelName,
		Stream: lo.ToPtr(false),
	}
	for _, m := range req.Messages {
		msg := dto.Message{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallId: m.ToolCallID,
		}
		if m.Name != "" {
			msg.Name = lo.ToPtr(m.Name)
		}
		if len(m.ToolCalls) > 0 {
			msg.ToolCalls = m.ToolCalls
			// OpenAI 语义：带工具调用的 assistant 消息 content 应为空/null
			if msg.Content == "" {
				msg.Content = nil
			}
		}
		request.Messages = append(request.Messages, msg)
	}
	for _, tool := range req.Tools {
		request.Tools = append(request.Tools, dto.ToolCallRequest{
			Type: "function",
			Function: dto.FunctionRequest{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		})
	}
	if len(request.Tools) > 0 {
		request.ToolChoice = "auto"
	}
	return request
}

// WireWecomAIBot 注入智能机器人依赖：对话模型驱动（adaptor 直驱，免计费）与
// 渠道余额实时刷新工具（复用渠道余额查询链路）。必须在 wecomaibot.Start 之前调用。
func WireWecomAIBot() {
	wecomaibot.SetLLMDriver(wecomAIBotLLMDriver{})
	wecomaibot.RegisterTool(wecomAIBotRefreshBalanceTool())
}

// wecomAIBotRefreshBalanceTool 把渠道实时余额查询包装为对话工具：结果写回
// 平台缓存，每渠道 5 分钟限流一次，避免对话高频触发真实上游调用。
func wecomAIBotRefreshBalanceTool() wecomaibot.Tool {
	return wecomaibot.Tool{
		Name:        "refresh_channel_balance",
		AdminOnly:   true,
		Description: "实时请求上游查询指定渠道当前余额并写回平台缓存（会真实调用上游接口，每渠道 5 分钟限一次）。仅当用户明确要求最新/实时余额时使用；普通查询用 get_channel_detail 的缓存余额即可。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"channel_id": map[string]any{"type": "integer", "description": "渠道 ID"},
			},
			"required": []string{"channel_id"},
		},
		Handler: func(ctx context.Context, args map[string]any) (any, error) {
			channelID := 0
			if value, ok := args["channel_id"].(float64); ok {
				channelID = int(value)
			}
			if channelID <= 0 {
				return nil, errors.New("channel_id is required")
			}
			if !wecomaibot.AcquireToolSlot(fmt.Sprintf("balance:%d", channelID), 5*time.Minute) {
				return map[string]any{
					"throttled": true,
					"message":   "该渠道 5 分钟内已实时刷新过余额，请稍后再试或先参考缓存余额",
				}, nil
			}
			channel, err := model.GetChannelById(channelID, true)
			if err != nil {
				return nil, fmt.Errorf("channel #%d not found", channelID)
			}
			result, err := updateChannelBalance(channel)
			if err != nil {
				return nil, err
			}
			if result.ResponseSummary != "" {
				// 部分渠道类型返回的是余额摘要文本而非数值
				return map[string]any{
					"channel_id":       channelID,
					"channel_name":     channel.Name,
					"response_summary": result.ResponseSummary,
				}, nil
			}
			channel.UpdateBalance(result.Balance)
			return map[string]any{
				"channel_id":         channelID,
				"channel_name":       channel.Name,
				"balance":            result.Balance,
				"balance_updated_at": time.Now().Format("2006-01-02 15:04:05"),
				"note":               "余额已实时刷新并更新平台缓存",
			}, nil
		},
	}
}

type wecomAIBotConfigPayload struct {
	Enabled           bool   `json:"enabled"`
	BotID             string `json:"bot_id"`
	Secret            string `json:"secret"`
	ChannelID         int    `json:"channel_id"`
	Model             string `json:"model"`
	MaxToolRounds     int    `json:"max_tool_rounds"`
	SessionRounds     int    `json:"session_rounds"`
	SessionTTLMinutes int    `json:"session_ttl_minutes"`
	AllowAllUsers     bool   `json:"allow_all_users"`
	UserWhitelist     string `json:"user_whitelist"`
	GroupEnabled      bool   `json:"group_enabled"`
	WelcomeText       string `json:"welcome_text"`
	AdminUserIDs      string `json:"admin_userids"`
	ReportEnabled     bool   `json:"report_enabled"`
	ReportHour        int    `json:"report_hour"`
}

// GetWecomAIBotConfig 返回智能机器人配置，Secret 仅返回掩码。
func GetWecomAIBotConfig(c *gin.Context) {
	setting := operation_setting.GetWecomAIBotSetting()
	common.ApiSuccess(c, gin.H{
		"enabled":             setting.Enabled,
		"bot_id":              setting.BotID,
		"secret_masked":       maskWecomAIBotSecret(setting.Secret),
		"channel_id":          setting.ChannelID,
		"model":               setting.Model,
		"max_tool_rounds":     setting.MaxToolRounds,
		"session_rounds":      setting.SessionRounds,
		"session_ttl_minutes": setting.SessionTTLMinutes,
		"allow_all_users":     setting.AllowAllUsers,
		"user_whitelist":      setting.UserWhitelist,
		"group_enabled":       setting.GroupEnabled,
		"welcome_text":        setting.WelcomeText,
	})
}

// UpdateWecomAIBotConfig 保存智能机器人配置；Secret 留空表示保留现有凭据。
func UpdateWecomAIBotConfig(c *gin.Context) {
	var payload wecomAIBotConfigPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	values, err := normalizeWecomAIBotConfig(payload)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if err := model.UpdateOptionsBulk(values); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	recordManageAudit(c, "option.update", map[string]any{
		"keys": wecomAIBotOptionKeys(),
	})
	common.ApiSuccess(c, nil)
}

func wecomAIBotOptionKeys() []string {
	return []string{
		"wecom_aibot_setting.enabled",
		"wecom_aibot_setting.bot_id",
		operation_setting.WecomAIBotSecretOptionKey,
		"wecom_aibot_setting.channel_id",
		"wecom_aibot_setting.model",
		"wecom_aibot_setting.max_tool_rounds",
		"wecom_aibot_setting.session_rounds",
		"wecom_aibot_setting.session_ttl_minutes",
		"wecom_aibot_setting.allow_all_users",
		"wecom_aibot_setting.user_whitelist",
		"wecom_aibot_setting.group_enabled",
		"wecom_aibot_setting.welcome_text",
		"wecom_aibot_setting.admin_userids",
		"wecom_aibot_setting.report_enabled",
		"wecom_aibot_setting.report_hour",
	}
}

// normalizeWecomAIBotConfig 校验并拼装全部配置键值；启用时要求核心配置齐备，
// Secret 为空表示沿用现有值。
func normalizeWecomAIBotConfig(payload wecomAIBotConfigPayload) (map[string]string, error) {
	current := operation_setting.GetWecomAIBotSetting()
	botID := strings.TrimSpace(payload.BotID)
	secret := strings.TrimSpace(payload.Secret)
	if secret == "" {
		secret = current.Secret
	}
	modelName := strings.TrimSpace(payload.Model)
	channelID := payload.ChannelID

	if err := operation_setting.ValidateWecomAIBotID(botID); err != nil {
		return nil, err
	}
	if err := operation_setting.ValidateWecomAIBotSecret(secret); err != nil {
		return nil, err
	}
	if err := operation_setting.ValidateWecomAIBotModel(modelName); err != nil {
		return nil, err
	}
	if channelID < 0 {
		return nil, errors.New("wecom ai bot channel id must be a non-negative integer")
	}
	if err := operation_setting.ValidateWecomAIBotUserWhitelist(payload.UserWhitelist); err != nil {
		return nil, err
	}
	if err := operation_setting.ValidateWecomAIBotAdminUserids(payload.AdminUserIDs); err != nil {
		return nil, err
	}
	if err := operation_setting.ValidateWecomAIBotReportHour(strconv.Itoa(payload.ReportHour)); payload.ReportHour != 0 && err != nil {
		return nil, err
	}
	if err := operation_setting.ValidateWecomAIBotWelcomeText(payload.WelcomeText); err != nil {
		return nil, err
	}
	maxToolRounds := operation_setting.NormalizeWecomAIBotToolRounds(payload.MaxToolRounds)
	sessionRounds := operation_setting.NormalizeWecomAIBotSessionRounds(payload.SessionRounds)
	sessionTTL := operation_setting.NormalizeWecomAIBotSessionTTL(payload.SessionTTLMinutes)
	if payload.Enabled && (botID == "" || secret == "" || channelID <= 0 || modelName == "") {
		return nil, errors.New("启用前需先配置 BotID、Secret、模型渠道与模型名")
	}

	return map[string]string{
		"wecom_aibot_setting.enabled":               strconv.FormatBool(payload.Enabled),
		"wecom_aibot_setting.bot_id":                botID,
		operation_setting.WecomAIBotSecretOptionKey: secret,
		"wecom_aibot_setting.channel_id":            strconv.Itoa(channelID),
		"wecom_aibot_setting.model":                 modelName,
		"wecom_aibot_setting.max_tool_rounds":       strconv.Itoa(maxToolRounds),
		"wecom_aibot_setting.session_rounds":        strconv.Itoa(sessionRounds),
		"wecom_aibot_setting.session_ttl_minutes":   strconv.Itoa(sessionTTL),
		"wecom_aibot_setting.allow_all_users":       strconv.FormatBool(payload.AllowAllUsers),
		"wecom_aibot_setting.user_whitelist":        payload.UserWhitelist,
		"wecom_aibot_setting.group_enabled":         strconv.FormatBool(payload.GroupEnabled),
		"wecom_aibot_setting.welcome_text":          payload.WelcomeText,
		"wecom_aibot_setting.admin_userids":         payload.AdminUserIDs,
		"wecom_aibot_setting.report_enabled":        strconv.FormatBool(payload.ReportEnabled),
		"wecom_aibot_setting.report_hour":           strconv.Itoa(operation_setting.NormalizeWecomAIBotReportHour(payload.ReportHour)),
	}, nil
}

// maskWecomAIBotSecret 掩码长连接 Secret，仅保留末 4 位。
func maskWecomAIBotSecret(secret string) string {
	trimmed := strings.TrimSpace(secret)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= 4 {
		return "****"
	}
	return "****" + trimmed[len(trimmed)-4:]
}

// GetWecomAIBotStatus 返回长连接当前状态（是否启用、租约持有、连接态、最近错误）。
func GetWecomAIBotStatus(c *gin.Context) {
	common.ApiSuccess(c, wecomaibot.GetStatus())
}
