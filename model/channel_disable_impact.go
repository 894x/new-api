package model

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// ChannelDisableImpactSnapshot contains routing metadata only. Credentials and
// request contents are deliberately excluded from the database projections.
type ChannelDisableImpactSnapshot struct {
	Channels       []Channel
	Users          []User
	Tokens         []Token
	Overrides      []UserChannelRoutingOverride
	GroupOverrides []UserGroupChannelRoutingOverride
}

func LoadChannelDisableImpactSnapshot(ctx context.Context, ids []int, tag *string, operatorRole int) (ChannelDisableImpactSnapshot, error) {
	var snapshot ChannelDisableImpactSnapshot
	db := DB.WithContext(ctx)
	channels := db.Model(&Channel{}).Where("status = ?", common.ChannelStatusEnabled)
	if tag != nil {
		channels = channels.Where("tag = ?", *tag)
	} else {
		channels = channels.Where("id IN ?", ids)
	}
	if err := channels.Select([]string{"id", "name", "models", "group", "status"}).Order("id").Find(&snapshot.Channels).Error; err != nil {
		return snapshot, err
	}
	if len(snapshot.Channels) == 0 {
		return snapshot, nil
	}
	groups := []string{"", "auto"}
	var models []string
	for _, channel := range snapshot.Channels {
		for group := range strings.SplitSeq(channel.Group, ",") {
			groups = append(groups, group)
		}
		for name := range strings.SplitSeq(channel.Models, ",") {
			if name != "" {
				models = append(models, name)
			}
		}
	}
	slices.Sort(groups)
	groups = slices.Compact(groups)
	slices.Sort(models)
	models = slices.Compact(models)
	tokens := db.Model(&Token{}).
		Where("status = ? AND (expired_time = -1 OR expired_time >= ?)", common.TokenStatusEnabled, time.Now().Unix()).
		Where(map[string]any{"group": groups})
	userIDs := tokens.Session(&gorm.Session{}).Select("user_id")
	if err := tokens.Session(&gorm.Session{}).Select([]string{"user_id", "group", "auto_groups", "model_limits_enabled", "model_limits"}).Find(&snapshot.Tokens).Error; err != nil {
		return snapshot, err
	}
	users := db.Model(&User{}).Where("status = ?", common.UserStatusEnabled).Where("id IN (?)", userIDs)
	if operatorRole != common.RoleRootUser {
		users = users.Where("role < ?", operatorRole)
	}
	if err := users.Select([]string{"id", "username", "display_name", "group"}).Order("id").Find(&snapshot.Users).Error; err != nil {
		return snapshot, err
	}
	if len(models) == 0 || len(snapshot.Users) == 0 {
		return snapshot, nil
	}
	visibleUsers := users.Session(&gorm.Session{}).Select("id")
	if err := db.Where("model IN ?", models).Where("user_id IN (?)", visibleUsers).Find(&snapshot.Overrides).Error; err != nil {
		return snapshot, err
	}
	if err := db.Where("model IN ?", models).Where("user_id IN (?)", visibleUsers).Find(&snapshot.GroupOverrides).Error; err != nil {
		return snapshot, err
	}
	return snapshot, nil
}
