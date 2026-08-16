package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelModelOverrideSQLiteSchemaSupportsSparseCompositeKey(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&ChannelModelOverride{}))
	require.NoError(t, DB.AutoMigrate(&ChannelModelOverride{}))

	priority := int64(0)
	first := ChannelModelOverride{ChannelId: 7101, Model: "model-a", Priority: &priority}
	require.NoError(t, DB.Create(&first).Error)
	duplicate := ChannelModelOverride{ChannelId: 7101, Model: "model-a"}
	require.Error(t, DB.Create(&duplicate).Error)
	secondModel := ChannelModelOverride{ChannelId: 7101, Model: "model-b"}
	secondChannel := ChannelModelOverride{ChannelId: 7102, Model: "model-a"}
	require.NoError(t, DB.Create(&secondModel).Error)
	require.NoError(t, DB.Create(&secondChannel).Error)

	var persisted ChannelModelOverride
	require.NoError(t, DB.Where("channel_id = ? AND model = ?", 7101, "model-a").First(&persisted).Error)
	require.NotNil(t, persisted.Priority)
	assert.Equal(t, int64(0), *persisted.Priority)
	assert.Nil(t, persisted.Weight)

	require.NoError(t, DB.Where("channel_id IN ?", []int{7101, 7102}).Delete(&ChannelModelOverride{}).Error)
}
