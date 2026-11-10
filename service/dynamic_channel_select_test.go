package service

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/dynamicrouting"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	settingconfig "github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/dynamic_routing_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func resetDynamicRoutingControllerForTest() {
	dynamicRoutingRuntime.Lock()
	defer dynamicRoutingRuntime.Unlock()
	dynamicRoutingRuntime.controller = nil
	dynamicRoutingRuntime.version = 0
	priorityRoutingRuntime.Lock()
	priorityRoutingRuntime.controller = nil
	priorityRoutingRuntime.Unlock()
}

func recoveryProbeFixture(t *testing.T, client *redis.Client, clock func() time.Time, probe DynamicRoutingRecoveryProbe) (*recoveryProbeRunner, *dynamicrouting.Controller, dynamicrouting.ObservationKey) {
	t.Helper()
	controller, err := dynamicrouting.NewController(dynamicrouting.Config{
		Enabled: true, MaxSamples: 20, MaxAge: 90 * time.Second, MinSamples: 3,
		ProbeFraction: 0.05, RecoveryObservationWindow: 10 * time.Second,
	})
	require.NoError(t, err)
	key := dynamicrouting.ObservationKey{ChannelID: 1, Model: "active-recovery-fixture"}
	failedAt := clock().Add(-30 * time.Second)
	controller.Observe(key, dynamicrouting.Sample{ObservedAt: failedAt, HardFailure: true})
	controller.Select(dynamicrouting.RouteKey{Group: "default", Model: key.Model}, []dynamicrouting.Candidate{{ChannelID: key.ChannelID}}, failedAt)
	return &recoveryProbeRunner{
		client: client, probe: probe, clock: clock, launch: func(work func()) { work() },
		workers: make(chan struct{}, 1), running: make(map[dynamicrouting.ObservationKey]bool),
		next: make(map[dynamicrouting.ObservationKey]time.Time), seen: make(map[dynamicrouting.ObservationKey]int64),
	}, controller, key
}

