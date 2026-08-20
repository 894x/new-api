package service

import (
	"fmt"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/dynamicrouting"
	settingconfig "github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/dynamic_routing_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func resetDynamicRoutingControllerForTest() {
	dynamicRoutingRuntime.Lock()
	defer dynamicRoutingRuntime.Unlock()
	dynamicRoutingRuntime.controller = nil
	dynamicRoutingRuntime.version = 0
}

func configureDynamicRoutingForTest(t *testing.T, enabled bool) {
	t.Helper()
	raw := settingconfig.GlobalConfig.Get("dynamic_routing_setting")
	require.NotNil(t, raw)
	original := dynamic_routing_setting.GetSetting()
	originalMap, err := settingconfig.ConfigToMap(original)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, settingconfig.UpdateConfigFromMap(raw, originalMap))
		require.NoError(t, dynamic_routing_setting.UpdateAndSync())
		resetDynamicRoutingControllerForTest()
	})

	require.NoError(t, settingconfig.UpdateConfigFromMap(raw, map[string]string{
		"enabled":        strconv.FormatBool(enabled),
		"probe_fraction": "0.05",
	}))
	require.NoError(t, dynamic_routing_setting.UpdateAndSync())
	resetDynamicRoutingControllerForTest()
}

func createPrioritizedChannelSelectFixture(
	t *testing.T,
	db *gorm.DB,
	id int,
	group string,
	modelName string,
	priority int64,
) {
	t.Helper()
	createChannelSelectAutoGroupsChannel(t, db, id, group, modelName)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", id).Update("priority", priority).Error)
	var ability model.Ability
	require.NoError(t, db.Where(&model.Ability{ChannelId: id, Group: group, Model: modelName}).First(&ability).Error)
	require.NoError(t, db.Model(&ability).Update("priority", priority).Error)
}

func TestDynamicChannelSelectionPromotesVerifiedLowerPriorityCandidate(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	configureDynamicRoutingForTest(t, true)
	require.True(t, DynamicRoutingEnabled())
	const modelName = "dynamic-route-model"
	createPrioritizedChannelSelectFixture(t, db, 3101, "default", modelName, 100)
	createPrioritizedChannelSelectFixture(t, db, 3102, "default", modelName, 10)
	model.InitChannelCache()

	ctx := newChannelSelectContext()
	retry := 0
	param := &RetryParam{
		Ctx:                    ctx,
		TokenGroup:             "default",
		ModelName:              modelName,
		RequestPath:            "/v1/chat/completions",
		DynamicRoutingEligible: true,
		Retry:                  &retry,
	}

	initial, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, initial)
	assert.Equal(t, 3101, initial.Id)

	now := time.Now()
	for i := 0; i < 4; i++ {
		ObserveDynamicRoutingSample(dynamicrouting.ObservationKey{ChannelID: 3102, Model: modelName}, dynamicrouting.Sample{
			ObservedAt: now.Add(time.Duration(i) * time.Millisecond),
			TTFT:       100 * time.Millisecond,
			TPOT:       10 * time.Millisecond,
			HasTTFT:    true,
			HasTPOT:    true,
			Success:    true,
		})
	}
	ObserveDynamicRoutingSample(dynamicrouting.ObservationKey{ChannelID: 3101, Model: modelName}, dynamicrouting.Sample{
		ObservedAt:  now.Add(10 * time.Millisecond),
		Success:     false,
		HardFailure: true,
	})

	selected, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "default", selectedGroup)
	assert.Equal(t, 3102, selected.Id)
}

func TestDynamicRetryAvoidsAlreadyAttemptedChannelWithoutChangingRouteCandidates(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	configureDynamicRoutingForTest(t, true)
	const modelName = "dynamic-retry-model"
	createPrioritizedChannelSelectFixture(t, db, 3301, "default", modelName, 100)
	createPrioritizedChannelSelectFixture(t, db, 3302, "default", modelName, 10)
	createPrioritizedChannelSelectFixture(t, db, 3303, "default", modelName, 0)
	model.InitChannelCache()

	now := time.Now()
	ObserveDynamicRoutingSample(dynamicrouting.ObservationKey{ChannelID: 3301, Model: modelName}, dynamicrouting.Sample{
		ObservedAt:        now,
		UpstreamStartedAt: now.Add(-time.Millisecond),
		Success:           false,
		HardFailure:       true,
	})
	for i := 0; i < 4; i++ {
		ObserveDynamicRoutingSample(dynamicrouting.ObservationKey{ChannelID: 3302, Model: modelName}, dynamicrouting.Sample{
			ObservedAt: now.Add(time.Duration(i+1) * time.Millisecond),
			TTFT:       100 * time.Millisecond,
			TPOT:       10 * time.Millisecond,
			HasTTFT:    true,
			HasTPOT:    true,
			Success:    true,
		})
	}

	retry := 0
	param := &RetryParam{
		Ctx:                    newChannelSelectContext(),
		TokenGroup:             "default",
		ModelName:              modelName,
		RequestPath:            "/v1/chat/completions",
		DynamicRoutingEligible: true,
		Retry:                  &retry,
	}
	first, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 3302, first.Id)
	param.MarkAttempted(first.Id)
	param.SetRetry(1)

	second, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, 3303, second.Id)
	assert.NotEqual(t, first.Id, second.Id)

	// The retry actually went to 3303. Subsequent independent requests should
	// repay channel 3302's weighted-fair debt instead of probing 3303 again as
	// if the retry had been charged to the excluded channel.
	for request := 0; request < 20; request++ {
		freshRetry := 0
		selected, _, selectErr := CacheGetRandomSatisfiedChannel(&RetryParam{
			Ctx:                    newChannelSelectContext(),
			TokenGroup:             "default",
			ModelName:              modelName,
			RequestPath:            "/v1/chat/completions",
			DynamicRoutingEligible: true,
			Retry:                  &freshRetry,
		})
		require.NoError(t, selectErr)
		require.NotNil(t, selected)
		assert.Equal(t, 3302, selected.Id, "request %d", request)
	}
}

