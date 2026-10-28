package model

import (
	"errors"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ResourceGroupChannel struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type ResourceGroupConflict struct {
	GroupName string                 `json:"group_name"`
	Ratio     float64                `json:"ratio"`
	Channels  []ResourceGroupChannel `json:"channels"`
}

func (conflict *ResourceGroupConflict) Error() string {
	return "Resource group already exists. Confirm reuse or choose another name."
}

type ChannelResourceGroupResult struct {
	GroupName     string   `json:"group_name"`
	ChannelGroups []string `json:"channel_groups"`
	Created       bool     `json:"created"`
	Ratio         float64  `json:"ratio"`
}

// CreateChannelResourceGroup registers a private pricing group and appends it to
// the channel in the same transaction. Existing groups require explicit reuse.
func CreateChannelResourceGroup(channelID int, name string, reuseExisting bool) (*ChannelResourceGroupResult, error) {
	name = strings.TrimSpace(name)
	suffix, prefixed := strings.CutPrefix(name, "资源-")
	if channelID <= 0 || !prefixed || strings.TrimSpace(suffix) == "" ||
		utf8.RuneCountInString(name) > 64 || strings.ContainsRune(name, ',') || strings.ContainsFunc(name, unicode.IsControl) {
		return nil, errors.New("Resource group names must start with 资源-, contain a suffix, and use at most 64 characters without commas or control characters.")
	}

	groupPricingOptionMutex.Lock()
	defer groupPricingOptionMutex.Unlock()

	var result ChannelResourceGroupResult
	var groupRatiosJSON, tieredRatiosJSON string
	err := DB.Transaction(func(tx *gorm.DB) error {
		// The persisted registry is the merge source, including changes made by
		// another gateway node before this request acquired the row lock.
		registry := Option{Key: "GroupRatio", Value: ratio_setting.GroupRatio2JSONString()}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&registry).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where(&Option{Key: "GroupRatio"}).First(&registry).Error; err != nil {
			return err
		}
		values := map[string]string{
			"UserUsableGroups": setting.UserUsableGroups2JSONString(),
			"AutoGroups":       setting.AutoGroups2JsonString(),
			"group_ratio_setting.group_special_usable_group": ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.MarshalJSONString(),
			ratio_setting.ModelTieredRatiosOptionKey:         ratio_setting.ModelTieredRatios2JSONString(),
		}
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		var options []Option
		if err := tx.Where(map[string]any{"key": keys}).Find(&options).Error; err != nil {
			return err
		}
		for _, option := range options {
			values[option.Key] = option.Value
		}
		var ratios map[string]float64
		var usable map[string]string
		var special map[string]map[string]string
		var auto []string
		if err := common.UnmarshalJsonStr(registry.Value, &ratios); err != nil {
			return err
		}
		if ratios == nil {
			return errors.New("The pricing group registry is invalid.")
		}
		if err := common.UnmarshalJsonStr(values["UserUsableGroups"], &usable); err != nil {
			return err
		}
		if err := common.UnmarshalJsonStr(values["group_ratio_setting.group_special_usable_group"], &special); err != nil {
			return err
		}
		if err := common.UnmarshalJsonStr(values["AutoGroups"], &auto); err != nil {
			return err
		}
		if _, public := usable[name]; public || slices.Contains(auto, name) {
			return errors.New("This group is publicly selectable or used by automatic routing. Choose another resource group name.")
		}
		for _, rules := range special {
			for key := range rules {
				key = strings.TrimPrefix(key, "+:")
				if key == name {
					return errors.New("This group has a visibility rule. Choose another resource group name.")
				}
			}
		}
		var channel Channel
		if err := lockForUpdate(tx).First(&channel, channelID).Error; err != nil {
			return err
		}
		groups := channel.GetGroups()
		ratio, exists := ratios[name]
		if !exists {
			ratio = 1
		}
		if !slices.Contains(groups, name) {
			// Check exact group membership rather than matching substrings or SQL
			// LIKE patterns; names may contain underscores and percent signs.
			var channels []Channel
			if err := tx.Select("id", "name", "group").Order("id").Find(&channels).Error; err != nil {
				return err
			}
			associated := make([]ResourceGroupChannel, 0)
			for _, candidate := range channels {
				if slices.Contains(candidate.GetGroups(), name) {
					associated = append(associated, ResourceGroupChannel{ID: candidate.Id, Name: candidate.Name})
				}
			}
			if (exists || len(associated) > 0) && !reuseExisting {
				return &ResourceGroupConflict{GroupName: name, Ratio: ratio, Channels: associated}
			}
			groups = append(groups, name)
		}
		joined := strings.Join(groups, ",")
		if utf8.RuneCountInString(joined) > 64 {
			return errors.New("The channel's combined group names exceed the 64-character limit. Shorten the resource group name.")
		}
		ratios[name] = ratio
		encoded, err := common.Marshal(ratios)
		if err != nil {
			return err
		}
		groupRatiosJSON = string(encoded)
		tieredRatiosJSON = values[ratio_setting.ModelTieredRatiosOptionKey]
		if err := ratio_setting.CheckGroupPricingConfiguration(groupRatiosJSON, tieredRatiosJSON); err != nil {
			return err
		}
		if err := tx.Model(&registry).Update("value", groupRatiosJSON).Error; err != nil {
			return err
		}
		if channel.Group != joined {
			if err := tx.Model(&channel).Update("group", joined).Error; err != nil {
				return err
			}
			channel.Group = joined
			if err := channel.UpdateAbilities(tx); err != nil {
				return err
			}
		}
		result = ChannelResourceGroupResult{GroupName: name, ChannelGroups: groups, Created: !exists, Ratio: ratio}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := applyGroupPricingOptions(groupRatiosJSON, tieredRatiosJSON); err != nil {
		return nil, err
	}
	return &result, nil
}
