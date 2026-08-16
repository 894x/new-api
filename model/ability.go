package model

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Ability struct {
	Group     string  `json:"group" gorm:"type:varchar(64);primaryKey;autoIncrement:false"`
	Model     string  `json:"model" gorm:"type:varchar(255);primaryKey;autoIncrement:false"`
	ChannelId int     `json:"channel_id" gorm:"primaryKey;autoIncrement:false;index"`
	Enabled   bool    `json:"enabled"`
	Priority  *int64  `json:"priority" gorm:"bigint;default:0;index"`
	Weight    uint    `json:"weight" gorm:"default:0;index"`
	Tag       *string `json:"tag" gorm:"index"`
}

type AbilityWithChannel struct {
	Ability
	ChannelType int `json:"channel_type"`
}

func effectiveAbilityPriority(ability *Ability) int64 {
	if ability == nil || ability.Priority == nil {
		return 0
	}
	return *ability.Priority
}

func chooseChannelIdByWeight(abilities []Ability, randomInt func(int) int) int {
	if len(abilities) == 0 {
		return 0
	}
	weightSum := 0
	for _, ability := range abilities {
		effectiveWeight := int(ability.Weight) + 10
		if effectiveWeight <= 0 || weightSum > math.MaxInt-effectiveWeight {
			return abilities[0].ChannelId
		}
		weightSum += effectiveWeight
	}
	randomWeight := randomInt(weightSum)
	for _, ability := range abilities {
		randomWeight -= int(ability.Weight) + 10
		if randomWeight < 0 {
			return ability.ChannelId
		}
	}
	return abilities[len(abilities)-1].ChannelId
}

func GetAllEnableAbilityWithChannels() ([]AbilityWithChannel, error) {
	var abilities []AbilityWithChannel
	err := DB.Table("abilities").
		Select("abilities.*, channels.type as channel_type").
		Joins("left join channels on abilities.channel_id = channels.id").
		Where("abilities.enabled = ?", true).
		Scan(&abilities).Error
	return abilities, err
}

func GetGroupEnabledModels(group string) []string {
	var models []string
	// Find distinct models
	DB.Table("abilities").Where(commonGroupCol+" = ? and enabled = ?", group, true).Distinct("model").Pluck("model", &models)
	return models
}

func GetEnabledModels() []string {
	var models []string
	// Find distinct models
	DB.Table("abilities").Where("enabled = ?", true).Distinct("model").Pluck("model", &models)
	return models
}

func GetAllEnableAbilities() []Ability {
	var abilities []Ability
	DB.Find(&abilities, "enabled = ?", true)
	return abilities
}

func GetChannel(group string, model string, retry int, requestPath string) (*Channel, error) {
	return GetChannelWithFilters(group, model, retry, requestPath, "", nil)
}

// GetChannelWithFilter selects a DB-backed channel from the supplied allowed
// set. A nil set preserves the ordinary unconstrained selection behavior.
func GetChannelWithFilter(group string, model string, retry int, requestPath string, allowedChannelIds map[int]struct{}) (*Channel, error) {
	return GetChannelWithFilters(group, model, retry, requestPath, "", allowedChannelIds)
}

