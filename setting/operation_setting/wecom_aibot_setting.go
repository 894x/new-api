package operation_setting

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

// WecomAIBotSetting 企业微信智能机器人（API 模式长连接）配置。Secret 为长连接
// 鉴权密钥，与群机器人 Webhook Key 是两套独立凭证；ChannelID/Model 指定对话
// 使用的模型渠道。UserWhitelist 存放允许对话的企微 userid（可能是加密的
// open_userid），逗号分隔。AdminUserIDs 为具备管理员工具权限的企微 userid；
// ReportEnabled/ReportHour 控制每日运营日报的推送。
type WecomAIBotSetting struct {
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
	// ReportTargets 为显式授权的日报目标：userid / single:userid / group:chatid。
	// 单聊仍需在访问白名单内，群聊需开启；留空仅推可访问的管理员，两者均空则不发送。
	ReportTargets string `json:"report_targets"`
}

const (
	WecomAIBotSecretOptionKey = "wecom_aibot_setting.secret"

	DefaultWecomAIBotMaxToolRounds     = 5
	MaxWecomAIBotToolRounds            = 10
	DefaultWecomAIBotSessionRounds     = 10
	MaxWecomAIBotSessionRounds         = 50
	DefaultWecomAIBotSessionTTLMinutes = 10
	MaxWecomAIBotSessionTTLMinutes     = 1440

	MaxWecomAIBotIDLength      = 128
	MaxWecomAIBotSecretLength  = 512
	MaxWecomAIBotModelLength   = 128
	MaxWecomAIBotWelcomeLength = 1024

	MaxWecomAIBotAdminUserRawLength = 2048
	MaxWecomAIBotAdminUserLen       = 64
	DefaultWecomAIBotReportHour     = 9

	maxWecomAIBotWhitelistRawLength = 2048
	maxWecomAIBotWhitelistUserLen   = 64
)

var wecomAIBotSetting = WecomAIBotSetting{
	MaxToolRounds:     DefaultWecomAIBotMaxToolRounds,
	SessionRounds:     DefaultWecomAIBotSessionRounds,
	SessionTTLMinutes: DefaultWecomAIBotSessionTTLMinutes,
	GroupEnabled:      true,
}

func init() {
	config.GlobalConfig.Register("wecom_aibot_setting", &wecomAIBotSetting)
	if err := wecomAIBotSetting.AfterConfigUpdate(); err != nil {
		panic(err)
	}
}

func GetWecomAIBotSetting() *WecomAIBotSetting {
	wecomAIBotSetting.MaxToolRounds = NormalizeWecomAIBotToolRounds(wecomAIBotSetting.MaxToolRounds)
	wecomAIBotSetting.SessionRounds = NormalizeWecomAIBotSessionRounds(wecomAIBotSetting.SessionRounds)
	wecomAIBotSetting.SessionTTLMinutes = NormalizeWecomAIBotSessionTTL(wecomAIBotSetting.SessionTTLMinutes)
	wecomAIBotSetting.ReportHour = NormalizeWecomAIBotReportHour(wecomAIBotSetting.ReportHour)
	return &wecomAIBotSetting
}

// NormalizeWecomAIBotReportHour 日报时点收敛到 0-23，缺省 9 点。
func NormalizeWecomAIBotReportHour(hour int) int {
	if hour < 0 || hour > 23 {
		return DefaultWecomAIBotReportHour
	}
	return hour
}

// ReadyForEnable 判断启用长连接所需的核心配置是否齐备。
func (s *WecomAIBotSetting) ReadyForEnable() bool {
	return strings.TrimSpace(s.BotID) != "" &&
		strings.TrimSpace(s.Secret) != "" &&
		s.ChannelID > 0 &&
		strings.TrimSpace(s.Model) != ""
}

func NormalizeWecomAIBotToolRounds(rounds int) int {
	if rounds <= 0 {
		return DefaultWecomAIBotMaxToolRounds
	}
	return min(rounds, MaxWecomAIBotToolRounds)
}

func NormalizeWecomAIBotSessionRounds(rounds int) int {
	if rounds <= 0 {
		return DefaultWecomAIBotSessionRounds
	}
	return min(rounds, MaxWecomAIBotSessionRounds)
}

func NormalizeWecomAIBotSessionTTL(minutes int) int {
	if minutes <= 0 {
		return DefaultWecomAIBotSessionTTLMinutes
	}
	return min(minutes, MaxWecomAIBotSessionTTLMinutes)
}

func ValidateWecomAIBotID(value string) error {
	value = strings.TrimSpace(value)
	if len(value) > MaxWecomAIBotIDLength {
		return fmt.Errorf("wecom ai bot id cannot exceed %d bytes", MaxWecomAIBotIDLength)
	}
	return nil
}

