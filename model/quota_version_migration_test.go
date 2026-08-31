package model

import (
	"bytes"
	stdlog "log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestQuotaVersionSQLiteLegacyMigrationStartsAtZeroAndAdvances(t *testing.T) {
	databasePath := filepath.ToSlash(filepath.Join(t.TempDir(), "quota-version-legacy.db"))
	db, err := gorm.Open(sqlite.Open("file:"+databasePath+"?_pragma=busy_timeout(10000)"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	// Build the immediately preceding schema, then remove only the two new
	// columns. This avoids testing unrelated historical User/Token migrations.
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}))
	require.NoError(t, db.Migrator().DropColumn(&User{}, "QuotaVersion"))
	require.NoError(t, db.Migrator().DropColumn(&Token{}, "QuotaVersion"))
	require.False(t, db.Migrator().HasColumn(&User{}, "QuotaVersion"))
	require.False(t, db.Migrator().HasColumn(&Token{}, "QuotaVersion"))

	require.NoError(t, db.Table("users").Create(map[string]interface{}{
		"id":           901,
		"username":     "quota-version-legacy-user",
		"password":     "unused-password-hash",
		"auth_version": 1,
		"quota":        100,
		"aff_code":     "quota-version-legacy-aff",
	}).Error)
	require.NoError(t, db.Table("tokens").Create(map[string]interface{}{
		"id":           902,
		"user_id":      901,
		"key":          "quota-version-legacy-token",
		"name":         "legacy",
		"status":       common.TokenStatusEnabled,
		"expired_time": -1,
		"remain_quota": 80,
	}).Error)

	require.NoError(t, prepareQuotaVersionMigration(db))
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}))
	require.NoError(t, finalizeQuotaVersionMigration(db))
	// Startup migration is deliberately idempotent, including the interrupted
	// state where nullable columns already exist and have been backfilled.
	require.NoError(t, prepareQuotaVersionMigration(db))
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}))
	require.NoError(t, finalizeQuotaVersionMigration(db))
	require.True(t, db.Migrator().HasColumn(&User{}, "QuotaVersion"))
	require.True(t, db.Migrator().HasColumn(&Token{}, "QuotaVersion"))
	assertQuotaVersionNullability(t, db, &User{}, nil)
	assertQuotaVersionNullability(t, db, &Token{}, nil)

	previousDB := DB
	previousRedisEnabled := common.RedisEnabled
	DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		DB = previousDB
		common.RedisEnabled = previousRedisEnabled
	})

	var legacyUser User
	require.NoError(t, DB.First(&legacyUser, 901).Error)
	assert.Zero(t, legacyUser.QuotaVersion)
	require.NoError(t, DB.Table("users").Where("id = ?", legacyUser.Id).Update("quota_version", nil).Error)
	reserved, err := ReserveUserQuotaForBilling(legacyUser.Id, 25, true)
	require.NoError(t, err)
	require.True(t, reserved)
	require.NoError(t, DB.First(&legacyUser, 901).Error)
	assert.Equal(t, 75, legacyUser.Quota)
	assert.EqualValues(t, 1, legacyUser.QuotaVersion)

	var legacyToken Token
	require.NoError(t, DB.First(&legacyToken, 902).Error)
	assert.Zero(t, legacyToken.QuotaVersion)
	require.NoError(t, DB.Table("tokens").Where("id = ?", legacyToken.Id).Update("quota_version", nil).Error)
	reserved, err = ReserveTokenQuotaForBilling(legacyToken.Id, legacyToken.Key, 20, false)
	require.NoError(t, err)
	require.True(t, reserved)
	require.NoError(t, DB.First(&legacyToken, 902).Error)
	assert.Equal(t, 60, legacyToken.RemainQuota)
	assert.Equal(t, 20, legacyToken.UsedQuota)
	assert.EqualValues(t, 1, legacyToken.QuotaVersion)
}

type quotaVersionContractUser struct {
	Id           int   `gorm:"primaryKey"`
	Quota        int   `gorm:"type:int;not null"`
	QuotaVersion int64 `gorm:"type:bigint;column:quota_version"`
}

