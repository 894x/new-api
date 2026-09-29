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

// UserChannelRoutingOverride is a sparse user/public-model/channel policy.
// Disabled retains the priority so re-enabling restores the user's ordering.
type UserChannelRoutingOverride struct {
	UserId    int    `json:"-" gorm:"primaryKey;autoIncrement:false"`
	Model     string `json:"model" gorm:"type:varchar(255);primaryKey;autoIncrement:false"`
	ChannelId int    `json:"channel_id" gorm:"primaryKey;autoIncrement:false"`
	Priority  *int64 `json:"priority_override" gorm:"bigint"`
	Disabled  bool   `json:"disabled"`
}

type UserChannelRoutingPatch struct {
	ChannelId int    `json:"channel_id"`
	Priority  *int64 `json:"priority_override"`
	Enabled   *bool  `json:"enabled"`
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

// PatchUserChannelRouting atomically patches final row values, with optimistic
// concurrency per user/model. Omitted rows are untouched; enabled+nil inherits.
func PatchUserChannelRouting(userID int, modelName, revision string, patches []UserChannelRoutingPatch) error {
	if userID <= 0 || modelName == "" || strings.TrimSpace(modelName) != modelName || len(modelName) > 255 || revision == "" || len(patches) == 0 || len(patches) > 1000 {
		return ErrInvalidUserChannelRouting
	}
	seen := make(map[int]struct{}, len(patches))
	for _, patch := range patches {
		_, duplicate := seen[patch.ChannelId]
		if patch.ChannelId <= 0 || duplicate || patch.Enabled == nil || (patch.Priority != nil && (*patch.Priority < 0 || *patch.Priority > 2147483647)) {
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
		if err := tx.Where("user_id = ? AND model = ?", userID, modelName).Order("model, channel_id").Find(&rows).Error; err != nil {
			return err
		}
		current, err := UserChannelRoutingRevision(rows)
		if err != nil {
			return err
		}
		if current != revision {
			return ErrUserChannelRoutingConflict
		}
		for _, patch := range patches {
			if patch.Priority == nil && *patch.Enabled {
				if err := tx.Where("user_id = ? AND model = ? AND channel_id = ?", userID, modelName, patch.ChannelId).Delete(&UserChannelRoutingOverride{}).Error; err != nil {
					return err
				}
				continue
			}
			var channel Channel
			if err := tx.Select("id", "models").First(&channel, patch.ChannelId).Error; err != nil {
				return err
			}
			models := normalizeChannelModels(&channel)
			if !slices.Contains(models, modelName) && !slices.Contains(models, ratio_setting.RoutingMatchModelName(modelName)) {
				return fmt.Errorf("%w: channel %d does not support model %s", ErrInvalidUserChannelRouting, patch.ChannelId, modelName)
			}
			row := UserChannelRoutingOverride{UserId: userID, Model: modelName, ChannelId: patch.ChannelId, Priority: patch.Priority, Disabled: !*patch.Enabled}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "user_id"}, {Name: "model"}, {Name: "channel_id"}},
				DoUpdates: clause.AssignmentColumns([]string{"priority", "disabled"}),
			}).Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
