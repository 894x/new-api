package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/chat_early_chunk_setting"
	"github.com/QuantumNous/new-api/setting/dynamic_routing_setting"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func earlyChunkTestCell(t *testing.T, now time.Time, latencies ...int64) *earlyChunkCell {
	t.Helper()
	cell := &earlyChunkCell{}
	for _, ms := range latencies {
		record := earlyChunkRecord{At: now, LatencyMS: ms, Kind: "content", Outcome: "completed"}
		if ms < 0 {
			record.LatencyMS = 2000
			record.Kind = "failure"
			record.Outcome = "failed"
		}
		require.True(t, cell.record(record, now, 900))
	}
	return cell
}

func TestChatEarlyChunkConditionalPrediction(t *testing.T) {
	now := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, mode                   string
		times                        []int64
		elapsed                      int
		unknown, upstream, old, want bool
	}{
		{name: "imminent content", mode: "enabled", times: []int64{750, 900}, elapsed: 500, want: true},
		{name: "observe predicts without writing", mode: "observe", times: []int64{750}, elapsed: 500},
		{name: "off collects without predicting", mode: "off", times: []int64{750}, elapsed: 500},
		{name: "slow samples", mode: "enabled", times: []int64{2000}, elapsed: 500},
		{name: "elapsed fast samples cannot explain stalled requests", mode: "enabled", times: []int64{200, 2000}, elapsed: 500},
		{name: "failure remains in risk set", mode: "enabled", times: []int64{750, -1}, elapsed: 500},
		{name: "unknown estimate is diagnostic only", mode: "enabled", times: []int64{750}, elapsed: 500, unknown: true},
		{name: "upstream wins", mode: "enabled", times: []int64{750}, elapsed: 500, upstream: true},
		{name: "before assigned time", mode: "enabled", times: []int64{750}, elapsed: 400},
		{name: "expired history", mode: "enabled", times: []int64{750}, elapsed: 500, old: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := chat_early_chunk_setting.Default()
			cfg.Mode = tc.mode
			cfg.EarliestMS = 500
			cfg.MinLeadMS, cfg.MaxLeadMS = 500, 500
			cfg.MinSamples = 20
			times := make([]int64, 20)
			for i := range times {
				times[i] = tc.times[i%len(tc.times)]
			}
			at := now
			if tc.old {
				at = now.Add(-time.Hour)
			}
			a := &ChatEarlyChunkAttempt{config: cfg, started: now, cells: map[int]*earlyChunkCell{0: earlyChunkTestCell(t, at, times...)}}
			if tc.unknown {
				a.cacheBand = earlyChunkUnknownCache
			}
			if tc.upstream {
				a.UpstreamArrived()
			}
			sent := 0
			_, err := a.Try(now.Add(time.Duration(tc.elapsed)*time.Millisecond), func() error { sent++; return nil })
			require.NoError(t, err)
			assert.Equal(t, tc.want, sent == 1)
			if tc.mode == "observe" {
				assert.NotNil(t, a.observation.DecisionMS)
			}
			if sent > 0 {
				_, err = a.Try(now.Add(600*time.Millisecond), func() error { sent++; return nil })
				require.NoError(t, err)
				assert.Equal(t, 1, sent)
			}
		})
	}
}

func TestChatEarlyChunkSparseNeighborsAndConservativeBounds(t *testing.T) {
	now := time.Now()
	cfg := chat_early_chunk_setting.Default()
	cfg.MinSamples = 20
	cfg.EarliestMS = 0
	cfg.MinLeadMS, cfg.MaxLeadMS = 500, 500
	cells := map[int]*earlyChunkCell{}
	times := make([]int64, 10)
	for i := range times {
		times[i] = 750
	}
	cells[earlyChunkCellID(2, 3)] = earlyChunkTestCell(t, now, times...)
	cells[earlyChunkCellID(1, 3)] = earlyChunkTestCell(t, now, times...)
	prediction := earlyChunkPredict(cells, 2, 3, now, 500, cfg)
	assert.True(t, prediction.Eligible)
	assert.EqualValues(t, 20, prediction.Pending)
	assert.Equal(t, 2, prediction.MergedCells)
	assert.False(t, earlyChunkPredict(cells, 2, 4, now, 500, cfg).Eligible, "different cache bands are not merged")
	assert.False(t, earlyChunkPredict(cells, 2, 3, now, 600, cfg).Eligible, "straddling bins cannot claim imminent content")
	assert.Equal(t, 0, earlyChunkTokenBand(1024))
	assert.Equal(t, 1, earlyChunkTokenBand(1025))
	assert.Equal(t, earlyChunkOverflowTokens, earlyChunkTokenBand(524289))
	assert.Equal(t, earlyChunkUnknownTokens, earlyChunkTokenBand(0))
	assert.Equal(t, 19, earlyChunkCacheBand(10000, 10000, true))
	assert.Equal(t, 18, earlyChunkCacheBand(9950, 10000, true))
	assert.Equal(t, earlyChunkUnknownCache, earlyChunkCacheBand(0, 10000, false))
}

