package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRequestCaptureStorageSettingsValidationAndPersistence(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB := DB
	DB = db
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		DB = previousDB
		_ = sqlDB.Close()
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})
	assert.Equal(t, RequestCaptureStorageSettings{RetentionDays: 3, MaxGiB: 10}, GetRequestCaptureStorageSettings())
	const savedValue = `{"retention_days":7,"max_gib":20}`
	require.NoError(t, UpdateOption(RequestCaptureStorageOptionKey, savedValue))
	for _, invalid := range []string{`{}`, `null`, `{"retention_days":0,"max_gib":20}`, `{"retention_days":1.5,"max_gib":20}`, `{"retention_days":7,"max_gib":-1}`, `{"retention_days":366,"max_gib":20}`, `{"retention_days":7,"max_gib":366}`, `{"retention_days":"7","max_gib":20}`} {
		t.Run(invalid, func(t *testing.T) {
			require.Error(t, UpdateOption(RequestCaptureStorageOptionKey, invalid))
			require.Error(t, UpdateOptionsBulk(map[string]string{RequestCaptureStorageOptionKey: invalid}))
			require.Error(t, updateOptionMap(RequestCaptureStorageOptionKey, invalid))
			assert.Equal(t, RequestCaptureStorageSettings{RetentionDays: 7, MaxGiB: 20}, GetRequestCaptureStorageSettings())
			var option Option
			require.NoError(t, db.First(&option, "key = ?", RequestCaptureStorageOptionKey).Error)
			assert.Equal(t, savedValue, option.Value)
		})
	}
	// Simulate another node/restart loading the persisted setting.
	common.OptionMapRWMutex.Lock()
	delete(common.OptionMap, RequestCaptureStorageOptionKey)
	common.OptionMapRWMutex.Unlock()
	loadOptionsFromDatabase()
	assert.Equal(t, RequestCaptureStorageSettings{RetentionDays: 7, MaxGiB: 20}, GetRequestCaptureStorageSettings())
	// A failed database commit must not publish new runtime settings.
	require.NoError(t, sqlDB.Close())
	require.Error(t, UpdateOption(RequestCaptureStorageOptionKey, `{"retention_days":1,"max_gib":1}`))
	assert.Equal(t, RequestCaptureStorageSettings{RetentionDays: 7, MaxGiB: 20}, GetRequestCaptureStorageSettings())
}
