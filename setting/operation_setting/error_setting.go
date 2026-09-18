package operation_setting

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"golang.org/x/net/http/httpguts"
)

const (
	MaxBlockedResponseHeaderCount         = 32
	MaxBlockedResponseHeaderNameLength    = 128
	MaxErrorResponseReplacementRuleCount  = 32
	MaxErrorResponseReplacementTextLength = 4096
)

type ErrorResponseReplacementRule struct {
	StatusCode  int    `json:"status_code"`
	Match       string `json:"match"`
	Replacement string `json:"replacement"`
}

type ErrorSetting struct {
	HideErrorDetails         bool                           `json:"hide_error_details"`
	BlockedResponseHeaders   []string                       `json:"blocked_response_headers"`
	ResponseReplacementRules []ErrorResponseReplacementRule `json:"response_replacement_rules"`
}

var errorSetting = ErrorSetting{
	HideErrorDetails: true,
	BlockedResponseHeaders: []string{
		"X-Modelverse-Request-Id",
		"X-Request-Id",
		"X-Trace-Id",
	},
	ResponseReplacementRules: []ErrorResponseReplacementRule{
		{StatusCode: http.StatusInternalServerError, Match: "Moonshot AI", Replacement: "rhzs"},
		{StatusCode: http.StatusTooManyRequests, Match: "Tencent Cloud", Replacement: "rhzs"},
	},
}

type blockedResponseHeaderIndex struct {
	headers []string
	names   map[string]struct{}
}

type errorResponseReplacementRuleIndex struct {
	rules []ErrorResponseReplacementRule
}

var currentBlockedResponseHeaders atomic.Pointer[blockedResponseHeaderIndex]
var currentErrorResponseReplacementRules atomic.Pointer[errorResponseReplacementRuleIndex]
var currentHideErrorDetails atomic.Bool

func init() {
	config.GlobalConfig.Register("error_setting", &errorSetting)
	if err := errorSetting.AfterConfigUpdate(); err != nil {
		panic(err)
	}
}

func GetErrorSetting() *ErrorSetting {
	snapshot := &ErrorSetting{HideErrorDetails: currentHideErrorDetails.Load()}
	if index := currentBlockedResponseHeaders.Load(); index != nil {
		snapshot.BlockedResponseHeaders = append([]string(nil), index.headers...)
	}
	if index := currentErrorResponseReplacementRules.Load(); index != nil {
		snapshot.ResponseReplacementRules = append([]ErrorResponseReplacementRule(nil), index.rules...)
	}
	return snapshot
}

func ShouldHideErrorDetails() bool {
	return currentHideErrorDetails.Load()
}

func ShouldBlockUpstreamResponseHeader(header string) bool {
	index := currentBlockedResponseHeaders.Load()
	if index == nil {
		return false
	}
	_, blocked := index.names[strings.ToLower(strings.TrimSpace(header))]
	return blocked
}

func MatchErrorResponseReplacement(statusCode int, message string) (string, bool) {
	index := currentErrorResponseReplacementRules.Load()
	if index == nil || message == "" {
		return "", false
	}
	replaced := message
	matched := false
	for _, rule := range index.rules {
		if rule.StatusCode == statusCode && strings.Contains(replaced, rule.Match) {
			replaced = strings.ReplaceAll(replaced, rule.Match, rule.Replacement)
			matched = true
		}
	}
	return replaced, matched
}

func ValidateBlockedResponseHeadersJSON(value string) ([]string, error) {
	var headers []string
	if err := common.Unmarshal([]byte(value), &headers); err != nil {
		return nil, fmt.Errorf("invalid blocked response headers: %w", err)
	}
	if headers == nil {
		return nil, fmt.Errorf("blocked response headers must be a JSON array")
	}
	return normalizeBlockedResponseHeaders(headers)
}

func UpdateBlockedResponseHeadersFromJSON(value string) error {
	headers, err := ValidateBlockedResponseHeadersJSON(value)
	if err != nil {
		return err
	}
	return UpdateBlockedResponseHeaders(headers)
}

func UpdateBlockedResponseHeaders(headers []string) error {
	normalized, err := normalizeBlockedResponseHeaders(headers)
	if err != nil {
		return err
	}

	names := make(map[string]struct{}, len(normalized))
	for _, header := range normalized {
		names[strings.ToLower(header)] = struct{}{}
	}
	errorSetting.BlockedResponseHeaders = append([]string(nil), normalized...)
	currentBlockedResponseHeaders.Store(&blockedResponseHeaderIndex{
		headers: append([]string(nil), normalized...),
		names:   names,
	})
	return nil
}