func TestActiveRecoveryProbesCoordinateNodesWithoutCustomerTraffic(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	now := time.Date(2026, 11, 10, 0, 1, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	calls := 0
	probe := func(ctx context.Context, key dynamicrouting.ObservationKey) (dynamicrouting.Sample, bool) {
		calls++
		_, bounded := ctx.Deadline()
		assert.True(t, bounded)
		return dynamicrouting.Sample{UpstreamStartedAt: now, ObservedAt: now, Success: true, HasTTFT: true, TTFT: time.Millisecond}, true
	}
	first, firstController, key := recoveryProbeFixture(t, client, clock, probe)
	second, secondController, _ := recoveryProbeFixture(t, client, clock, probe)
	for cycle := range 5 {
		first.tick(context.Background(), firstController, now)
		second.tick(context.Background(), secondController, now)
		assert.Equal(t, cycle+1, calls, "two nodes must share one upstream probe")
		first.tick(context.Background(), firstController, now.Add(time.Second))
		second.tick(context.Background(), secondController, now.Add(time.Second))
		assert.Equal(t, cycle+1, calls, "the interval cannot be shortened by repeated ticks")
		if cycle < 4 {
			now = now.Add(5 * time.Second)
			server.FastForward(5 * time.Second)
		}
	}
	assert.Empty(t, firstController.RecoveryTargets(now))
	assert.Empty(t, secondController.RecoveryTargets(now))
	assert.Zero(t, firstController.ObservationStats(key, now).SampleCount, "synthetic probes must not change customer latency calibration")
}

func TestActiveRecoveryIgnoresDuplicateAndOutOfOrderFeedback(t *testing.T) {
	now := time.Date(2026, 11, 10, 0, 1, 0, 0, time.UTC)
	runner, controller, key := recoveryProbeFixture(t, nil, func() time.Time { return now }, nil)
	first := recoveryProbeResult{ID: "first", StartedAt: now.UnixMilli(), EndedAt: now.UnixMilli(), Success: true}
	second := recoveryProbeResult{ID: "second", StartedAt: now.Add(5 * time.Second).UnixMilli(), EndedAt: now.Add(5 * time.Second).UnixMilli(), Success: true}
	runner.applyResult(controller, key, first)
	controller.RecoveryTargets(now)
	runner.applyResult(controller, key, second)
	controller.RecoveryTargets(now.Add(5 * time.Second))
	runner.applyResult(controller, key, first)
	runner.applyResult(controller, key, second)
	controller.RecoveryTargets(now.Add(5 * time.Second))
	third := recoveryProbeResult{ID: "third", StartedAt: now.Add(15 * time.Second).UnixMilli(), EndedAt: now.Add(15 * time.Second).UnixMilli(), Success: true}
	runner.applyResult(controller, key, third)
	assert.Len(t, controller.RecoveryTargets(now.Add(15*time.Second)), 1, "replayed samples must not start the observation window early")
}

func TestActiveRecoveryChecksContinueDuringCooldown(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	now := time.Date(2026, 11, 10, 0, 1, 0, 0, time.UTC)
	calls := 0
	runner, controller, _ := recoveryProbeFixture(t, client, func() time.Time { return now }, func(context.Context, dynamicrouting.ObservationKey) (dynamicrouting.Sample, bool) {
		calls++
		return dynamicrouting.Sample{UpstreamStartedAt: now, ObservedAt: now, Success: true}, true
	})
	now = now.Add(-15 * time.Second)
	runner.tick(context.Background(), controller, now)
	now = now.Add(5 * time.Second)
	server.FastForward(5 * time.Second)
	runner.tick(context.Background(), controller, now)
	assert.Equal(t, 2, calls)
	assert.Len(t, controller.RecoveryTargets(now), 1, "early probe successes do not bypass the traffic cooldown")
}

func TestActiveRecoveryIntervalIncludesProbeDuration(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	now := time.Date(2026, 11, 10, 0, 1, 0, 0, time.UTC)
	calls := 0
	runner, controller, _ := recoveryProbeFixture(t, client, func() time.Time { return now }, func(context.Context, dynamicrouting.ObservationKey) (dynamicrouting.Sample, bool) {
		calls++
		started := now
		now = now.Add(6 * time.Second)
		server.FastForward(6 * time.Second)
		return dynamicrouting.Sample{UpstreamStartedAt: started, ObservedAt: now, Success: true}, true
	})
	runner.tick(context.Background(), controller, now)
	runner.tick(context.Background(), controller, now)
	assert.Equal(t, 2, calls, "a slow completed probe must not add another five-second delay")
}

func TestActiveRecoveryProbeRejectsExpiredLeaseBeforeDispatchAndPublish(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	now := time.Date(2026, 11, 10, 0, 1, 0, 0, time.UTC)
	calls := 0
	runner, controller, key := recoveryProbeFixture(t, client, func() time.Time { return now }, func(context.Context, dynamicrouting.ObservationKey) (dynamicrouting.Sample, bool) {
		calls++
		return dynamicrouting.Sample{UpstreamStartedAt: now, ObservedAt: now, Success: true}, true
	})
	var queued []func()
	runner.launch = func(work func()) { queued = append(queued, work) }
	runner.tick(context.Background(), controller, now)
	runner.tick(context.Background(), controller, now.Add(5*time.Second))
	require.Len(t, queued, 1, "one channel/model cannot have overlapping work")
	leaseKey, sampleKey := recoveryProbeKeys(key)
	oldToken, err := client.Get(context.Background(), leaseKey).Result()
	require.NoError(t, err)
	server.FastForward(time.Minute)
	require.NoError(t, server.Set(leaseKey, "new-owner"))
	queued[0]()
	assert.Zero(t, calls, "queued work must recheck lease ownership before calling the provider")
	written, err := recoveryProbePublishScript.Run(context.Background(), client, []string{leaseKey, sampleKey}, oldToken, "old-result", recoverySampleTTL.Milliseconds(), recoveryProbeInterval.Milliseconds()).Int()
	require.NoError(t, err)
	assert.Zero(t, written)
	assert.False(t, server.Exists(sampleKey), "a stale owner cannot publish recovery feedback")
	owner, err := server.Get(leaseKey)
	require.NoError(t, err)
	assert.Equal(t, "new-owner", owner)
}

func TestActiveRecoveryAlsoFiltersNonStreamingRequests(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	configureDynamicRoutingForTest(t, true)
	const name = "non-streaming-recovery-model"
	createPrioritizedChannelSelectFixture(t, db, 3911, "default", name, 100)
	createPrioritizedChannelSelectFixture(t, db, 3912, "default", name, 10)
	model.InitChannelCache()
	controller, enabled, err := currentDynamicRoutingController(true)
	require.NoError(t, err)
	require.True(t, enabled)
	now := time.Now()
	controller.Observe(dynamicrouting.ObservationKey{ChannelID: 3911, Model: name}, dynamicrouting.Sample{ObservedAt: now, HardFailure: true})
	controller.Select(dynamicrouting.RouteKey{Group: "default", Model: name}, []dynamicrouting.Candidate{{ChannelID: 3911, Priority: 100}, {ChannelID: 3912, Priority: 10}}, now)
	ctx := newChannelSelectContext()
	param := &RetryParam{Ctx: ctx, ModelName: name, TokenGroup: "default", RequestPath: "/v1/chat/completions", DynamicRoutingEligible: false}
	selected, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, 3912, selected.Id)
}

