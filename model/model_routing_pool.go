package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/dynamic_routing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	RoutingModePriority            = "priority"
	RoutingModeSmart               = "smart"
	RoutingModeDiscount            = "discount"
	DefaultDiscountMaxTTFTMS int64 = 15000
	DefaultDiscountMaxTPOTMS int64 = 200
)

var (
	ErrInvalidRoutingPool  = errors.New("invalid model routing pool")
	ErrRoutingPoolConflict = errors.New("the routing pool has changed; reload before saving")
)

// ModelRoutingPool is an explicit replacement for the default candidate pool
// of one public model and outlet group. An empty Channels list is deny-all.
// Channel defaults, credentials and shared channel/model capacity are untouched.
type ModelRoutingPool struct {
	RoutingGroup string                             `json:"routing_group" gorm:"type:varchar(64);primaryKey;autoIncrement:false"`
	Model        string                             `json:"model" gorm:"type:varchar(255);primaryKey;autoIncrement:false"`
	Mode         string                             `json:"mode" gorm:"type:varchar(16);not null"`
	Revision     string                             `json:"revision" gorm:"type:varchar(64);not null"`
	ChannelsJSON string                             `json:"-" gorm:"type:text;not null"`
	Channels     []ModelRoutingPoolChannel          `json:"channels" gorm:"-"`
	MaxTTFTMS    int64                              `json:"max_ttft_ms" gorm:"column:max_ttft_ms"`
	MaxTPOTMS    int64                              `json:"max_tpot_ms" gorm:"column:max_tpot_ms"`
	TuningJSON   string                             `json:"-" gorm:"type:text"`
	Tuning       *dynamic_routing_setting.Overrides `json:"tuning,omitempty" gorm:"-"`
}

func (pool *ModelRoutingPool) DecodeTuning() error {
	pool.Tuning = nil
	if pool.TuningJSON == "" {
		return nil
	}
	return common.UnmarshalJsonStr(pool.TuningJSON, &pool.Tuning)
}

func (pool *ModelRoutingPool) UsesDynamicRouting() bool {
	return pool != nil && (pool.Mode == RoutingModeSmart || pool.Mode == RoutingModeDiscount)
}

type ModelRoutingPoolChannel struct {
	ChannelID int   `json:"channel_id"`
	Priority  int64 `json:"priority"`
	Weight    uint  `json:"weight"`
	Enabled   bool  `json:"enabled"`
}

type ModelRoutingPoolCandidate struct {
	ChannelID             int      `json:"channel_id"`
	ChannelName           string   `json:"channel_name"`
	Priority              int64    `json:"priority"`
	Weight                uint     `json:"weight"`
	Available             bool     `json:"available"`
	EffectiveCostDiscount *float64 `json:"effective_cost_discount"`
}

func GetModelRoutingPool(group, modelName string) (*ModelRoutingPool, error) {
	var rows []ModelRoutingPool
	if err := DB.Where("routing_group = ? AND model = ?", group, modelName).Limit(1).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	pool := &rows[0]
	if err := common.UnmarshalJsonStr(pool.ChannelsJSON, &pool.Channels); err != nil {
		return nil, fmt.Errorf("decode model routing pool: %w", err)
	}
	if err := pool.DecodeTuning(); err != nil {
		return nil, fmt.Errorf("decode model routing tuning: %w", err)
	}
	return pool, nil
}

// Candidate metadata deliberately excludes credentials and provider settings.
func ListModelRoutingPoolCandidates(group, modelName string) ([]ModelRoutingPoolCandidate, error) {
	names := []string{modelName}
	if normalized := ratio_setting.RoutingMatchModelName(modelName); normalized != "" && normalized != modelName {
		names = append(names, normalized)
	}
	var abilities []Ability
	if err := DB.Where(map[string]any{"group": group}).Where("model IN ?", names).Order("model, channel_id").Find(&abilities).Error; err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(abilities))
	for _, ability := range abilities {
		ids = append(ids, ability.ChannelId)
	}
	channels := make([]Channel, 0)
	if len(ids) > 0 {
		if err := DB.Select("id", "name", "status", "cost_discount").Where("id IN ?", ids).Find(&channels).Error; err != nil {
			return nil, err
		}
	}
	byID := make(map[int]Channel, len(channels))
	for _, channel := range channels {
		byID[channel.Id] = channel
	}
	var overrides []ChannelModelOverride
	if len(ids) > 0 {
		if err := DB.Select("channel_id", "model", "cost_discount").Where("channel_id IN ? AND model IN ?", ids, names).Find(&overrides).Error; err != nil {
			return nil, err
		}
	}
	overridesByChannel := make(map[int]map[string]*float64)
	for _, override := range overrides {
		if overridesByChannel[override.ChannelId] == nil {
			overridesByChannel[override.ChannelId] = make(map[string]*float64)
		}
		overridesByChannel[override.ChannelId][override.Model] = override.CostDiscount
	}
	result := make([]ModelRoutingPoolCandidate, 0, len(channels))
	seen := make(map[int]bool)
	// Prefer exact-model defaults when a modifier also has base-model abilities.
	for _, name := range names {
		for _, ability := range abilities {
			channel, found := byID[ability.ChannelId]
			if ability.Model != name || seen[ability.ChannelId] || !found {
				continue
			}
			seen[ability.ChannelId] = true
			discount := channel.CostDiscount
			if override := overridesByChannel[channel.Id][name]; override != nil {
				discount = override
			}
			result = append(result, ModelRoutingPoolCandidate{
				ChannelID: channel.Id, ChannelName: channel.Name,
				Priority: effectiveAbilityPriority(&ability), Weight: ability.Weight,
				Available:             ability.Enabled && channel.Status == common.ChannelStatusEnabled,
				EffectiveCostDiscount: discount,
			})
		}
	}
	return result, nil
}