func ValidateErrorResponseReplacementRulesJSON(value string) ([]ErrorResponseReplacementRule, error) {
	var rules []ErrorResponseReplacementRule
	if err := common.Unmarshal([]byte(value), &rules); err != nil {
		return nil, fmt.Errorf("invalid error response replacement rules: %w", err)
	}
	if rules == nil {
		return nil, fmt.Errorf("error response replacement rules must be a JSON array")
	}
	return normalizeErrorResponseReplacementRules(rules)
}

func UpdateErrorResponseReplacementRulesFromJSON(value string) error {
	rules, err := ValidateErrorResponseReplacementRulesJSON(value)
	if err != nil {
		return err
	}
	return UpdateErrorResponseReplacementRules(rules)
}

func UpdateErrorResponseReplacementRules(rules []ErrorResponseReplacementRule) error {
	normalized, err := normalizeErrorResponseReplacementRules(rules)
	if err != nil {
		return err
	}
	errorSetting.ResponseReplacementRules = append([]ErrorResponseReplacementRule(nil), normalized...)
	currentErrorResponseReplacementRules.Store(&errorResponseReplacementRuleIndex{
		rules: append([]ErrorResponseReplacementRule(nil), normalized...),
	})
	return nil
}

func UpdateHideErrorDetails(hide bool) {
	errorSetting.HideErrorDetails = hide
	currentHideErrorDetails.Store(hide)
}

func (setting *ErrorSetting) AfterConfigUpdate() error {
	previous := GetErrorSetting()
	normalizedHeaders, err := normalizeBlockedResponseHeaders(setting.BlockedResponseHeaders)
	if err != nil {
		*setting = *previous
		return err
	}
	normalizedRules, err := normalizeErrorResponseReplacementRules(setting.ResponseReplacementRules)
	if err != nil {
		*setting = *previous
		return err
	}
	UpdateHideErrorDetails(setting.HideErrorDetails)
	if err := UpdateBlockedResponseHeaders(normalizedHeaders); err != nil {
		*setting = *previous
		return err
	}
	if err := UpdateErrorResponseReplacementRules(normalizedRules); err != nil {
		UpdateHideErrorDetails(previous.HideErrorDetails)
		_ = UpdateBlockedResponseHeaders(previous.BlockedResponseHeaders)
		*setting = *previous
		return err
	}
	return nil
}

func normalizeBlockedResponseHeaders(headers []string) ([]string, error) {
	if len(headers) > MaxBlockedResponseHeaderCount {
		return nil, fmt.Errorf("blocked response headers cannot exceed %d entries", MaxBlockedResponseHeaderCount)
	}

	normalized := make([]string, 0, len(headers))
	seen := make(map[string]struct{}, len(headers))
	for _, header := range headers {
		header = strings.TrimSpace(header)
		if header == "" {
			return nil, fmt.Errorf("blocked response header names cannot be empty")
		}
		if len(header) > MaxBlockedResponseHeaderNameLength {
			return nil, fmt.Errorf("blocked response header name cannot exceed %d bytes", MaxBlockedResponseHeaderNameLength)
		}
		if !httpguts.ValidHeaderFieldName(header) {
			return nil, fmt.Errorf("invalid blocked response header name %q", header)
		}
		canonicalHeader := http.CanonicalHeaderKey(header)
		lookupKey := strings.ToLower(canonicalHeader)
		if _, ok := seen[lookupKey]; ok {
			continue
		}
		seen[lookupKey] = struct{}{}
		normalized = append(normalized, canonicalHeader)
	}
	return normalized, nil
}

func normalizeErrorResponseReplacementRules(rules []ErrorResponseReplacementRule) ([]ErrorResponseReplacementRule, error) {
	if len(rules) > MaxErrorResponseReplacementRuleCount {
		return nil, fmt.Errorf("error response replacement rules cannot exceed %d entries", MaxErrorResponseReplacementRuleCount)
	}

	normalized := make([]ErrorResponseReplacementRule, 0, len(rules))
	for index, rule := range rules {
		if rule.StatusCode < http.StatusBadRequest || rule.StatusCode > 599 {
			return nil, fmt.Errorf("error response replacement rule %d status_code must be between 400 and 599", index+1)
		}
		rule.Match = strings.TrimSpace(rule.Match)
		rule.Replacement = strings.TrimSpace(rule.Replacement)
		if rule.Match == "" {
			return nil, fmt.Errorf("error response replacement rule %d match cannot be empty", index+1)
		}
		if len(rule.Match) > MaxErrorResponseReplacementTextLength {
			return nil, fmt.Errorf("error response replacement rule %d match cannot exceed %d bytes", index+1, MaxErrorResponseReplacementTextLength)
		}
		if len(rule.Replacement) > MaxErrorResponseReplacementTextLength {
			return nil, fmt.Errorf("error response replacement rule %d replacement cannot exceed %d bytes", index+1, MaxErrorResponseReplacementTextLength)
		}
		normalized = append(normalized, rule)
	}
	return normalized, nil
}
