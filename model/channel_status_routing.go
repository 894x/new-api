package model

import (
	"cmp"
	"context"
	"slices"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/dynamic_routing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
)

// ChannelStatusRoutingPool describes a configured membership, rather than
// repeating an inherited default pool for every customer using it.
type ChannelStatusRoutingPool struct {
	Source       string `json:"source"`
	UserID       int    `json:"user_id"`
	Username     string `json:"username"`
	ChannelID    int    `json:"channel_id"`
	Model        string `json:"model"`
	RoutingGroup string `json:"routing_group"`
	Priority     *int64 `json:"priority"`
	Enabled      bool   `json:"enabled"`
}

func LoadChannelStatusRoutingPools(ctx context.Context, ids []int, operatorRole int) ([]ChannelStatusRoutingPool, error) {
	result := make([]ChannelStatusRoutingPool, 0)
	if len(ids) == 0 {
		return result, nil
	}
	db := DB.WithContext(ctx)
	selected := make(map[int]bool, len(ids))
	for _, id := range ids {
		selected[id] = true
	}
	type poolKey struct{ group, model string }
	configured := make(map[poolKey]bool)
	var pools []ModelRoutingPool
	if err := db.Order("routing_group, model").Find(&pools).Error; err != nil {
		return nil, err
	}
	for _, pool := range pools {
		configured[poolKey{pool.RoutingGroup, pool.Model}] = true
		if err := common.UnmarshalJsonStr(pool.ChannelsJSON, &pool.Channels); err != nil {
			return nil, err
		}
		for _, entry := range pool.Channels {
			if selected[entry.ChannelID] {
				result = append(result, ChannelStatusRoutingPool{Source: "default_pool", ChannelID: entry.ChannelID, Model: pool.Model, RoutingGroup: pool.RoutingGroup, Priority: common.GetPointer(entry.Priority), Enabled: entry.Enabled})
			}
		}
	}
	// Unconfigured pools inherit channel/model defaults. Their ability rows
	// already have a unique group/model/channel key and need no user expansion.
	var abilities []Ability
	if err := db.Where("channel_id IN ?", ids).Find(&abilities).Error; err != nil {
		return nil, err
	}
	for _, ability := range abilities {
		if !configured[poolKey{ability.Group, ability.Model}] && !configured[poolKey{ability.Group, ratio_setting.RoutingMatchModelName(ability.Model)}] {
			result = append(result, ChannelStatusRoutingPool{Source: "channel_defaults", ChannelID: ability.ChannelId, Model: ability.Model, RoutingGroup: ability.Group, Priority: common.GetPointer(effectiveAbilityPriority(&ability)), Enabled: ability.Enabled})
		}
	}
	users := db.Model(&User{}).Where("(id IN (?) OR id IN (?))",
		db.Model(&UserGroupChannelRoutingOverride{}).Select("user_id").Where("channel_id IN ?", ids),
		db.Model(&UserChannelRoutingOverride{}).Select("user_id").Where("channel_id IN ?", ids))
	if operatorRole != common.RoleRootUser {
		users = users.Where("role < ?", operatorRole)
	}
	var visible []User
	if err := users.Select("id", "username").Find(&visible).Error; err != nil {
		return nil, err
	}
	byID := make(map[int]string, len(visible))
	for _, user := range visible {
		byID[user.Id] = user.Username
	}
	// Include inactive customers and customers without keys: their stored pool
	// memberships must also stay disabled when the channel is enabled later.
	var overrides []UserGroupChannelRoutingOverride
	if err := db.Where("channel_id IN ?", ids).Where("user_id IN (?)", users.Select("id")).Find(&overrides).Error; err != nil {
		return nil, err
	}
	for _, row := range overrides {
		result = append(result, ChannelStatusRoutingPool{Source: "user", UserID: row.UserId, Username: byID[row.UserId], ChannelID: row.ChannelId, Model: row.Model, RoutingGroup: row.RoutingGroup, Priority: row.Priority, Enabled: !row.Disabled})
	}
	var legacy []UserChannelRoutingOverride
	if err := db.Where("channel_id IN ?", ids).Where("user_id IN (?)", users.Select("id")).Find(&legacy).Error; err != nil {
		return nil, err
	}
	for _, row := range legacy {
		result = append(result, ChannelStatusRoutingPool{Source: "legacy_user", UserID: row.UserId, Username: byID[row.UserId], ChannelID: row.ChannelId, Model: row.Model, Priority: row.Priority, Enabled: !row.Disabled})
	}
	slices.SortFunc(result, func(a, b ChannelStatusRoutingPool) int {
		return cmp.Or(cmp.Compare(a.UserID, b.UserID), cmp.Compare(a.Model, b.Model), cmp.Compare(a.RoutingGroup, b.RoutingGroup), cmp.Compare(a.ChannelID, b.ChannelID), cmp.Compare(a.Source, b.Source))
	})
	return result, nil
}

