package common

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/relaykit/relayparam"
	"github.com/QuantumNous/new-api/relaykit/types"
)

func ApplyParameterCapabilitiesWithRelayInfo(jsonData []byte, info *RelayInfo) ([]byte, error) {
	if info == nil {
		return jsonData, nil
	}
	info.ParameterCapabilityAudit = nil
	if info.ChannelMeta == nil || info.ChannelOtherSettings.ParameterCapabilities == nil {
		return jsonData, nil
	}
	model := info.UpstreamModelName
	if model == "" {
		model = info.OriginModelName
	}
	result, changes, err := relayparam.ApplyCapabilities(
		jsonData,
		info.ChannelOtherSettings.ParameterCapabilities,
		model,
	)
	info.ParameterCapabilityAudit = changes
	return result, err
}

// ApplyRequestPoliciesWithRelayInfo applies administrator request rewrites
// first, then validates the final upstream body against model capabilities.
func ApplyRequestPoliciesWithRelayInfo(jsonData []byte, info *RelayInfo) ([]byte, error) {
	if info != nil && len(info.ParamOverride) == 0 {
		info.ParamOverrideAudit = nil
	}
	result, err := ApplyParamOverrideWithRelayInfo(jsonData, info)
	if err != nil {
		return nil, err
	}
	return ApplyParameterCapabilitiesWithRelayInfo(result, info)
}

func AsParameterCapabilityViolation(err error) (*relayparam.CapabilityViolationError, bool) {
	if err == nil {
		return nil, false
	}
	var target *relayparam.CapabilityViolationError
	ok := errors.As(err, &target)
	return target, ok
}

func NewAPIErrorFromParameterCapability(err *relayparam.CapabilityViolationError) *types.NewAPIError {
	if err == nil {
		return types.NewError(errors.New("parameter capability violation is nil"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	return types.WithOpenAIError(types.OpenAIError{
		Message: err.Error(),
		Type:    "invalid_request_error",
		Param:   err.Parameter,
		Code:    types.ErrorCodeInvalidRequest,
	}, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
}
