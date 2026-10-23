package operation_setting

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

// WecomBotConfig 一台企业微信群机器人：WebhookApiKey 存储裸机器人 Key 或官方完整 Webhook 地址。
type WecomBotConfig struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	WebhookApiKey  string `json:"webhook_api_key"`
	MentionUserids string `json:"mention_userids"`
	Enabled        bool   `json:"enabled"`
}

// WecomBotSetting 企业微信群机器人渠道告警（渠道监控机器人）配置。
// Bots 为机器人列表，告警广播到其中所有启用的机器人；WebhookApiKey/MentionUserids
// 为旧版单机器人字段，仅作为列表为空时的迁移来源保留。
type WecomBotSetting struct {
	Enabled              bool             `json:"enabled"`
	WebhookApiKey        string           `json:"webhook_api_key"`
	MentionUserids       string           `json:"mention_userids"`
	Bots                 []WecomBotConfig `json:"bots"`
	ErrorNotifyEnabled   bool             `json:"error_notify_enabled"`
	ErrorCooldownMinutes int              `json:"error_cooldown_minutes"`
	DisableNotifyEnabled bool             `json:"disable_notify_enabled"`
	EnableNotifyEnabled  bool             `json:"enable_notify_enabled"`
}

const (
	WecomBotBotsOptionKey = "wecom_bot_setting.bots"

	MaxWecomBotCount      = 10
	MaxWecomBotNameLength = 64
)

const (
	WecomBotWebhookHost = "qyapi.weixin.qq.com"

	DefaultWecomErrorCooldownMinutes = 10
	MaxWecomErrorCooldownMinutes     = 1440
)

var wecomBotSetting = WecomBotSetting{
	ErrorNotifyEnabled:   true,
	ErrorCooldownMinutes: DefaultWecomErrorCooldownMinutes,
	DisableNotifyEnabled: true,
	EnableNotifyEnabled:  true,
}

func init() {
	config.GlobalConfig.Register("wecom_bot_setting", &wecomBotSetting)
	if err := wecomBotSetting.AfterConfigUpdate(); err != nil {
		panic(err)
	}
}

func GetWecomBotSetting() *WecomBotSetting {
	wecomBotSetting.ErrorCooldownMinutes = NormalizeWecomErrorCooldownMinutes(wecomBotSetting.ErrorCooldownMinutes)
	return &wecomBotSetting
}

func NormalizeWecomErrorCooldownMinutes(minutes int) int {
	if minutes <= 0 {
		return DefaultWecomErrorCooldownMinutes
	}
	if minutes > MaxWecomErrorCooldownMinutes {
		return MaxWecomErrorCooldownMinutes
	}
	return minutes
}

func ValidateWecomErrorCooldownMinutes(value string) error {
	minutes, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || minutes < 1 || minutes > MaxWecomErrorCooldownMinutes {
		return fmt.Errorf("wecom bot error cooldown minutes must be between 1 and %d", MaxWecomErrorCooldownMinutes)
	}
	return nil
}

var (
	wecomBotKeyPattern    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	wecomMentionPattern   = regexp.MustCompile(`^[A-Za-z0-9_@.\-]{1,64}$`)
	wecomMentionMaxRawLen = 512
)

// ValidateWecomBotWebhookApiKey 接受空值、裸机器人 Key 或官方完整 Webhook 地址。
func ValidateWecomBotWebhookApiKey(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if strings.HasPrefix(value, "http://") {
		return fmt.Errorf("wecom bot webhook url must use https")
	}
	if !strings.HasPrefix(value, "https://") {
		if !wecomBotKeyPattern.MatchString(value) {
			return fmt.Errorf("wecom bot webhook key contains invalid characters")
		}
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("wecom bot webhook url is invalid: %v", err)
	}
	if parsed.Hostname() != WecomBotWebhookHost {
		return fmt.Errorf("wecom bot webhook url host must be %s", WecomBotWebhookHost)
	}
	if parsed.Query().Get("key") == "" {
		return fmt.Errorf("wecom bot webhook url is missing the key parameter")
	}
	return nil
}

