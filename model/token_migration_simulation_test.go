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

// These simulations assert migration decisions and transaction boundaries, not
// PostgreSQL catalog-query compatibility or server-enforced uniqueness.
func TestTokenKeyMigrationPostgreSQLSimulation(t *testing.T) {
	for _, tc := range []struct {
		name, constraint, definition                   string
		deferrable, unvalidated                        bool
		invalidIndex, standalone, createFail, dropFail bool
		wantError                                      string
	}{
		{name: "already migrated"},
		{name: "preserve unknown constraint", constraint: "custom_key_unique", wantError: "unsupported unique constraint"},
		{name: "preserve deferrable semantics", constraint: postgresTokenKeyConstraint, deferrable: true, wantError: "unsupported definition"},
		{name: "preserve nulls not distinct semantics", constraint: postgresTokenKeyConstraint, definition: "UNIQUE NULLS NOT DISTINCT (key)", wantError: "unsupported definition"},
		{name: "reject unvalidated constraint", constraint: postgresTokenKeyConstraint, unvalidated: true, wantError: "unsupported definition"},
		{name: "replace postgres constraint", constraint: postgresTokenKeyConstraint},
		{name: "replace gorm constraint", constraint: gormTokenKeyConstraint},
		{name: "replace constraint owned target index", constraint: tokenKeyIndex},
		{name: "retain valid standalone index", constraint: postgresTokenKeyConstraint, standalone: true},
		{name: "invalid index preserves constraint", constraint: postgresTokenKeyConstraint, invalidIndex: true, wantError: "unexpected definition"},
		{name: "failed creation rolls back removal", constraint: tokenKeyIndex, createFail: true, wantError: "create token key unique index"},
		{name: "failed removal rolls back", constraint: postgresTokenKeyConstraint, dropFail: true, wantError: "drop token key unique constraint"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			connection, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = connection.Close() })
			db, err := gorm.Open(postgres.New(postgres.Config{Conn: connection, PreferSimpleProtocol: true}), &gorm.Config{
				DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent),
			})
			require.NoError(t, err)
			definition := tc.definition
			if definition == "" {
				definition = "UNIQUE (key)"
			}
			constraint := tokenKeyUniqueConstraint{Name: tc.constraint, Definition: definition, Deferrable: tc.deferrable, Validated: !tc.unvalidated}
			expectTokenMigrationConstraints(mock, constraint)
			migratable := tc.constraint != "" && tc.constraint != "custom_key_unique" && !tc.deferrable && !tc.unvalidated && tc.definition == ""
			if migratable {
				mock.ExpectBegin()
				mock.ExpectQuery(`SELECT count\(\*\) FROM information_schema.tables`).WithArgs("tokens", "BASE TABLE").
					WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectExec(regexp.QuoteMeta(`LOCK TABLE "tokens" IN ACCESS EXCLUSIVE MODE`)).WillReturnResult(sqlmock.NewResult(0, 0))
				expectTokenMigrationConstraints(mock, constraint)
				indexExists := tc.invalidIndex || tc.standalone || tc.constraint == tokenKeyIndex
				expectTokenMigrationIndex(mock, indexExists, indexExists && !tc.invalidIndex, tc.standalone)
				if !tc.invalidIndex {
					drop := mock.ExpectExec(regexp.QuoteMeta(`ALTER TABLE "tokens" DROP CONSTRAINT "` + tc.constraint + `"`))
					if tc.dropFail {
						drop.WillReturnError(errors.New("simulated constraint removal failure"))
					} else {
						drop.WillReturnResult(sqlmock.NewResult(0, 0))
						expectTokenMigrationIndex(mock, tc.standalone, tc.standalone, tc.standalone)
						if !tc.standalone {
							create := mock.ExpectExec(regexp.QuoteMeta(`CREATE UNIQUE INDEX IF NOT EXISTS "idx_tokens_key" ON "tokens" ("key")`))
							if tc.createFail {
								create.WillReturnError(errors.New("simulated index creation failure"))
							} else {
								create.WillReturnResult(sqlmock.NewResult(0, 0))
								expectTokenMigrationIndex(mock, true, true, true)
							}
						}
						if !tc.createFail {
							expectTokenMigrationConstraints(mock, tokenKeyUniqueConstraint{})
						}
					}
				}
				if tc.wantError != "" {
					mock.ExpectRollback()
				} else {
					mock.ExpectCommit()
				}
			}
			err = migrateTokenKeyUniqueness(db)
			if tc.wantError == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantError)
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func expectTokenMigrationConstraints(mock sqlmock.Sqlmock, constraint tokenKeyUniqueConstraint) {
	rows := sqlmock.NewRows([]string{"constraint_name", "constraint_definition", "is_deferrable", "is_validated"})
	if constraint.Name != "" {
		rows.AddRow(constraint.Name, constraint.Definition, constraint.Deferrable, constraint.Validated)
	}
	mock.ExpectQuery("SELECT constraint_meta.conname").WithArgs("tokens", "key").WillReturnRows(rows)
}

func expectTokenMigrationIndex(mock sqlmock.Sqlmock, exists, valid, standalone bool) {
	mock.ExpectQuery(`SELECT count\(\*\) > 0 AS index_exists`).WithArgs("key", "key", "tokens", "idx_tokens_key").
		WillReturnRows(sqlmock.NewRows([]string{"index_exists", "definition_valid", "standalone_valid"}).AddRow(exists, valid, standalone))
}

func TestTokenKeyMigrationMySQLSimulationDoesNotRunPostgreSQLDDL(t *testing.T) {
	connection, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: connection, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)
	require.NoError(t, migrateTokenKeyUniqueness(db))
	assert.NoError(t, mock.ExpectationsWereMet())
}