func TestPriorityRoutingMonitorCountsOnlyPhysicalDispatchAndDoesNotEnableDynamicRouting(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	configureDynamicRoutingForTest(t, false)
	const name = "priority-monitor-model"
	createPrioritizedChannelSelectFixture(t, db, 3901, "default", name, 100)
	createPrioritizedChannelSelectFixture(t, db, 3902, "default", name, 10)
	pool := model.ModelRoutingPool{RoutingGroup: "default", Model: name, Mode: model.RoutingModePriority, Channels: []model.ModelRoutingPoolChannel{{ChannelID: 3901, Priority: 100, Weight: 10, Enabled: true}, {ChannelID: 3902, Priority: 10, Weight: 10, Enabled: true}}}
	require.NoError(t, model.SaveModelRoutingPool(&pool, ""))
	model.InitChannelCache()
	ctx := newChannelSelectContext()
	param := &RetryParam{Ctx: ctx, ModelName: name, TokenGroup: "default", RequestPath: "/v1/chat/completions"}
	selected, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, 3901, selected.Id)
	assert.False(t, DynamicRoutingEnabled())
	assert.Empty(t, GetRoutingMonitor(context.Background(), 0).Nodes[0].Routes, "selection without physical dispatch is not traffic")
	require.True(t, PriorityRoutingMonitorEnabled(ctx))
	info := &relaycommon.RelayInfo{}
	info.BeginChannelMonitoringAttempt(selected.Id, selected.Type, name)
	BindRoutingMonitorAttempt(ctx, info, selected.Id)
	info.MarkAttemptUpstreamStarted()
	info.MarkAttemptUpstreamStarted()
	response := GetRoutingMonitor(context.Background(), 0)
	require.Len(t, response.Nodes[0].Routes, 1)
	route := response.Nodes[0].Routes[0]
	assert.Equal(t, "priority", route.Mode)
	assert.EqualValues(t, 1, route.SelectionCount)
	require.Len(t, route.Requests, 1)
	assert.Equal(t, selected.Id, route.Requests[0].ChannelID)
	assert.False(t, DynamicRoutingEnabled())
	param.MarkAttempted(selected.Id)
	fallback, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, fallback)
	require.Equal(t, 3902, fallback.Id)
	info.BeginChannelMonitoringAttempt(fallback.Id, fallback.Type, name)
	BindRoutingMonitorAttempt(ctx, info, fallback.Id)
	info.MarkAttemptUpstreamStarted()
	route = GetRoutingMonitor(context.Background(), 0).Nodes[0].Routes[0]
	assert.EqualValues(t, 2, route.SelectionCount)
	require.Len(t, route.Channels, 2, "retry exclusion must not retire the primary from the monitor")
	assert.True(t, route.Channels[0].Active)
	assert.EqualValues(t, 100, route.Channels[0].Priority)
	assert.Equal(t, 3901, route.Requests[0].ChannelID)
	assert.Equal(t, 3902, route.Requests[1].ChannelID)
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

