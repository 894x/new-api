package model

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// The released schema stores every channel group in a single VARCHAR(64).
type releasedChannelGroups struct {
	Channel
	Group string `gorm:"type:varchar(64);default:'default'"`
}

func (releasedChannelGroups) TableName() string { return "channels" }

func TestChannelGroupsStorage(t *testing.T) {
	for _, tc := range []struct{ dialect, scenario string }{
		{"sqlite", "fresh"}, {"sqlite", "upgrade"},
		{"mysql", "fresh"}, {"mysql", "upgrade"},
		{"postgres", "fresh"}, {"postgres", "upgrade"},
	} {
		t.Run(tc.dialect+"/"+tc.scenario, func(t *testing.T) {
			dialect, scenario := tc.dialect, tc.scenario
			var driver gorm.Dialector
			dbType := common.DatabaseTypeSQLite
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(filepath.Join(t.TempDir(), "channel-groups.db"))
			case "mysql":
				dsn := os.Getenv("TEST_RESOURCE_GROUP_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_RESOURCE_GROUP_MYSQL_DSN is not configured")
				}
				config, err := mysqldriver.ParseDSN(dsn)
				require.NoError(t, err)
				require.True(t, strings.HasPrefix(config.DBName, "newapi_resource_groups_"), "use a task-owned disposable database")
				driver = mysqlMigrationDialector{mysql.Dialector{Config: &mysql.Config{DSN: dsn}}}
				dbType = common.DatabaseTypeMySQL
			case "postgres":
				dsn := os.Getenv("TEST_RESOURCE_GROUP_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_RESOURCE_GROUP_POSTGRES_DSN is not configured")
				}
				parsed, err := url.Parse(dsn)
				require.NoError(t, err)
				require.True(t, strings.HasPrefix(parsed.Path, "/newapi_resource_groups_"), "use a task-owned disposable database")
				driver = postgresMigrationDialector{postgres.Dialector{Config: &postgres.Config{DSN: dsn, PreferSimpleProtocol: true}}}
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
			require.NoError(t, migrateChannelGroups(db), "fresh databases have no channel column to migrate")
			t.Cleanup(func() {
				require.NoError(t, db.Migrator().DropTable(&Ability{}, &ChannelModelOverride{}, &Channel{}, &Option{}))
			})
			if scenario == "upgrade" {
				require.NoError(t, db.AutoMigrate(&releasedChannelGroups{}, &Ability{}, &ChannelModelOverride{}, &Option{}))
				columns, err := db.Migrator().ColumnTypes(&Channel{})
				require.NoError(t, err)
				for _, column := range columns {
					if column.Name() == "group" {
						length, ok := column.Length()
						require.True(t, ok)
						require.EqualValues(t, 64, length)
					}
				}
			} else {
				require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}, &ChannelModelOverride{}, &Option{}))
			}
			historical := Channel{Name: "Historical", Type: 1, Key: "historical-key", Group: "default", Models: "historical-model", Status: common.ChannelStatusEnabled}
			require.NoError(t, db.Create(&historical).Error)
			historicalPriority := int64(23)
			require.NoError(t, db.Create(&ChannelModelOverride{ChannelId: historical.Id, Model: historical.Models, Priority: &historicalPriority}).Error)
			require.NoError(t, historical.AddAbilities(nil))
			recorder := &migrationSQLRecorder{}
			migrationDB := db.Session(&gorm.Session{Logger: recorder})
			for iteration := range 2 {
				recorder.reset()
				require.NoError(t, migrateChannelGroups(migrationDB))
				require.NoError(t, migrationDB.AutoMigrate(&Channel{}, &Ability{}, &ChannelModelOverride{}, &Option{}))
				if iteration > 0 {
					assert.Empty(t, recorder.schemaMutations(), "repeated migration must not change the schema")
				}
				var saved Channel
				require.NoError(t, db.First(&saved, historical.Id).Error)
				assert.Equal(t, historical, saved)
				assert.True(t, db.Migrator().HasIndex(&Channel{}, "Name"))
				assert.True(t, db.Migrator().HasIndex(&Channel{}, "Tag"))
				var ability Ability
				require.NoError(t, db.Where("channel_id = ?", historical.Id).First(&ability).Error)
				require.NotNil(t, ability.Priority)
				assert.Equal(t, historicalPriority, *ability.Priority)
				assert.Error(t, db.Create(&ability).Error, "routing uniqueness survives migration")
				var override ChannelModelOverride
				require.NoError(t, db.Where("channel_id = ?", historical.Id).First(&override).Error)
				require.NotNil(t, override.Priority)
				assert.Equal(t, historicalPriority, *override.Priority)
				assert.Error(t, db.Create(&override).Error, "override uniqueness survives migration")
			}
			versionQuery := "SELECT version()"
			if dialect == "sqlite" {
				versionQuery = "SELECT sqlite_version()"
			}
			var version string
			require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
			t.Logf("%s: %s", dialect, version)
			t.Run("sixteen groups persist beyond sixty four characters and all writes reject seventeen", func(t *testing.T) {
				groups := make([]string, 16)
				for i := range groups {
					groups[i] = fmt.Sprintf("%02d-%s", i, strings.Repeat("长", 61))
				}
				tag := "group-limit"
				limited := Channel{Name: "Group limit", Type: 1, Key: "limit-key", Group: strings.Join(groups, ","), Models: "limit-model", Status: common.ChannelStatusEnabled, Tag: &tag}
				require.NoError(t, limited.Insert())
				var saved Channel
				require.NoError(t, db.First(&saved, limited.Id).Error)
				assert.Equal(t, strings.Join(groups, ","), saved.Group)
				var abilities []Ability
				require.NoError(t, db.Where("channel_id = ?", limited.Id).Find(&abilities).Error)
				require.Len(t, abilities, 16)
				storedGroups := make([]string, 0, len(abilities))
				for _, ability := range abilities {
					storedGroups = append(storedGroups, ability.Group)
				}
				assert.ElementsMatch(t, groups, storedGroups)
				_, err := CloneChannelWithModelOverrides(limited.Id, "-copy", false)
				require.NoError(t, err)
				overLimit := saved
				overLimit.Id = 0
				overLimit.Name = "Rejected"
				overLimit.Group = "a,b,c,d,e,f,g,h,i,j,k,l,m,n,o,p,q"
				require.ErrorContains(t, overLimit.Insert(), "at most 16 groups")
				validBatch := saved
				validBatch.Id = 0
				require.ErrorContains(t, BatchInsertChannels([]Channel{validBatch, overLimit}), "at most 16 groups")
				overLimit.Id = limited.Id
				require.ErrorContains(t, overLimit.Update(), "at most 16 groups")
				require.ErrorContains(t, EditChannelByTag(tag, nil, nil, nil, &overLimit.Group, nil, nil, nil, nil), "at most 16 groups")
				var after Channel
				require.NoError(t, db.First(&after, limited.Id).Error)
				assert.Equal(t, saved, after, "rejected edits preserve every channel field")
				var rejected int64
				require.NoError(t, db.Model(&Channel{}).Where("name = ?", "Rejected").Count(&rejected).Error)
				assert.Zero(t, rejected)
			})
		})
	}
}
