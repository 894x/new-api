package service

import (
	"fmt"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/chat_early_chunk_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const chatEarlyChunkContextKey = "chat_early_chunk_attempt"

type ChatEarlyChunkObservation struct {
	Mode         string  `json:"mode"`
	MatchedBytes int     `json:"matched_bytes"`
	InputBytes   int     `json:"input_bytes"`
	InputTokens  int     `json:"input_tokens"`
	TokenBand    int     `json:"token_band"`
	CacheBand    int     `json:"cache_band"`
	MergedCells  int     `json:"merged_cells"`
	Samples      int     `json:"samples"`
	Probability  float64 `json:"probability"`
	DecisionMS   *int64  `json:"decision_ms,omitempty"`
	ContentMS    *int64  `json:"content_ms,omitempty"`
	GapMS        *int64  `json:"gap_ms,omitempty"`
	Sent         bool    `json:"sent"`
	MetTarget    bool    `json:"met_target"`
	Failed       bool    `json:"failed"`
}

// Each attempt captures its pool's mode. Statistics are shared by model/channel.
type ChatEarlyChunkAttempt struct {
	mu           sync.Mutex
	config       chat_early_chunk_setting.Config
	key          string // model + channel generation; never contains pool/user IDs
	started      time.Time
	notBefore    time.Time
	created      int64
	model        string
	channelID    int
	group        string
	cells        map[int]*earlyChunkCell
	tokenBand    int
	cacheBand    int
	loaded       bool
	lookupFailed bool
	upstream     bool
	terminal     bool
	finished     bool
	info         *relaycommon.RelayInfo
	observation  ChatEarlyChunkObservation
}

func ChatEarlyChunk(c *gin.Context) *ChatEarlyChunkAttempt {
	if c == nil {
		return nil
	}
	value, _ := c.Get(chatEarlyChunkContextKey)
	attempt, _ := value.(*ChatEarlyChunkAttempt)
	return attempt
}

func BeginChatEarlyChunkAttempt(p *RetryParam, info *relaycommon.RelayInfo, channel *model.Channel) {
	c := p.Ctx
	c.Set(chatEarlyChunkContextKey, (*ChatEarlyChunkAttempt)(nil))
	if !info.IsStream || info.RelayFormat != types.RelayFormatOpenAI || c.Request.Method != "POST" || c.Request.URL.Path != "/v1/chat/completions" || info.IsPlayground {
		return
	}
	request := p.chatPrefixRequest()
	group := info.UsingGroup
	pool, err := p.ModelRoutingPool(group)
	if err != nil {
		return
	}
	userRows, err := p.UserChannelRoutingForGroup(group)
	if err != nil {
		return
	}
	var userConfig *chat_early_chunk_setting.Pool
	userPool := len(userRows) > 0
	foundStrategy := false
	// These are independent pools: a user pool inherits the system mode, not
	// channel overrides belonging to the default group/model pool.
	names := []string{p.ModelName}
	if normalized := ratio_setting.RoutingMatchModelName(p.ModelName); normalized != "" && normalized != p.ModelName {
		names = append(names, normalized)
	}
	for _, name := range names {
		strategies, lookupErr := model.ListUserRoutingStrategies(info.UserId, name)
		if lookupErr != nil {
			return
		}
		for _, strategy := range strategies {
			if strategy.RoutingGroup == group {
				userPool = true
				foundStrategy = true
				userConfig = strategy.EarlyChunk
				break
			}
		}
		if foundStrategy {
			break
		}
	}
	config := model.CurrentRequestPolicy().ChatEarlyChunk
	if userPool {
		config.Mode = userConfig.Resolve(config.Mode, channel.Id)
	} else if pool != nil && pool.Tuning != nil {
		config.Mode = pool.Tuning.ChatEarlyChunk.Resolve(config.Mode, channel.Id)
	}
	if config.Validate() != nil {
		return
	}
	match := ChatPrefixMatch{}
	if value, exists := c.Get("chat_prefix_selection"); exists {
		if selection, ok := value.(chatPrefixSelection); ok && selection.Group == group && selection.ChannelID == channel.Id {
			match = selection.Match
		}
	}
	cacheKnown := request != nil && match.MatchedBytes > 0
	if request != nil && !cacheKnown {
		lookupParam := *p
		lookupParam.AttemptedChannelIds = nil
		var matches map[int]ChatPrefixMatch
		matches, cacheKnown = lookupParam.chatPrefixMatches(group, []model.Ability{{ChannelId: channel.Id}})
		match = matches[channel.Id]
	}
	if common.RedisEnabled && common.RDB == nil {
		cacheKnown = false
	}
	inputBytes := len(p.RequestBody)
	if request != nil {
		inputBytes = request.Bytes
	}
	inputTokens := info.GetEstimatePromptTokens()
	if inputTokens <= 0 {
		parsed := info.Request
		if parsed == nil && len(p.RequestBody) > 0 {
			var body dto.GeneralOpenAIRequest
			if common.Unmarshal(p.RequestBody, &body) == nil {
				parsed = &body
			}
		}
		if parsed != nil {
			meta := parsed.GetTokenCountMeta()
			// Prediction never initiates media downloads or changes the billing
			// estimate. Unknown media estimates get their own diagnostic row.
			if meta != nil && len(meta.Files) == 0 {
				if estimated, countErr := EstimateRequestTokenForCapacity(c, meta, info); countErr == nil {
					inputTokens = estimated
				}
			}
		}
	}
	tokenBand := earlyChunkTokenBand(inputTokens)
	cacheBand := earlyChunkCacheBand(match.MatchedBytes, inputBytes, cacheKnown)
	started := info.StartTime
	if info.RequestTiming != nil {
		if received := info.RequestTiming.Snapshot().RequestReceivedAtMs; received > 0 && received < started.UnixMilli() {
			started = started.Add(-time.Duration(started.UnixMilli()-received) * time.Millisecond)
		}
	}
	// Jitter spreads acknowledgments around the configured window. It only sets
	// the earliest eligible time; prediction and upstream-arrival gates still apply.
	delayMS := config.WindowStartMS + rand.IntN(config.WindowEndMS-config.WindowStartMS)
	c.Set(chatEarlyChunkContextKey, &ChatEarlyChunkAttempt{
		info: info, config: config, key: earlyChunkScopeKey(p.ModelName, channel, config.MaxAgeSeconds),
		started: started, notBefore: started.Add(time.Duration(delayMS) * time.Millisecond),
		created: started.Unix(), model: p.ModelName, channelID: channel.Id, group: group,
		tokenBand: tokenBand, cacheBand: cacheBand,
		observation: ChatEarlyChunkObservation{Mode: config.Mode, MatchedBytes: match.MatchedBytes, InputBytes: inputBytes, InputTokens: inputTokens, TokenBand: tokenBand, CacheBand: cacheBand},
	})
}

func (a *ChatEarlyChunkAttempt) Load(c *gin.Context, _ string, now time.Time) {
	if a == nil {
		return
	}
	a.mu.Lock()
	if a.loaded {
		a.mu.Unlock()
		return
	}
	a.loaded = true
	a.mu.Unlock()
	if !a.ShouldPredict() {
		return
	}
	cells, _, _, err := earlyChunkReadCells(c.Request.Context(), a.key, earlyChunkNeighborIDs(a.tokenBand, a.cacheBand), now, a.config.MaxAgeSeconds)
	a.mu.Lock()
	a.cells = cells
	a.lookupFailed = err != nil
	a.mu.Unlock()
	if err != nil {
		earlyChunkReadErrors.Add(1)
		common.SysError("early chunk history lookup failed: " + err.Error())
	}
}

func (a *ChatEarlyChunkAttempt) ShouldPredict() bool {
	return a != nil && a.config.Mode != "off" && a.tokenBand != earlyChunkUnknownTokens && a.cacheBand != earlyChunkUnknownCache
}

func (a *ChatEarlyChunkAttempt) UpstreamArrived() {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.upstream = true
	a.mu.Unlock()
}

func (a *ChatEarlyChunkAttempt) NextCheck(now time.Time) time.Duration {
	start := a.started.Add(time.Duration(a.config.WindowStartMS) * time.Millisecond)
	if a.notBefore.After(start) {
		start = a.notBefore
	}
	if now.Before(start) {
		return start.Sub(now)
	}
	end := a.started.Add(time.Duration(a.config.WindowEndMS+1) * time.Millisecond)
	return max(time.Millisecond, min(50*time.Millisecond, end.Sub(now)))
}

// The caller serializes this callback with normal stream writes. Upstream
// arrival and a timer firing are linearized under the same attempt lock.
func (a *ChatEarlyChunkAttempt) Try(now time.Time, emit func() error) (bool, error) {
	if a == nil {
		return true, nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	elapsed := now.Sub(a.started).Milliseconds()
	if !a.ShouldPredict() || a.finished || a.upstream || a.terminal || a.observation.DecisionMS != nil || elapsed > int64(a.config.WindowEndMS) || a.lookupFailed {
		return true, nil
	}
	if elapsed < int64(a.config.WindowStartMS) || now.Before(a.notBefore) {
		return false, nil
	}
	prediction := earlyChunkPredict(a.cells, a.tokenBand, a.cacheBand, now, elapsed, a.config)
	a.observation.Samples = int(prediction.Pending)
	a.observation.Probability = prediction.Probability
	a.observation.MergedCells = prediction.MergedCells
	if !prediction.Eligible {
		return false, nil
	}
	a.observation.DecisionMS = &elapsed
	if a.config.Mode == "observe" {
		return true, nil
	}
	a.observation.Sent = true
	return true, emit()
}

func (a *ChatEarlyChunkAttempt) Metadata() (string, int64) { return a.model, a.created }

// Only the pre-emitted stream pins its public metadata. Upstream IDs are still
// captured by the normal writer; the synthetic event bypasses that capture.
func NormalizeChatEarlyChunkData(c *gin.Context, data string) (string, error) {
	a := ChatEarlyChunk(c)
	if a == nil {
		return data, nil
	}
	a.mu.Lock()
	sent := a.observation.Sent
	a.mu.Unlock()
	if !sent || !gjson.Get(data, "choices").Exists() {
		return data, nil
	}
	var err error
	for _, field := range []struct {
		name  string
		value any
	}{{"model", a.model}, {"created", a.created}} {
		data, err = sjson.Set(data, field.name, field.value)
		if err != nil {
			return "", err
		}
	}
	return data, nil
}

func ObserveChatEarlyChunkData(c *gin.Context, data string, now time.Time) {
	a := ChatEarlyChunk(c)
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	errorField := gjson.Get(data, "error")
	if data == "[DONE]" || errorField.Exists() && errorField.Type != gjson.Null {
		a.terminal = true
		return
	}
	if a.observation.ContentMS != nil {
		return
	}
	for _, choice := range gjson.Get(data, "choices").Array() {
		delta := choice.Get("delta")
		if delta.Get("content").String() != "" || delta.Get("reasoning_content").String() != "" || delta.Get("reasoning").String() != "" || len(delta.Get("tool_calls").Array()) > 0 || delta.Get("function_call").Exists() {
			ms := now.Sub(a.started).Milliseconds()
			a.observation.ContentMS = &ms
			return
		}
	}
}

func earlyChunkStreamSucceeded(info *relaycommon.RelayInfo) bool {
	if info == nil || info.StreamStatus == nil || info.PerformanceBusinessRejection {
		return false
	}
	outcome := info.StreamStatus.OutcomeSnapshot()
	if outcome.HasErrors || outcome.Response == relaycommon.ResponseOutcomeFailed || outcome.Response == relaycommon.ResponseOutcomeIncomplete || outcome.Response == relaycommon.ResponseOutcomeCancelled {
		return false
	}
	if outcome.EndReason == relaycommon.StreamEndReasonDone {
		return true
	}
	return outcome.Response == relaycommon.ResponseOutcomeCompleted && (outcome.EndReason == relaycommon.StreamEndReasonEOF || outcome.EndReason == relaycommon.StreamEndReasonHandlerStop || outcome.EndReason == "")
}

// The controller calls this only after all stream writers have exited. A
// synthetic start never marks success, contributes usage or becomes a sample
// of actual upstream first-token time.
func FinishChatEarlyChunkAttempt(c *gin.Context, info *relaycommon.RelayInfo, handlerErr *types.NewAPIError) {
	a := ChatEarlyChunk(c)
	if a == nil {
		return
	}
	a.Load(c, info.UpstreamModelName, time.Now())
	a.mu.Lock()
	if a.finished {
		a.mu.Unlock()
		return
	}
	a.finished = true
	success := handlerErr == nil && a.observation.ContentMS != nil && earlyChunkStreamSucceeded(info)
	a.observation.Failed = !success
	now := time.Now()
	record := earlyChunkRecord{At: now, LatencyMS: max(int64(0), now.Sub(a.started).Milliseconds()), Kind: "failure", Outcome: "failed"}
	cancelled := c.Request.Context().Err() != nil || handlerErr != nil && handlerErr.GetErrorCode() == types.ErrorCodeClientGone
	if info.StreamStatus != nil && info.StreamStatus.OutcomeSnapshot().Response == relaycommon.ResponseOutcomeCancelled {
		cancelled = true
	}
	if cancelled {
		record.Kind, record.Outcome = "cancel", "cancelled"
	}
	if success {
		record.Outcome = "completed"
	}
	if a.observation.ContentMS != nil {
		// A later stream failure must not erase an observed first-content time.
		record.Kind, record.LatencyMS = "content", *a.observation.ContentMS
		record.At = a.started.Add(time.Duration(record.LatencyMS) * time.Millisecond)
	}
	if a.observation.DecisionMS != nil && a.observation.ContentMS != nil {
		gap := *a.observation.ContentMS - *a.observation.DecisionMS
		a.observation.GapMS = &gap
		a.observation.MetTarget = gap >= 0 && gap <= int64(a.config.TargetGapMS)
	}
	observation := a.observation
	a.mu.Unlock()
	record.Decision, record.Sent, record.Met, record.GapMS = observation.DecisionMS != nil, observation.Sent, observation.MetTarget, observation.GapMS
	if err := earlyChunkRecordCell(c.Request.Context(), a.key, earlyChunkCellID(a.tokenBand, a.cacheBand), record, now, a.config.MaxAgeSeconds); err != nil {
		earlyChunkWriteErrors.Add(1)
		common.SysError("early chunk history write failed: " + err.Error())
	}
	reason := "early_chunk_observed"
	if observation.Mode == "off" {
		reason = "early_chunk_recorded"
	}
	if observation.Sent {
		reason = "early_chunk_sent"
	}
	RequestPolicy(c).AddEvent(PolicyEvent{ChannelID: a.channelID, Group: a.group, Decision: PolicyDecision{Action: "observe", Reason: reason, Source: "early_chunk"}, EarlyChunk: &observation})
}

func ClaimChatEarlyChunkError(c *gin.Context) bool {
	a := ChatEarlyChunk(c)
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.observation.Sent || a.terminal {
		return false
	}
	a.terminal = true
	return true
}

// A pre-emitted stream must not turn a scanner failure or a truncated upstream
// response into a successful [DONE]. Normal, untouched streams keep their
// existing compatibility behavior.
func ChatEarlyChunkStreamError(c *gin.Context) *types.NewAPIError {
	a := ChatEarlyChunk(c)
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.observation.Sent || a.terminal || a.info == nil || a.info.StreamStatus == nil {
		return nil
	}
	if earlyChunkStreamSucceeded(a.info) {
		return nil
	}
	return types.NewOpenAIError(fmt.Errorf("upstream stream ended before completing the response"), types.ErrorCodeBadResponse, 502, types.ErrOptionWithSkipRetry())
}

func (a *ChatEarlyChunkAttempt) Snapshot(info *relaycommon.RelayInfo) ChatEarlyChunkObservation {
	a.mu.Lock()
	defer a.mu.Unlock()
	observation := a.observation
	if !a.finished && info != nil && info.StreamStatus != nil {
		observation.Failed = observation.ContentMS == nil || !earlyChunkStreamSucceeded(info)
	}
	if observation.DecisionMS != nil && observation.ContentMS != nil {
		gap := *observation.ContentMS - *observation.DecisionMS
		observation.GapMS = &gap
		observation.MetTarget = gap >= 0 && gap <= int64(a.config.TargetGapMS)
	}
	return observation
}