func TestChatEarlyChunkIndependentDefaultAndUserPools(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	require.NoError(t, db.AutoMigrate(&model.UserGroupChannelRoutingOverride{}, &model.UserRoutingStrategy{}))
	isolateChatPrefixCache(t)
	channel := &model.Channel{Id: 101, Type: 1}
	sharedKey := earlyChunkScopeKey("prefix-model", channel, model.CurrentRequestPolicy().ChatEarlyChunk.MaxAgeSeconds)
	for _, tc := range []struct {
		group, mode, channelMode string
		user                     bool
		want                     string
	}{
		{"first", "enabled", "", false, "enabled"},
		{"second", "observe", "", false, "observe"},
		{"first", "enabled", "off", false, "off"},
		{"second", "off", "observe", false, "observe"},
		{"first", "enabled", "", true, "off"},
		{"prefix-disabled", "enabled", "", false, "enabled"},
	} {
		t.Run(fmt.Sprintf("%s/%s/%s/user_%t", tc.group, tc.mode, tc.channelMode, tc.user), func(t *testing.T) {
			param := chatPrefixTestParam(chatPrefixTestBody(t, 4, nil, nil))
			param.routingPools = map[string]*model.ModelRoutingPool{tc.group: {RoutingGroup: tc.group, Model: param.ModelName, Tuning: &dynamic_routing_setting.Overrides{ChatEarlyChunk: &chat_early_chunk_setting.Pool{Mode: tc.mode, Channels: map[int]string{101: tc.channelMode}}}}}
			param.userRoutingLoaded = true
			if tc.group == "prefix-disabled" {
				param.routingPools[tc.group].Tuning.ChatPrefixEnabled = common.GetPointer(false)
			}
			if tc.user {
				param.userGroupRouting = []model.UserGroupChannelRoutingOverride{{UserChannelRoutingOverride: model.UserChannelRoutingOverride{UserId: 42, Model: param.ModelName, ChannelId: 101}, RoutingGroup: tc.group}}
			}
			info := &relaycommon.RelayInfo{IsStream: true, RelayFormat: types.RelayFormatOpenAI, UsingGroup: tc.group, UserId: 42, StartTime: time.Now()}
			BeginChatEarlyChunkAttempt(param, info, channel)
			attempt := ChatEarlyChunk(param.Ctx)
			require.NotNil(t, attempt)
			assert.Equal(t, tc.want, attempt.config.Mode)
			assert.Equal(t, sharedKey, attempt.key, "pool and user mode must not split the model/channel table")
			assert.Positive(t, attempt.observation.InputTokens, "text requests use a local token estimate")
			if tc.group == "prefix-disabled" {
				assert.Equal(t, earlyChunkUnknownCache, attempt.cacheBand)
				assert.False(t, attempt.ShouldPredict())
			}
		})
	}
}

