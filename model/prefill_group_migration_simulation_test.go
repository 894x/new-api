package model

import (
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// SQL simulations verify the host's safety decisions and transaction boundary,
// not whether a PostgreSQL server accepts the catalog SQL or enforces the index.
func TestPrefillGroupMigrationPostgreSQLSimulation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		constraint string
		index      string
		create     bool
		invalid    bool
		createFail bool
		dropFail   bool
		wantError  string
	}{
		{name: "already migrated"},
		{name: "preserve unknown constraint", constraint: "custom_name_constraint", wantError: "unsupported global unique"},
		{name: "preserve unknown index", index: "custom_name_index", wantError: "unsupported global unique"},
		{name: "replace legacy constraint", constraint: legacyPrefillGroupNameUnique, create: true},
		{name: "replace legacy index", index: legacyPrefillGroupNameUnique},
		{name: "invalid replacement preserves legacy", constraint: legacyPrefillGroupNameUnique, invalid: true, wantError: "unexpected definition"},
		{name: "failed replacement rolls back", constraint: legacyPrefillGroupNameUnique, create: true, createFail: true, wantError: "create prefill group partial unique index"},
		{name: "failed legacy removal rolls back", constraint: legacyPrefillGroupNameUnique, dropFail: true, wantError: "drop conflicting prefill group constraint"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			connection, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = connection.Close() })
			db, err := gorm.Open(postgres.New(postgres.Config{Conn: connection, PreferSimpleProtocol: true}), &gorm.Config{
				DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent),
			})
			require.NoError(t, err)
			expectPrefillMigrationMetadata(mock, tc.constraint, tc.index)
			knownConflict := tc.constraint == legacyPrefillGroupNameUnique || tc.index == legacyPrefillGroupNameUnique
			if knownConflict {
				mock.ExpectBegin()
				mock.ExpectQuery(`SELECT count\(\*\) FROM information_schema.tables`).WithArgs("prefill_groups", "BASE TABLE").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectExec(regexp.QuoteMeta(`LOCK TABLE "prefill_groups" IN ACCESS EXCLUSIVE MODE`)).
					WillReturnResult(sqlmock.NewResult(0, 0))
				expectPrefillMigrationMetadata(mock, tc.constraint, tc.index)
				mock.ExpectQuery(`SELECT count\(\*\) FROM INFORMATION_SCHEMA.columns`).WithArgs("prefill_groups", "deleted_at").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectQuery(`SELECT count\(\*\) > 0 AS index_exists`).WithArgs("name", "prefill_groups", prefillGroupNameIndex).
					WillReturnRows(sqlmock.NewRows([]string{"index_exists", "index_valid"}).AddRow(!tc.create, !tc.create && !tc.invalid))
				if tc.create {
					create := mock.ExpectExec(regexp.QuoteMeta(`CREATE UNIQUE INDEX IF NOT EXISTS "uk_prefill_name" ON "prefill_groups" ("name") WHERE deleted_at IS NULL`))
					if tc.createFail {
						create.WillReturnError(errors.New("simulated index creation failure"))
					} else {
						create.WillReturnResult(sqlmock.NewResult(0, 0))
						mock.ExpectQuery(`SELECT count\(\*\) > 0 AS index_exists`).WithArgs("name", "prefill_groups", prefillGroupNameIndex).
							WillReturnRows(sqlmock.NewRows([]string{"index_exists", "index_valid"}).AddRow(true, true))
					}
				}
				if !tc.invalid && !tc.createFail {
					dropSQL := `DROP INDEX "idx_prefill_groups_name"`
					if tc.constraint != "" {
						dropSQL = `ALTER TABLE "prefill_groups" DROP CONSTRAINT "idx_prefill_groups_name"`
					}
					drop := mock.ExpectExec(regexp.QuoteMeta(dropSQL))
					if tc.dropFail {
						drop.WillReturnError(errors.New("simulated constraint removal failure"))
					} else {
						drop.WillReturnResult(sqlmock.NewResult(0, 0))
					}
				}
				if tc.wantError != "" {
					mock.ExpectRollback()
				} else {
					mock.ExpectCommit()
				}
			}
			err = migratePrefillGroupUniqueness(db)
			if tc.wantError == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantError)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func expectPrefillMigrationMetadata(mock sqlmock.Sqlmock, constraint, index string) {
	constraints := sqlmock.NewRows([]string{"conname"})
	if constraint != "" {
		constraints.AddRow(constraint)
	}
	indexes := sqlmock.NewRows([]string{"relname"})
	if index != "" {
		indexes.AddRow(index)
	}
	mock.ExpectQuery("SELECT constraint_meta.conname").WithArgs("prefill_groups", "name").WillReturnRows(constraints)
	mock.ExpectQuery("SELECT index_class.relname").WithArgs("prefill_groups", "name").WillReturnRows(indexes)
}

func TestPrefillGroupMigrationMySQLSimulationDoesNotRunPostgreSQLDDL(t *testing.T) {
	connection, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: connection, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)
	require.NoError(t, migratePrefillGroupUniqueness(db))
	assert.NoError(t, mock.ExpectationsWereMet())
}
