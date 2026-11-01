package model

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestChannelCostDiscountValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value *float64
		valid bool
	}{
		{"unknown", nil, true},
		{"free", common.GetPointer(0.0), true},
		{"discounted", common.GetPointer(0.003125), true},
		{"reference price", common.GetPointer(1.0), true},
		{"negative", common.GetPointer(-0.01), false},
		{"above one", common.GetPointer(1.01), false},
		{"nan", common.GetPointer(math.NaN()), false},
		{"infinite", common.GetPointer(math.Inf(1)), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateChannelCostDiscount(tc.value)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

// The fork's pre-feature override schema. The public rc.47 release did not yet
// have this table; exercise both the released schema and the fork's upgrade.
type preCostDiscountOverride struct {
	ChannelId int    `gorm:"primaryKey;autoIncrement:false;index"`
	Model     string `gorm:"type:varchar(255);primaryKey;autoIncrement:false;index"`
	Priority  *int64 `gorm:"bigint"`
	Weight    *uint
	RPM       *int64 `gorm:"bigint"`
	TPM       *int64 `gorm:"bigint"`
}

func (preCostDiscountOverride) TableName() string { return "channel_model_overrides" }

func TestChannelCostDiscountDatabaseMatrix(t *testing.T) {
	for _, engine := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			var driver gorm.Dialector
			databaseType := common.DatabaseTypeSQLite
			switch engine {
			case "sqlite":
				driver = sqlite.Open(filepath.Join(t.TempDir(), "cost-discount.db"))
			case "mysql":
				dsn := os.Getenv("TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN is not configured")
				}
				driver = mysql.Open(dsn)
				databaseType = common.DatabaseTypeMySQL
			case "postgres":
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN is not configured")
				}
				// Each layout recreates the same tables; do not reuse prepared plans
				// from a previous layout's different SELECT * result type.
				driver = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
				databaseType = common.DatabaseTypePostgreSQL
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			previousDB := DB
			previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
			DB = db
			common.SetDatabaseTypes(databaseType, previousLog)
			initCol()
			t.Cleanup(func() { DB = previousDB; common.SetDatabaseTypes(previousMain, previousLog); initCol() })
			var version string
			query := "select version()"
			if engine == "sqlite" {
				query = "select sqlite_version()"
			}
			require.NoError(t, db.Raw(query).Scan(&version).Error)
			t.Logf("%s version: %s", engine, version)

			for _, layout := range []string{"fresh", "released-rc47", "fork-before-discount"} {
				t.Run(layout, func(t *testing.T) {
					require.NoError(t, db.Migrator().DropTable(&Ability{}, &ChannelModelOverride{}, &Channel{}))
					if layout != "fresh" {
						require.NoError(t, db.AutoMigrate(&releasedCostDiscountChannel{}))
						require.NoError(t, db.Create(&releasedCostDiscountChannel{Type: 1, Key: "fixture", Name: "Existing channel", Status: common.ChannelStatusEnabled, Models: "model-a,model-b", Group: "default", Priority: common.GetPointer(int64(10))}).Error)
						if layout == "fork-before-discount" {
							require.NoError(t, db.AutoMigrate(&preCostDiscountOverride{}))
							require.NoError(t, db.Create(&preCostDiscountOverride{ChannelId: 1, Model: "model-a", Priority: common.GetPointer(int64(20))}).Error)
						}
					}
					for range 2 {
						require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelModelOverride{}, &Ability{}))
					}
					var channel Channel
					if layout == "fresh" {
						channel = Channel{Type: 1, Key: "fixture", Name: "Existing channel", Status: common.ChannelStatusEnabled, Models: "model-a,model-b", Group: "default", Priority: common.GetPointer(int64(10))}
						require.NoError(t, db.Create(&channel).Error)
					} else {
						require.NoError(t, db.First(&channel, 1).Error)
						assert.Equal(t, "Existing channel", channel.Name)
						assert.Equal(t, "fixture", channel.Key)
						assert.True(t, db.Migrator().HasIndex(&Channel{}, "idx_channels_name"))
					}
					require.NoError(t, channel.UpdateAbilities(nil))
					routes, err := ListChannelModelRoutings(channel.Id)
					require.NoError(t, err)
					require.Len(t, routes, 2)
					assert.Nil(t, routes[0].EffectiveCostDiscount)
					assert.Equal(t, "unknown", routes[0].CostDiscountSource)
					if layout == "fork-before-discount" {
						assert.EqualValues(t, 20, routes[0].EffectivePriority)
					}

					channel.CostDiscount = common.GetPointer(0.3)
					require.NoError(t, channel.Update("cost_discount"))
					patch := ChannelModelOverridePatch{ChannelId: channel.Id, Model: "model-a", CostDiscount: common.GetPointer(0.0)}
					require.NoError(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{patch}))
					var previousClient ChannelModelOverridePatch
					require.NoError(t, common.UnmarshalJsonStr(fmt.Sprintf(`{"channel_id":%d,"model":"model-a","priority_override":15}`, channel.Id), &previousClient))
					require.NoError(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{previousClient}))
					routes, err = ListChannelModelRoutings(channel.Id)
					require.NoError(t, err)
					require.NotNil(t, routes[0].EffectiveCostDiscount)
					assert.Equal(t, 0.0, *routes[0].EffectiveCostDiscount)
					assert.Equal(t, "model", routes[0].CostDiscountSource)
					require.NotNil(t, routes[1].EffectiveCostDiscount)
					assert.Equal(t, 0.3, *routes[1].EffectiveCostDiscount)
					assert.Equal(t, "channel", routes[1].CostDiscountSource)
					matrix, err := ListChannelModelMatrix(ChannelModelMatrixFilter{ModelPageSize: 20, ChannelPageSize: 20})
					require.NoError(t, err)
					require.Len(t, matrix.Cells, 2)
					assert.Equal(t, routes[1].EffectiveCostDiscount, matrix.Cells[1].EffectiveCostDiscount)

					duplicate := ChannelModelOverride{ChannelId: channel.Id, Model: "model-a"}
					require.Error(t, db.Create(&duplicate).Error, "composite uniqueness survives migration")
					require.Error(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{
						{ChannelId: channel.Id, Model: "model-b", CostDiscount: common.GetPointer(0.1)},
						{ChannelId: channel.Id, Model: "model-a", CostDiscount: common.GetPointer(1.1)},
					}))
					unchanged, err := ListChannelModelRoutings(channel.Id)
					require.NoError(t, err)
					assert.Equal(t, routes, unchanged, "invalid batches are atomic")

					clone, err := CloneChannelWithModelOverrides(channel.Id, " copy", false)
					require.NoError(t, err)
					cloned, err := ListChannelModelRoutings(clone.Id)
					require.NoError(t, err)
					assert.Equal(t, routes[0].EffectiveCostDiscount, cloned[0].EffectiveCostDiscount)
					assert.Equal(t, routes[1].EffectiveCostDiscount, cloned[1].EffectiveCostDiscount)

					var reset ChannelModelOverridePatch
					require.NoError(t, common.UnmarshalJsonStr(fmt.Sprintf(`{"channel_id":%d,"model":"model-a","cost_discount_override":null}`, channel.Id), &reset))
					require.NoError(t, PatchChannelModelOverrides([]ChannelModelOverridePatch{reset}))
					routes, err = ListChannelModelRoutings(channel.Id)
					require.NoError(t, err)
					assert.Equal(t, common.GetPointer(0.3), routes[0].EffectiveCostDiscount)
					assert.Equal(t, "channel", routes[0].CostDiscountSource)
					var remaining int64
					require.NoError(t, db.Model(&ChannelModelOverride{}).Where("channel_id = ?", channel.Id).Count(&remaining).Error)
					assert.Zero(t, remaining, "clearing the last override prunes the row")

					sparseUpdate := Channel{Id: channel.Id, Name: "Renamed"}
					require.NoError(t, sparseUpdate.Update())
					assert.Equal(t, common.GetPointer(0.3), sparseUpdate.CostDiscount, "omitted defaults are preserved")
					sparseUpdate.CostDiscount = common.GetPointer(0.0)
					require.NoError(t, sparseUpdate.Update("cost_discount"))
					assert.Equal(t, common.GetPointer(0.0), sparseUpdate.CostDiscount)
					sparseUpdate.CostDiscount = nil
					require.NoError(t, sparseUpdate.Update("cost_discount"))
					routes, err = ListChannelModelRoutings(channel.Id)
					require.NoError(t, err)
					assert.Nil(t, routes[0].EffectiveCostDiscount)
					assert.Equal(t, "unknown", routes[0].CostDiscountSource)
				})
			}
		})
	}
}