func ValidateWecomAIBotSecret(value string) error {
	if len(value) > MaxWecomAIBotSecretLength {
		return fmt.Errorf("wecom ai bot secret cannot exceed %d bytes", MaxWecomAIBotSecretLength)
	}
	return nil
}

func ValidateWecomAIBotChannelID(value string) error {
	id, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || id < 0 {
		return fmt.Errorf("wecom ai bot channel id must be a non-negative integer")
	}
	return nil
}

func ValidateWecomAIBotModel(value string) error {
	value = strings.TrimSpace(value)
	if len(value) > MaxWecomAIBotModelLength {
		return fmt.Errorf("wecom ai bot model name cannot exceed %d bytes", MaxWecomAIBotModelLength)
	}
	return nil
}

func ValidateWecomAIBotToolRounds(value string) error {
	rounds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || rounds < 1 || rounds > MaxWecomAIBotToolRounds {
		return fmt.Errorf("wecom ai bot max tool rounds must be between 1 and %d", MaxWecomAIBotToolRounds)
	}
	return nil
}

func ValidateWecomAIBotSessionRounds(value string) error {
	rounds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || rounds < 1 || rounds > MaxWecomAIBotSessionRounds {
		return fmt.Errorf("wecom ai bot session rounds must be between 1 and %d", MaxWecomAIBotSessionRounds)
	}
	return nil
}

func ValidateWecomAIBotSessionTTL(value string) error {
	minutes, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || minutes < 1 || minutes > MaxWecomAIBotSessionTTLMinutes {
		return fmt.Errorf("wecom ai bot session ttl must be between 1 and %d minutes", MaxWecomAIBotSessionTTLMinutes)
	}
	return nil
}

// ParseWecomAIBotUserWhitelist 解析逗号/空格分隔的企微 userid 白名单，去重且保持顺序。
func ParseWecomAIBotUserWhitelist(raw string) []string {
	var userids []string
	for _, token := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == ' ' || r == '\n'
	}) {
		token = strings.TrimSpace(token)
		if token == "" || slices.Contains(userids, token) {
			continue
		}
		userids = append(userids, token)
	}
	return userids
}

func ValidateWecomAIBotUserWhitelist(value string) error {
	if len(value) > maxWecomAIBotWhitelistRawLength {
		return fmt.Errorf("wecom ai bot user whitelist is too long")
	}
	for _, token := range ParseWecomAIBotUserWhitelist(value) {
		if len(token) > maxWecomAIBotWhitelistUserLen {
			return fmt.Errorf("wecom ai bot user whitelist contains an invalid userid: %s", token)
		}
	}
	return nil
}

// ParseWecomAIBotAdminUserids 解析具备管理员工具权限的企微 userid 列表。
func ParseWecomAIBotAdminUserids(raw string) []string {
	return ParseWecomAIBotUserWhitelist(raw)
}

func ValidateWecomAIBotAdminUserids(value string) error {
	if len(value) > MaxWecomAIBotAdminUserRawLength {
		return fmt.Errorf("wecom ai bot admin userids are too long")
	}
	for _, token := range ParseWecomAIBotAdminUserids(value) {
		if len(token) > MaxWecomAIBotAdminUserLen {
			return fmt.Errorf("wecom ai bot admin userids contain an invalid userid: %s", token)
		}
	}
	return nil
}

func ValidateWecomAIBotReportHour(value string) error {
	hour, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || hour < 0 || hour > 23 {
		return fmt.Errorf("wecom ai bot report hour must be between 0 and 23")
	}
	return nil
}

func ValidateWecomAIBotWelcomeText(value string) error {
	if len(value) > MaxWecomAIBotWelcomeLength {
		return fmt.Errorf("wecom ai bot welcome text cannot exceed %d bytes", MaxWecomAIBotWelcomeLength)
	}
	return nil
}

// AfterConfigUpdate 规范化存量配置（连接重建由运行器按配置指纹检测，无需在此触发）。
func (s *WecomAIBotSetting) AfterConfigUpdate() error {
	s.BotID = strings.TrimSpace(s.BotID)
	s.Secret = strings.TrimSpace(s.Secret)
	s.Model = strings.TrimSpace(s.Model)
	s.MaxToolRounds = NormalizeWecomAIBotToolRounds(s.MaxToolRounds)
	s.SessionRounds = NormalizeWecomAIBotSessionRounds(s.SessionRounds)
	s.SessionTTLMinutes = NormalizeWecomAIBotSessionTTL(s.SessionTTLMinutes)
	if s.ChannelID < 0 {
		s.ChannelID = 0
	}
	return nil
}