// GetChannelWithFilters selects a DB-backed channel after applying request
// path, video resolution, and optional asset-replica constraints.
func GetChannelWithFilters(group string, model string, retry int, requestPath string, videoResolution string, allowedChannelIds map[int]struct{}) (*Channel, error) {
	var abilities []Ability
	allowedChannelIdSlice := make([]int, 0, len(allowedChannelIds))
	if allowedChannelIds != nil {
		if len(allowedChannelIds) == 0 {
			return nil, nil
		}
		for channelId := range allowedChannelIds {
			allowedChannelIdSlice = append(allowedChannelIdSlice, channelId)
		}
	}

	query := DB.Where(commonGroupCol+" = ? and model = ? and enabled = ?", group, model, true)
	if allowedChannelIds != nil {
		query = query.Where("channel_id IN ?", allowedChannelIdSlice)
	}
	err := query.Order("weight DESC").Find(&abilities).Error
	if err != nil {
		return nil, err
	}
	abilities, pathEligibleCount, err := filterAbilitiesByRequestPathModelAndVideoResolution(abilities, requestPath, model, videoResolution)
	if err != nil {
		return nil, err
	}
	if len(abilities) == 0 {
		normalizedModel := ratio_setting.FormatMatchingModelName(model)
		if normalizedModel != "" && normalizedModel != model {
			query = DB.Where(commonGroupCol+" = ? and model = ? and enabled = ?", group, normalizedModel, true)
			if allowedChannelIds != nil {
				query = query.Where("channel_id IN ?", allowedChannelIdSlice)
			}
			err = query.Order("weight DESC").Find(&abilities).Error
			if err != nil {
				return nil, err
			}
			var normalizedPathEligibleCount int
			abilities, normalizedPathEligibleCount, err = filterAbilitiesByRequestPathModelAndVideoResolution(abilities, requestPath, model, videoResolution)
			if err != nil {
				return nil, err
			}
			pathEligibleCount += normalizedPathEligibleCount
		}
	}
	if videoResolution != "" && pathEligibleCount > 0 && len(abilities) == 0 {
		return nil, newVideoResolutionUnsupportedError(model, videoResolution)
	}
	if len(abilities) == 0 {
		return nil, nil
	}

	priorities := make([]int64, 0)
	seenPriorities := make(map[int64]struct{})
	for _, ability := range abilities {
		priority := effectiveAbilityPriority(&ability)
		if _, ok := seenPriorities[priority]; ok {
			continue
		}
		seenPriorities[priority] = struct{}{}
		priorities = append(priorities, priority)
	}
	sort.Slice(priorities, func(i, j int) bool { return priorities[i] > priorities[j] })
	if retry < 0 {
		retry = 0
	}
	if retry >= len(priorities) {
		retry = len(priorities) - 1
	}
	targetPriority := priorities[retry]
	targetAbilities := make([]Ability, 0, len(abilities))
	for _, ability := range abilities {
		if effectiveAbilityPriority(&ability) == targetPriority {
			targetAbilities = append(targetAbilities, ability)
		}
	}
	channel := Channel{Id: chooseChannelIdByWeight(targetAbilities, common.GetRandomInt)}
	err = DB.First(&channel, "id = ?", channel.Id).Error
	return &channel, err
}

// filterAbilitiesByRequestPathModelAndVideoResolution filters DB-backed
// candidates before priority selection. Path checks apply only to Advanced
// Custom channels; video resolution checks are per public model and preserve
// wildcard behavior when the channel or model has no capability rule.
func filterAbilitiesByRequestPathModelAndVideoResolution(abilities []Ability, requestPath string, model string, videoResolution string) ([]Ability, int, error) {
	if (requestPath == "" && videoResolution == "") || len(abilities) == 0 {
		return abilities, len(abilities), nil
	}

	channelIds := make([]int, 0, len(abilities))
	seen := make(map[int]struct{}, len(abilities))
	for _, ability := range abilities {
		if _, ok := seen[ability.ChannelId]; ok {
			continue
		}
		seen[ability.ChannelId] = struct{}{}
		channelIds = append(channelIds, ability.ChannelId)
	}

	var channels []*Channel
	if err := DB.Where("id IN ?", channelIds).Find(&channels).Error; err != nil {
		return nil, 0, fmt.Errorf("load channels for capability filtering: %w", err)
	}

	advancedConfigs := make(map[int]*dto.AdvancedCustomConfig)
	videoConfigs := make(map[int]*dto.VideoCapabilityConfig)
	for _, channel := range channels {
		if channel.Type == constant.ChannelTypeAdvancedCustom {
			advancedConfigs[channel.Id] = channel.GetOtherSettings().AdvancedCustom
		}
		if config := channel.GetOtherSettings().VideoCapabilities; config != nil {
			videoConfigs[channel.Id] = config
		}
	}

	filtered := make([]Ability, 0, len(abilities))
	pathEligibleCount := 0
	for _, ability := range abilities {
		config, isAdvancedCustom := advancedConfigs[ability.ChannelId]
		if requestPath != "" && isAdvancedCustom && (config == nil || !config.SupportsPathForModel(requestPath, model)) {
			continue
		}
		pathEligibleCount++
		if !videoConfigs[ability.ChannelId].SupportsResolution(model, videoResolution) {
			continue
		}
		filtered = append(filtered, ability)
	}
	return filtered, pathEligibleCount, nil
}

func (channel *Channel) AddAbilities(tx *gorm.DB) error {
	if err := ValidateChannelWeight(channel.Weight); err != nil {
		return err
	}
	useDB := DB
	if tx != nil {
		useDB = tx
	}
	overrides, err := getChannelModelOverrideMap(useDB, channel.Id)
	if err != nil {
		return err
	}
	models_ := normalizeChannelModels(channel)
	groups_ := strings.Split(channel.Group, ",")
	abilitySet := make(map[string]struct{})
	abilities := make([]Ability, 0, len(models_))
	for _, model := range models_ {
		routing := effectiveChannelModelRouting(channel, model, nil)
		if override, ok := overrides[model]; ok {
			routing = effectiveChannelModelRouting(channel, model, &override)
		}
		for _, group := range groups_ {
			key := group + "|" + model
			if _, exists := abilitySet[key]; exists {
				continue
			}
			abilitySet[key] = struct{}{}
			ability := Ability{
				Group:     group,
				Model:     model,
				ChannelId: channel.Id,
				Enabled:   channel.Status == common.ChannelStatusEnabled,
				Priority:  common.GetPointer(routing.EffectivePriority),
				Weight:    routing.EffectiveWeight,
				Tag:       channel.Tag,
			}
			abilities = append(abilities, ability)
		}
	}
	if len(abilities) == 0 {
		return nil
	}
	for _, chunk := range lo.Chunk(abilities, 50) {
		err := useDB.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error
		if err != nil {
			return err
		}
	}
	return nil
}

