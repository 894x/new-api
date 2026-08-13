package xunfei_maas

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/types"
)

var rateLimitErrorCodes = map[types.ErrorCode]struct{}{
	"10006": {},
	"10007": {},
	"11201": {},
	"11202": {},
	"11203": {},
	"11210": {},
}

var transientErrorCodes = map[types.ErrorCode]struct{}{
	"10000": {},
	"10001": {},
	"10002": {},
	"10008": {},
	"10009": {},
	"10010": {},
	"10011": {},
	"10110": {},
	"10222": {},
	"10223": {},
}

var badRequestErrorCodes = map[types.ErrorCode]struct{}{
	"10003": {},
	"10004": {},
	"10005": {},
	"10013": {},
	"10014": {},
	"10019": {},
	"10163": {},
	"10404": {},
	"10907": {},
	"10910": {},
}

var authorizationErrorCodes = map[types.ErrorCode]struct{}{
	"10015": {},
	"10016": {},
	"11200": {},
	"11221": {},
}

func classifyError(err *types.NewAPIError) *types.NewAPIError {
	if err == nil {
		return nil
	}

	code := err.GetErrorCode()
	openAIError := err.ToOpenAIError()
	var options []types.NewAPIErrorOptions
	if types.IsResponseCommittedError(err) {
		options = append(options, types.ErrOptionWithResponseCommitted())
	}
	if err.StatusCode == http.StatusUnauthorized || err.StatusCode == http.StatusForbidden {
		options = append(options, types.ErrOptionWithSkipRetry())
		return types.WithOpenAIError(openAIError, err.StatusCode, options...)
	}
	if _, ok := rateLimitErrorCodes[code]; ok {
		return types.WithOpenAIError(openAIError, http.StatusTooManyRequests, options...)
	}
	if code == "10012" {
		if isContextLengthError(openAIError.Message) {
			options = append(options, types.ErrOptionWithSkipRetry())
			return types.WithOpenAIError(openAIError, http.StatusBadRequest, options...)
		}
		return types.WithOpenAIError(openAIError, http.StatusServiceUnavailable, options...)
	}
	if _, ok := transientErrorCodes[code]; ok {
		return types.WithOpenAIError(openAIError, http.StatusServiceUnavailable, options...)
	}
	if _, ok := badRequestErrorCodes[code]; ok {
		options = append(options, types.ErrOptionWithSkipRetry())
		return types.WithOpenAIError(openAIError, http.StatusBadRequest, options...)
	}
	if _, ok := authorizationErrorCodes[code]; ok {
		options = append(options, types.ErrOptionWithSkipRetry())
		return types.WithOpenAIError(openAIError, http.StatusForbidden, options...)
	}
	if err.StatusCode == http.StatusOK {
		return types.WithOpenAIError(openAIError, http.StatusBadGateway, options...)
	}
	return err
}

func isContextLengthError(message string) bool {
	message = strings.ToLower(strings.TrimSpace(message))
	patterns := []string{
		"上下文超长",
		"上下文过长",
		"上下文长度超过",
		"context_length_exceeded",
		"context length exceeded",
		"maximum context length",
		"context window exceeded",
		"prompt is too long",
	}
	for _, pattern := range patterns {
		if strings.Contains(message, pattern) {
			return true
		}
	}
	return false
}