func SaveModelRoutingPool(pool *ModelRoutingPool, expectedRevision string) error {
	dynamicRoutingOptionsMu.Lock()
	defer dynamicRoutingOptionsMu.Unlock()
	if pool == nil || pool.RoutingGroup == "" || pool.RoutingGroup == "auto" || len(pool.RoutingGroup) > 64 || strings.TrimSpace(pool.RoutingGroup) != pool.RoutingGroup || strings.ContainsAny(pool.RoutingGroup, ",\x00\r\n") || pool.Model == "" || len(pool.Model) > 255 || strings.TrimSpace(pool.Model) != pool.Model || strings.ContainsAny(pool.Model, "\x00\r\n") || !slices.Contains([]string{RoutingModePriority, RoutingModeSmart, RoutingModeDiscount}, pool.Mode) || len(pool.Channels) > 500 || pool.MaxTTFTMS < 0 || pool.MaxTTFTMS > 3600000 || pool.MaxTPOTMS < 0 || pool.MaxTPOTMS > 60000 {
		return ErrInvalidRoutingPool
	}
	if pool.Mode == RoutingModeDiscount {
		if pool.MaxTTFTMS == 0 {
			pool.MaxTTFTMS = DefaultDiscountMaxTTFTMS
		}
		if pool.MaxTPOTMS == 0 {
			pool.MaxTPOTMS = DefaultDiscountMaxTPOTMS
		}
	}
	pool.TuningJSON = ""
	if pool.Tuning != nil {
		encoded, err := common.Marshal(pool.Tuning)
		if err != nil {
			return err
		}
		if string(encoded) != "{}" {
			pool.TuningJSON = string(encoded)
		} else {
			pool.Tuning = nil
		}
	}
	if pool.Tuning != nil {
		if _, err := pool.Tuning.Resolve(dynamic_routing_setting.GetSetting()); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidRoutingPool, err)
		}
	}
	candidates, err := ListModelRoutingPoolCandidates(pool.RoutingGroup, pool.Model)
	if err != nil {
		return err
	}
	validIDs := make(map[int]bool, len(candidates))
	for _, candidate := range candidates {
		validIDs[candidate.ChannelID] = true
	}
	seen := make(map[int]bool, len(pool.Channels))
	for _, entry := range pool.Channels {
		if entry.ChannelID <= 0 || seen[entry.ChannelID] || entry.Priority < 0 || entry.Priority > 2147483647 || ValidateChannelWeight(&entry.Weight) != nil || !validIDs[entry.ChannelID] {
			return ErrInvalidRoutingPool
		}
		seen[entry.ChannelID] = true
	}
	if pool.Channels == nil {
		pool.Channels = []ModelRoutingPoolChannel{}
	}
	encoded, err := common.Marshal(pool.Channels)
	if err != nil {
		return err
	}
	pool.ChannelsJSON = string(encoded)
	revisionData := pool.RoutingGroup + "\x00" + pool.Model + "\x00" + pool.Mode + "\x00" + pool.ChannelsJSON
	if pool.MaxTTFTMS != 0 || pool.MaxTPOTMS != 0 {
		revisionData += fmt.Sprintf("\x00%d\x00%d", pool.MaxTTFTMS, pool.MaxTPOTMS)
	}
	if pool.TuningJSON != "" {
		revisionData += "\x00" + pool.TuningJSON
	}
	pool.Revision = fmt.Sprintf("%x", sha256.Sum256([]byte(revisionData)))
	if expectedRevision == "" {
		result := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(pool)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrRoutingPoolConflict
		}
		// MySQL connections with clientFoundRows can report a matched no-op
		// conflict as affected. Verify the stored value before reporting success.
		current, err := GetModelRoutingPool(pool.RoutingGroup, pool.Model)
		if err != nil {
			return err
		}
		if current == nil || current.Revision != pool.Revision {
			return ErrRoutingPoolConflict
		}
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var current ModelRoutingPool
		if err := lockForUpdate(tx).Where("routing_group = ? AND model = ?", pool.RoutingGroup, pool.Model).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRoutingPoolConflict
			}
			return err
		}
		if current.Revision != expectedRevision {
			return ErrRoutingPoolConflict
		}
		return tx.Model(&current).Updates(map[string]any{"mode": pool.Mode, "revision": pool.Revision, "channels_json": pool.ChannelsJSON, "max_ttft_ms": pool.MaxTTFTMS, "max_tpot_ms": pool.MaxTPOTMS, "tuning_json": pool.TuningJSON}).Error
	})
}

// Deleting the explicit pool restores inheritance; emptying it does not.
func DeleteModelRoutingPool(group, modelName, revision string) error {
	if revision == "" {
		return ErrRoutingPoolConflict
	}
	result := DB.Where("routing_group = ? AND model = ? AND revision = ?", group, modelName, revision).Delete(&ModelRoutingPool{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrRoutingPoolConflict
	}
	return nil
}
