package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrInvalidChannelSettings = errors.New("invalid channel settings")

// ChannelModelOverride stores sparse routing overrides for one model on one
// channel. A nil field inherits the channel-level default; an all-nil record is
// deleted instead of being persisted.
type ChannelModelOverride struct {
	ChannelId int    `json:"channel_id" gorm:"primaryKey;autoIncrement:false;index"`
	Model     string `json:"model" gorm:"type:varchar(255);primaryKey;autoIncrement:false;index"`
	Priority  *int64 `json:"priority_override" gorm:"bigint"`
	Weight    *uint  `json:"weight_override"`
}

type ChannelModelOverridePatch struct {
	ChannelId int    `json:"channel_id"`
	Model     string `json:"model"`
	Priority  *int64 `json:"priority_override"`
	Weight    *uint  `json:"weight_override"`
}

type ChannelModelRouting struct {
	ChannelId         int    `json:"channel_id"`
	ChannelName       string `json:"channel_name"`
	ChannelType       int    `json:"channel_type"`
	ChannelStatus     int    `json:"channel_status"`
	Model             string `json:"model"`
	DefaultPriority   int64  `json:"default_priority"`
	DefaultWeight     uint   `json:"default_weight"`
	PriorityOverride  *int64 `json:"priority_override"`
	WeightOverride    *uint  `json:"weight_override"`
	EffectivePriority int64  `json:"effective_priority"`
	EffectiveWeight   uint   `json:"effective_weight"`
}

func normalizeChannelModels(channel *Channel) []string {
	if channel == nil || channel.Models == "" {
		return nil
	}
	seen := make(map[string]struct{})
	models := make([]string, 0)
	for _, modelName := range strings.Split(channel.Models, ",") {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		if _, ok := seen[modelName]; ok {
			continue
		}
		seen[modelName] = struct{}{}
		models = append(models, modelName)
	}
	return models
}

func channelDefaultWeight(channel *Channel) uint {
	if channel == nil || channel.Weight == nil {
		return 0
	}
	return *channel.Weight
}

func getChannelModelOverrideMap(tx *gorm.DB, channelId int) (map[string]ChannelModelOverride, error) {
	var overrides []ChannelModelOverride
	if err := tx.Where("channel_id = ?", channelId).Find(&overrides).Error; err != nil {
		return nil, err
	}
	result := make(map[string]ChannelModelOverride, len(overrides))
	for _, override := range overrides {
		result[override.Model] = override
	}
	return result, nil
}

func effectiveChannelModelRouting(channel *Channel, modelName string, override *ChannelModelOverride) ChannelModelRouting {
	defaultPriority := channel.GetPriority()
	defaultWeight := channelDefaultWeight(channel)
	effectivePriority := defaultPriority
	effectiveWeight := defaultWeight
	var priorityOverride *int64
	var weightOverride *uint
	if override != nil {
		priorityOverride = override.Priority
		weightOverride = override.Weight
		if override.Priority != nil {
			effectivePriority = *override.Priority
		}
		if override.Weight != nil {
			effectiveWeight = *override.Weight
		}
	}
	return ChannelModelRouting{
		ChannelId:         channel.Id,
		ChannelName:       channel.Name,
		ChannelType:       channel.Type,
		ChannelStatus:     channel.Status,
		Model:             modelName,
		DefaultPriority:   defaultPriority,
		DefaultWeight:     defaultWeight,
		PriorityOverride:  priorityOverride,
		WeightOverride:    weightOverride,
		EffectivePriority: effectivePriority,
		EffectiveWeight:   effectiveWeight,
	}
}

func ListChannelModelRoutings(channelId int) ([]ChannelModelRouting, error) {
	channel, err := GetChannelById(channelId, false)
	if err != nil {
		return nil, err
	}
	overrides, err := getChannelModelOverrideMap(DB, channelId)
	if err != nil {
		return nil, err
	}
	models := normalizeChannelModels(channel)
	routings := make([]ChannelModelRouting, 0, len(models))
	for _, modelName := range models {
		override, ok := overrides[modelName]
		if ok {
			routings = append(routings, effectiveChannelModelRouting(channel, modelName, &override))
			continue
		}
		routings = append(routings, effectiveChannelModelRouting(channel, modelName, nil))
	}
	return routings, nil
}

func ListModelChannelRoutings(modelName string) ([]ChannelModelRouting, error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return nil, fmt.Errorf("model cannot be empty")
	}
	var channelIds []int
	if err := DB.Model(&Ability{}).
		Where("model = ?", modelName).
		Distinct("channel_id").
		Pluck("channel_id", &channelIds).Error; err != nil {
		return nil, err
	}
	if len(channelIds) == 0 {
		return []ChannelModelRouting{}, nil
	}
	var channels []*Channel
	if err := DB.Where("id IN ?", channelIds).Find(&channels).Error; err != nil {
		return nil, err
	}
	var overrides []ChannelModelOverride
	if err := DB.Where("channel_id IN ? AND model = ?", channelIds, modelName).Find(&overrides).Error; err != nil {
		return nil, err
	}
	overrideByChannel := make(map[int]ChannelModelOverride, len(overrides))
	for _, override := range overrides {
		overrideByChannel[override.ChannelId] = override
	}
	routings := make([]ChannelModelRouting, 0, len(channels))
	for _, channel := range channels {
		override, ok := overrideByChannel[channel.Id]
		if ok {
			routings = append(routings, effectiveChannelModelRouting(channel, modelName, &override))
			continue
		}
		routings = append(routings, effectiveChannelModelRouting(channel, modelName, nil))
	}
	sort.Slice(routings, func(i, j int) bool {
		if routings[i].EffectivePriority != routings[j].EffectivePriority {
			return routings[i].EffectivePriority > routings[j].EffectivePriority
		}
		return routings[i].ChannelId < routings[j].ChannelId
	})
	return routings, nil
}