// ParseWecomMentionUserids 解析逗号分隔的企微 userid 列表，支持 @all，去重且保持顺序。
func ParseWecomMentionUserids(raw string) []string {
	var userids []string
	for _, token := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == ' '
	}) {
		token = strings.TrimSpace(token)
		if token == "" || slices.Contains(userids, token) {
			continue
		}
		userids = append(userids, token)
	}
	return userids
}

func ValidateWecomBotMentionUserids(value string) error {
	if len(value) > wecomMentionMaxRawLen {
		return fmt.Errorf("wecom bot mention list is too long")
	}
	for _, token := range ParseWecomMentionUserids(value) {
		if !wecomMentionPattern.MatchString(token) {
			return fmt.Errorf("wecom bot mention list contains invalid userid: %s", token)
		}
	}
	return nil
}

// HasEnabledBot 判断是否存在至少一台启用的机器人。
func (s *WecomBotSetting) HasEnabledBot() bool {
	for _, bot := range s.Bots {
		if bot.Enabled && bot.WebhookApiKey != "" {
			return true
		}
	}
	return false
}

// NormalizeWecomBots 规范化机器人列表：名称非空/唯一/限长、启用的机器人必须有 Webhook、
// 逐项复用 Webhook 与提醒名单校验，并为缺失的条目补齐 ID。
func NormalizeWecomBots(bots []WecomBotConfig) ([]WecomBotConfig, error) {
	if len(bots) > MaxWecomBotCount {
		return nil, fmt.Errorf("wecom bots cannot exceed %d entries", MaxWecomBotCount)
	}
	normalized := make([]WecomBotConfig, 0, len(bots))
	seenNames := make(map[string]struct{}, len(bots))
	for _, bot := range bots {
		bot.Name = strings.TrimSpace(bot.Name)
		if bot.Name == "" {
			return nil, fmt.Errorf("wecom bot name cannot be empty")
		}
		if len(bot.Name) > MaxWecomBotNameLength {
			return nil, fmt.Errorf("wecom bot name cannot exceed %d bytes", MaxWecomBotNameLength)
		}
		lowerName := strings.ToLower(bot.Name)
		if _, ok := seenNames[lowerName]; ok {
			return nil, fmt.Errorf("duplicate wecom bot name %q", bot.Name)
		}
		seenNames[lowerName] = struct{}{}
		bot.WebhookApiKey = strings.TrimSpace(bot.WebhookApiKey)
		if bot.Enabled && bot.WebhookApiKey == "" {
			return nil, fmt.Errorf("wecom bot %q must have a webhook key to be enabled", bot.Name)
		}
		if err := ValidateWecomBotWebhookApiKey(bot.WebhookApiKey); err != nil {
			return nil, fmt.Errorf("wecom bot %q: %w", bot.Name, err)
		}
		if err := ValidateWecomBotMentionUserids(bot.MentionUserids); err != nil {
			return nil, fmt.Errorf("wecom bot %q: %w", bot.Name, err)
		}
		if bot.ID == "" {
			bot.ID = common.GetUUID()
		}
		normalized = append(normalized, bot)
	}
	return normalized, nil
}

// ValidateWecomBotsJSON 校验机器人列表 JSON 字符串，空字符串表示清空列表。
func ValidateWecomBotsJSON(value string) ([]WecomBotConfig, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var bots []WecomBotConfig
	if err := common.Unmarshal([]byte(value), &bots); err != nil {
		return nil, fmt.Errorf("invalid wecom bots: %w", err)
	}
	return NormalizeWecomBots(bots)
}

// AfterConfigUpdate 规范化机器人列表（失败回滚）；列表为空但存在旧版全局 Key 时，
// 自动把旧配置迁移为列表第一条机器人，保证升级后告警行为不变。
func (s *WecomBotSetting) AfterConfigUpdate() error {
	previous := *s
	normalized, err := NormalizeWecomBots(s.Bots)
	if err != nil {
		*s = previous
		return err
	}
	s.Bots = normalized
	if len(s.Bots) == 0 && s.WebhookApiKey != "" {
		s.Bots = []WecomBotConfig{{
			ID:             common.GetUUID(),
			Name:           "默认机器人",
			WebhookApiKey:  s.WebhookApiKey,
			MentionUserids: s.MentionUserids,
			Enabled:        true,
		}}
	}
	return nil
}
