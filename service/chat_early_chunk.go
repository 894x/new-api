package service

import (
	"fmt"
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
const chatEarlyChunkStreamContextKey = "chat_early_chunk_stream"

// The public SSE belongs to the request and survives channel retries. Attempt
// statistics still belong to the channel that actually performed each attempt.
type chatEarlyChunkStream struct {
	mu       sync.Mutex
	sent     bool
	upstream bool
	terminal bool
	bytes    int
	created  int64
	model    string
	info     *relaycommon.RelayInfo
}

func chatEarlyChunkStreamFor(c *gin.Context) *chatEarlyChunkStream {
	if c == nil {
		return nil
	}
	value, _ := c.Get(chatEarlyChunkStreamContextKey)
	stream, _ := value.(*chatEarlyChunkStream)
	return stream
}

// Only gateway-owned empty bytes may cross the normal committed-response retry
// boundary. Any upstream event, terminal, or untracked write closes this gate.
func CanRetryChatEarlyChunk(c *gin.Context) bool {
	stream := chatEarlyChunkStreamFor(c)
	if stream == nil || c.Writer == nil {
		return false
	}
	stream.mu.Lock()
	defer stream.mu.Unlock()
	return stream.sent && !stream.upstream && !stream.terminal && c.Writer.Size() == stream.bytes
}

type ChatEarlyChunkObservation struct {
	Mode         string                `json:"mode"`
	Prediction   *EarlyChunkPrediction `json:"prediction,omitempty"`
	MatchedBytes int                   `json:"matched_bytes"`
	InputBytes   int                   `json:"input_bytes"`
	InputTokens  int                   `json:"input_tokens"`
	TokenBand    int                   `json:"token_band"`
	CacheBand    int                   `json:"cache_band"`
	MergedCells  int                   `json:"merged_cells"`
	Samples      int                   `json:"samples"`
	Probability  float64               `json:"probability"`
	DecisionMS   *int64                `json:"decision_ms,omitempty"`
	ContentMS    *int64                `json:"content_ms,omitempty"`
	GapMS        *int64                `json:"gap_ms,omitempty"`
	Sent         bool                  `json:"sent"`
	MetTarget    bool                  `json:"met_target"`
	Failed       bool                  `json:"failed"`
}

// Each attempt captures its pool's mode. Statistics are shared by model/channel.
type ChatEarlyChunkAttempt struct {
	mu           sync.Mutex
	config       chat_early_chunk_setting.Config
	key          string // model + channel generation; never contains pool/user IDs
	started      time.Time
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
	observation  ChatEarlyChunkObservation
	stream       *chatEarlyChunkStream
	writer       gin.ResponseWriter
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
	stream := chatEarlyChunkStreamFor(c)
	if stream != nil {
		stream.mu.Lock()
		stream.info = info
		stream.mu.Unlock()
	}
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
	cacheBand := earlyChunkCacheBand(tokenBand, match.MatchedBytes, inputBytes, cacheKnown)
	started := info.StartTime
	if info.RequestTiming != nil {
		if received := info.RequestTiming.Snapshot().RequestReceivedAtMs; received > 0 && received < started.UnixMilli() {
			started = started.Add(-time.Duration(started.UnixMilli()-received) * time.Millisecond)
		}
	}
	if stream == nil {
		stream = &chatEarlyChunkStream{created: started.Unix(), model: p.ModelName, info: info}
		c.Set(chatEarlyChunkStreamContextKey, stream)
	}
	// The first controller attempt can carry only a channel summary. Middleware
	// already captured the immutable full upstream generation used for dispatch.
	generation := chatPrefixChannelIdentity(channel)
	if value, exists := c.Get(chatPrefixChannelContextKey); exists {
		if bound, ok := value.(chatPrefixChannel); ok && bound.ID == channel.Id {
			generation = bound
		}
	}
	c.Set(chatEarlyChunkContextKey, &ChatEarlyChunkAttempt{
		config: config, key: earlyChunkChannelScopeKey(p.ModelName, generation, config.MaxAgeSeconds),
		started: started,
		created: started.Unix(), model: p.ModelName, channelID: channel.Id, group: group,
		tokenBand: tokenBand, cacheBand: cacheBand,
		observation: ChatEarlyChunkObservation{Mode: config.Mode, MatchedBytes: match.MatchedBytes, InputBytes: inputBytes, InputTokens: inputTokens, TokenBand: tokenBand, CacheBand: cacheBand},
		stream:      stream,
		writer:      c.Writer,
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
	if err == nil {
		a.refreshPrediction(now)
	}
	a.mu.Unlock()
	if err != nil {
		earlyChunkReadErrors.Add(1)
		common.SysError("early chunk history lookup failed: " + err.Error())
	}
}

func (a *ChatEarlyChunkAttempt) ShouldPredict() bool {
	if a == nil || a.config.Mode == "off" || a.tokenBand == earlyChunkUnknownTokens || a.cacheBand == earlyChunkUnknownCache {
		return false
	}
	if a.stream != nil {
		a.stream.mu.Lock()
		defer a.stream.mu.Unlock()
		return !a.stream.sent && !a.stream.upstream && !a.stream.terminal
	}
	return true
}

func (a *ChatEarlyChunkAttempt) UpstreamArrived() {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.upstream = true
	a.mu.Unlock()
}

// The caller holds a.mu; the config and cells are captured for this attempt.
func (a *ChatEarlyChunkAttempt) refreshPrediction(now time.Time) EarlyChunkPrediction {
	prediction := earlyChunkPredict(a.cells, a.tokenBand, a.cacheBand, now, max(int64(0), now.Sub(a.started).Milliseconds()), a.config)
	a.observation.Prediction = &prediction
	a.observation.Samples = int(prediction.Pending)
	a.observation.Probability = prediction.Probability
	a.observation.MergedCells = prediction.MergedCells
	return prediction
}

func (a *ChatEarlyChunkAttempt) NextCheck(now time.Time) time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.finished || a.upstream || a.terminal || a.observation.DecisionMS != nil || a.lookupFailed {
		return time.Millisecond
	}
	prediction := a.refreshPrediction(now)
	if prediction.EstimatedTTFTMS == nil || prediction.Pending < uint64(a.config.MinSamples) {
		return time.Millisecond
	}
	if prediction.CandidateMS != nil && *prediction.CandidateMS > prediction.AtMS {
		return time.Duration(*prediction.CandidateMS-prediction.AtMS) * time.Millisecond
	}
	return 50 * time.Millisecond
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
	if !a.ShouldPredict() || a.finished || a.upstream || a.terminal || a.observation.DecisionMS != nil || a.lookupFailed {
		return true, nil
	}
	prediction := a.refreshPrediction(now)
	if prediction.EstimatedTTFTMS == nil || prediction.Pending < uint64(a.config.MinSamples) {
		return true, nil
	}
	if !prediction.Eligible {
		return false, nil
	}
	a.observation.DecisionMS = &elapsed
	if a.config.Mode == "observe" {
		return true, nil
	}
	a.observation.Sent = true
	if a.stream != nil {
		a.stream.mu.Lock()
		a.stream.sent = true
		a.stream.mu.Unlock()
	}
	err := emit()
	if a.stream != nil {
		a.stream.mu.Lock()
		// The sender and ordinary writes share the stream write mutex.
		// Capture the exact boundary after the empty event has been flushed.
		a.stream.bytes = a.writer.Size()
		a.stream.mu.Unlock()
	}
	return true, err
}

func (a *ChatEarlyChunkAttempt) Metadata() (string, int64) {
	if a.stream != nil {
		return a.stream.model, a.stream.created
	}
	return a.model, a.created
}

// Only the pre-emitted stream pins its public metadata. Upstream IDs are still
// captured by the normal writer; the synthetic event bypasses that capture.
func NormalizeChatEarlyChunkData(c *gin.Context, data string) (string, error) {
	stream := chatEarlyChunkStreamFor(c)
	if stream == nil {
		return data, nil
	}
	stream.mu.Lock()
	sent, modelName, created := stream.sent, stream.model, stream.created
	stream.mu.Unlock()
	if !sent || !gjson.Get(data, "choices").Exists() {
		return data, nil
	}
	var err error
	data, err = sjson.Set(data, "model", modelName)
	if err != nil {
		return "", err
	}
	return sjson.Set(data, "created", created)
}

func ObserveChatEarlyChunkData(c *gin.Context, data string, now time.Time) {
	if stream := chatEarlyChunkStreamFor(c); stream != nil {
		stream.mu.Lock()
		if data == "[DONE]" || gjson.Get(data, "error").Exists() && gjson.Get(data, "error").Type != gjson.Null {
			stream.terminal = true
		} else {
			stream.upstream = true
		}
		stream.mu.Unlock()
	}
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
		lead := int64(a.config.MinLeadMS)
		if a.observation.Prediction != nil && a.observation.Prediction.LeadMS != nil {
			lead = *a.observation.Prediction.LeadMS
		}
		a.observation.MetTarget = gap >= 0 && gap <= lead
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
	stream := chatEarlyChunkStreamFor(c)
	if stream == nil {
		return false
	}
	stream.mu.Lock()
	defer stream.mu.Unlock()
	if !stream.sent || stream.terminal {
		return false
	}
	stream.terminal = true
	return true
}

// A pre-emitted stream must not turn a scanner failure or a truncated upstream
// response into a successful [DONE]. Normal, untouched streams keep their
// existing compatibility behavior.
func ChatEarlyChunkStreamError(c *gin.Context) *types.NewAPIError {
	stream := chatEarlyChunkStreamFor(c)
	if stream == nil {
		return nil
	}
	stream.mu.Lock()
	sent, terminal, info := stream.sent, stream.terminal, stream.info
	stream.mu.Unlock()
	if !sent || terminal || info == nil || info.StreamStatus == nil || earlyChunkStreamSucceeded(info) {
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
		lead := int64(a.config.MinLeadMS)
		if observation.Prediction != nil && observation.Prediction.LeadMS != nil {
			lead = *observation.Prediction.LeadMS
		}
		observation.MetTarget = gap >= 0 && gap <= lead
	}
	return observation
}
