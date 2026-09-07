package service

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/channelcapacity"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

const channelCapacityContextKey = "__channel_model_capacity"

var channelCapacityMemoryLimiter = channelcapacity.NewMemoryLimiter()
var channelCapacityNow = time.Now

type ChannelModelCapacityError struct {
	ChannelID  int
	Model      string
	RetryAfter time.Duration
}

func (e *ChannelModelCapacityError) Error() string {
	return fmt.Sprintf("available channel capacity for model %s is exhausted", e.Model)
}

type ChannelCapacityState struct {
	blocked       map[int]struct{}
	eligible      map[int]struct{}
	retryAfter    time.Duration
	selectedRetry int
	selectedGroup string
	PromptTokens  int64
}

// ConfigureChannelModelCapacity is called after validation, before token counting.
// The state is request-local; counters are shared by public model and channel ID.
func ConfigureChannelModelCapacity(param *RetryParam, info *relaycommon.RelayInfo) (bool, error) {
	if info.RelayFormat == types.RelayFormatOpenAIRealtime {
		return false, nil
	}
	var candidates []model.Ability
	if _, forced := param.Ctx.Get("specific_channel_id"); forced {
		channel, err := model.CacheGetChannel(common.GetContextKeyInt(param.Ctx, constant.ContextKeyChannelId))
		if err != nil {
			return false, err
		}
		rpm, tpm, err := model.ResolveChannelModelRateLimits(channel, param.ModelName)
		if err != nil {
			return false, err
		}
		candidates = []model.Ability{{ChannelId: channel.Id, RPM: rpm, TPM: tpm}}
	} else {
		groups := []string{param.TokenGroup}
		if param.TokenGroup == "auto" {
			groups = GetRequestAutoGroups(param.Ctx, common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup))
		}
		for _, group := range groups {
			groupCandidates, err := model.ListChannelSelectionCandidates(group, param.ModelName, model.ChannelSelectionFilters{RequestPath: param.RequestPath, RequestBody: param.RequestBody, AllowedChannelIds: param.AllowedChannelIds})
			if err != nil {
				if param.TokenGroup == "auto" && errors.Is(err, model.ErrParameterCapabilityUnsupported) {
					continue
				}
				return false, err
			}
			candidates = append(candidates, groupCandidates...)
		}
	}
	enabled, tokens := false, false
	for _, candidate := range candidates {
		enabled = enabled || candidate.RPM > 0 || candidate.TPM > 0
		tokens = tokens || candidate.TPM > 0
	}
	if enabled {
		param.Capacity = &ChannelCapacityState{blocked: make(map[int]struct{}), eligible: make(map[int]struct{})}
		param.Ctx.Set(channelCapacityContextKey, param)
	}
	return tokens, nil
}

func (p *RetryParam) RecordCapacitySelection(group string, retry int) {
	if p.Capacity == nil {
		return
	}
	p.Capacity.selectedGroup, p.Capacity.selectedRetry = group, retry
}

// A local denial restores the selected group's cursor, including when normal
// upstream retry bookkeeping has already advanced it to the following group.
func (p *RetryParam) RetryAfterCapacityDenial() {
	state := p.Capacity
	if state == nil {
		return
	}
	p.SetRetry(state.selectedRetry)
	p.ResetRetryNextTry()
	if p.TokenGroup == "auto" {
		groups := GetRequestAutoGroups(p.Ctx, common.GetContextKeyString(p.Ctx, constant.ContextKeyUserGroup))
		for i, group := range groups {
			if group == state.selectedGroup {
				common.SetContextKey(p.Ctx, constant.ContextKeyAutoGroupIndex, i)
				common.SetContextKey(p.Ctx, constant.ContextKeyAutoGroup, group)
				break
			}
		}
	}
	p.Ctx.Set(ginKeyChannelAffinitySkipRetry, false)
	p.Ctx.Set(ginKeyChannelAffinityLogInfo, nil)
}

