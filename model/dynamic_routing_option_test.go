package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/dynamic_routing_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenericOptionPathsRejectDynamicRoutingFields(t *testing.T) {
	db := useFrontendOptionMigrationDB(t)
	originalSetting := dynamic_routing_setting.GetSetting()
	originalOptionMap := common.OptionMap
	t.Cleanup(func() {
		common.OptionMap = originalOptionMap
		require.NoError(t, dynamic_routing_setting.ReplaceAndSync(originalSetting))
	})
	common.OptionMap = map[string]string{"sentinel": "unchanged"}
	before := dynamic_routing_setting.GetSnapshot()

	require.Error(t, UpdateOptionsBulk(map[string]string{
		"dynamic_routing_setting.enabled": "true",
		"WaffoPayMethods":                 `{"card":"Card"}`,
	}))
	require.Error(t, updateOptionMap("dynamic_routing_setting.enabled", "true"))

	var count int64
	require.NoError(t, db.Model(&Option{}).Count(&count).Error)
	assert.Zero(t, count)
	assert.Equal(t, map[string]string{"sentinel": "unchanged"}, common.OptionMap)
	assert.Equal(t, before, dynamic_routing_setting.GetSnapshot())
}

func TestUpdateDynamicRoutingOptionsPersistsAndPublishesWholeSettingOnce(t *testing.T) {
	db := useFrontendOptionMigrationDB(t)
	originalSetting := dynamic_routing_setting.GetSetting()
	originalOptionMap := common.OptionMap
	t.Cleanup(func() {
		common.OptionMap = originalOptionMap
		require.NoError(t, dynamic_routing_setting.ReplaceAndSync(originalSetting))
	})
	common.OptionMap = map[string]string{}

	next := originalSetting
	next.Enabled = true
	next.MaxSamples = 80
	next.MinSamples = 4
	next.ProbeFraction = 0.02
	next.Aggressiveness = 0.95
	before := dynamic_routing_setting.GetSnapshot()

	require.NoError(t, UpdateDynamicRoutingOptions(next))
	after := dynamic_routing_setting.GetSnapshot()

	assert.Equal(t, before.Version+1, after.Version)
	assert.Equal(t, next, dynamic_routing_setting.GetSetting())
	wantValues := dynamic_routing_setting.ToOptionValues(next)
	var count int64
	require.NoError(t, db.Model(&Option{}).
		Where("key LIKE ?", dynamic_routing_setting.OptionPrefix+"%").
		Count(&count).Error)
	assert.Equal(t, int64(len(wantValues)), count)
	for key, want := range wantValues {
		assert.Equal(t, want, requireOptionValue(t, db, key))
		assert.Equal(t, want, common.OptionMap[key])
	}
}

func TestUpdateDynamicRoutingOptionsRejectsInvalidSettingWithoutWritesOrPublish(t *testing.T) {
	db := useFrontendOptionMigrationDB(t)
	originalSetting := dynamic_routing_setting.GetSetting()
	originalOptionMap := common.OptionMap
	t.Cleanup(func() {
		common.OptionMap = originalOptionMap
		require.NoError(t, dynamic_routing_setting.ReplaceAndSync(originalSetting))
	})
	common.OptionMap = map[string]string{"sentinel": "unchanged"}
	before := dynamic_routing_setting.GetSnapshot()
	invalid := originalSetting
	invalid.Enabled = true
	invalid.ProbeFraction = 0

	require.Error(t, UpdateDynamicRoutingOptions(invalid))

	var count int64
	require.NoError(t, db.Model(&Option{}).Count(&count).Error)
	assert.Zero(t, count)
	assert.Equal(t, map[string]string{"sentinel": "unchanged"}, common.OptionMap)
	assert.Equal(t, originalSetting, dynamic_routing_setting.GetSetting())
	assert.Equal(t, before, dynamic_routing_setting.GetSnapshot())
}
