package groupdiscount

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

const (
	MaxUsingGroupLength                      = 128
	MaxOriginModelLength                     = 255
	chargedProgressBoundaryDivisionPrecision = 16
)

var (
	ErrInvalidOriginalQuota = errors.New("original quota must be a positive, safely converted quota value")
	ErrMonthlyQuotaOverflow = errors.New("monthly original quota overflow")
	ErrChargeOverflow       = errors.New("tiered discount charge overflow")
	ErrInvalidProgressQuota = errors.New("progress quota must be a safely bounded canonical decimal")
)

func isJSONNull(data []byte) bool {
	return strings.TrimSpace(string(data)) == "null"
}

type ProgressBasis string

const (
	ProgressBasisOriginal ProgressBasis = "original"
	ProgressBasisCharged  ProgressBasis = "charged"
)

// Tier applies Ratio to the portion of a request that lies at or above
// MinMonthlyOriginalQuota and below the next tier's threshold.
type Tier struct {
	MinMonthlyOriginalQuota int64   `json:"min_monthly_original_quota"`
	Ratio                   float64 `json:"ratio"`
}

func (t *Tier) UnmarshalJSON(data []byte) error {
	fields, err := decodeStrictObject(data, map[string]struct{}{
		"min_monthly_original_quota": {},
		"ratio":                      {},
	})
	if err != nil {
		return err
	}
	thresholdJSON, hasThreshold := fields["min_monthly_original_quota"]
	ratioJSON, hasRatio := fields["ratio"]
	if !hasThreshold || !hasRatio {
		return errors.New("tier requires min_monthly_original_quota and ratio")
	}
	if isJSONNull(thresholdJSON) || isJSONNull(ratioJSON) {
		return errors.New("tier min_monthly_original_quota and ratio cannot be null")
	}
	var threshold int64
	var ratio float64
	if err := common.Unmarshal(thresholdJSON, &threshold); err != nil {
		return fmt.Errorf("invalid tier threshold: %w", err)
	}
	if err := common.Unmarshal(ratioJSON, &ratio); err != nil {
		return fmt.Errorf("invalid tier ratio: %w", err)
	}
	*t = Tier{MinMonthlyOriginalQuota: threshold, Ratio: ratio}
	return nil
}

// Policy is the administrator-owned configuration for one group and model.
// EffectiveUntil is exclusive. A disabled exact-model policy intentionally
// suppresses the group's wildcard policy for that model.
type Policy struct {
	Enabled        bool   `json:"enabled"`
	EffectiveFrom  int64  `json:"effective_from"`
	EffectiveUntil *int64 `json:"effective_until"`
	Timezone       string `json:"timezone"`
	Tiers          []Tier `json:"tiers"`
}

func (p *Policy) UnmarshalJSON(data []byte) error {
	fields, err := decodeStrictObject(data, map[string]struct{}{
		"enabled":         {},
		"effective_from":  {},
		"effective_until": {},
		"timezone":        {},
		"tiers":           {},
	})
	if err != nil {
		return err
	}
	for _, required := range []string{"enabled", "effective_from", "effective_until", "timezone", "tiers"} {
		value, ok := fields[required]
		if !ok {
			return fmt.Errorf("policy requires field %s", required)
		}
		if required != "effective_until" && isJSONNull(value) {
			return fmt.Errorf("policy field %s cannot be null", required)
		}
	}
	var policy Policy
	if err := common.Unmarshal(fields["enabled"], &policy.Enabled); err != nil {
		return fmt.Errorf("invalid policy enabled: %w", err)
	}
	if err := common.Unmarshal(fields["effective_from"], &policy.EffectiveFrom); err != nil {
		return fmt.Errorf("invalid policy effective_from: %w", err)
	}
	if !isJSONNull(fields["effective_until"]) {
		var effectiveUntil int64
		if err := common.Unmarshal(fields["effective_until"], &effectiveUntil); err != nil {
			return fmt.Errorf("invalid policy effective_until: %w", err)
		}
		policy.EffectiveUntil = &effectiveUntil
	}
	if err := common.Unmarshal(fields["timezone"], &policy.Timezone); err != nil {
		return fmt.Errorf("invalid policy timezone: %w", err)
	}
	if err := common.Unmarshal(fields["tiers"], &policy.Tiers); err != nil {
		return fmt.Errorf("invalid policy tiers: %w", err)
	}
	*p = policy
	return nil
}

type GroupPolicy struct {
	ProgressBasis ProgressBasis     `json:"progress_basis"`
	Models        map[string]Policy `json:"models"`
}

type PolicyMap map[string]GroupPolicy