// Channel schema verified against the latest public release v1.0.0-rc.47.
type releasedCostDiscountChannel struct {
	Id                 int     `json:"id"`
	Type               int     `json:"type" gorm:"default:0"`
	Key                string  `json:"key" gorm:"not null"`
	OpenAIOrganization *string `json:"openai_organization"`
	TestModel          *string `json:"test_model"`
	Status             int     `json:"status" gorm:"default:1"`
	Name               string  `json:"name" gorm:"index"`
	Weight             *uint   `json:"weight" gorm:"default:0"`
	CreatedTime        int64   `json:"created_time" gorm:"bigint"`
	TestTime           int64   `json:"test_time" gorm:"bigint"`
	ResponseTime       int     `json:"response_time"` // in milliseconds
	BaseURL            *string `json:"base_url" gorm:"column:base_url;default:''"`
	Other              string  `json:"other"`
	Balance            float64 `json:"balance"` // in USD
	BalanceUpdatedTime int64   `json:"balance_updated_time" gorm:"bigint"`
	Models             string  `json:"models"`
	Group              string  `json:"group" gorm:"type:varchar(64);default:'default'"`
	UsedQuota          int64   `json:"used_quota" gorm:"bigint;default:0"`
	ModelMapping       *string `json:"model_mapping" gorm:"type:text"`
	//MaxInputTokens     *int    `json:"max_input_tokens" gorm:"default:0"`
	StatusCodeMapping *string `json:"status_code_mapping" gorm:"type:varchar(1024);default:''"`
	Priority          *int64  `json:"priority" gorm:"bigint;default:0"`
	AutoBan           *int    `json:"auto_ban" gorm:"default:1"`
	OtherInfo         string  `json:"other_info"`
	Tag               *string `json:"tag" gorm:"index"`
	Setting           *string `json:"setting" gorm:"type:text"` // 渠道额外设置
	ParamOverride     *string `json:"param_override" gorm:"type:text"`
	HeaderOverride    *string `json:"header_override" gorm:"type:text"`
	Remark            *string `json:"remark" gorm:"type:varchar(255)" validate:"max=255"`
	// add after v0.8.5
	ChannelInfo ChannelInfo `json:"channel_info" gorm:"type:json"`

	OtherSettings string `json:"settings" gorm:"column:settings"` // 其他设置，存储azure版本等不需要检索的信息，详见dto.ChannelOtherSettings

	// cache info
	Keys []string `json:"-" gorm:"-"`
}

func (releasedCostDiscountChannel) TableName() string { return "channels" }