func (p *RetryParam) CapacityError() error {
	if p.Capacity == nil || len(p.Capacity.blocked) == 0 {
		return nil
	}
	for id := range p.Capacity.eligible {
		if _, blocked := p.Capacity.blocked[id]; !blocked {
			return nil
		}
	}
	return &ChannelModelCapacityError{Model: p.ModelName, RetryAfter: p.Capacity.retryAfter}
}

func selectChannelWithCapacity(param *RetryParam, group string, retry int) (*model.Channel, error) {
	filters := model.ChannelSelectionFilters{RequestPath: param.RequestPath, RequestBody: param.RequestBody, AllowedChannelIds: param.AllowedChannelIds}
	if param.Capacity == nil {
		return model.GetRandomSatisfiedChannelWithSelectionFilters(group, param.ModelName, retry, filters)
	}
	candidates, err := model.ListChannelSelectionCandidates(group, param.ModelName, filters)
	if err != nil || len(candidates) == 0 {
		return nil, err
	}
	priorities := make([]int64, 0)
	seen := make(map[int64]bool)
	for _, candidate := range candidates {
		param.Capacity.eligible[candidate.ChannelId] = struct{}{}
		priority := int64(0)
		if candidate.Priority != nil {
			priority = *candidate.Priority
		}
		if !seen[priority] {
			seen[priority] = true
			priorities = append(priorities, priority)
		}
	}
	sort.Slice(priorities, func(i, j int) bool { return priorities[i] > priorities[j] })
	for _, priority := range priorities[min(max(retry, 0), len(priorities)-1):] {
		var tier []model.Ability
		weightSum := int64(0)
		for _, candidate := range candidates {
			value := int64(0)
			if candidate.Priority != nil {
				value = *candidate.Priority
			}
			if _, blocked := param.Capacity.blocked[candidate.ChannelId]; blocked || value != priority {
				continue
			}
			tier = append(tier, candidate)
			weightSum += int64(candidate.Weight) + 10
		}
		if len(tier) == 0 {
			continue
		}
		choice := common.GetRandomInt(int(weightSum))
		for _, candidate := range tier {
			choice -= int(candidate.Weight) + 10
			if choice < 0 {
				return model.CacheGetChannel(candidate.ChannelId)
			}
		}
	}
	return nil, nil
}

// AdmitFinalChannelModelCapacity checks the actual outbound payload after
// provider conversion and request policies, immediately before dispatch.
func AdmitFinalChannelModelCapacity(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) error {
	value, exists := c.Get(channelCapacityContextKey)
	if !exists {
		return nil
	}
	param, ok := value.(*RetryParam)
	if !ok || param.Capacity == nil {
		return errors.New("invalid channel capacity state")
	}
	channel, err := model.CacheGetChannel(info.ChannelId)
	if err != nil {
		return err
	}
	rpm, tpm, err := model.ResolveChannelModelRateLimits(channel, param.ModelName)
	if err != nil {
		return err
	}
	if rpm == 0 && tpm == 0 {
		return nil
	}
	tokens := int64(0)
	if tpm > 0 {
		tokens = finalChannelModelCapacityReservation(info, param.Capacity.PromptTokens, nil, 1)
		if replayable, ok := body.(common.ReplayableBody); ok {
			tokens, err = EstimateFinalChannelModelCapacityTokens(c, info, replayable, tokens)
			if err != nil {
				return err
			}
		}
	}
	limiter := channelCapacityMemoryLimiter
	if common.RedisEnabled {
		limiter = channelcapacity.NewRedisLimiter(common.RDB)
	}
	decision, err := limiter.Acquire(c.Request.Context(), channelcapacity.Key{ChannelID: channel.Id, Model: param.ModelName}, channelcapacity.Limits{RPM: rpm, TPM: tpm}, tokens, channelCapacityNow())
	if err != nil {
		return err
	}
	if decision.Allowed {
		return nil
	}
	state := param.Capacity
	state.eligible[channel.Id] = struct{}{}
	state.blocked[channel.Id] = struct{}{}
	if state.retryAfter == 0 || decision.RetryAfter < state.retryAfter {
		state.retryAfter = decision.RetryAfter
	}
	return &ChannelModelCapacityError{ChannelID: channel.Id, Model: param.ModelName, RetryAfter: decision.RetryAfter}
}