// UnmarshalJSON accepts the legacy group -> model -> policy shape and
// normalizes it to the canonical group wrapper. This keeps existing option
// rows valid while making progress basis and model policies one atomic value.
func (p *PolicyMap) UnmarshalJSON(data []byte) error {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return err
	}
	var rawGroups map[string]json.RawMessage
	if err := common.Unmarshal(data, &rawGroups); err != nil {
		return err
	}
	if rawGroups == nil {
		return errors.New("model tiered ratios must be a JSON object")
	}
	parsed := make(PolicyMap, len(rawGroups))
	for group, rawGroup := range rawGroups {
		// Try the complete legacy shape first. Model names are unrestricted and
		// may legitimately be "models" or "progress_basis"; a key-name-only
		// discriminator would break those existing configurations.
		var legacyModels map[string]Policy
		if err := common.Unmarshal(rawGroup, &legacyModels); err == nil {
			parsed[group] = GroupPolicy{ProgressBasis: ProgressBasisOriginal, Models: legacyModels}
			continue
		}

		fields, err := decodeStrictObject(rawGroup, map[string]struct{}{
			"progress_basis": {},
			"models":         {},
		})
		if err != nil {
			return fmt.Errorf("invalid group %q: %w", group, err)
		}
		_, hasBasis := fields["progress_basis"]
		_, hasModels := fields["models"]
		if !hasBasis || !hasModels {
			return fmt.Errorf("group %q wrapper requires progress_basis and models", group)
		}
		if isJSONNull(fields["progress_basis"]) {
			return fmt.Errorf("progress_basis for group %q cannot be null", group)
		}
		var basis ProgressBasis
		if err := common.Unmarshal(fields["progress_basis"], &basis); err != nil {
			return fmt.Errorf("invalid progress_basis for group %q: %w", group, err)
		}
		var models map[string]Policy
		if err := common.Unmarshal(fields["models"], &models); err != nil {
			return fmt.Errorf("invalid models for group %q: %w", group, err)
		}
		parsed[group] = GroupPolicy{ProgressBasis: basis, Models: models}
	}
	*p = parsed
	return nil
}

// Snapshot freezes the policy and calendar period at request admission time.
// Subsequent configuration changes cannot alter an in-flight settlement.
type Snapshot struct {
	PolicyHash    string        `json:"policy_hash"`
	ProgressBasis ProgressBasis `json:"progress_basis"`
	UsingGroup    string        `json:"using_group"`
	OriginModel   string        `json:"origin_model"`
	MatchedModel  string        `json:"matched_model"`
	Timezone      string        `json:"timezone"`
	PeriodStart   int64         `json:"period_start"`
	PeriodEnd     int64         `json:"period_end"`
	Tiers         []Tier        `json:"tiers"`
}

type Segment struct {
	TierMin            int64   `json:"tier_min"`
	OriginalQuota      int64   `json:"original_quota"`
	Ratio              float64 `json:"ratio"`
	TierMinProgress    string  `json:"tier_min_progress,omitempty"`
	OriginalQuotaExact string  `json:"original_quota_exact,omitempty"`
	ProgressBefore     string  `json:"progress_before,omitempty"`
	ProgressAfter      string  `json:"progress_after,omitempty"`
	ProgressQuota      string  `json:"progress_quota,omitempty"`
}

type Calculation struct {
	MonthlyOriginalBefore int64     `json:"monthly_original_before"`
	MonthlyOriginalAfter  int64     `json:"monthly_original_after"`
	MonthlyProgressBefore string    `json:"monthly_progress_before"`
	MonthlyProgressAfter  string    `json:"monthly_progress_after"`
	OriginalQuota         int       `json:"original_quota"`
	ChargedQuota          int       `json:"charged_quota"`
	ProgressQuota         string    `json:"progress_quota"`
	Segments              []Segment `json:"segments"`
}

// PolicyStore provides atomic replacement of the complete configuration map.
// Its JSON unmarshaller validates into a temporary value before publishing it,
// so a malformed database/config update cannot partially corrupt live pricing.
type PolicyStore struct {
	mu   sync.RWMutex
	data PolicyMap
}

// FrozenResolver is the request-admission snapshot of every policy that may
// be selected by a later routing attempt. It also freezes the current user's
// explicit GroupGroup contracts, which take precedence over dynamic pricing.
// Neither live configuration changes nor auto-group retries mutate it.
type FrozenResolver struct {
	policies            PolicyMap
	contractUsingGroups map[string]struct{}
	originModel         string
	requestAt           time.Time
}

func NewFrozenResolver(
	policies PolicyMap,
	contractUsingGroups map[string]struct{},
	originModel string,
	requestAt time.Time,
) *FrozenResolver {
	return newFrozenResolver(clonePolicyMap(policies), contractUsingGroups, originModel, requestAt)
}