func validateChannelModelOverridePatch(channel *Channel, patch ChannelModelOverridePatch) (ChannelModelOverride, error) {
	if err := ValidateChannelWeight(channel.Weight); err != nil {
		return ChannelModelOverride{}, err
	}
	patch.Model = strings.TrimSpace(patch.Model)
	if patch.Model == "" {
		return ChannelModelOverride{}, fmt.Errorf("model cannot be empty")
	}
	if len(patch.Model) > 255 {
		return ChannelModelOverride{}, fmt.Errorf("model name exceeds 255 bytes")
	}
	supported := false
	for _, modelName := range normalizeChannelModels(channel) {
		if modelName == patch.Model {
			supported = true
			break
		}
	}
	if !supported {
		return ChannelModelOverride{}, fmt.Errorf("channel %d does not support model %s", channel.Id, patch.Model)
	}
	if patch.Weight != nil && *patch.Weight > MaxChannelWeight {
		return ChannelModelOverride{}, fmt.Errorf("weight override exceeds %d", MaxChannelWeight)
	}
	return ChannelModelOverride{
		ChannelId: channel.Id,
		Model:     patch.Model,
		Priority:  patch.Priority,
		Weight:    patch.Weight,
	}, nil
}

// PatchChannelModelOverrides atomically applies final sparse override states.
// Pairs not included in patches remain unchanged. A patch with both override
// fields nil clears the pair and restores channel-level inheritance.
func PatchChannelModelOverrides(patches []ChannelModelOverridePatch) error {
	if len(patches) == 0 {
		return nil
	}
	channelIds := make([]int, 0, len(patches))
	seenChannels := make(map[int]struct{})
	seenPairs := make(map[string]struct{}, len(patches))
	for _, patch := range patches {
		if patch.ChannelId <= 0 {
			return fmt.Errorf("channel id must be positive")
		}
		pairKey := fmt.Sprintf("%d\x00%s", patch.ChannelId, strings.TrimSpace(patch.Model))
		if _, ok := seenPairs[pairKey]; ok {
			return fmt.Errorf("duplicate channel-model override: %d/%s", patch.ChannelId, strings.TrimSpace(patch.Model))
		}
		seenPairs[pairKey] = struct{}{}
		if _, ok := seenChannels[patch.ChannelId]; ok {
			continue
		}
		seenChannels[patch.ChannelId] = struct{}{}
		channelIds = append(channelIds, patch.ChannelId)
	}

	sort.Ints(channelIds)
	tx := DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	var channels []*Channel
	if err := lockForUpdate(tx).Where("id IN ?", channelIds).Order("id").Find(&channels).Error; err != nil {
		tx.Rollback()
		return err
	}
	if len(channels) != len(channelIds) {
		tx.Rollback()
		return fmt.Errorf("one or more channels do not exist")
	}
	channelById := make(map[int]*Channel, len(channels))
	for _, channel := range channels {
		channelById[channel.Id] = channel
	}
	validated := make([]ChannelModelOverride, 0, len(patches))
	for _, patch := range patches {
		override, err := validateChannelModelOverridePatch(channelById[patch.ChannelId], patch)
		if err != nil {
			tx.Rollback()
			return err
		}
		validated = append(validated, override)
	}

	for _, override := range validated {
		if override.Priority == nil && override.Weight == nil {
			if err := tx.Where("channel_id = ? AND model = ?", override.ChannelId, override.Model).
				Delete(&ChannelModelOverride{}).Error; err != nil {
				tx.Rollback()
				return err
			}
			continue
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "channel_id"}, {Name: "model"}},
			DoUpdates: clause.AssignmentColumns([]string{"priority", "weight"}),
		}).Create(&override).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	for _, channel := range channels {
		if err := channel.UpdateAbilities(tx); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit().Error
}

func pruneChannelModelOverrides(tx *gorm.DB, channel *Channel) error {
	models := normalizeChannelModels(channel)
	query := tx.Where("channel_id = ?", channel.Id)
	if len(models) > 0 {
		query = query.Where("model NOT IN ?", models)
	}
	return query.Delete(&ChannelModelOverride{}).Error
}

func CloneChannelWithModelOverrides(sourceChannelId int, suffix string, resetBalance bool) (*Channel, error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	var sourceChannel Channel
	if err := lockForUpdate(tx).First(&sourceChannel, sourceChannelId).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := ValidateChannelWeight(sourceChannel.Weight); err != nil {
		tx.Rollback()
		return nil, err
	}

	clone := sourceChannel
	clone.Id = 0
	clone.CreatedTime = common.GetTimestamp()
	clone.Name = sourceChannel.Name + suffix
	clone.TestTime = 0
	clone.ResponseTime = 0
	clone.Keys = nil
	if resetBalance {
		clone.Balance = 0
		clone.UsedQuota = 0
	}
	if err := clone.ValidateSettings(); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("%w: %v", ErrInvalidChannelSettings, err)
	}
	var sourceOverrides []ChannelModelOverride
	if err := tx.Where("channel_id = ?", sourceChannelId).Find(&sourceOverrides).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Create(&clone).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	for i := range sourceOverrides {
		sourceOverrides[i].ChannelId = clone.Id
	}
	if len(sourceOverrides) > 0 {
		if err := tx.Create(&sourceOverrides).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	if err := clone.AddAbilities(tx); err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return &clone, nil
}
