package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const GroupModelAccessOptionKey = "GroupModelAccess"

var ErrGroupModelAccessConflict = errors.New("the group-model matrix has changed; reload before saving")
var ErrInvalidGroupModelAccess = errors.New("invalid group-model matrix")

// Missing groups inherit all channel models. Present empty lists deny all models.
type GroupModelAccess map[string][]string

var groupModelAccessState = struct {
	sync.RWMutex
	models map[string]map[string]bool
	value  string
}{models: make(map[string]map[string]bool)}

var groupModelAccessSaveLock sync.Mutex

func ParseGroupModelAccess(value string) (GroupModelAccess, error) {
	var config GroupModelAccess
	if err := common.UnmarshalJsonStr(value, &config); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidGroupModelAccess, err)
	}
	if config == nil || len(config) > 1000 {
		return nil, ErrInvalidGroupModelAccess
	}
	for group, models := range config {
		if group == "" || group == "auto" || len(group) > 64 || strings.TrimSpace(group) != group || strings.ContainsAny(group, ",\x00\r\n") || models == nil || len(models) > 10000 {
			return nil, fmt.Errorf("%w group", ErrInvalidGroupModelAccess)
		}
		for i, name := range models {
			if name == "" || len(name) > 255 || strings.TrimSpace(name) != name || strings.ContainsAny(name, "\x00\r\n") {
				return nil, fmt.Errorf("%w model", ErrInvalidGroupModelAccess)
			}
			models[i] = ratio_setting.RoutingMatchModelName(name)
			if models[i] == "" {
				return nil, fmt.Errorf("%w model", ErrInvalidGroupModelAccess)
			}
		}
		slices.Sort(models)
		config[group] = slices.Compact(models)
	}
	return config, nil
}

func IsGroupModelAllowed(group, modelName string) bool {
	groupModelAccessState.RLock()
	defer groupModelAccessState.RUnlock()
	models, restricted := groupModelAccessState.models[group]
	return !restricted || models[ratio_setting.RoutingMatchModelName(modelName)]
}

// GetAllowedGroupModelPairs lists published matrix combinations supported by
// channel abilities. Disabled abilities remain editable in routing pools.
func GetAllowedGroupModelPairs() (map[string]map[string]bool, error) {
	groups := make([]string, 0)
	for group := range setting.GetUserUsableGroupsCopy() {
		if group != "" && group != "auto" && ratio_setting.ContainsGroupRatio(group) {
			groups = append(groups, group)
		}
	}
	pairs := make(map[string]map[string]bool)
	if len(groups) == 0 {
		return pairs, nil
	}
	var abilities []Ability
	if err := DB.Select("group", "model").Where(map[string]any{"group": groups}).Distinct().Find(&abilities).Error; err != nil {
		return nil, err
	}
	for _, ability := range abilities {
		if !IsGroupModelAllowed(ability.Group, ability.Model) {
			continue
		}
		if pairs[ability.Group] == nil {
			pairs[ability.Group] = make(map[string]bool)
		}
		pairs[ability.Group][ratio_setting.RoutingMatchModelName(ability.Model)] = true
	}
	return pairs, nil
}

func applyGroupModelAccess(value string) error {
	groupModelAccessState.RLock()
	unchanged := groupModelAccessState.value == value
	groupModelAccessState.RUnlock()
	if unchanged {
		return nil
	}
	config, err := ParseGroupModelAccess(value)
	if err != nil {
		return err
	}
	models := make(map[string]map[string]bool, len(config))
	for group, names := range config {
		models[group] = make(map[string]bool, len(names))
		for _, name := range names {
			models[group][name] = true
		}
	}
	groupModelAccessState.Lock()
	groupModelAccessState.models = models
	groupModelAccessState.value = value
	groupModelAccessState.Unlock()
	InvalidatePricingCache()
	return nil
}

func GetGroupModelAccess() (GroupModelAccess, string, error) {
	var options []Option
	if err := DB.Where(&Option{Key: GroupModelAccessOptionKey}).Limit(1).Find(&options).Error; err != nil {
		return nil, "", err
	}
	value := "{}"
	if len(options) > 0 {
		value = options[0].Value
	}
	config, err := ParseGroupModelAccess(value)
	return config, fmt.Sprintf("%x", sha256.Sum256([]byte(value))), err
}

// Persist before publishing. Compare the stored revision under a row lock so
// two administrators or gateway nodes cannot silently overwrite each other.
func SaveGroupModelAccess(config GroupModelAccess, revision string) (string, error) {
	encoded, err := common.Marshal(config)
	if err != nil {
		return "", err
	}
	config, err = ParseGroupModelAccess(string(encoded))
	if err != nil {
		return "", err
	}
	encoded, err = common.Marshal(config)
	if err != nil {
		return "", err
	}
	value := string(encoded)
	groupModelAccessSaveLock.Lock()
	defer groupModelAccessSaveLock.Unlock()
	err = DB.Transaction(func(tx *gorm.DB) error {
		initial := Option{Key: GroupModelAccessOptionKey, Value: "{}"}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&initial).Error; err != nil {
			return err
		}
		var current Option
		if err := lockForUpdate(tx).Where(&Option{Key: GroupModelAccessOptionKey}).First(&current).Error; err != nil {
			return err
		}
		if revision != fmt.Sprintf("%x", sha256.Sum256([]byte(current.Value))) {
			return ErrGroupModelAccessConflict
		}
		return tx.Model(&current).Update("value", value).Error
	})
	if err != nil {
		return "", err
	}
	if err := updateOptionMap(GroupModelAccessOptionKey, value); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(value))), nil
}