func (quotaVersionContractUser) TableName() string { return "quota_version_contract_users" }

type quotaVersionContractToken struct {
	Id           int   `gorm:"primaryKey"`
	RemainQuota  int   `gorm:"type:int;not null;column:remain_quota"`
	UsedQuota    int   `gorm:"type:int;not null;column:used_quota"`
	QuotaVersion int64 `gorm:"type:bigint;column:quota_version"`
}

func (quotaVersionContractToken) TableName() string { return "quota_version_contract_tokens" }

type quotaVersionContractLegacyUser struct {
	Id    int `gorm:"primaryKey"`
	Quota int `gorm:"type:int;not null"`
}

func (quotaVersionContractLegacyUser) TableName() string { return "quota_version_contract_users" }

type quotaVersionContractLegacyToken struct {
	Id          int `gorm:"primaryKey"`
	RemainQuota int `gorm:"type:int;not null;column:remain_quota"`
	UsedQuota   int `gorm:"type:int;not null;column:used_quota"`
}

func (quotaVersionContractLegacyToken) TableName() string { return "quota_version_contract_tokens" }

type quotaVersionContractNotNullUser struct {
	QuotaVersion int64 `gorm:"type:bigint;not null;column:quota_version"`
}

func (quotaVersionContractNotNullUser) TableName() string { return "quota_version_contract_users" }

type quotaVersionContractNotNullToken struct {
	QuotaVersion int64 `gorm:"type:bigint;not null;column:quota_version"`
}

func (quotaVersionContractNotNullToken) TableName() string { return "quota_version_contract_tokens" }

