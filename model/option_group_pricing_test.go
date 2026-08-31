package model

import (
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupGroupPricingOptionTest(t *testing.T, groupRatios, modelTieredRatios string) *gorm.DB {
	t.Helper()

	originalDB := DB
	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	originalModelTieredRatios := ratio_setting.ModelTieredRatios2JSONString()
	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		DB = originalDB
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
		require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(originalModelTieredRatios))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(groupRatios))
	require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(modelTieredRatios))

	db, err := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/options.db?_pragma=busy_timeout(10000)"), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, dbErr := db.DB()
		require.NoError(t, dbErr)
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&Option{}))
	require.NoError(t, db.Create([]Option{
		{Key: "GroupRatio", Value: groupRatios},
		{Key: ratio_setting.ModelTieredRatiosOptionKey, Value: modelTieredRatios},
	}).Error)
	DB = db
	return db
}

func TestUpdateOptionsBulkAtomicallyReplacesGroupPricingConfiguration(t *testing.T) {
	const oldGroups = `{"old":1}`
	const oldTiered = `{"old":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.9}]}}}`
	const newGroups = `{"new":1.2}`
	const newTiered = `{"new":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]}}}`
	db := setupGroupPricingOptionTest(t, oldGroups, oldTiered)

	require.NoError(t, UpdateOptionsBulk(map[string]string{
		"GroupRatio":                             newGroups,
		ratio_setting.ModelTieredRatiosOptionKey: newTiered,
	}))

	var saved []Option
	require.NoError(t, db.Where("key IN ?", []string{"GroupRatio", ratio_setting.ModelTieredRatiosOptionKey}).Find(&saved).Error)
	values := make(map[string]string, len(saved))
	for _, option := range saved {
		values[option.Key] = option.Value
	}
	assert.JSONEq(t, newGroups, values["GroupRatio"])
	assert.JSONEq(t, newTiered, values[ratio_setting.ModelTieredRatiosOptionKey])
	assert.False(t, ratio_setting.ContainsGroupRatio("old"))
	assert.True(t, ratio_setting.ContainsGroupRatio("new"))

	snapshot, active, err := ratio_setting.ResolveModelTieredDiscount("ordinary", "new", "gpt-5", time.Unix(10, 0))
	require.NoError(t, err)
	require.True(t, active)
	assert.Equal(t, 0.8, snapshot.Tiers[0].Ratio)
}

func TestLoadOptionsFromDatabasePublishesGroupPricingPairTogether(t *testing.T) {
	const oldGroups = `{"old":1}`
	const oldTiered = `{"old":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.9}]}}}`
	const newGroups = `{"new":1.2}`
	const newTiered = `{"new":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]}}}`
	db := setupGroupPricingOptionTest(t, oldGroups, oldTiered)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Option{}).Where("key = ?", "GroupRatio").Update("value", newGroups).Error; err != nil {
			return err
		}
		return tx.Model(&Option{}).
			Where("key = ?", ratio_setting.ModelTieredRatiosOptionKey).
			Update("value", newTiered).Error
	}))

	loadOptionsFromDatabase()

	assert.False(t, ratio_setting.ContainsGroupRatio("old"))
	assert.True(t, ratio_setting.ContainsGroupRatio("new"))
	_, active, err := ratio_setting.ResolveModelTieredDiscount("ordinary", "new", "gpt-5", time.Unix(10, 0))
	require.NoError(t, err)
	assert.True(t, active)
}

func TestUpdateOptionsBulkRejectsOrphanGroupPricingPairWithoutMutation(t *testing.T) {
	const groups = `{"premium":1}`
	const tiered = `{"premium":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.9}]}}}`
	db := setupGroupPricingOptionTest(t, groups, tiered)

	err := UpdateOptionsBulk(map[string]string{
		"GroupRatio":                             `{"replacement":1}`,
		ratio_setting.ModelTieredRatiosOptionKey: tiered,
	})
	assert.EqualError(t, err, `model tiered ratio group "premium" is not configured in GroupRatio`)

	var saved []Option
	require.NoError(t, db.Where("key IN ?", []string{"GroupRatio", ratio_setting.ModelTieredRatiosOptionKey}).Find(&saved).Error)
	values := make(map[string]string, len(saved))
	for _, option := range saved {
		values[option.Key] = option.Value
	}
	assert.JSONEq(t, groups, values["GroupRatio"])
	assert.JSONEq(t, tiered, values[ratio_setting.ModelTieredRatiosOptionKey])
	assert.True(t, ratio_setting.ContainsGroupRatio("premium"))
	_, active, resolveErr := ratio_setting.ResolveModelTieredDiscount("ordinary", "premium", "gpt-5", time.Unix(10, 0))
	require.NoError(t, resolveErr)
	assert.True(t, active)
}

