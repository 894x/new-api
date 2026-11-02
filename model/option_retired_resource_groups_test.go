package model

import (
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRetiredResourceGroupOption(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := setupGroupPricingOptionTest(t, `{"default":1}`, `{}`)
			if dialect != "sqlite" {
				dsn := os.Getenv("TEST_RESOURCE_GROUP_" + strings.ToUpper(dialect) + "_DSN")
				if dsn == "" {
					t.Skip("isolated database DSN not configured")
				}
				require.Contains(t, dsn, "newapi_resource_groups_removed_", "use a task-owned disposable database")
				var driver gorm.Dialector = mysql.Open(dsn)
				if dialect == "postgres" {
					driver = postgres.Open(dsn)
				}
				var err error
				db, err = gorm.Open(driver, &gorm.Config{})
				require.NoError(t, err)
				sqlDB, err := db.DB()
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
				require.NoError(t, db.AutoMigrate(&Option{}))
				t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(&Option{})) })
				DB = db
			}
			const key = "GroupModelChannelGroups"
			require.ErrorContains(t, UpdateOption(key, `{}`), "resource group policies have been removed")
			require.Error(t, UpdateOptionsBulk(map[string]string{key: `{}`, "Notice": "must not persist"}))
			var count int64
			require.NoError(t, db.Model(&Option{}).Where(map[string]any{"key": []string{key, "Notice"}}).Count(&count).Error)
			assert.Zero(t, count, "retired writes cannot partially persist a bulk update")

			// Historical values survive reloads without entering active settings.
			for _, value := range []string{`{"default":{"model-a":["资源-vendor"]}}`, `null`} {
				require.NoError(t, db.Save(&Option{Key: key, Value: value}).Error)
				for range 2 {
					require.NoError(t, db.AutoMigrate(&Option{}))
					loadOptionsFromDatabase()
					assert.NotContains(t, common.OptionMap, key)
					var stored Option
					require.NoError(t, db.Where(map[string]any{"key": key}).First(&stored).Error)
					assert.Equal(t, value, stored.Value, "retiring settings must not delete historical data")
				}
			}
		})
	}
}
