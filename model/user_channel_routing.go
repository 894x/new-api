package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UserChannelRoutingOverride stores one member of a user's routing pool.
// Disabled retains the priority so re-enabling restores the user's ordering.
type UserChannelRoutingOverride struct {
	UserId     int    `json:"-" gorm:"primaryKey;autoIncrement:false"`
	Model      string `json:"model" gorm:"type:varchar(255);primaryKey;autoIncrement:false"`
	ChannelId  int    `json:"channel_id" gorm:"primaryKey;autoIncrement:false"`
	Priority   *int64 `json:"priority_override" gorm:"bigint"`
	Weight     *uint  `json:"weight_override" gorm:"bigint"`
	Disabled   bool   `json:"disabled"`
	RetryTimes *int   `json:"retry_times_override,omitempty"`
}

// Each public group has an independent, complete user routing pool.
type UserGroupChannelRoutingOverride struct {
	UserChannelRoutingOverride `gorm:"embedded"`
	RoutingGroup               string `json:"routing_group" gorm:"type:varchar(64);primaryKey;autoIncrement:false"`
}

// Strategy overrides are independent of channel membership. Removing one
// restores the model/public-group strategy without changing the user's pool.
type UserRoutingStrategy struct {
	UserId       int    `json:"-" gorm:"primaryKey;autoIncrement:false"`
	Model        string `json:"model" gorm:"type:varchar(255);primaryKey;autoIncrement:false"`
	RoutingGroup string `json:"routing_group" gorm:"type:varchar(64);primaryKey;autoIncrement:false"`
	Mode         string `json:"mode" gorm:"type:varchar(16);not null"`
	Revision     string `json:"revision" gorm:"type:varchar(64);not null"`
	MaxTTFTMS    int64  `json:"max_ttft_ms" gorm:"column:max_ttft_ms"`
	MaxTPOTMS    int64  `json:"max_tpot_ms" gorm:"column:max_tpot_ms"`
}

func (strategy *UserRoutingStrategy) UsesDynamicRouting() bool {
	return strategy != nil && (strategy.Mode == RoutingModeSmart || strategy.Mode == RoutingModeDiscount)
}

func ListUserRoutingStrategies(userID int, modelName string) ([]UserRoutingStrategy, error) {
	rows := make([]UserRoutingStrategy, 0)
	if userID <= 0 {
		return rows, nil
	}
	query := DB.Where("user_id = ?", userID)
	if modelName != "" {
		query = query.Where("model = ?", modelName)
	}
	err := query.Order("model, routing_group").Find(&rows).Error
	return rows, err
}

func SaveUserRoutingStrategy(strategy *UserRoutingStrategy, expectedRevision string) error {
	if strategy == nil || strategy.UserId <= 0 || strategy.Model == "" || len(strategy.Model) > 255 || strings.TrimSpace(strategy.Model) != strategy.Model || strings.ContainsAny(strategy.Model, "\x00\r\n") || strategy.RoutingGroup == "" || strategy.RoutingGroup == "auto" || len(strategy.RoutingGroup) > 64 || strings.TrimSpace(strategy.RoutingGroup) != strategy.RoutingGroup || strings.ContainsAny(strategy.RoutingGroup, ",\x00\r\n") || !slices.Contains([]string{"inherit", RoutingModePriority, RoutingModeSmart, RoutingModeDiscount}, strategy.Mode) || strategy.MaxTTFTMS < 0 || strategy.MaxTTFTMS > 3600000 || strategy.MaxTPOTMS < 0 || strategy.MaxTPOTMS > 60000 {
		return ErrInvalidUserChannelRouting
	}
	if strategy.Mode == RoutingModeDiscount {
		if strategy.MaxTTFTMS == 0 {
			strategy.MaxTTFTMS = DefaultDiscountMaxTTFTMS
		}
		if strategy.MaxTPOTMS == 0 {
			strategy.MaxTPOTMS = DefaultDiscountMaxTPOTMS
		}
	} else {
		strategy.MaxTTFTMS, strategy.MaxTPOTMS = 0, 0
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Select("id").First(&user, strategy.UserId).Error; err != nil {
			return err
		}
		query := tx.Where("user_id = ? AND model = ? AND routing_group = ?", strategy.UserId, strategy.Model, strategy.RoutingGroup)
		var current UserRoutingStrategy
		err := query.First(&current).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if current.Revision != expectedRevision {
			return ErrUserChannelRoutingConflict
		}
		if strategy.Mode == "inherit" {
			return query.Delete(&UserRoutingStrategy{}).Error
		}
		strategy.Revision = common.GetUUID()
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "model"}, {Name: "routing_group"}},
			DoUpdates: clause.AssignmentColumns([]string{"mode", "revision", "max_ttft_ms", "max_tpot_ms"}),
		}).Create(strategy).Error
	})
}

