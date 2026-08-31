package model

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// Quota versions deliberately have no database default. Existing databases
// therefore need a two-phase migration: add a nullable column, backfill zero,
// then tighten it where the dialect can do so without rebuilding the table.
type quotaVersionNullableUserColumn struct {
	QuotaVersion *int64 `gorm:"type:bigint;column:quota_version"`
}

func (quotaVersionNullableUserColumn) TableName() string { return "users" }

type quotaVersionNullableTokenColumn struct {
	QuotaVersion *int64 `gorm:"type:bigint;column:quota_version"`
}

func (quotaVersionNullableTokenColumn) TableName() string { return "tokens" }

type quotaVersionNotNullUserColumn struct {
	QuotaVersion int64 `gorm:"type:bigint;not null;column:quota_version"`
}

func (quotaVersionNotNullUserColumn) TableName() string { return "users" }

type quotaVersionNotNullTokenColumn struct {
	QuotaVersion int64 `gorm:"type:bigint;not null;column:quota_version"`
}

func (quotaVersionNotNullTokenColumn) TableName() string { return "tokens" }

type quotaVersionMigrationTarget struct {
	table    string
	nullable interface{}
	notNull  interface{}
}

func quotaVersionMigrationTargets() []quotaVersionMigrationTarget {
	return []quotaVersionMigrationTarget{
		{
			table:    "users",
			nullable: &quotaVersionNullableUserColumn{},
			notNull:  &quotaVersionNotNullUserColumn{},
		},
		{
			table:    "tokens",
			nullable: &quotaVersionNullableTokenColumn{},
			notNull:  &quotaVersionNotNullTokenColumn{},
		},
	}
}

func prepareQuotaVersionMigration(db *gorm.DB) error {
	return prepareQuotaVersionMigrationTargets(db, quotaVersionMigrationTargets())
}

func prepareQuotaVersionMigrationTargets(db *gorm.DB, targets []quotaVersionMigrationTarget) error {
	if db == nil {
		return fmt.Errorf("quota version migration database is nil")
	}
	for _, target := range targets {
		if !db.Migrator().HasTable(target.nullable) {
			continue
		}
		if !db.Migrator().HasColumn(target.nullable, "QuotaVersion") {
			if err := db.Migrator().AddColumn(target.nullable, "QuotaVersion"); err != nil {
				return fmt.Errorf("add %s.quota_version: %w", target.table, err)
			}
		}
		if err := db.Table(target.table).
			Where("quota_version IS NULL").
			Update("quota_version", int64(0)).Error; err != nil {
			return fmt.Errorf("backfill %s.quota_version: %w", target.table, err)
		}
	}
	return nil
}

func finalizeQuotaVersionMigration(db *gorm.DB) error {
	return finalizeQuotaVersionMigrationTargets(db, quotaVersionMigrationTargets())
}

func finalizeQuotaVersionMigrationTargets(db *gorm.DB, targets []quotaVersionMigrationTarget) error {
	if err := prepareQuotaVersionMigrationTargets(db, targets); err != nil {
		return err
	}
	// SQLite requires a full table rebuild to tighten nullability. Keeping the
	// physical column nullable avoids destructive migration; startup backfill,
	// COALESCE increments, and cache reads enforce the logical non-null value.
	if strings.EqualFold(db.Dialector.Name(), "sqlite") {
		return nil
	}
	for _, target := range targets {
		if !db.Migrator().HasTable(target.notNull) {
			continue
		}
		columnTypes, err := db.Migrator().ColumnTypes(target.notNull)
		if err != nil {
			return fmt.Errorf("inspect %s.quota_version: %w", target.table, err)
		}
		alreadyNotNull := false
		for _, columnType := range columnTypes {
			if !strings.EqualFold(columnType.Name(), "quota_version") {
				continue
			}
			if nullable, ok := columnType.Nullable(); ok && !nullable {
				alreadyNotNull = true
			}
			break
		}
		if alreadyNotNull {
			continue
		}
		if err := db.Migrator().AlterColumn(target.notNull, "QuotaVersion"); err != nil {
			return fmt.Errorf("set %s.quota_version not null: %w", target.table, err)
		}
	}
	return nil
}