func (channel *Channel) DeleteAbilities() error {
	return DB.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
}

// UpdateAbilities updates abilities of this channel.
// Make sure the channel is completed before calling this function.
func (channel *Channel) UpdateAbilities(tx *gorm.DB) error {
	if err := ValidateChannelWeight(channel.Weight); err != nil {
		return err
	}
	isNewTx := false
	// 如果没有传入事务，创建新的事务
	if tx == nil {
		tx = DB.Begin()
		if tx.Error != nil {
			return tx.Error
		}
		isNewTx = true
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()
	}

	// First delete all abilities of this channel
	err := tx.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
	if err != nil {
		if isNewTx {
			tx.Rollback()
		}
		return err
	}
	if err := pruneChannelModelOverrides(tx, channel); err != nil {
		if isNewTx {
			tx.Rollback()
		}
		return err
	}

	// Then add new abilities
	overrides, err := getChannelModelOverrideMap(tx, channel.Id)
	if err != nil {
		if isNewTx {
			tx.Rollback()
		}
		return err
	}
	models_ := normalizeChannelModels(channel)
	groups_ := strings.Split(channel.Group, ",")
	abilitySet := make(map[string]struct{})
	abilities := make([]Ability, 0, len(models_))
	for _, model := range models_ {
		routing := effectiveChannelModelRouting(channel, model, nil)
		if override, ok := overrides[model]; ok {
			routing = effectiveChannelModelRouting(channel, model, &override)
		}
		for _, group := range groups_ {
			key := group + "|" + model
			if _, exists := abilitySet[key]; exists {
				continue
			}
			abilitySet[key] = struct{}{}
			ability := Ability{
				Group:     group,
				Model:     model,
				ChannelId: channel.Id,
				Enabled:   channel.Status == common.ChannelStatusEnabled,
				Priority:  common.GetPointer(routing.EffectivePriority),
				Weight:    routing.EffectiveWeight,
				Tag:       channel.Tag,
			}
			abilities = append(abilities, ability)
		}
	}

	if len(abilities) > 0 {
		for _, chunk := range lo.Chunk(abilities, 50) {
			err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error
			if err != nil {
				if isNewTx {
					tx.Rollback()
				}
				return err
			}
		}
	}

	// 如果是新创建的事务，需要提交
	if isNewTx {
		return tx.Commit().Error
	}

	return nil
}

func UpdateAbilityStatus(channelId int, status bool) error {
	return DB.Model(&Ability{}).Where("channel_id = ?", channelId).Select("enabled").Update("enabled", status).Error
}

var fixLock = sync.Mutex{}

func FixAbility() (int, int, error) {
	lock := fixLock.TryLock()
	if !lock {
		return 0, 0, errors.New("已经有一个修复任务在运行中，请稍后再试")
	}
	defer fixLock.Unlock()

	// truncate abilities table
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		err := DB.Exec("DELETE FROM abilities").Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Delete abilities failed: %s", err.Error()))
			return 0, 0, err
		}
	} else {
		err := DB.Exec("TRUNCATE TABLE abilities").Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Truncate abilities failed: %s", err.Error()))
			return 0, 0, err
		}
	}
	var channels []*Channel
	// Find all channels
	err := DB.Model(&Channel{}).Find(&channels).Error
	if err != nil {
		return 0, 0, err
	}
	if len(channels) == 0 {
		return 0, 0, nil
	}
	successCount := 0
	failCount := 0
	for _, chunk := range lo.Chunk(channels, 50) {
		ids := lo.Map(chunk, func(c *Channel, _ int) int { return c.Id })
		// Delete all abilities of this channel
		err = DB.Where("channel_id IN ?", ids).Delete(&Ability{}).Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Delete abilities failed: %s", err.Error()))
			failCount += len(chunk)
			continue
		}
		// Then add new abilities
		for _, channel := range chunk {
			err = channel.AddAbilities(nil)
			if err != nil {
				common.SysLog(fmt.Sprintf("Add abilities for channel %d failed: %s", channel.Id, err.Error()))
				failCount++
			} else {
				successCount++
			}
		}
	}
	InitChannelCache()
	return successCount, failCount, nil
}