func TestDynamicRetryWithMixedUpstreamFailureAndCapacityDenialHonorsGroupPolicy(t *testing.T) {
	for _, crossGroup := range []bool{false, true} {
		t.Run(fmt.Sprint(crossGroup), func(t *testing.T) {
			db := setupChannelSelectAutoGroupsTest(t)
			configureDynamicRoutingForTest(t, true)
			const publicModel = "dynamic-mixed-exhaustion"
			createPrioritizedChannelSelectFixture(t, db, 3601, "vip", publicModel, 100)
			createPrioritizedChannelSelectFixture(t, db, 3602, "vip", publicModel, 10)
			createPrioritizedChannelSelectFixture(t, db, 3603, "default", publicModel, 100)
			model.InitChannelCache()
			ctx := newChannelSelectContext()
			common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"vip", "default"})
			common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, crossGroup)
			param := &RetryParam{
				Ctx: ctx, TokenGroup: "auto", ModelName: publicModel, RequestPath: "/v1/chat/completions",
				DynamicRoutingEligible: true, AttemptedChannelIds: map[int]struct{}{3601: {}},
				Capacity: &ChannelCapacityState{blocked: map[int]struct{}{3602: {}}, eligible: map[int]struct{}{}},
			}
			selected, group, err := CacheGetRandomSatisfiedChannel(param)
			require.NoError(t, err)
			if !crossGroup {
				assert.Nil(t, selected)
				assert.Equal(t, "vip", group)
				assert.Zero(t, common.GetContextKeyInt(ctx, constant.ContextKeyAutoGroupIndex))
			} else {
				require.NotNil(t, selected)
				assert.Equal(t, 3603, selected.Id)
				assert.Equal(t, "default", group)
			}
		})
	}
}

func TestRoutingMonitorReadPreservesUninitializedRuntime(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(strconv.FormatBool(enabled), func(t *testing.T) {
			configureDynamicRoutingForTest(t, enabled)
			previousClient, previousEnabled := common.RDB, common.RedisEnabled
			common.RDB, common.RedisEnabled = nil, false
			t.Cleanup(func() { common.RDB, common.RedisEnabled = previousClient, previousEnabled })
			response := GetRoutingMonitor(context.Background(), 0)
			assert.Equal(t, "local", response.Coverage)
			assert.Equal(t, enabled, response.LegacyEnabled)
			assert.Equal(t, "base", response.Scope)
			require.Len(t, response.Nodes, 1)
			assert.Empty(t, response.Nodes[0].Routes)
			dynamicRoutingRuntime.Lock()
			defer dynamicRoutingRuntime.Unlock()
			assert.Nil(t, dynamicRoutingRuntime.controller, "monitor reads must not initialize or exercise routing")
		})
	}
}

