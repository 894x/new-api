package model

import (
	"sort"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/dynamic_routing_setting"
	"gorm.io/gorm"
)

var dynamicRoutingOptionsMu sync.Mutex

// UpdateDynamicRoutingOptions validates one complete controller setting,
// persists every flat option in one transaction, and then publishes one
// immutable runtime snapshot. No partial configuration is exposed in-process.
func UpdateDynamicRoutingOptions(setting dynamic_routing_setting.DynamicRoutingSetting) error {
	dynamicRoutingOptionsMu.Lock()
	defer dynamicRoutingOptionsMu.Unlock()

	if err := dynamic_routing_setting.Validate(setting); err != nil {
		return err
	}
	values := dynamic_routing_setting.ToOptionValues(setting)
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if err := DB.Transaction(func(tx *gorm.DB) error {
		for _, key := range keys {
			value := values[key]
			option := Option{Key: key}
			if err := tx.FirstOrCreate(&option, Option{Key: key}).Error; err != nil {
				return err
			}
			option.Value = value
			if err := tx.Save(&option).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	common.OptionMapRWMutex.Lock()
	defer common.OptionMapRWMutex.Unlock()
	if err := dynamic_routing_setting.ReplaceAndSync(setting); err != nil {
		return err
	}
	for _, key := range keys {
		common.OptionMap[key] = values[key]
	}
	return nil
}
