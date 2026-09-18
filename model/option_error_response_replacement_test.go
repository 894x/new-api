package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpdateOptionValidatesErrorResponseReplacementRulesBeforePersistence(t *testing.T) {
	originalDB := DB
	originalRules := append([]operation_setting.ErrorResponseReplacementRule(nil), operation_setting.GetErrorSetting().ResponseReplacementRules...)
	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		DB = originalDB
		require.NoError(t, operation_setting.UpdateErrorResponseReplacementRules(originalRules))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	DB = db

	const key = "error_setting.response_replacement_rules"
	require.Error(t, UpdateOption(key, `[{"status_code":200,"match":"error","replacement":"retry"}]`))
	var count int64
	require.NoError(t, db.Model(&Option{}).Where("key = ?", key).Count(&count).Error)
	assert.Zero(t, count)

	value := `[{"status_code":500,"match":" Moonshot AI ","replacement":" Service unavailable "}]`
	require.NoError(t, UpdateOption(key, value))
	replacement, matched := operation_setting.MatchErrorResponseReplacement(500, "Moonshot AI")
	require.True(t, matched)
	assert.Equal(t, "Service unavailable", replacement)

	common.OptionMapRWMutex.RLock()
	savedValue := common.OptionMap[key]
	common.OptionMapRWMutex.RUnlock()
	assert.JSONEq(t, `[{"status_code":500,"match":"Moonshot AI","replacement":"Service unavailable"}]`, savedValue)
}

func TestUpdateOptionDoesNotPublishReplacementRulesWhenPersistenceFails(t *testing.T) {
	originalDB := DB
	originalRules := append([]operation_setting.ErrorResponseReplacementRule(nil), operation_setting.GetErrorSetting().ResponseReplacementRules...)
	t.Cleanup(func() {
		DB = originalDB
		require.NoError(t, operation_setting.UpdateErrorResponseReplacementRules(originalRules))
	})
	require.NoError(t, operation_setting.UpdateErrorResponseReplacementRules([]operation_setting.ErrorResponseReplacementRule{{
		StatusCode: 500, Match: "existing", Replacement: "existing replacement",
	}}))

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	DB = db

	err = UpdateOption(
		"error_setting.response_replacement_rules",
		`[{"status_code":500,"match":"new","replacement":"new replacement"}]`,
	)
	require.Error(t, err)
	replacement, matched := operation_setting.MatchErrorResponseReplacement(500, "existing")
	require.True(t, matched)
	assert.Equal(t, "existing replacement", replacement)
	_, matched = operation_setting.MatchErrorResponseReplacement(500, "new")
	assert.False(t, matched)
}