func newFrozenResolver(
	immutablePolicies PolicyMap,
	contractUsingGroups map[string]struct{},
	originModel string,
	requestAt time.Time,
) *FrozenResolver {
	contracts := make(map[string]struct{}, len(contractUsingGroups))
	for usingGroup := range contractUsingGroups {
		contracts[usingGroup] = struct{}{}
	}
	return &FrozenResolver{
		policies:            immutablePolicies,
		contractUsingGroups: contracts,
		originModel:         originModel,
		requestAt:           requestAt,
	}
}

// WithOriginModel binds a model discovered after request admission without
// observing newer policy configuration. Continuations can therefore restore
// their durable client model while retaining the original policy, contracts,
// and billing-period timestamp.
func (r *FrozenResolver) WithOriginModel(originModel string) *FrozenResolver {
	if r == nil {
		return nil
	}
	return newFrozenResolver(r.policies, r.contractUsingGroups, originModel, r.requestAt)
}

func (r *FrozenResolver) Resolve(usingGroup string) (Snapshot, bool, error) {
	if r == nil {
		return Snapshot{}, false, nil
	}
	if _, hasContract := r.contractUsingGroups[usingGroup]; hasContract {
		return Snapshot{}, false, nil
	}
	groupPolicy, ok := r.policies[usingGroup]
	if !ok {
		return Snapshot{}, false, nil
	}
	policy, matchedModel, ok := resolvePolicy(groupPolicy.Models, r.originModel)
	if !ok {
		return Snapshot{}, false, nil
	}
	return FreezePolicyWithBasis(policy, groupPolicy.ProgressBasis, usingGroup, r.originModel, matchedModel, r.requestAt)
}

func NewPolicyStore(initial PolicyMap) (*PolicyStore, error) {
	store := &PolicyStore{}
	if err := store.Replace(initial); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *PolicyStore) Replace(policies PolicyMap) error {
	if err := ValidatePolicyMap(policies); err != nil {
		return err
	}
	s.mu.Lock()
	s.data = clonePolicyMap(policies)
	s.mu.Unlock()
	return nil
}

func (s *PolicyStore) Resolve(usingGroup, originModel string) (Policy, string, bool) {
	policy, matchedModel, _, ok := s.ResolveWithBasis(usingGroup, originModel)
	return policy, matchedModel, ok
}

func (s *PolicyStore) ResolveWithBasis(usingGroup, originModel string) (Policy, string, ProgressBasis, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	groupPolicy, ok := s.data[usingGroup]
	if !ok {
		return Policy{}, "", "", false
	}
	policy, matchedModel, ok := resolvePolicy(groupPolicy.Models, originModel)
	return policy, matchedModel, normalizeProgressBasis(groupPolicy.ProgressBasis), ok
}

// FreezeResolver captures the store's current copy-on-write policy map without
// cloning every configured group. Replace always publishes a newly cloned map,
// so the captured reference remains immutable for the request's lifetime.
func (s *PolicyStore) FreezeResolver(
	contractUsingGroups map[string]struct{},
	originModel string,
	requestAt time.Time,
) *FrozenResolver {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return newFrozenResolver(s.data, contractUsingGroups, originModel, requestAt)
}

func (s *PolicyStore) ReadAll() PolicyMap {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clonePolicyMap(s.data)
}

func (s *PolicyStore) MarshalJSON() ([]byte, error) {
	return common.Marshal(s.ReadAll())
}

func (s *PolicyStore) UnmarshalJSON(data []byte) error {
	var policies PolicyMap
	if err := common.Unmarshal(data, &policies); err != nil {
		return err
	}
	return s.Replace(policies)
}

func ParsePoliciesJSON(raw string) (PolicyMap, error) {
	var policies PolicyMap
	if err := common.UnmarshalJsonStr(raw, &policies); err != nil {
		return nil, err
	}
	if err := ValidatePolicyMap(policies); err != nil {
		return nil, err
	}
	return policies, nil
}

func ValidatePoliciesJSON(raw string) error {
	_, err := ParsePoliciesJSON(raw)
	return err
}

