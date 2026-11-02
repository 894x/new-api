package service

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupGroupModelChannelPolicyTest(t *testing.T) *gorm.DB {
	t.Helper()
	// Initialize the dialect-specific columns through the normal DB entry point.
	originalDB, originalPath, originalMaster := model.DB, common.SQLitePath, common.IsMasterNode
	originalMainType, originalLogType := common.MainDatabaseType(), common.LogDatabaseType()
	t.Setenv("SQL_DSN", "local")
	t.Setenv("LOG_SQL_DSN", "")
	common.SQLitePath, common.IsMasterNode = filepath.Join(t.TempDir(), "policy.db"), false
	require.NoError(t, model.InitDB())
	bootstrapDB, err := model.DB.DB()
	require.NoError(t, err)
	common.SQLitePath, common.IsMasterNode = originalPath, originalMaster
	t.Cleanup(func() {
		require.NoError(t, bootstrapDB.Close())
		model.DB = originalDB
		common.SetDatabaseTypes(originalMainType, originalLogType)
	})
	db := setupChannelSelectAutoGroupsTest(t)
	createChannelSelectAutoGroupsChannel(t, db, 4101, "vip", "model-a")
	createChannelSelectAutoGroupsChannel(t, db, 4102, "vip", "model-a")
	createChannelSelectAutoGroupsChannel(t, db, 4103, "pool", "model-a")
	createChannelSelectAutoGroupsChannel(t, db, 4104, "vip", "denied")
	// A disallowed high-priority channel must not prevent X from being selected.
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 4102).Update("priority", 100).Error)
	require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", 4102).Update("priority", 100).Error)
	pool := model.ModelRoutingPool{RoutingGroup: "vip", Model: "model-a", Mode: model.RoutingModePriority, Channels: []model.ModelRoutingPoolChannel{{ChannelID: 4101, Priority: 0, Weight: 100, Enabled: true}}}
	require.NoError(t, model.SaveModelRoutingPool(&pool, ""))
	model.InitChannelCache()
	return db
}

func TestRoutingPoolEnforcesPublicGroupMembership(t *testing.T) {
	for _, cache := range []bool{true, false} {
		for _, dynamic := range []bool{true, false} {
			t.Run(fmt.Sprintf("cache=%t/dynamic=%t", cache, dynamic), func(t *testing.T) {
				setupGroupModelChannelPolicyTest(t)
				common.MemoryCacheEnabled = cache
				configureDynamicRoutingForTest(t, dynamic)
				ctx := newChannelSelectContext()
				common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "vip")
				param := &RetryParam{Ctx: ctx, TokenGroup: "vip", ModelName: "model-a", RequestPath: "/v1/chat/completions", DynamicRoutingEligible: dynamic}
				channel, group, err := CacheGetRandomSatisfiedChannel(param)
				require.NoError(t, err)
				require.NotNil(t, channel)
				assert.Equal(t, 4101, channel.Id)
				assert.Equal(t, "vip", group, "billing group remains the token group")
				assert.NoError(t, ValidateSelectedChannelGroupPolicy(ctx, 4101, "model-a"))
				assert.ErrorIs(t, ValidateSelectedChannelGroupPolicy(ctx, 4102, "model-a"), ErrGroupModelChannelDenied)
				assert.ErrorIs(t, ValidateSelectedChannelGroupPolicy(ctx, 4103, "model-a"), ErrGroupModelChannelDenied)
				models, err := GetUserGroupsEnabledModels("default", []string{"vip"})
				require.NoError(t, err)
				assert.ElementsMatch(t, []string{"model-a", "denied"}, models, "pool membership does not change public model visibility")
			})
		}
	}
}

func TestRoutingPoolComposesAssetsAutoAndCapacitySpillover(t *testing.T) {
	setupGroupModelChannelPolicyTest(t)
	ctx := newChannelSelectContext()
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "auto")
	common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"default", "vip"})
	param := &RetryParam{Ctx: ctx, TokenGroup: "auto", ModelName: "model-a", RequestPath: "/v1/chat/completions", AllowedChannelIds: map[int]struct{}{4101: {}, 4102: {}}}
	channel, group, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 4101, channel.Id)
	assert.Equal(t, "vip", group)
	assert.Len(t, param.AllowedChannelIds, 2, "policy intersection must not mutate the asset constraint")

	param.TokenGroup = "vip"
	param.Capacity = &ChannelCapacityState{blocked: map[int]struct{}{4101: {}}, eligible: map[int]struct{}{}}
	channel, _, err = CacheGetRandomSatisfiedChannel(param)
	assert.Nil(t, channel, "spillover must not escape to channel 4102")
	var capacityErr *ChannelModelCapacityError
	assert.ErrorAs(t, err, &capacityErr)

	param.Capacity = nil
	param.AllowedChannelIds = map[int]struct{}{4102: {}}
	channel, _, err = CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	assert.Nil(t, channel, "an empty intersection must not fall back to unrestricted routing")
}

func TestRoutingPoolReloadsMembership(t *testing.T) {
	setupGroupModelChannelPolicyTest(t)
	ctx := newChannelSelectContext()
	param := &RetryParam{Ctx: ctx, TokenGroup: "vip", ModelName: "model-a"}
	channel, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 4101, channel.Id, "the routing pool selects its explicit member")
	pool, err := model.GetModelRoutingPool("vip", "model-a")
	require.NoError(t, err)
	require.NotNil(t, pool)
	revision := pool.Revision
	pool.Channels = []model.ModelRoutingPoolChannel{}
	require.NoError(t, model.SaveModelRoutingPool(pool, revision))
	param = &RetryParam{Ctx: ctx, TokenGroup: "vip", ModelName: "model-a"}
	channel, _, err = CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	assert.Nil(t, channel)

	revision = pool.Revision
	pool.Channels = []model.ModelRoutingPoolChannel{{ChannelID: 4102, Priority: 100, Weight: 10, Enabled: true}}
	require.NoError(t, model.SaveModelRoutingPool(pool, revision))
	param = &RetryParam{Ctx: ctx, TokenGroup: "vip", ModelName: "model-a"}
	channel, _, err = CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 4102, channel.Id, "a new request sees the edited pool")

	param.ModelName = "denied"
	param = &RetryParam{Ctx: ctx, TokenGroup: "vip", ModelName: "denied"}
	channel, _, err = CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 4104, channel.Id, "missing policy preserves existing behavior")
}
