package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupModelChannelPolicyOptionPersistsReloadsAndRejectsInvalidWrites(t *testing.T) {
	db := setupGroupPricingOptionTest(t, `{"default":1}`, `{}`)
	original := setting.GroupModelChannelGroupsJSON()
	t.Cleanup(func() { require.NoError(t, setting.UpdateGroupModelChannelGroups(original)) })
	key := setting.GroupModelChannelGroupsOptionKey
	const value = `{"default":{"model-a":["official"],"model-b":[]}}`
	require.NoError(t, UpdateOption(key, value))
	var option Option
	require.NoError(t, db.First(&option, "key = ?", key).Error)
	assert.JSONEq(t, value, option.Value)
	require.NoError(t, setting.UpdateGroupModelChannelGroups(`{}`))
	loadOptionsFromDatabase()
	assert.JSONEq(t, value, setting.GroupModelChannelGroupsJSON())

	require.Error(t, UpdateOption(key, `null`))
	require.Error(t, UpdateOptionsBulk(map[string]string{key: `{"default":{"m":null}}`, "Notice": "must not persist"}))
	require.Error(t, updateOptionMap(key, `null`))
	require.NoError(t, db.First(&option, "key = ?", key).Error)
	assert.JSONEq(t, value, option.Value)
	assert.JSONEq(t, value, setting.GroupModelChannelGroupsJSON())
	assert.JSONEq(t, value, common.OptionMap[key])
	var count int64
	require.NoError(t, db.Model(&Option{}).Where("key = ?", "Notice").Count(&count).Error)
	assert.Zero(t, count)

	require.NoError(t, UpdateOptionsBulk(map[string]string{key: `{}`}))
	assert.JSONEq(t, `{}`, setting.GroupModelChannelGroupsJSON())
}