func ValidatePolicyMap(policies PolicyMap) error {
	if policies == nil {
		return errors.New("model tiered ratios must be a JSON object")
	}
	for group, groupPolicy := range policies {
		if strings.TrimSpace(group) == "" {
			return errors.New("using group must not be empty")
		}
		if group == "__proto__" {
			return errors.New("using group __proto__ is not supported")
		}
		if len(group) > MaxUsingGroupLength {
			return fmt.Errorf("using group %q exceeds %d bytes", group, MaxUsingGroupLength)
		}
		basis := normalizeProgressBasis(groupPolicy.ProgressBasis)
		if basis != ProgressBasisOriginal && basis != ProgressBasisCharged {
			return fmt.Errorf("progress_basis for group %q must be original or charged", group)
		}
		models := groupPolicy.Models
		if models == nil {
			return fmt.Errorf("model policies for group %q must be a JSON object", group)
		}
		for model, policy := range models {
			if strings.TrimSpace(model) == "" {
				return fmt.Errorf("origin model in group %q must not be empty", group)
			}
			if model == "__proto__" {
				return fmt.Errorf("origin model __proto__ in group %q is not supported", group)
			}
			if len(model) > MaxOriginModelLength {
				return fmt.Errorf("origin model %q exceeds %d bytes", model, MaxOriginModelLength)
			}
			if policy.EffectiveFrom < 0 {
				return fmt.Errorf("effective_from for %s/%s must be non-negative", group, model)
			}
			if policy.EffectiveUntil != nil && *policy.EffectiveUntil <= policy.EffectiveFrom {
				return fmt.Errorf("effective_until for %s/%s must be greater than effective_from", group, model)
			}
			if policy.Timezone == "" {
				return fmt.Errorf("timezone for %s/%s must not be empty", group, model)
			}
			if policy.Timezone == "Local" {
				return fmt.Errorf("timezone for %s/%s must not depend on the server local setting", group, model)
			}
			if _, err := time.LoadLocation(policy.Timezone); err != nil {
				return fmt.Errorf("invalid timezone for %s/%s: %w", group, model, err)
			}
			if err := validateTiers(policy.Tiers); err != nil {
				return fmt.Errorf("invalid tiers for %s/%s: %w", group, model, err)
			}
			if basis == ProgressBasisCharged {
				for index, tier := range policy.Tiers {
					if tier.Ratio == 0 {
						return fmt.Errorf("invalid tiers for %s/%s: tier %d ratio must be positive when progress_basis is charged", group, model, index)
					}
				}
			}
		}
	}
	return nil
}

func decodeStrictObject(data []byte, allowed map[string]struct{}) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, errors.New("value must be a JSON object")
	}
	if allowed != nil {
		for field := range fields {
			if _, ok := allowed[field]; !ok {
				return nil, fmt.Errorf("unknown field %s", field)
			}
		}
	}
	return fields, nil
}

func FreezePolicy(policy Policy, usingGroup, originModel, matchedModel string, requestAt time.Time) (Snapshot, bool, error) {
	return FreezePolicyWithBasis(policy, ProgressBasisOriginal, usingGroup, originModel, matchedModel, requestAt)
}

func FreezePolicyWithBasis(
	policy Policy,
	progressBasis ProgressBasis,
	usingGroup,
	originModel,
	matchedModel string,
	requestAt time.Time,
) (Snapshot, bool, error) {
	if !policy.Enabled {
		return Snapshot{}, false, nil
	}
	progressBasis = normalizeProgressBasis(progressBasis)
	if err := ValidatePolicyMap(PolicyMap{usingGroup: {ProgressBasis: progressBasis, Models: map[string]Policy{matchedModel: policy}}}); err != nil {
		return Snapshot{}, false, err
	}
	now := requestAt.Unix()
	if now < policy.EffectiveFrom || (policy.EffectiveUntil != nil && now >= *policy.EffectiveUntil) {
		return Snapshot{}, false, nil
	}
	location, err := time.LoadLocation(policy.Timezone)
	if err != nil {
		return Snapshot{}, false, err
	}
	local := requestAt.In(location)
	monthStart := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, location).Unix()
	monthEnd := time.Date(local.Year(), local.Month()+1, 1, 0, 0, 0, 0, location).Unix()
	periodStart := monthStart
	if policy.EffectiveFrom > periodStart {
		periodStart = policy.EffectiveFrom
	}
	periodEnd := monthEnd
	if policy.EffectiveUntil != nil && *policy.EffectiveUntil < periodEnd {
		periodEnd = *policy.EffectiveUntil
	}
	if periodStart >= periodEnd {
		return Snapshot{}, false, nil
	}
	policyJSON, err := common.Marshal(struct {
		ProgressBasis ProgressBasis `json:"progress_basis"`
		Policy        Policy        `json:"policy"`
	}{ProgressBasis: progressBasis, Policy: policy})
	if err != nil {
		return Snapshot{}, false, err
	}
	digest := sha256.Sum256(policyJSON)
	return Snapshot{
		PolicyHash:    hex.EncodeToString(digest[:]),
		ProgressBasis: progressBasis,
		UsingGroup:    usingGroup,
		OriginModel:   originModel,
		MatchedModel:  matchedModel,
		Timezone:      policy.Timezone,
		PeriodStart:   periodStart,
		PeriodEnd:     periodEnd,
		Tiers:         append([]Tier(nil), policy.Tiers...),
	}, true, nil
}

func Calculate(snapshot Snapshot, monthlyBefore int64, originalQuota int) (Calculation, error) {
	return CalculateWithProgress(snapshot, monthlyBefore, decimal.NewFromInt(monthlyBefore).String(), originalQuota)
}

