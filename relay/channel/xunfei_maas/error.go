package xunfei_maas

import (
	"net/http"

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
	"10012": {},
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
	if _, ok := rateLimitErrorCodes[code]; ok {
		return types.WithOpenAIError(openAIError, http.StatusTooManyRequests, options...)
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