func TestDynamicAutoRetryHonorsCrossGroupRetry(t *testing.T) {
	tests := []struct {
		name            string
		crossGroupRetry bool
		wantChannelID   int
	}{
		{name: "disabled stops after current group is attempted", crossGroupRetry: false},
		{name: "enabled advances to next group", crossGroupRetry: true, wantChannelID: 3502},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := setupChannelSelectAutoGroupsTest(t)
			configureDynamicRoutingForTest(t, true)
			const modelName = "dynamic-auto-retry-model"
			createPrioritizedChannelSelectFixture(t, db, 3501, "vip", modelName, 100)
			createPrioritizedChannelSelectFixture(t, db, 3502, "default", modelName, 100)
			model.InitChannelCache()

			ctx := newChannelSelectContext()
			common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"vip", "default"})
			common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, test.crossGroupRetry)
			retry := 0
			param := &RetryParam{
				Ctx:                    ctx,
				TokenGroup:             "auto",
				ModelName:              modelName,
				RequestPath:            "/v1/chat/completions",
				DynamicRoutingEligible: true,
				Retry:                  &retry,
			}

			first, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
			require.NoError(t, err)
			require.NotNil(t, first)
			assert.Equal(t, 3501, first.Id)
			assert.Equal(t, "vip", selectedGroup)

			param.MarkAttempted(first.Id)
			param.IncreaseRetry()
			second, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
			require.NoError(t, err)
			if test.wantChannelID == 0 {
				assert.Nil(t, second)
				assert.Equal(t, "vip", selectedGroup)
				assert.Equal(t, 0, common.GetContextKeyInt(ctx, constant.ContextKeyAutoGroupIndex))
				return
			}
			require.NotNil(t, second)
			assert.Equal(t, test.wantChannelID, second.Id)
			assert.Equal(t, "default", selectedGroup)
		})
	}
}

func TestIneligibleRequestUsesStaticPriorityEvenWhenDynamicPrefersLowerChannel(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	configureDynamicRoutingForTest(t, true)
	const modelName = "non-stream-static-model"
	createPrioritizedChannelSelectFixture(t, db, 3401, "default", modelName, 100)
	createPrioritizedChannelSelectFixture(t, db, 3402, "default", modelName, 10)
	model.InitChannelCache()

	now := time.Now()
	ObserveDynamicRoutingSample(dynamicrouting.ObservationKey{ChannelID: 3401, Model: modelName}, dynamicrouting.Sample{
		ObservedAt:        now,
		UpstreamStartedAt: now.Add(-time.Millisecond),
		Success:           false,
		HardFailure:       true,
	})
	for i := 0; i < 4; i++ {
		ObserveDynamicRoutingSample(dynamicrouting.ObservationKey{ChannelID: 3402, Model: modelName}, dynamicrouting.Sample{
			ObservedAt: now.Add(time.Duration(i+1) * time.Millisecond),
			TTFT:       100 * time.Millisecond,
			TPOT:       10 * time.Millisecond,
			HasTTFT:    true,
			HasTPOT:    true,
			Success:    true,
		})
	}

	retry := 0
	selected, _, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:         newChannelSelectContext(),
		TokenGroup:  "default",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       &retry,
	})
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, 3401, selected.Id)
}

func TestDisabledDynamicRoutingPreservesStaticPrioritySelection(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	configureDynamicRoutingForTest(t, false)
	require.False(t, DynamicRoutingEnabled())
	const modelName = "static-fallback-model"
	createPrioritizedChannelSelectFixture(t, db, 3201, "default", modelName, 100)
	createPrioritizedChannelSelectFixture(t, db, 3202, "default", modelName, 10)
	model.InitChannelCache()

	ctx := newChannelSelectContext()
	retry := 0
	for i := 0; i < 20; i++ {
		selected, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
			Ctx:         ctx,
			TokenGroup:  "default",
			ModelName:   modelName,
			RequestPath: "/v1/chat/completions",
			Retry:       &retry,
		})
		require.NoError(t, err, fmt.Sprintf("selection %d", i))
		require.NotNil(t, selected)
		assert.Equal(t, "default", selectedGroup)
		assert.Equal(t, 3201, selected.Id)
	}
}

func newChannelSelectContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	return ctx
}