func CalculateWithProgress(snapshot Snapshot, monthlyOriginalBefore int64, monthlyProgressBefore string, originalQuota int) (Calculation, error) {
	if originalQuota <= 0 || originalQuota > common.MaxQuota {
		return Calculation{}, ErrInvalidOriginalQuota
	}
	if monthlyOriginalBefore < 0 || monthlyOriginalBefore > math.MaxInt64-int64(originalQuota) {
		return Calculation{}, ErrMonthlyQuotaOverflow
	}
	progressBefore, err := parseProgressQuota(monthlyProgressBefore)
	if err != nil {
		return Calculation{}, err
	}
	progressBasis := normalizeProgressBasis(snapshot.ProgressBasis)
	if err := validateSnapshot(snapshot, progressBasis); err != nil {
		return Calculation{}, err
	}
	monthlyOriginalAfter := monthlyOriginalBefore + int64(originalQuota)
	if progressBasis == ProgressBasisCharged {
		return calculateChargedProgress(snapshot, monthlyOriginalBefore, monthlyOriginalAfter, progressBefore, originalQuota)
	}
	if !progressBefore.Equal(decimal.NewFromInt(monthlyOriginalBefore)) {
		return Calculation{}, ErrInvalidProgressQuota
	}

	segments := make([]Segment, 0, len(snapshot.Tiers))
	for index, tier := range snapshot.Tiers {
		segmentStart := monthlyOriginalBefore
		if tier.MinMonthlyOriginalQuota > segmentStart {
			segmentStart = tier.MinMonthlyOriginalQuota
		}
		segmentEnd := monthlyOriginalAfter
		if index+1 < len(snapshot.Tiers) && snapshot.Tiers[index+1].MinMonthlyOriginalQuota < segmentEnd {
			segmentEnd = snapshot.Tiers[index+1].MinMonthlyOriginalQuota
		}
		if segmentEnd <= segmentStart {
			continue
		}
		amount := segmentEnd - segmentStart
		segments = append(segments, Segment{
			TierMin:       tier.MinMonthlyOriginalQuota,
			OriginalQuota: amount,
			Ratio:         tier.Ratio,
		})
	}
	// Bill the delta between cumulatively rounded totals. Rounding a request in
	// isolation would let 1+1 quota at ratio 0.5 cost 2 while a single 2-quota
	// request costs 1. The cumulative delta makes splitting/combining requests
	// invariant while still returning an integer charge for this settlement.
	cumulativeBefore := cumulativeDiscountedQuota(snapshot.Tiers, monthlyOriginalBefore).Round(0)
	cumulativeAfter := cumulativeDiscountedQuota(snapshot.Tiers, monthlyOriginalAfter).Round(0)
	chargedQuota, clamp := common.QuotaFromDecimalChecked(cumulativeAfter.Sub(cumulativeBefore))
	if clamp != nil || chargedQuota < 0 {
		return Calculation{}, fmt.Errorf("%w: %v", ErrChargeOverflow, clamp)
	}
	return Calculation{
		MonthlyOriginalBefore: monthlyOriginalBefore,
		MonthlyOriginalAfter:  monthlyOriginalAfter,
		MonthlyProgressBefore: progressBefore.String(),
		MonthlyProgressAfter:  decimal.NewFromInt(monthlyOriginalAfter).String(),
		OriginalQuota:         originalQuota,
		ChargedQuota:          chargedQuota,
		ProgressQuota:         decimal.NewFromInt(int64(originalQuota)).String(),
		Segments:              segments,
	}, nil
}