func TestLoadOptionsFromDatabaseKeepsLegacyOrphanVisibleButInactive(t *testing.T) {
	const safeGroups = `{"default":1}`
	const orphanTiered = `{"orphan":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.9}]}}}`
	db := setupGroupPricingOptionTest(t, safeGroups, `{}`)
	require.NoError(t, db.Model(&Option{}).
		Where("key = ?", ratio_setting.ModelTieredRatiosOptionKey).
		Update("value", orphanTiered).Error)

	loadOptionsFromDatabase()

	common.OptionMapRWMutex.RLock()
	visibleGroups := common.OptionMap["GroupRatio"]
	visibleTiered := common.OptionMap[ratio_setting.ModelTieredRatiosOptionKey]
	common.OptionMapRWMutex.RUnlock()
	assert.JSONEq(t, safeGroups, visibleGroups)
	assert.JSONEq(t, orphanTiered, visibleTiered)
	assert.False(t, ratio_setting.ContainsGroupRatio("orphan"))
	_, active, err := ratio_setting.ResolveModelTieredDiscount("ordinary", "orphan", "gpt-5", time.Unix(10, 0))
	require.NoError(t, err)
	assert.False(t, active)
}

func TestUpdateOptionUsesVisibleLegacyTieredCounterpart(t *testing.T) {
	const safeGroups = `{"premium":1}`
	const orphanTiered = `{"premium":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.9}]}},"orphan":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]}}}`
	db := setupGroupPricingOptionTest(t, safeGroups, `{}`)
	require.NoError(t, db.Model(&Option{}).
		Where("key = ?", ratio_setting.ModelTieredRatiosOptionKey).
		Update("value", orphanTiered).Error)
	loadOptionsFromDatabase()

	err := UpdateOption("GroupRatio", `{}`)
	assert.EqualError(t, err, `model tiered ratio group "orphan" is not configured in GroupRatio`)
	var saved Option
	require.NoError(t, db.First(&saved, "key = ?", "GroupRatio").Error)
	assert.JSONEq(t, safeGroups, saved.Value)

	require.NoError(t, UpdateOption("GroupRatio", `{"premium":1,"orphan":1}`))
	_, active, resolveErr := ratio_setting.ResolveModelTieredDiscount("ordinary", "orphan", "gpt-5", time.Unix(10, 0))
	require.NoError(t, resolveErr)
	assert.True(t, active)
}

func TestSingleKeyBulkUpdateUsesVisibleLegacyTieredCounterpart(t *testing.T) {
	const safeGroups = `{"default":1}`
	const orphanTiered = `{"premium":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.9}]}}}`
	db := setupGroupPricingOptionTest(t, safeGroups, `{}`)
	require.NoError(t, db.Model(&Option{}).
		Where("key = ?", ratio_setting.ModelTieredRatiosOptionKey).
		Update("value", orphanTiered).Error)
	loadOptionsFromDatabase()

	err := UpdateOptionsBulk(map[string]string{"GroupRatio": `{}`})
	assert.EqualError(t, err, `model tiered ratio group "premium" is not configured in GroupRatio`)
	var saved Option
	require.NoError(t, db.First(&saved, "key = ?", "GroupRatio").Error)
	assert.JSONEq(t, safeGroups, saved.Value)
}

func TestLoadOptionsFromDatabaseValidatesLoneTieredOptionAgainstCurrentGroups(t *testing.T) {
	const safeGroups = `{"default":1}`
	const orphanTiered = `{"premium":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.9}]}}}`
	db := setupGroupPricingOptionTest(t, safeGroups, `{}`)
	require.NoError(t, db.Delete(&Option{}, "key = ?", "GroupRatio").Error)
	require.NoError(t, db.Model(&Option{}).
		Where("key = ?", ratio_setting.ModelTieredRatiosOptionKey).
		Update("value", orphanTiered).Error)

	loadOptionsFromDatabase()

	common.OptionMapRWMutex.RLock()
	visibleGroups := common.OptionMap["GroupRatio"]
	visibleTiered := common.OptionMap[ratio_setting.ModelTieredRatiosOptionKey]
	common.OptionMapRWMutex.RUnlock()
	assert.JSONEq(t, safeGroups, visibleGroups)
	assert.JSONEq(t, orphanTiered, visibleTiered)
	_, active, err := ratio_setting.ResolveModelTieredDiscount("ordinary", "premium", "gpt-5", time.Unix(10, 0))
	require.NoError(t, err)
	assert.False(t, active)
}

func TestConcurrentGroupPricingOptionUpdatesCannotCommitOrphanPolicy(t *testing.T) {
	const groups = `{"premium":1}`
	const tiered = `{"premium":{"gpt-5":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.9}]}}}`
	setupGroupPricingOptionTest(t, groups, `{}`)

	start := make(chan struct{})
	errors := make(chan error, 2)
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-start
		errors <- UpdateOption("GroupRatio", `{}`)
	}()
	go func() {
		defer workers.Done()
		<-start
		errors <- UpdateOption(ratio_setting.ModelTieredRatiosOptionKey, tiered)
	}()
	close(start)
	workers.Wait()
	close(errors)

	successes := 0
	for err := range errors {
		if err == nil {
			successes++
		}
	}
	assert.Equal(t, 1, successes)

	groupsRemain := ratio_setting.ContainsGroupRatio("premium")
	_, policyActive, err := ratio_setting.ResolveModelTieredDiscount("ordinary", "premium", "gpt-5", time.Unix(10, 0))
	require.NoError(t, err)
	assert.Equal(t, groupsRemain, policyActive, "a committed policy must always have a pricing group")
}