func ListUserGroupChannelRoutingOverrides(userID int, modelName string) ([]UserGroupChannelRoutingOverride, error) {
	rows := make([]UserGroupChannelRoutingOverride, 0)
	if userID <= 0 {
		return rows, nil
	}
	query := DB.Where("user_id = ?", userID)
	if modelName != "" {
		query = query.Where("model = ?", modelName)
	}
	err := query.Order("model, routing_group, channel_id").Find(&rows).Error
	return rows, err
}

type UserChannelRoutingPatch struct {
	ChannelId  int    `json:"channel_id"`
	Priority   *int64 `json:"priority_override"`
	Weight     *uint  `json:"weight_override"`
	Enabled    *bool  `json:"enabled"`
	RetryTimes *int   `json:"retry_times_override"`
}

var ErrUserChannelRoutingConflict = errors.New("user channel routing changed; reload the configuration")
var ErrInvalidUserChannelRouting = errors.New("invalid user channel routing")

func ListUserChannelRoutingOverrides(userID int, modelName string) ([]UserChannelRoutingOverride, error) {
	rows := make([]UserChannelRoutingOverride, 0)
	if userID <= 0 {
		return rows, nil
	}
	query := DB.Where("user_id = ?", userID)
	if modelName != "" {
		query = query.Where("model = ?", modelName)
	}
	err := query.Order("model, channel_id").Find(&rows).Error
	return rows, err
}

func UserChannelRoutingRevision(rows []UserChannelRoutingOverride) (string, error) {
	encoded, err := common.Marshal(rows)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(encoded)), nil
}

// PatchUserChannelRouting atomically replaces a complete pool with optimistic
// concurrency. Empty pools restore defaults; enabled members with nil values
// inherit channel values without losing their explicit membership.
func PatchUserChannelRouting(userID int, modelName, revision string, patches []UserChannelRoutingPatch) error {
	return PatchUserChannelRoutingForGroup(userID, modelName, "", revision, patches)
}