// CalculateDecrease prices a negative adjustment from the current marginal
// cursor. It does not retroactively reprice other settlements.
func CalculateDecrease(snapshot Snapshot, monthlyOriginalBefore int64, monthlyProgressBefore string, originalQuota int) (Calculation, error) {
	if originalQuota <= 0 || originalQuota > common.MaxQuota || monthlyOriginalBefore < int64(originalQuota) {
		return Calculation{}, ErrInvalidOriginalQuota
	}
	progressBefore, err := parseProgressQuota(monthlyProgressBefore)
	if err != nil {
		return Calculation{}, err
	}
	progressBasis := normalizeProgressBasis(snapshot.ProgressBasis)
	if err := validateSnapshot(snapshot, progressBasis); err != nil {
		return Calculation{}, err
	}
	monthlyOriginalAfter := monthlyOriginalBefore - int64(originalQuota)
	if progressBasis == ProgressBasisOriginal {
		if !progressBefore.Equal(decimal.NewFromInt(monthlyOriginalBefore)) {
			return Calculation{}, ErrInvalidProgressQuota
		}
		removed, calculateErr := CalculateWithProgress(
			snapshot,
			monthlyOriginalAfter,
			decimal.NewFromInt(monthlyOriginalAfter).String(),
			originalQuota,
		)
		if calculateErr != nil {
			return Calculation{}, calculateErr
		}
		return Calculation{
			MonthlyOriginalBefore: monthlyOriginalBefore,
			MonthlyOriginalAfter:  monthlyOriginalAfter,
			MonthlyProgressBefore: progressBefore.String(),
			MonthlyProgressAfter:  decimal.NewFromInt(monthlyOriginalAfter).String(),
			OriginalQuota:         -originalQuota,
			ChargedQuota:          -removed.ChargedQuota,
			ProgressQuota:         decimal.NewFromInt(-int64(originalQuota)).String(),
			Segments:              removed.Segments,
		}, nil
	}

	remainingOriginal := decimal.NewFromInt(int64(originalQuota))
	cursor := progressBefore
	for remainingOriginal.IsPositive() {
		index := tierIndexBeforeCursor(snapshot.Tiers, cursor)
		if index < 0 {
			return Calculation{}, ErrInvalidProgressQuota
		}
		tier := snapshot.Tiers[index]
		ratio := decimal.NewFromFloat(tier.Ratio)
		tierMin := decimal.NewFromInt(tier.MinMonthlyOriginalQuota)
		availableProgress := cursor.Sub(tierMin)
		if !availableProgress.IsPositive() {
			return Calculation{}, ErrInvalidProgressQuota
		}
		amount := remainingOriginal
		progressDelta := remainingOriginal.Mul(ratio)
		if progressDelta.GreaterThan(availableProgress) {
			// A division is necessary only when this removal crosses a tier
			// boundary. The explicit precision is part of the persisted cursor
			// contract; do not depend on decimal.DivisionPrecision globals.
			amount = availableProgress.DivRound(ratio, chargedProgressBoundaryDivisionPrecision)
			progressDelta = availableProgress
		}
		after := cursor.Sub(progressDelta)
		cursor = after
		remainingOriginal = remainingOriginal.Sub(amount)
	}
	if cursor.IsNegative() {
		return Calculation{}, ErrInvalidProgressQuota
	}
	canonicalAfter, forward, err := findChargedDecreasePreimage(
		snapshot,
		monthlyOriginalAfter,
		monthlyOriginalBefore,
		originalQuota,
		cursor,
		progressBefore,
	)
	if err != nil {
		return Calculation{}, err
	}
	segments := make([]Segment, 0, len(forward.Segments))
	for index := len(forward.Segments) - 1; index >= 0; index-- {
		segment := forward.Segments[index]
		progressDelta, parseErr := decimal.NewFromString(segment.ProgressQuota)
		if parseErr != nil {
			return Calculation{}, ErrInvalidProgressQuota
		}
		segment.ProgressBefore, segment.ProgressAfter = segment.ProgressAfter, segment.ProgressBefore
		segment.ProgressQuota = progressDelta.Neg().String()
		segments = append(segments, segment)
	}
	return Calculation{
		MonthlyOriginalBefore: monthlyOriginalBefore,
		MonthlyOriginalAfter:  monthlyOriginalAfter,
		MonthlyProgressBefore: progressBefore.String(),
		MonthlyProgressAfter:  canonicalAfter.String(),
		OriginalQuota:         -originalQuota,
		ChargedQuota:          -forward.ChargedQuota,
		ProgressQuota:         canonicalAfter.Sub(progressBefore).String(),
		Segments:              segments,
	}, nil
}

// findChargedDecreasePreimage canonicalizes a backward calculation only when
// replaying the removed original quota from that candidate returns the exact
// persisted cursor. Recurring division can create several finite-decimal
// preimages; the lowest verified scale is the stable representation. This
// preserves the historical cursor instead of recomputing the whole month.
func findChargedDecreasePreimage(
	snapshot Snapshot,
	monthlyOriginalAfter int64,
	monthlyOriginalBefore int64,
	originalQuota int,
	rawCandidate decimal.Decimal,
	target decimal.Decimal,
) (decimal.Decimal, Calculation, error) {
	maxSearchScale := int32(chargedProgressBoundaryDivisionPrecision)
	for _, tier := range snapshot.Tiers {
		ratioScale := -decimal.NewFromFloat(tier.Ratio).Exponent()
		if ratioScale < 0 {
			ratioScale = 0
		}
		if candidateScale := int32(chargedProgressBoundaryDivisionPrecision) + ratioScale; candidateScale > maxSearchScale {
			maxSearchScale = candidateScale
		}
	}
	rawScale := -rawCandidate.Exponent()
	if rawScale < 0 {
		rawScale = 0
	}
	searchScale := rawScale
	if searchScale > maxSearchScale {
		searchScale = maxSearchScale
	}
	lastCandidate := ""
	for scale := int32(0); scale <= searchScale; scale++ {
		candidate := rawCandidate.Round(scale)
		if candidate.IsNegative() || candidate.String() == lastCandidate {
			continue
		}
		lastCandidate = candidate.String()
		forward, err := calculateChargedProgress(
			snapshot,
			monthlyOriginalAfter,
			monthlyOriginalBefore,
			candidate,
			originalQuota,
		)
		if err != nil {
			continue
		}
		forwardAfter, parseErr := parseProgressQuota(forward.MonthlyProgressAfter)
		if parseErr == nil && forwardAfter.Equal(target) {
			return candidate, forward, nil
		}
	}
	if rawScale > searchScale && rawCandidate.String() != lastCandidate {
		forward, err := calculateChargedProgress(
			snapshot,
			monthlyOriginalAfter,
			monthlyOriginalBefore,
			rawCandidate,
			originalQuota,
		)
		if err == nil {
			forwardAfter, parseErr := parseProgressQuota(forward.MonthlyProgressAfter)
			if parseErr == nil && forwardAfter.Equal(target) {
				return rawCandidate, forward, nil
			}
		}
	}
	return decimal.Zero, Calculation{}, ErrInvalidProgressQuota
}

