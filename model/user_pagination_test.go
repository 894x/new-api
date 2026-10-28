package model

import (
	"fmt"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func insertUsersForPaginationTest(t *testing.T, total int) {
	t.Helper()
	for id := 1; id <= total; id++ {
		user := &User{
			Id:          id,
			Username:    fmt.Sprintf("user%02d", id),
			Password:    "password123",
			DisplayName: fmt.Sprintf("User %02d", id),
			Email:       fmt.Sprintf("user%02d@example.com", id),
			Role:        common.RoleCommonUser,
			Status:      common.UserStatusEnabled,
			Group:       "default",
			AffCode:     fmt.Sprintf("aff%02d", id),
		}
		require.NoError(t, DB.Create(user).Error)
	}
}

func collectUserIDs(users []*User) []int {
	ids := make([]int, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.Id)
	}
	return ids
}

func TestGetAllUsersSortsBeforePagination(t *testing.T) {
	truncateTables(t)
	insertUsersForPaginationTest(t, 42)

	pageOne, total, err := GetAllUsers(&common.PageInfo{Page: 1, PageSize: 20}, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.Equal(t, int64(42), total)
	assert.Equal(t, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}, collectUserIDs(pageOne))

	pageTwo, total, err := GetAllUsers(&common.PageInfo{Page: 2, PageSize: 20}, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.Equal(t, int64(42), total)
	assert.Equal(t, []int{21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40}, collectUserIDs(pageTwo))

	pageThree, total, err := GetAllUsers(&common.PageInfo{Page: 3, PageSize: 20}, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.Equal(t, int64(42), total)
	assert.Equal(t, []int{41, 42}, collectUserIDs(pageThree))
}

func TestSearchUsersSortsBeforePagination(t *testing.T) {
	truncateTables(t)
	insertUsersForPaginationTest(t, 42)

	users, total, err := SearchUsers("user", "", nil, nil, 20, 20, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.Equal(t, int64(42), total)
	assert.Equal(t, []int{21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40}, collectUserIDs(users))
}

func TestUserDefaultBalanceOrderDatabaseMatrix(t *testing.T) {
	dialect := os.Getenv("TEST_USER_SORT_DIALECT")
	var driver gorm.Dialector
	switch dialect {
	case "", "sqlite":
		dialect = "sqlite"
		driver = sqlite.Open(":memory:")
	case "mysql":
		require.NotEmpty(t, os.Getenv("TEST_MYSQL_DSN"))
		driver = mysql.Open(os.Getenv("TEST_MYSQL_DSN"))
	case "postgres":
		require.NotEmpty(t, os.Getenv("TEST_POSTGRES_DSN"))
		driver = postgres.Open(os.Getenv("TEST_POSTGRES_DSN"))
	default:
		t.Fatalf("unsupported test dialect: %s", dialect)
	}
	db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "user_sort_test_"}})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB, previousType := DB, common.MainDatabaseType()
	DB = db
	common.SetMainDatabaseType(common.DatabaseType(dialect))
	initCol()
	t.Cleanup(func() {
		assert.NoError(t, db.Migrator().DropTable(&User{}))
		DB = previousDB
		common.SetMainDatabaseType(previousType)
		initCol()
		assert.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&User{}))
	versionQuery := "SELECT version()"
	if dialect == "sqlite" {
		versionQuery = "SELECT sqlite_version()"
	}
	var version string
	require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
	t.Logf("%s version: %s", dialect, version)
	insertUsersForPaginationTest(t, 4)
	for i, values := range []struct{ used, remaining int }{{200, -10}, {100, 40}, {200, 0}, {0, 40}} {
		require.NoError(t, db.Model(&User{}).Where("id = ?", i+1).Updates(map[string]any{
			"used_quota": values.used,
			"quota":      values.remaining,
		}).Error)
	}
	for _, tc := range []struct {
		name    string
		page    int
		options []UserSortOptions
		want    []int
	}{
		{name: "omitted sort starts with highest balance and breaks ties by ID", page: 1, want: []int{4, 2}},
		{name: "omitted sort continues across pages", page: 2, want: []int{3, 1}},
		{name: "empty API sort defaults to descending balance", page: 1, options: []UserSortOptions{NewUserSortOptions("", "")}, want: []int{4, 2}},
		{name: "manual balance sort overrides the default", page: 1, options: []UserSortOptions{NewUserSortOptions("quota", "asc")}, want: []int{1, 3}},
		{name: "manual ID sort overrides the default", page: 1, options: []UserSortOptions{NewUserSortOptions("id", "asc")}, want: []int{1, 2}},
		{name: "invalid sort retains safe ID fallback", page: 1, options: []UserSortOptions{NewUserSortOptions("quota; DROP TABLE users", "asc")}, want: []int{4, 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			users, total, err := GetAllUsers(&common.PageInfo{Page: tc.page, PageSize: 2}, tc.options...)
			require.NoError(t, err)
			assert.Equal(t, int64(4), total)
			assert.Equal(t, tc.want, collectUserIDs(users))

			users, total, err = SearchUsers("user", "default", nil, nil, (tc.page-1)*2, 2, tc.options...)
			require.NoError(t, err)
			assert.Equal(t, int64(4), total)
			assert.Equal(t, tc.want, collectUserIDs(users))
		})
	}
}