func TestChatEarlyChunkPublicFramesAndCommittedFailure(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	start := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	a := &ChatEarlyChunkAttempt{started: start, created: start.Unix(), model: "public-model", config: chat_early_chunk_setting.Default(), observation: ChatEarlyChunkObservation{Sent: true, DecisionMS: common.GetPointer(int64(300))}}
	c.Set(chatEarlyChunkContextKey, a)
	c.Set(chatEarlyChunkStreamContextKey, &chatEarlyChunkStream{sent: true, model: a.model, created: a.created})
	data, err := NormalizeChatEarlyChunkData(c, `{"id":"upstream","created":999,"model":"internal","choices":[{"delta":{"content":"hello"}}],"usage":{"completion_tokens":2}}`)
	require.NoError(t, err)
	assert.Contains(t, data, `"model":"public-model"`)
	assert.Contains(t, data, `"completion_tokens":2`, "real provider usage is preserved")
	ObserveChatEarlyChunkData(c, data, start.Add(450*time.Millisecond))
	snapshot := a.Snapshot(nil)
	require.NotNil(t, snapshot.GapMS)
	assert.EqualValues(t, 150, *snapshot.GapMS)
	assert.True(t, ClaimChatEarlyChunkError(c))
	assert.False(t, ClaimChatEarlyChunkError(c), "failure terminal is emitted once")
	ObserveChatEarlyChunkData(c, "[DONE]", start.Add(time.Second))
	assert.EqualValues(t, 450, *a.observation.ContentMS, "synthetic terminals never reset actual content time")
	common.SetContextKey(c, constant.ContextKeyResponseId, "public-id")
	c.Writer.WriteHeaderNow()
	decision := DecideRelayRetry(c, types.NewOpenAIError(fmt.Errorf("upstream failed"), types.ErrorCodeBadResponse, http.StatusBadGateway), 2)
	assert.Equal(t, "response_committed", decision.Reason)
}

func TestChatEarlyChunkUserPoolOverridesAreIndependentForTheSameChannel(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	require.NoError(t, db.AutoMigrate(&model.UserRoutingStrategy{}, &model.UserGroupChannelRoutingOverride{}))
	for _, row := range []model.UserRoutingStrategy{
		{UserId: 42, Model: "prefix-model", RoutingGroup: "first", Mode: "inherit", Revision: "first", EarlyChunkJSON: `{"mode":"observe","channels":{"101":"enabled"}}`},
		{UserId: 42, Model: "prefix-model", RoutingGroup: "second", Mode: "inherit", Revision: "second", EarlyChunkJSON: `{"mode":"off","channels":{"101":"observe"}}`},
	} {
		require.NoError(t, db.Create(&row).Error)
	}
	for _, tc := range []struct {
		user        int
		group, want string
	}{{42, "first", "enabled"}, {42, "second", "observe"}, {43, "first", "off"}} {
		p := chatPrefixTestParam(chatPrefixTestBody(t, 4, nil, nil))
		common.SetContextKey(p.Ctx, constant.ContextKeyUserId, tc.user)
		p.userRoutingLoaded = true
		p.routingPools = map[string]*model.ModelRoutingPool{tc.group: {RoutingGroup: tc.group, Model: p.ModelName, Tuning: &dynamic_routing_setting.Overrides{ChatEarlyChunk: &chat_early_chunk_setting.Pool{Mode: "off", Channels: map[int]string{101: "off"}}}}}
		info := &relaycommon.RelayInfo{IsStream: true, RelayFormat: types.RelayFormatOpenAI, UsingGroup: tc.group, UserId: tc.user, StartTime: time.Now()}
		BeginChatEarlyChunkAttempt(p, info, &model.Channel{Id: 101, Type: 1})
		require.NotNil(t, ChatEarlyChunk(p.Ctx))
		assert.Equal(t, tc.want, ChatEarlyChunk(p.Ctx).Snapshot(info).Mode)
	}
}