func calculateChargedProgress(
	snapshot Snapshot,
	monthlyOriginalBefore int64,
	monthlyOriginalAfter int64,
	progressBefore decimal.Decimal,
	originalQuota int,
) (Calculation, error) {
	remainingOriginal := decimal.NewFromInt(int64(originalQuota))
	cursor := progressBefore
	segments := make([]Segment, 0, len(snapshot.Tiers))
	for remainingOriginal.IsPositive() {
		index := tierIndexAtCursor(snapshot.Tiers, cursor)
		tier := snapshot.Tiers[index]
		ratio := decimal.NewFromFloat(tier.Ratio)
		amount := remainingOriginal
		progressDelta := amount.Mul(ratio)
		if index+1 < len(snapshot.Tiers) {
			nextThreshold := decimal.NewFromInt(snapshot.Tiers[index+1].MinMonthlyOriginalQuota)
			availableProgress := nextThreshold.Sub(cursor)
			if availableProgress.IsPositive() && progressDelta.GreaterThan(availableProgress) {
				// Boundary conversion is deliberately rounded once to a fixed
				// decimal scale. Decrease verifies its inverse by replaying this
				// same forward contract.
				amount = availableProgress.DivRound(ratio, chargedProgressBoundaryDivisionPrecision)
				progressDelta = availableProgress
			}
		}
		after := cursor.Add(progressDelta)
		segments = append(segments, exactProgressSegment(tier, amount, cursor, after, progressDelta))
		cursor = after
		remainingOriginal = remainingOriginal.Sub(amount)
	}
	if cursor.GreaterThan(decimal.NewFromInt(math.MaxInt64)) {
		return Calculation{}, ErrMonthlyQuotaOverflow
	}
	chargedQuota, clamp := common.QuotaFromDecimalChecked(cursor.Round(0).Sub(progressBefore.Round(0)))
	if clamp != nil || chargedQuota < 0 {
		return Calculation{}, fmt.Errorf("%w: %v", ErrChargeOverflow, clamp)
	}
	return Calculation{
		MonthlyOriginalBefore: monthlyOriginalBefore,
		MonthlyOriginalAfter:  monthlyOriginalAfter,
		MonthlyProgressBefore: progressBefore.String(),
		MonthlyProgressAfter:  cursor.String(),
		OriginalQuota:         originalQuota,
		ChargedQuota:          chargedQuota,
		ProgressQuota:         cursor.Sub(progressBefore).String(),
		Segments:              segments,
	}, nil
}

func exactProgressSegment(tier Tier, original, before, after, progressDelta decimal.Decimal) Segment {
	segment := Segment{
		TierMin:            tier.MinMonthlyOriginalQuota,
		Ratio:              tier.Ratio,
		TierMinProgress:    decimal.NewFromInt(tier.MinMonthlyOriginalQuota).String(),
		OriginalQuotaExact: original.String(),
		ProgressBefore:     before.String(),
		ProgressAfter:      after.String(),
		ProgressQuota:      progressDelta.String(),
	}
	integer := original.IntPart()
	if original.Equal(decimal.NewFromInt(integer)) {
		segment.OriginalQuota = integer
	}
	return segment
}

func tierIndexAtCursor(tiers []Tier, cursor decimal.Decimal) int {
	index := 0
	for candidate := 1; candidate < len(tiers); candidate++ {
		if decimal.NewFromInt(tiers[candidate].MinMonthlyOriginalQuota).GreaterThan(cursor) {
			break
		}
		index = candidate
	}
	return index
}

func tierIndexBeforeCursor(tiers []Tier, cursor decimal.Decimal) int {
	index := -1
	for candidate := range tiers {
		if !decimal.NewFromInt(tiers[candidate].MinMonthlyOriginalQuota).LessThan(cursor) {
			break
		}
		index = candidate
	}
	return index
}