func TestQuotaVersionCrossDatabaseAtomicMutationContract(t *testing.T) {
	tests := []struct {
		name    string
		envName string
		open    func(string) gorm.Dialector
	}{
		{
			name: "sqlite",
			open: func(dsn string) gorm.Dialector {
				return sqlite.Open(dsn)
			},
		},
		{
			name:    "mysql",
			envName: "TEST_MYSQL_DSN",
			open: func(dsn string) gorm.Dialector {
				return mysql.Open(dsn)
			},
		},
		{
			name:    "postgres",
			envName: "TEST_POSTGRES_DSN",
			open: func(dsn string) gorm.Dialector {
				return postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dsn := ""
			if test.envName == "" {
				databasePath := filepath.ToSlash(filepath.Join(t.TempDir(), "quota-version-contract.db"))
				dsn = "file:" + databasePath + "?_pragma=busy_timeout(10000)"
			} else {
				dsn = strings.TrimSpace(os.Getenv(test.envName))
				if dsn == "" {
					t.Skip(test.envName + " is not configured")
				}
			}

			db, err := gorm.Open(test.open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			require.NoError(t, sqlDB.Ping())
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

			require.NoError(t, db.Migrator().DropTable(&quotaVersionContractToken{}, &quotaVersionContractUser{}))
			require.NoError(t, db.AutoMigrate(&quotaVersionContractLegacyUser{}, &quotaVersionContractLegacyToken{}))
			t.Cleanup(func() {
				require.NoError(t, db.Migrator().DropTable(&quotaVersionContractToken{}, &quotaVersionContractUser{}))
			})

			// Users exercises an existing populated table; tokens exercises an
			// existing empty table. Both are then replayed to prove idempotency.
			require.NoError(t, db.Create(&quotaVersionContractLegacyUser{Id: 903, Quota: 100}).Error)
			targets := []quotaVersionMigrationTarget{
				{
					table:    "quota_version_contract_users",
					nullable: &quotaVersionContractUser{},
					notNull:  &quotaVersionContractNotNullUser{},
				},
				{
					table:    "quota_version_contract_tokens",
					nullable: &quotaVersionContractToken{},
					notNull:  &quotaVersionContractNotNullToken{},
				},
			}
			require.NoError(t, prepareQuotaVersionMigrationTargets(db, targets))
			require.NoError(t, db.AutoMigrate(&quotaVersionContractUser{}, &quotaVersionContractToken{}))
			require.NoError(t, finalizeQuotaVersionMigrationTargets(db, targets))

			var secondMigrationSQL bytes.Buffer
			secondMigrationDB := db.Session(&gorm.Session{Logger: logger.New(
				stdlog.New(&secondMigrationSQL, "", 0),
				logger.Config{LogLevel: logger.Info},
			)})
			require.NoError(t, prepareQuotaVersionMigrationTargets(secondMigrationDB, targets))
			require.NoError(t, secondMigrationDB.AutoMigrate(&quotaVersionContractUser{}, &quotaVersionContractToken{}))
			require.NoError(t, finalizeQuotaVersionMigrationTargets(secondMigrationDB, targets))
			if test.name != "sqlite" {
				for _, statement := range strings.Split(strings.ToLower(secondMigrationSQL.String()), "\n") {
					if strings.Contains(statement, "alter table") && strings.Contains(statement, "quota_version") {
						require.Failf(t, "quota_version migration emitted repeat DDL", "second full startup sequence emitted: %s", statement)
					}
				}
			}

			var wantNullable *bool
			if test.name != "sqlite" {
				notNullable := false
				wantNullable = &notNullable
			}
			assertQuotaVersionNullability(t, db, &quotaVersionContractUser{}, wantNullable)
			assertQuotaVersionNullability(t, db, &quotaVersionContractToken{}, wantNullable)
			assertQuotaVersionHasNoNulls(t, db, "quota_version_contract_users")
			assertQuotaVersionHasNoNulls(t, db, "quota_version_contract_tokens")

			var user quotaVersionContractUser
			require.NoError(t, db.First(&user, 903).Error)
			token := quotaVersionContractToken{Id: 904, RemainQuota: 80}
			require.NoError(t, db.Create(&token).Error)
			assert.Zero(t, user.QuotaVersion)
			assert.Zero(t, token.QuotaVersion)

			result := db.Model(&quotaVersionContractUser{}).
				Where("id = ? AND quota >= ?", user.Id, 30).
				Updates(map[string]interface{}{
					"quota":         gorm.Expr("quota - ?", 30),
					"quota_version": gorm.Expr("COALESCE(quota_version, 0) + 1"),
				})
			require.NoError(t, result.Error)
			require.EqualValues(t, 1, result.RowsAffected)
			require.NoError(t, db.First(&user, user.Id).Error)
			assert.Equal(t, 70, user.Quota)
			assert.EqualValues(t, 1, user.QuotaVersion)

			result = db.Model(&quotaVersionContractToken{}).
				Where("id = ? AND remain_quota >= ?", token.Id, 25).
				Updates(map[string]interface{}{
					"remain_quota":  gorm.Expr("remain_quota - ?", 25),
					"used_quota":    gorm.Expr("used_quota + ?", 25),
					"quota_version": gorm.Expr("COALESCE(quota_version, 0) + 1"),
				})
			require.NoError(t, result.Error)
			require.EqualValues(t, 1, result.RowsAffected)
			require.NoError(t, db.First(&token, token.Id).Error)
			assert.Equal(t, 55, token.RemainQuota)
			assert.Equal(t, 25, token.UsedQuota)
			assert.EqualValues(t, 1, token.QuotaVersion)
		})
	}
}

func assertQuotaVersionNullability(t *testing.T, db *gorm.DB, model interface{}, wantNullable *bool) {
	t.Helper()
	columnTypes, err := db.Migrator().ColumnTypes(model)
	require.NoError(t, err)
	for _, columnType := range columnTypes {
		if strings.EqualFold(columnType.Name(), "quota_version") {
			nullable, ok := columnType.Nullable()
			require.True(t, ok)
			if wantNullable != nil {
				assert.Equal(t, *wantNullable, nullable)
			}
			return
		}
	}
	require.Fail(t, "quota_version column was not migrated")
}

func assertQuotaVersionHasNoNulls(t *testing.T, db *gorm.DB, table string) {
	t.Helper()
	var count int64
	require.NoError(t, db.Table(table).Where("quota_version IS NULL").Count(&count).Error)
	assert.Zero(t, count)
}