func TestChatEarlyChunkHistoriesAreSharedBoundedAndExpire(t *testing.T) {
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedis })
	chatEarlyChunkHistory.Lock()
	old := chatEarlyChunkHistory.entries
	chatEarlyChunkHistory.entries = make(map[earlyChunkStoredKey]*earlyChunkCell)
	chatEarlyChunkHistory.Unlock()
	t.Cleanup(func() {
		chatEarlyChunkHistory.Lock()
		chatEarlyChunkHistory.entries = old
		chatEarlyChunkHistory.Unlock()
	})
	now := time.Now()
	ctx := context.Background()
	channel := &model.Channel{Id: 49, Type: 1, Key: "secret"}
	key := earlyChunkScopeKey("model", channel, 900)
	require.Equal(t, key, earlyChunkScopeKey("model", channel, 900))
	assert.NotEqual(t, key, earlyChunkScopeKey("other-model", channel, 900))
	assert.NotEqual(t, key, earlyChunkScopeKey("model", &model.Channel{Id: 86, Type: 1, Key: "secret"}, 900))
	record := earlyChunkRecord{At: now, LatencyMS: 5500, Kind: "content", Outcome: "failed", Decision: true, Sent: true, Met: true, GapMS: common.GetPointer(int64(300))}
	require.NoError(t, earlyChunkRecordCell(ctx, key, 0, record, now, 900))
	cells, _, _, err := earlyChunkReadCells(ctx, key, nil, now, 900)
	require.NoError(t, err)
	require.Len(t, cells, 1)
	var sum earlyChunkAggregate
	sum.add(cells[0], now, 900)
	metrics := sum.metrics()
	assert.EqualValues(t, 1, metrics.ContentSamples)
	assert.EqualValues(t, 1, metrics.Failed)
	assert.EqualValues(t, 1, metrics.SentFailed)
	require.NotNil(t, metrics.AverageMS)
	assert.Equal(t, 5500.0, *metrics.AverageMS)
	raw, err := common.Marshal(cells[0])
	require.NoError(t, err)
	_, err = decodeEarlyChunkCell(string(raw), 900)
	require.NoError(t, err)
	cells, _, _, err = earlyChunkReadCells(ctx, key, nil, now.Add(901*time.Second), 900)
	require.NoError(t, err)
	assert.Empty(t, cells)
	// Fill the configured capacity, then verify bounded eviction through the public writer.
	chatEarlyChunkHistory.Lock()
	for i := range earlyChunkMaxBuckets {
		chatEarlyChunkHistory.entries[earlyChunkStoredKey{Scope: fmt.Sprint(i), Cell: 0}] = earlyChunkTestCell(t, now, 750)
	}
	chatEarlyChunkHistory.Unlock()
	require.NoError(t, earlyChunkRecordCell(ctx, "new", 0, record, now, 900))
	_, _, tracked, err := earlyChunkReadCells(ctx, "new", nil, now, 900)
	require.NoError(t, err)
	assert.Equal(t, earlyChunkMaxBuckets, tracked)
}

func TestChatEarlyChunkFinishRetainsContentOnLateFailure(t *testing.T) {
	old := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = old })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	now := time.Now()
	cfg := chat_early_chunk_setting.Default()
	a := &ChatEarlyChunkAttempt{config: cfg, loaded: true, key: common.GetUUID(), started: now.Add(-6 * time.Second), observation: ChatEarlyChunkObservation{Mode: "enabled", ContentMS: common.GetPointer(int64(5500)), DecisionMS: common.GetPointer(int64(5200)), Sent: true}}
	c.Set(chatEarlyChunkContextKey, a)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	FinishChatEarlyChunkAttempt(c, info, types.NewOpenAIError(fmt.Errorf("late failure"), types.ErrorCodeBadResponse, 502))
	FinishChatEarlyChunkAttempt(c, info, nil)
	cells, _, _, err := earlyChunkReadCells(context.Background(), a.key, nil, time.Now(), cfg.MaxAgeSeconds)
	require.NoError(t, err)
	var sum earlyChunkAggregate
	sum.add(cells[0], time.Now(), cfg.MaxAgeSeconds)
	assert.EqualValues(t, 1, sum.metrics().Samples)
	assert.EqualValues(t, 1, sum.metrics().ContentSamples)
	assert.EqualValues(t, 1, sum.metrics().SentFailed)
	assert.True(t, a.Snapshot(info).MetTarget)
	assert.True(t, a.Snapshot(info).Failed)
}

func TestChatEarlyChunkScopeRejectsOtherProtocols(t *testing.T) {
	for _, path := range []string{"/v1/responses", "/v1/messages", "/pg/chat/completions"} {
		param := chatPrefixTestParam([]byte(strings.Repeat("a", 2048)))
		param.Ctx.Request.URL.Path = path
		BeginChatEarlyChunkAttempt(param, &relaycommon.RelayInfo{IsStream: true, RelayFormat: types.RelayFormatOpenAI}, &model.Channel{})
		assert.Nil(t, ChatEarlyChunk(param.Ctx))
	}
}