// SetChannelsAndRoutingPoolsEnabled commits global status, default membership and
// visible user overrides together. It preserves membership and routing values.
func SetChannelsAndRoutingPoolsEnabled(ctx context.Context, ids []int, operatorRole int, enabled bool) (int, error) {
	targetStatus := common.ChannelStatusManuallyDisabled
	if enabled {
		targetStatus = common.ChannelStatusEnabled
	}
	ids = slices.Clone(ids)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	channelStatusLock.Lock()
	defer channelStatusLock.Unlock()
	for _, id := range ids {
		lock := GetChannelPollingLock(id)
		lock.Lock()
		defer lock.Unlock()
	}
	dynamicRoutingOptionsMu.Lock()
	defer dynamicRoutingOptionsMu.Unlock()
	selected := make(map[int]bool, len(ids))
	for _, id := range ids {
		selected[id] = true
	}
	changed := 0
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Share the user lock with pool editors so a stale edit cannot silently
		// overwrite the disabled memberships after this transaction commits.
		users := tx.Model(&User{}).Where("(id IN (?) OR id IN (?))",
			tx.Model(&UserGroupChannelRoutingOverride{}).Select("user_id").Where("channel_id IN ?", ids),
			tx.Model(&UserChannelRoutingOverride{}).Select("user_id").Where("channel_id IN ?", ids))
		if operatorRole != common.RoleRootUser {
			users = users.Where("role < ?", operatorRole)
		}
		var visible []User
		if err := lockForUpdate(users).Select("id").Order("id").Find(&visible).Error; err != nil {
			return err
		}
		userIDs := make([]int, 0, len(visible))
		for _, user := range visible {
			userIDs = append(userIDs, user.Id)
		}
		var pools []ModelRoutingPool
		if err := lockForUpdate(tx).Order("routing_group, model").Find(&pools).Error; err != nil {
			return err
		}
		type poolKey struct{ group, model string }
		configured := make(map[poolKey]bool, len(pools))
		for _, pool := range pools {
			configured[poolKey{pool.RoutingGroup, pool.Model}] = true
		}
		var selectedAbilities []Ability
		if err := tx.Where("channel_id IN ?", ids).Order("model, channel_id").Find(&selectedAbilities).Error; err != nil {
			return err
		}
		// Persist previously implicit defaults as a complete pool so enabling
		// the channel later does not silently re-enable its pool membership.
		for _, ability := range selectedAbilities {
			key := poolKey{ability.Group, ability.Model}
			if enabled || configured[key] || configured[poolKey{ability.Group, ratio_setting.RoutingMatchModelName(ability.Model)}] {
				continue
			}
			var members []Ability
			if err := tx.Where(map[string]any{"group": ability.Group, "model": ability.Model}).Order("channel_id").Find(&members).Error; err != nil {
				return err
			}
			pool := ModelRoutingPool{RoutingGroup: ability.Group, Model: ability.Model, Mode: RoutingModePriority, Revision: common.GetUUID(), Channels: make([]ModelRoutingPoolChannel, 0, len(members))}
			if dynamic_routing_setting.GetSnapshot().Config.Enabled {
				pool.Mode = RoutingModeSmart
			}
			for _, member := range members {
				pool.Channels = append(pool.Channels, ModelRoutingPoolChannel{ChannelID: member.ChannelId, Priority: effectiveAbilityPriority(&member), Weight: member.Weight, Enabled: !selected[member.ChannelId]})
			}
			encoded, err := common.Marshal(pool.Channels)
			if err != nil {
				return err
			}
			pool.ChannelsJSON = string(encoded)
			if err := tx.Create(&pool).Error; err != nil {
				return err
			}
			configured[key] = true
		}
		for _, pool := range pools {
			if err := common.UnmarshalJsonStr(pool.ChannelsJSON, &pool.Channels); err != nil {
				return err
			}
			updated := false
			for i := range pool.Channels {
				if selected[pool.Channels[i].ChannelID] && pool.Channels[i].Enabled != enabled {
					pool.Channels[i].Enabled = enabled
					updated = true
				}
			}
			if !updated {
				continue
			}
			encoded, err := common.Marshal(pool.Channels)
			if err != nil {
				return err
			}
			if err := tx.Model(&ModelRoutingPool{}).Where("routing_group = ? AND model = ? AND revision = ?", pool.RoutingGroup, pool.Model, pool.Revision).
				Updates(map[string]any{"channels_json": string(encoded), "revision": common.GetUUID()}).Error; err != nil {
				return err
			}
		}
		for _, record := range []any{&UserChannelRoutingOverride{}, &UserGroupChannelRoutingOverride{}} {
			if len(userIDs) > 0 {
				if err := tx.Model(record).Where("channel_id IN ? AND user_id IN ?", ids, userIDs).Update("disabled", !enabled).Error; err != nil {
					return err
				}
			}
		}
		var channels []Channel
		if err := lockForUpdate(tx).Where("id IN ?", ids).Order("id").Find(&channels).Error; err != nil {
			return err
		}
		for _, channel := range channels {
			if channel.Status == targetStatus {
				continue
			}
			channel.Status = targetStatus
			info := channel.GetOtherInfo()
			info["status_reason"] = "manual disable including routing pools"
			if enabled {
				info["status_reason"] = "manual enable including routing pools"
			}
			info["status_time"] = common.GetTimestamp()
			channel.SetOtherInfo(info)
			if err := channel.saveStatusStateWithDB(tx); err != nil {
				return err
			}
			changed++
		}
		return tx.Model(&Ability{}).Where("channel_id IN ?", ids).Update("enabled", enabled).Error
	})
	if err != nil {
		return 0, err
	}
	return changed, nil
}