func PatchUserChannelRoutingForGroup(userID int, modelName, routingGroup, revision string, patches []UserChannelRoutingPatch) error {
	if userID <= 0 || modelName == "" || strings.TrimSpace(modelName) != modelName || len(modelName) > 255 || revision == "" || len(patches) > 1000 {
		return ErrInvalidUserChannelRouting
	}
	if len(routingGroup) > 64 || strings.TrimSpace(routingGroup) != routingGroup || routingGroup == "auto" || strings.ContainsAny(routingGroup, ",\x00\r\n") {
		return ErrInvalidUserChannelRouting
	}
	seen := make(map[int]struct{}, len(patches))
	for _, patch := range patches {
		_, duplicate := seen[patch.ChannelId]
		if patch.ChannelId <= 0 || duplicate || patch.Enabled == nil || (patch.Priority != nil && (*patch.Priority < 0 || *patch.Priority > 2147483647)) || (patch.Weight != nil && *patch.Weight > MaxChannelWeight) || (patch.RetryTimes != nil && (*patch.RetryTimes < 0 || *patch.RetryTimes > 99)) {
			return ErrInvalidUserChannelRouting
		}
		seen[patch.ChannelId] = struct{}{}
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Select("id").First(&user, userID).Error; err != nil {
			return err
		}
		rows := make([]UserChannelRoutingOverride, 0)
		query := tx.Where("user_id = ? AND model = ?", userID, modelName)
		if routingGroup == "" {
			query = query.Model(&UserChannelRoutingOverride{})
		} else {
			query = query.Model(&UserGroupChannelRoutingOverride{}).Where("routing_group = ?", routingGroup)
		}
		if err := query.Order("model, channel_id").Find(&rows).Error; err != nil {
			return err
		}
		current, err := UserChannelRoutingRevision(rows)
		if err != nil {
			return err
		}
		if current != revision {
			return ErrUserChannelRoutingConflict
		}
		ids := make([]int, 0, len(patches))
		for _, patch := range patches {
			ids = append(ids, patch.ChannelId)
			var channel Channel
			if err := tx.Select("id", "models", "group").First(&channel, patch.ChannelId).Error; err != nil {
				return err
			}
			models := normalizeChannelModels(&channel)
			if !slices.Contains(models, modelName) && !slices.Contains(models, ratio_setting.RoutingMatchModelName(modelName)) {
				return fmt.Errorf("%w: channel %d does not support model %s", ErrInvalidUserChannelRouting, patch.ChannelId, modelName)
			}
			if routingGroup != "" && !slices.Contains(channel.GetGroups(), routingGroup) {
				return fmt.Errorf("%w: channel %d does not belong to group %s", ErrInvalidUserChannelRouting, patch.ChannelId, routingGroup)
			}
			row := UserChannelRoutingOverride{UserId: userID, Model: modelName, ChannelId: patch.ChannelId, Priority: patch.Priority, Weight: patch.Weight, Disabled: !*patch.Enabled, RetryTimes: patch.RetryTimes}
			columns := []clause.Column{{Name: "user_id"}, {Name: "model"}, {Name: "channel_id"}}
			var record any = &row
			if routingGroup != "" {
				columns = append(columns, clause.Column{Name: "routing_group"})
				record = &UserGroupChannelRoutingOverride{UserChannelRoutingOverride: row, RoutingGroup: routingGroup}
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   columns,
				DoUpdates: clause.AssignmentColumns([]string{"priority", "weight", "disabled", "retry_times"}),
			}).Create(record).Error; err != nil {
				return err
			}
		}
		if len(ids) > 0 {
			query = query.Where("channel_id NOT IN ?", ids)
		}
		return query.Delete(query.Statement.Model).Error
	})
}

// Reset removes the complete scoped overlay, including stale channel records.
// The same user lock and revision as edits protect concurrent administrators.
func ResetUserChannelRoutingForGroup(userID int, modelName, routingGroup, revision string) error {
	if userID <= 0 || modelName == "" || len(modelName) > 255 || strings.TrimSpace(modelName) != modelName || revision == "" || len(routingGroup) > 64 || strings.TrimSpace(routingGroup) != routingGroup || routingGroup == "auto" || strings.ContainsAny(routingGroup, ",\x00\r\n") {
		return ErrInvalidUserChannelRouting
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Select("id").First(&user, userID).Error; err != nil {
			return err
		}
		query := tx.Where("user_id = ? AND model = ?", userID, modelName)
		var record any = &UserChannelRoutingOverride{}
		if routingGroup != "" {
			record = &UserGroupChannelRoutingOverride{}
			query = query.Where("routing_group = ?", routingGroup)
		}
		rows := make([]UserChannelRoutingOverride, 0)
		if err := query.Model(record).Order("model, channel_id").Find(&rows).Error; err != nil {
			return err
		}
		current, err := UserChannelRoutingRevision(rows)
		if err != nil {
			return err
		}
		if current != revision {
			return ErrUserChannelRoutingConflict
		}
		return query.Delete(record).Error
	})
}