func TestChatEarlyChunkCancellationIsNotATokenTime(t *testing.T) {
	old := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = old })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil).WithContext(ctx)
	cfg := chat_early_chunk_setting.Default()
	a := &ChatEarlyChunkAttempt{config: cfg, loaded: true, key: common.GetUUID(), started: time.Now().Add(-5 * time.Second)}
	c.Set(chatEarlyChunkContextKey, a)
	FinishChatEarlyChunkAttempt(c, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}, nil)
	cells, _, _, err := earlyChunkReadCells(context.Background(), a.key, nil, time.Now(), cfg.MaxAgeSeconds)
	require.NoError(t, err)
	var sum earlyChunkAggregate
	sum.add(cells[0], time.Now(), cfg.MaxAgeSeconds)
	assert.EqualValues(t, 1, sum.metrics().Cancelled)
	assert.Zero(t, sum.metrics().ContentSamples)
	assert.Nil(t, sum.metrics().P50MS)
}

func TestChatEarlyChunkRedisSharedGrid(t *testing.T) {
	addr := os.Getenv("CHAT_EARLY_CHUNK_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("isolated Redis address not configured")
	}
	first, second := redis.NewClient(&redis.Options{Addr: addr}), redis.NewClient(&redis.Options{Addr: addr})
	oldClient, oldEnabled := common.RDB, common.RedisEnabled
	common.RDB, common.RedisEnabled = first, true
	t.Cleanup(func() { common.RDB, common.RedisEnabled = oldClient, oldEnabled; _ = first.Close(); _ = second.Close() })
	ctx := context.Background()
	key := common.GetUUID()
	now := time.Now()
	record := earlyChunkRecord{At: now, LatencyMS: 5500, Kind: "content", Outcome: "failed"}
	require.NoError(t, earlyChunkRecordCell(ctx, key, 0, record, now, 900))
	common.RDB = second
	// Simultaneous writers must not replace each other's histogram increments.
	var writers sync.WaitGroup
	errors := make(chan error, 2)
	for range 2 {
		writers.Go(func() { errors <- earlyChunkRecordCell(ctx, key, 0, record, now, 900) })
	}
	writers.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	cells, _, _, err := earlyChunkReadCells(ctx, key, nil, time.Now(), 900)
	require.NoError(t, err)
	require.Len(t, cells, 1)
	var sum earlyChunkAggregate
	sum.add(cells[0], time.Now(), 900)
	assert.EqualValues(t, 3, sum.metrics().ContentSamples)
	assert.EqualValues(t, 3, sum.metrics().Failed)
	selected, _, _, err := earlyChunkReadCells(ctx, key, []int{0, 21}, time.Now(), 900)
	require.NoError(t, err)
	require.Len(t, selected, 1)
	other, _, _, err := earlyChunkReadCells(ctx, key+"other", nil, time.Now(), 900)
	require.NoError(t, err)
	assert.Empty(t, other)
	require.NoError(t, second.HSet(ctx, earlyChunkRedisPrefix+"data:"+key, "0", "{}").Err())
	_, _, _, err = earlyChunkReadCells(ctx, key, nil, time.Now(), 900)
	assert.Error(t, err)
	require.NoError(t, second.Del(ctx, earlyChunkRedisPrefix+"data:"+key).Err())
	cells, _, _, err = earlyChunkReadCells(ctx, key, nil, time.Now(), 900)
	require.NoError(t, err)
	assert.Empty(t, cells)
	require.NoError(t, second.Close())
	_, _, _, err = earlyChunkReadCells(ctx, key, nil, time.Now(), 900)
	assert.Error(t, err, "unavailable Redis cannot fall back to node-local predictions")
}

func TestChatEarlyChunkDynamicCandidates(t *testing.T) {
	now := time.Now()
	cfg := chat_early_chunk_setting.Default()
	cfg.MinSamples = 20
	for _, tc := range []struct{ latency, lead, candidate int64 }{{6000, 1000, 5000}, {12000, 1800, 10200}, {20000, 3000, 17000}} {
		t.Run(fmt.Sprint(tc.latency), func(t *testing.T) {
			times := make([]int64, 20)
			for i := range times {
				times[i] = tc.latency
			}
			cells := map[int]*earlyChunkCell{0: earlyChunkTestCell(t, now, times...)}
			p := earlyChunkPredict(cells, 0, 0, now, 0, cfg)
			require.NotNil(t, p.LeadMS)
			assert.Equal(t, tc.lead, *p.LeadMS)
			require.NotNil(t, p.CandidateMS)
			assert.Equal(t, tc.candidate, *p.CandidateMS)
			assert.False(t, p.Eligible)
			p = earlyChunkPredict(cells, 0, 0, now, tc.candidate, cfg)
			assert.True(t, p.Eligible)
			require.NotNil(t, p.RemainingP50MS)
			assert.Equal(t, tc.lead, *p.RemainingP50MS)
			auto := earlyChunkPredict(cells, 0, 0, now, -1, cfg)
			assert.Equal(t, tc.candidate, auto.AtMS)
			assert.True(t, auto.Eligible)
		})
	}
}

