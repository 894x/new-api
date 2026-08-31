package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestApplyBillingUsageDeltaPersistsUserChannelAndRequestTogether(t *testing.T) {
	truncateTables(t)
	previousBatchUpdateEnabled := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = true
	t.Cleanup(func() { common.BatchUpdateEnabled = previousBatchUpdateEnabled })

	user := User{
		Username:     "sync-billing-usage-user",
		Password:     "password",
		Status:       common.UserStatusEnabled,
		UsedQuota:    100,
		RequestCount: 2,
	}
	channel := Channel{
		Name:      "sync-billing-usage-channel",
		Key:       "sk-sync-billing-usage",
		Status:    common.ChannelStatusEnabled,
		UsedQuota: 100,
	}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, DB.Create(&channel).Error)

	require.NoError(t, ApplyBillingUsageDelta(BillingUsageDelta{
		UserID:            user.Id,
		ChannelID:         channel.Id,
		QuotaDelta:        25,
		RequestCountDelta: 1,
	}))

	var storedUser User
	require.NoError(t, DB.Select("used_quota", "request_count").First(&storedUser, user.Id).Error)
	assert.Equal(t, 125, storedUser.UsedQuota)
	assert.Equal(t, 3, storedUser.RequestCount)
	var storedChannel Channel
	require.NoError(t, DB.Select("used_quota").First(&storedChannel, channel.Id).Error)
	assert.Equal(t, int64(125), storedChannel.UsedQuota)
}

func TestApplyBillingUsageDeltaRollsBackUserWhenChannelWriteFails(t *testing.T) {
	truncateTables(t)
	user := User{
		Username:     "rollback-billing-usage-user",
		Password:     "password",
		Status:       common.UserStatusEnabled,
		UsedQuota:    100,
		RequestCount: 2,
	}
	channel := Channel{
		Name:      "rollback-billing-usage-channel",
		Key:       "sk-rollback-billing-usage",
		Status:    common.ChannelStatusEnabled,
		UsedQuota: 100,
	}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, DB.Create(&channel).Error)

	forcedErr := errors.New("forced channel accounting failure")
	callbackName := "test:rollback_billing_usage_channel"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "channels" {
			tx.AddError(forcedErr)
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Update().Remove(callbackName) })

	err := ApplyBillingUsageDelta(BillingUsageDelta{
		UserID:            user.Id,
		ChannelID:         channel.Id,
		QuotaDelta:        25,
		RequestCountDelta: 1,
	})
	require.ErrorIs(t, err, forcedErr)

	var storedUser User
	require.NoError(t, DB.Select("used_quota", "request_count").First(&storedUser, user.Id).Error)
	assert.Equal(t, 100, storedUser.UsedQuota)
	assert.Equal(t, 2, storedUser.RequestCount)
	var storedChannel Channel
	require.NoError(t, DB.Select("used_quota").First(&storedChannel, channel.Id).Error)
	assert.Equal(t, int64(100), storedChannel.UsedQuota)
}
