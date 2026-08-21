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
	MaxBlockedResponseHeaderCount      = 32
	MaxBlockedResponseHeaderNameLength = 128
)

type ErrorSetting struct {
	HideErrorDetails       bool     `json:"hide_error_details"`
	BlockedResponseHeaders []string `json:"blocked_response_headers"`
}

var errorSetting = ErrorSetting{
	HideErrorDetails: true,
	BlockedResponseHeaders: []string{
		"X-Modelverse-Request-Id",
		"X-Request-Id",
		"X-Trace-Id",
	},
}

type blockedResponseHeaderIndex struct {
	headers []string
	names   map[string]struct{}
}

var currentBlockedResponseHeaders atomic.Pointer[blockedResponseHeaderIndex]
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

func UpdateHideErrorDetails(hide bool) {
	errorSetting.HideErrorDetails = hide
	currentHideErrorDetails.Store(hide)
}

func (setting *ErrorSetting) AfterConfigUpdate() error {
	normalized, err := normalizeBlockedResponseHeaders(setting.BlockedResponseHeaders)
	if err != nil {
		setting.HideErrorDetails = currentHideErrorDetails.Load()
		if index := currentBlockedResponseHeaders.Load(); index != nil {
			setting.BlockedResponseHeaders = append([]string(nil), index.headers...)
		}
		return err
	}
	UpdateHideErrorDetails(setting.HideErrorDetails)
	return UpdateBlockedResponseHeaders(normalized)
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
