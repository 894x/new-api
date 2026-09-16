package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUserAutoMigratePreservesLegacyRowsWithoutManager(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}))
	require.NoError(t, db.Migrator().DropColumn(&User{}, "ManagedByUserId"))
	require.NoError(t, db.Table("users").Create(map[string]any{
		"id": 1, "username": "legacy", "password": "", "role": 1, "status": 1,
	}).Error)

	require.NoError(t, db.AutoMigrate(&User{}))

	var user User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, "legacy", user.Username)
	assert.Nil(t, user.ManagedByUserId)
}
