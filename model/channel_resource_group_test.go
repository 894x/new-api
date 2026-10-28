package model

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/glebarez/sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestChannelResourceGroup(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			dbType := common.DatabaseTypeSQLite
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(filepath.Join(t.TempDir(), "resource-groups.db"))
			case "mysql":
				dsn := os.Getenv("TEST_RESOURCE_GROUP_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_RESOURCE_GROUP_MYSQL_DSN is not configured")
				}
				config, err := mysqldriver.ParseDSN(dsn)
				require.NoError(t, err)
				require.True(t, strings.HasPrefix(config.DBName, "newapi_resource_groups_"), "use a task-owned disposable database")
				driver = mysql.Open(dsn)
				dbType = common.DatabaseTypeMySQL
			case "postgres":
				dsn := os.Getenv("TEST_RESOURCE_GROUP_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_RESOURCE_GROUP_POSTGRES_DSN is not configured")
				}
				parsed, err := url.Parse(dsn)
				require.NoError(t, err)
				require.True(t, strings.HasPrefix(parsed.Path, "/newapi_resource_groups_"), "use a task-owned disposable database")
				driver = postgres.Open(dsn)
				dbType = common.DatabaseTypePostgreSQL
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			originalDB := DB
			originalMain, originalLog := common.MainDatabaseType(), common.LogDatabaseType()
			oldRatios, oldTiered := currentGroupPricingOptionValues()
			common.OptionMapRWMutex.Lock()
			oldOptions := common.OptionMap
			common.OptionMap = map[string]string{}
			common.OptionMapRWMutex.Unlock()
			t.Cleanup(func() {
				DB = originalDB
				common.SetDatabaseTypes(originalMain, originalLog)
				initCol()
				require.NoError(t, applyGroupPricingOptions(oldRatios, oldTiered))
				common.OptionMapRWMutex.Lock()
				common.OptionMap = oldOptions
				common.OptionMapRWMutex.Unlock()
			})
			DB = db
			common.SetDatabaseTypes(dbType, dbType)
			initCol()
			require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}, &ChannelModelOverride{}, &Option{}))
			versionQuery := "SELECT version()"
			if dialect == "sqlite" {
				versionQuery = "SELECT sqlite_version()"
			}
			var version string
			require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
			t.Logf("%s: %s", dialect, version)
			require.NoError(t, applyGroupPricingOptions(`{"default":1,"资源-existing":0.7,"资源-public":1,"资源-special":1,"资源-auto":1}`, `{}`))
			baseline := []Option{
				{Key: "GroupRatio", Value: ratio_setting.GroupRatio2JSONString()},
				{Key: "UserUsableGroups", Value: `{"default":"Default","资源-public":"Public"}`},
				{Key: "AutoGroups", Value: `["default","资源-auto"]`},
				{Key: "group_ratio_setting.group_special_usable_group", Value: `{"vip":{"+:资源-special":"Special"}}`},
			}
			require.NoError(t, db.Create(&baseline).Error)
			channel := Channel{Name: "Vendor", Type: 1, Key: "secret-must-stay", Group: "default", Models: "model-a,model-b", Status: common.ChannelStatusEnabled}
			require.NoError(t, db.Create(&channel).Error)
			priority := int64(17)
			require.NoError(t, db.Create(&ChannelModelOverride{ChannelId: channel.Id, Model: "model-a", Priority: &priority}).Error)
			require.NoError(t, channel.AddAbilities(nil))
			sibling := Channel{Name: "Sibling", Type: 1, Key: "other-secret", Group: "资源-existing", Models: "model-a", Status: common.ChannelStatusEnabled}
			require.NoError(t, db.Create(&sibling).Error)
			require.NoError(t, sibling.AddAbilities(nil))

			t.Run("create preserves groups credentials and private configuration", func(t *testing.T) {
				result, err := CreateChannelResourceGroup(channel.Id, " 资源-Vendor ", false)
				require.NoError(t, err)
				assert.True(t, result.Created)
				assert.Equal(t, []string{"default", "资源-Vendor"}, result.ChannelGroups)
				assert.Equal(t, 1.0, result.Ratio)
				var saved Channel
				require.NoError(t, db.First(&saved, channel.Id).Error)
				assert.Equal(t, "default,资源-Vendor", saved.Group)
				assert.Equal(t, channel.Key, saved.Key)
				assert.Equal(t, channel.Status, saved.Status)
				var abilities []Ability
				require.NoError(t, db.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
				assert.Len(t, abilities, 4)
				for _, ability := range abilities {
					if ability.Model == "model-a" {
						require.NotNil(t, ability.Priority)
						assert.Equal(t, priority, *ability.Priority)
					}
				}
				assert.Equal(t, 1.0, ratio_setting.GetGroupRatio("资源-Vendor"))
				for _, option := range baseline[1:] {
					var saved Option
					require.NoError(t, db.Where(&Option{Key: option.Key}).First(&saved).Error)
					assert.JSONEq(t, option.Value, saved.Value)
				}
				assert.NotContains(t, setting.GetUserUsableGroupsCopy(), "资源-Vendor")
			})
			t.Run("retry is idempotent", func(t *testing.T) {
				result, err := CreateChannelResourceGroup(channel.Id, "资源-Vendor", false)
				require.NoError(t, err)
				assert.False(t, result.Created)
				assert.Equal(t, []string{"default", "资源-Vendor"}, result.ChannelGroups)
			})
			t.Run("collision requires explicit reuse and preserves ratio", func(t *testing.T) {
				_, err := CreateChannelResourceGroup(channel.Id, "资源-existing", false)
				var conflict *ResourceGroupConflict
				require.ErrorAs(t, err, &conflict)
				assert.Equal(t, []ResourceGroupChannel{{ID: sibling.Id, Name: sibling.Name}}, conflict.Channels)
				assert.Equal(t, 0.7, conflict.Ratio)
				result, err := CreateChannelResourceGroup(channel.Id, "资源-existing", true)
				require.NoError(t, err)
				assert.False(t, result.Created)
				assert.Equal(t, 0.7, result.Ratio)
			})
			t.Run("invalid and exposed groups cannot be created or reused", func(t *testing.T) {
				for _, name := range []string{"", "资源-", "other", "资源-a,b", "资源-a\nb", "资源-" + strings.Repeat("长", 62), "资源-public", "资源-special", "资源-auto"} {
					_, err := CreateChannelResourceGroup(channel.Id, name, true)
					assert.Error(t, err, "name: %q", name)
				}
				_, err := CreateChannelResourceGroup(999999, "资源-missing", false)
				assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
				assert.False(t, ratio_setting.ContainsGroupRatio("资源-missing"))
				_, err = CreateChannelResourceGroup(channel.Id, "资源-"+strings.Repeat("长", 40), false)
				assert.ErrorContains(t, err, "combined group names")
			})
			t.Run("ability failure rolls back registry channel and routing", func(t *testing.T) {
				var before Channel
				require.NoError(t, db.First(&before, channel.Id).Error)
				beforeRatios := ratio_setting.GroupRatio2JSONString()
				const callback = "test:resource_group_ability_failure"
				require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
					if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "Ability" {
						tx.AddError(errors.New("routing write failed"))
					}
				}))
				_, err := CreateChannelResourceGroup(channel.Id, "资源-rollback", false)
				require.NoError(t, db.Callback().Create().Remove(callback))
				require.ErrorContains(t, err, "routing write failed")
				var after Channel
				require.NoError(t, db.First(&after, channel.Id).Error)
				assert.Equal(t, before.Group, after.Group)
				var registry Option
				require.NoError(t, db.Where(&Option{Key: "GroupRatio"}).First(&registry).Error)
				assert.JSONEq(t, beforeRatios, registry.Value)
				assert.JSONEq(t, beforeRatios, ratio_setting.GroupRatio2JSONString())
				var count int64
				require.NoError(t, db.Model(&Ability{}).Where("channel_id = ?", channel.Id).Count(&count).Error)
				assert.Equal(t, int64(6), count)
			})
			t.Run("persisted changes from another node survive concurrent creations", func(t *testing.T) {
				registry := ratio_setting.GetGroupRatioCopy()
				registry["external"] = 0.8
				encoded, err := common.Marshal(registry)
				require.NoError(t, err)
				require.NoError(t, db.Model(&Option{}).Where(&Option{Key: "GroupRatio"}).Update("value", string(encoded)).Error)
				var workers sync.WaitGroup
				errors := make(chan error, 2)
				for _, name := range []string{"资源-A", "资源-B"} {
					workers.Go(func() { _, err := CreateChannelResourceGroup(sibling.Id, name, false); errors <- err })
				}
				workers.Wait()
				close(errors)
				for err := range errors {
					require.NoError(t, err)
				}
				assert.Equal(t, 0.8, ratio_setting.GetGroupRatio("external"))
				assert.True(t, ratio_setting.ContainsGroupRatio("资源-A"))
				assert.True(t, ratio_setting.ContainsGroupRatio("资源-B"))
			})
		})
	}
}