func TestRoutingMonitorFiltersOtherUsersAndKeepsInheritedBase(t *testing.T) {
	configureDynamicRoutingForTest(t, true)
	previousClient, previousEnabled := common.RDB, common.RedisEnabled
	common.RDB, common.RedisEnabled = nil, false
	t.Cleanup(func() { common.RDB, common.RedisEnabled = previousClient, previousEnabled })
	controller, err := dynamicrouting.NewController(dynamic_routing_setting.GetSnapshot().Config)
	require.NoError(t, err)
	dynamicRoutingRuntime.Lock()
	dynamicRoutingRuntime.controller = controller
	dynamicRoutingRuntime.Unlock()
	now := time.Now()
	for _, policy := range []string{"", "user:12", "user:34"} {
		decision := controller.SelectAvoiding(dynamicrouting.RouteKey{Group: "default", Model: "monitor-model", Policy: policy}, []dynamicrouting.Candidate{{ChannelID: 1}}, nil, now)
		require.True(t, decision.HasSelection)
	}
	base := GetRoutingMonitor(context.Background(), 0)
	assert.True(t, base.LegacyEnabled)
	require.Len(t, base.Nodes, 1)
	require.Len(t, base.Nodes[0].Routes, 1)
	assert.Empty(t, base.Nodes[0].Routes[0].Policy)
	user := GetRoutingMonitor(context.Background(), 12)
	require.Len(t, user.Nodes, 1)
	require.Len(t, user.Nodes[0].Routes, 2)
	policies := []string{user.Nodes[0].Routes[0].Policy, user.Nodes[0].Routes[1].Policy}
	assert.ElementsMatch(t, []string{"", "user:12"}, policies)
	for _, route := range user.Nodes[0].Routes {
		assert.EqualValues(t, 1, route.SelectionCount, "reading the monitor must not add decisions")
	}
	encoded, err := common.Marshal(user)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"ttft_ms":null`)
	assert.Contains(t, string(encoded), `"effective_cost_discount":null`)
	assert.NotContains(t, string(encoded), "user:34")
}

func TestRoutingMonitorRedisIncludesFreshNodesAndExcludesExpiredSnapshots(t *testing.T) {
	configureDynamicRoutingForTest(t, false)
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	previousClient, previousEnabled := common.RDB, common.RedisEnabled
	common.RDB, common.RedisEnabled = client, true
	t.Cleanup(func() { common.RDB, common.RedisEnabled = previousClient, previousEnabled })
	now := time.Now()
	remote := RoutingMonitorNode{ID: "remote-instance", Name: "gateway-02", UpdatedAt: now.UnixMilli(), Routes: []dynamicrouting.RouteSnapshot{
		{Group: "default", Model: "m", Policy: "", Mode: "discount"},
		{Group: "default", Model: "m", Policy: "user:12", Mode: "discount"},
		{Group: "default", Model: "m", Policy: "user:34", Mode: "discount"},
	}}
	require.NoError(t, publishRoutingMonitorNode(context.Background(), client, remote))
	stale := RoutingMonitorNode{ID: "stale-instance", Name: "stale", UpdatedAt: now.Add(-time.Minute).UnixMilli(), Routes: []dynamicrouting.RouteSnapshot{}}
	payload, err := common.Marshal(stale)
	require.NoError(t, err)
	key := routingMonitorNodePrefix + stale.ID
	require.NoError(t, client.Set(context.Background(), key, payload, time.Minute).Err())
	require.NoError(t, client.ZAdd(context.Background(), routingMonitorRegistry, &redis.Z{Score: float64(now.UnixMilli()), Member: key}).Err())
	response := GetRoutingMonitor(context.Background(), 12)
	assert.Equal(t, "cluster", response.Coverage)
	require.Len(t, response.Nodes, 2)
	assert.True(t, response.Nodes[0].Local)
	assert.False(t, response.Nodes[1].Local)
	assert.Equal(t, remote.ID, response.Nodes[1].ID)
	require.Len(t, response.Nodes[1].Routes, 2)
	for _, route := range response.Nodes[1].Routes {
		assert.NotEqual(t, "user:34", route.Policy)
	}
	assert.InDelta(t, routingMonitorNodeTTL.Seconds(), server.TTL(routingMonitorNodePrefix+remote.ID).Seconds(), 1)
}

func TestRoutingMonitorRedisFailureFallsBackToCurrentNode(t *testing.T) {
	configureDynamicRoutingForTest(t, false)
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	require.NoError(t, client.Close())
	previousClient, previousEnabled := common.RDB, common.RedisEnabled
	common.RDB, common.RedisEnabled = client, true
	t.Cleanup(func() { common.RDB, common.RedisEnabled = previousClient, previousEnabled })
	response := GetRoutingMonitor(context.Background(), 0)
	assert.Equal(t, "local", response.Coverage)
	assert.True(t, response.Partial)
	require.Len(t, response.Nodes, 1)
	assert.True(t, response.Nodes[0].Local)
	cached := GetRoutingMonitor(context.Background(), 0)
	assert.Equal(t, "local", cached.Coverage)
	assert.True(t, cached.Partial, "a cached Redis outage must remain visible")
}

func TestRoutingMonitorMalformedOrOversizedRemoteSnapshotIsOmitted(t *testing.T) {
	configureDynamicRoutingForTest(t, false)
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	previousClient, previousEnabled := common.RDB, common.RedisEnabled
	common.RDB, common.RedisEnabled = client, true
	t.Cleanup(func() { common.RDB, common.RedisEnabled = previousClient, previousEnabled })
	for id, payload := range map[string]string{
		"malformed": `{broken-json`,
		"oversized": strings.Repeat("x", routingMonitorMaxNodeBytes+1),
	} {
		key := routingMonitorNodePrefix + id
		require.NoError(t, client.Set(context.Background(), key, payload, time.Minute).Err())
		require.NoError(t, client.ZAdd(context.Background(), routingMonitorRegistry, &redis.Z{Score: float64(time.Now().UnixMilli()), Member: key}).Err())
	}
	response := GetRoutingMonitor(context.Background(), 0)
	assert.Equal(t, "cluster", response.Coverage)
	assert.True(t, response.Partial)
	require.Len(t, response.Nodes, 1)
	assert.True(t, response.Nodes[0].Local)
}