func TestChatEarlyChunkUpdatesStalledForecastAndKeepsObserveIndependent(t *testing.T) {
	now := time.Now()
	cfg := chat_early_chunk_setting.Default()
	cfg.MinSamples = 20
	times := make([]int64, 40)
	for i := range times {
		times[i] = 6000
		if i >= 20 {
			times[i] = 20000
		}
	}
	cells := map[int]*earlyChunkCell{0: earlyChunkTestCell(t, now, times...)}
	for _, mode := range []string{"observe", "enabled"} {
		t.Run(mode, func(t *testing.T) {
			cfg.Mode = mode
			a := &ChatEarlyChunkAttempt{config: cfg, started: now, cells: cells}
			assert.Equal(t, 5*time.Second, a.NextCheck(now))
			sent := 0
			emit := func() error { sent++; return nil }
			done, err := a.Try(now.Add(5*time.Second), emit)
			require.NoError(t, err)
			assert.False(t, done)
			assert.Zero(t, sent)
			assert.Equal(t, 11*time.Second, a.NextCheck(now.Add(6*time.Second)))
			_, err = a.Try(now.Add(17*time.Second), emit)
			require.NoError(t, err)
			assert.Equal(t, mode == "enabled", sent == 1, "slow requests have no fixed five-second cutoff")
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(chatEarlyChunkContextKey, a)
			ObserveChatEarlyChunkData(c, `{"choices":[{"delta":{"content":"real"}}]}`, now.Add(19500*time.Millisecond))
			snapshot := a.Snapshot(nil)
			assert.True(t, snapshot.MetTarget, "use this decision's three-second gap budget")
			require.NotNil(t, snapshot.Prediction)
			require.NotNil(t, snapshot.Prediction.LeadMS)
			assert.EqualValues(t, 3000, *snapshot.Prediction.LeadMS)
		})
	}
}

func TestChatEarlyChunkGridUsesTheRuntimeForecast(t *testing.T) {
	old := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = old })
	now := time.Now()
	name := common.GetUUID()
	channel := &model.Channel{Id: 49, Type: 1}
	config := model.CurrentRequestPolicy().ChatEarlyChunk
	times := make([]int64, config.MinSamples)
	for i := range times {
		times[i] = 12000
	}
	key := earlyChunkStoredKey{Scope: earlyChunkScopeKey(name, channel, config.MaxAgeSeconds), Cell: 0}
	chatEarlyChunkHistory.Lock()
	chatEarlyChunkHistory.entries[key] = earlyChunkTestCell(t, now, times...)
	chatEarlyChunkHistory.Unlock()
	t.Cleanup(func() {
		chatEarlyChunkHistory.Lock()
		delete(chatEarlyChunkHistory.entries, key)
		chatEarlyChunkHistory.Unlock()
	})
	grid, err := GetChatEarlyChunkGrid(context.Background(), name, channel, -1)
	require.NoError(t, err)
	require.Len(t, grid.Cells, 1)
	assert.EqualValues(t, -1, grid.ProbeMS)
	expected := earlyChunkPredict(map[int]*earlyChunkCell{0: earlyChunkTestCell(t, now, times...)}, 0, 0, now, -1, config)
	assert.Equal(t, expected, grid.Cells[0].Prediction)
	require.NotNil(t, grid.Cells[0].Prediction.LeadMS)
	assert.EqualValues(t, 1800, *grid.Cells[0].Prediction.LeadMS)
	assert.EqualValues(t, 10200, grid.Cells[0].Prediction.AtMS)
	grid, err = GetChatEarlyChunkGrid(context.Background(), name, channel, 13000)
	require.NoError(t, err)
	assert.False(t, grid.Cells[0].Prediction.Eligible)
	assert.Nil(t, grid.Cells[0].Prediction.EstimatedTTFTMS)
}
