package model

import (
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRequestCaptureStorageSettingsValidationAndPersistence(t *testing.T) {
	var driver gorm.Dialector = sqlite.Open(":memory:")
	switch os.Getenv("CAPTURE_TEST_DIALECT") {
	case "mysql":
		driver = mysql.Open(os.Getenv("TEST_MYSQL_DSN"))
	case "postgres":
		driver = postgres.Open(os.Getenv("TEST_POSTGRES_DSN"))
	}
	db, err := gorm.Open(driver, &gorm.Config{})
	require.NoError(t, err)
	versionQuery := "SELECT VERSION()"
	if db.Dialector.Name() == "sqlite" {
		versionQuery = "SELECT sqlite_version()"
	}
	var databaseVersion string
	require.NoError(t, db.Raw(versionQuery).Scan(&databaseVersion).Error)
	t.Logf("database version: %s", databaseVersion)
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
	assert.Equal(t, RequestCaptureStorageSettings{RetentionDays: 3, MaxGiB: 10, MaxConcurrency: 8}, GetRequestCaptureStorageSettings())
	const savedValue = `{"retention_days":7,"max_gib":20}`
	require.NoError(t, UpdateOption(RequestCaptureStorageOptionKey, savedValue))
	assert.Equal(t, 8, GetRequestCaptureStorageSettings().MaxConcurrency)
	assert.False(t, GetRequestCaptureStorageSettings().OnlyNon200)
	for _, invalid := range []string{`{}`, `null`, `{"retention_days":0,"max_gib":20}`, `{"retention_days":1.5,"max_gib":20}`, `{"retention_days":7,"max_gib":-1}`, `{"retention_days":366,"max_gib":20}`, `{"retention_days":7,"max_gib":366}`, `{"retention_days":"7","max_gib":20}`} {
		t.Run(invalid, func(t *testing.T) {
			require.Error(t, UpdateOption(RequestCaptureStorageOptionKey, invalid))
			require.Error(t, UpdateOptionsBulk(map[string]string{RequestCaptureStorageOptionKey: invalid}))
			require.Error(t, updateOptionMap(RequestCaptureStorageOptionKey, invalid))
			assert.Equal(t, RequestCaptureStorageSettings{RetentionDays: 7, MaxGiB: 20, MaxConcurrency: 8}, GetRequestCaptureStorageSettings())
			var option Option
			require.NoError(t, db.Where(&Option{Key: RequestCaptureStorageOptionKey}).First(&option).Error)
			assert.Equal(t, savedValue, option.Value)
		})
	}
	for _, invalid := range []string{
		`{"retention_days":7,"max_gib":20,"max_concurrency":0}`,
		`{"retention_days":7,"max_gib":20,"max_concurrency":-1}`,
		`{"retention_days":7,"max_gib":20,"max_concurrency":1025}`,
		`{"retention_days":7,"max_gib":20,"max_concurrency":1.5}`,
		`{"retention_days":7,"max_gib":20,"only_non_200":"true"}`,
	} {
		require.Error(t, UpdateOption(RequestCaptureStorageOptionKey, invalid))
		require.Error(t, UpdateOptionsBulk(map[string]string{RequestCaptureStorageOptionKey: invalid}))
		assert.Equal(t, 8, GetRequestCaptureStorageSettings().MaxConcurrency)
	}
	for _, limit := range []int{1, MaxRequestCaptureConcurrency} {
		value, err := common.Marshal(RequestCaptureStorageSettings{
			RetentionDays: 7, MaxGiB: 20, MaxConcurrency: limit,
		})
		require.NoError(t, err)
		require.NoError(t, UpdateOption(RequestCaptureStorageOptionKey, string(value)))
		assert.Equal(t, limit, GetRequestCaptureStorageSettings().MaxConcurrency)
	}
	const configuredValue = `{"retention_days":7,"max_gib":20,"max_concurrency":64,"only_non_200":true}`
	require.NoError(t, UpdateOption(RequestCaptureStorageOptionKey, configuredValue))
	// Simulate another node/restart loading the persisted setting.
	common.OptionMapRWMutex.Lock()
	delete(common.OptionMap, RequestCaptureStorageOptionKey)
	common.OptionMapRWMutex.Unlock()
	loadOptionsFromDatabase()
	assert.Equal(t, RequestCaptureStorageSettings{
		RetentionDays: 7, MaxGiB: 20, MaxConcurrency: 64, OnlyNon200: true,
	}, GetRequestCaptureStorageSettings())
	// A failed database commit must not publish new runtime settings.
	require.NoError(t, sqlDB.Close())
	require.Error(t, UpdateOption(RequestCaptureStorageOptionKey, `{"retention_days":1,"max_gib":1}`))
	assert.Equal(t, RequestCaptureStorageSettings{
		RetentionDays: 7, MaxGiB: 20, MaxConcurrency: 64, OnlyNon200: true,
	}, GetRequestCaptureStorageSettings())
}
