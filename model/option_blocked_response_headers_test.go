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

func TestUpdateOptionValidatesBlockedResponseHeadersBeforePersistence(t *testing.T) {
	originalDB := DB
	originalHeaders := append([]string(nil), operation_setting.GetErrorSetting().BlockedResponseHeaders...)
	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		DB = originalDB
		require.NoError(t, operation_setting.UpdateBlockedResponseHeaders(originalHeaders))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	DB = db

	const key = "error_setting.blocked_response_headers"
	require.Error(t, UpdateOption(key, `["X Invalid"]`))
	var count int64
	require.NoError(t, db.Model(&Option{}).Where("key = ?", key).Count(&count).Error)
	assert.Zero(t, count)

	require.NoError(t, UpdateOption(key, `[]`))
	var saved Option
	require.NoError(t, db.First(&saved, "key = ?", key).Error)
	assert.Equal(t, `[]`, saved.Value)
	assert.False(t, operation_setting.ShouldBlockUpstreamResponseHeader("X-Request-Id"))
}

func TestUpdateOptionDoesNotChangeBlockedHeadersWhenPersistenceFails(t *testing.T) {
	originalDB := DB
	originalHeaders := append([]string(nil), operation_setting.GetErrorSetting().BlockedResponseHeaders...)
	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		DB = originalDB
		require.NoError(t, operation_setting.UpdateBlockedResponseHeaders(originalHeaders))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	require.NoError(t, operation_setting.UpdateBlockedResponseHeaders([]string{"X-Request-Id"}))
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	DB = db

	err = UpdateOption("error_setting.blocked_response_headers", `["X-New-Trace-Id"]`)
	require.Error(t, err)
	assert.True(t, operation_setting.ShouldBlockUpstreamResponseHeader("X-Request-Id"))
	assert.False(t, operation_setting.ShouldBlockUpstreamResponseHeader("X-New-Trace-Id"))
	common.OptionMapRWMutex.RLock()
	_, optionUpdated := common.OptionMap["error_setting.blocked_response_headers"]
	common.OptionMapRWMutex.RUnlock()
	assert.False(t, optionUpdated)
}