func parseProgressQuota(raw string) (decimal.Decimal, error) {
	progress, err := decimal.NewFromString(raw)
	if err != nil || progress.IsNegative() || progress.String() != raw || progress.GreaterThan(decimal.NewFromInt(math.MaxInt64)) {
		return decimal.Zero, ErrInvalidProgressQuota
	}
	return progress, nil
}

func ParseProgressQuota(raw string) (decimal.Decimal, error) {
	return parseProgressQuota(raw)
}

func ParseProgressDelta(raw string) (decimal.Decimal, error) {
	progress, err := decimal.NewFromString(raw)
	if err != nil || progress.String() != raw || progress.Abs().GreaterThan(decimal.NewFromInt(math.MaxInt64)) {
		return decimal.Zero, ErrInvalidProgressQuota
	}
	return progress, nil
}

func validateSnapshot(snapshot Snapshot, basis ProgressBasis) error {
	if snapshot.PeriodStart >= snapshot.PeriodEnd {
		return errors.New("snapshot period must have a positive duration")
	}
	if basis != ProgressBasisOriginal && basis != ProgressBasisCharged {
		return errors.New("snapshot progress_basis must be original or charged")
	}
	if err := validateTiers(snapshot.Tiers); err != nil {
		return fmt.Errorf("invalid snapshot tiers: %w", err)
	}
	if basis == ProgressBasisCharged {
		for index, tier := range snapshot.Tiers {
			if tier.Ratio == 0 {
				return fmt.Errorf("invalid snapshot tiers: tier %d ratio must be positive when progress_basis is charged", index)
			}
		}
	}
	return nil
}

func cumulativeDiscountedQuota(tiers []Tier, originalQuota int64) decimal.Decimal {
	total := decimal.Zero
	for index, tier := range tiers {
		if originalQuota <= tier.MinMonthlyOriginalQuota {
			break
		}
		segmentEnd := originalQuota
		if index+1 < len(tiers) && tiers[index+1].MinMonthlyOriginalQuota < segmentEnd {
			segmentEnd = tiers[index+1].MinMonthlyOriginalQuota
		}
		amount := segmentEnd - tier.MinMonthlyOriginalQuota
		if amount > 0 {
			total = total.Add(decimal.NewFromInt(amount).Mul(decimal.NewFromFloat(tier.Ratio)))
		}
	}
	return total
}

func validateTiers(tiers []Tier) error {
	if len(tiers) == 0 {
		return errors.New("at least one tier is required")
	}
	if tiers[0].MinMonthlyOriginalQuota != 0 {
		return errors.New("the first tier threshold must be zero")
	}
	for index, tier := range tiers {
		if tier.MinMonthlyOriginalQuota < 0 {
			return fmt.Errorf("tier %d threshold must be non-negative", index)
		}
		if math.IsNaN(tier.Ratio) || math.IsInf(tier.Ratio, 0) || tier.Ratio < 0 || tier.Ratio > 1 {
			return fmt.Errorf("tier %d ratio must be finite and between 0 and 1", index)
		}
		if index == 0 {
			continue
		}
		if tier.MinMonthlyOriginalQuota <= tiers[index-1].MinMonthlyOriginalQuota {
			return fmt.Errorf("tier %d threshold must be strictly increasing", index)
		}
		if tier.Ratio > tiers[index-1].Ratio {
			return fmt.Errorf("tier %d ratio must not increase", index)
		}
	}
	return nil
}

func clonePolicyMap(source PolicyMap) PolicyMap {
	cloned := make(PolicyMap, len(source))
	for group, groupPolicy := range source {
		models := make(map[string]Policy, len(groupPolicy.Models))
		for model, policy := range groupPolicy.Models {
			models[model] = clonePolicy(policy)
		}
		cloned[group] = GroupPolicy{
			ProgressBasis: normalizeProgressBasis(groupPolicy.ProgressBasis),
			Models:        models,
		}
	}
	return cloned
}

func normalizeProgressBasis(basis ProgressBasis) ProgressBasis {
	if basis == "" {
		return ProgressBasisOriginal
	}
	return basis
}

func NormalizeProgressBasis(basis ProgressBasis) ProgressBasis {
	return normalizeProgressBasis(basis)
}

func clonePolicy(policy Policy) Policy {
	policy.Tiers = append([]Tier(nil), policy.Tiers...)
	if policy.EffectiveUntil != nil {
		until := *policy.EffectiveUntil
		policy.EffectiveUntil = &until
	}
	return policy
}

func resolvePolicy(models map[string]Policy, originModel string) (Policy, string, bool) {
	if policy, exists := models[originModel]; exists {
		return clonePolicy(policy), originModel, true
	}
	policy, exists := models["*"]
	if !exists {
		return Policy{}, "", false
	}
	return clonePolicy(policy), "*", true
}
