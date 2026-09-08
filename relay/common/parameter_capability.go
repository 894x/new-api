package common

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/relaykit/relayparam"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
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
func ApplyRequestPoliciesWithRelayInfo(jsonData []byte, info *RelayInfo, transformers ...relayparam.MediaTransformer) ([]byte, error) {
	if info != nil {
		info.ParameterCapabilityAudit = nil
	}
	if err := CheckMediaTransformPassThrough(info); err != nil {
		return nil, err
	}
	if info != nil && len(info.ParamOverride) == 0 {
		info.ParamOverrideAudit = nil
	}
	result, err := ApplyParamOverrideWithRelayInfo(jsonData, info)
	if err != nil {
		return nil, err
	}
	var changes []relayparam.CapabilityChange
	if info != nil && info.ChannelMeta != nil {
		model := info.UpstreamModelName
		if model == "" {
			model = info.OriginModelName
		}
		var transform relayparam.MediaTransformer
		if len(transformers) > 0 {
			transform = transformers[0]
		}
		result, changes, err = relayparam.ApplyMediaTransforms(result, info.ChannelOtherSettings.ParameterCapabilities, model, transform)
		if err != nil {
			info.ParameterCapabilityAudit = changes
			return nil, err
		}
	}
	result, err = ApplyParameterCapabilitiesWithRelayInfo(result, info)
	if info != nil {
		info.ParameterCapabilityAudit = append(changes, info.ParameterCapabilityAudit...)
	}
	return result, err
}

// CheckMediaTransformPassThrough prevents a configured conversion from silently
// disappearing when the original body is replayed instead of applying policies.
func CheckMediaTransformPassThrough(info *RelayInfo) error {
	if info == nil || info.ChannelMeta == nil {
		return nil
	}
	if !model_setting.GetGlobalSettings().PassThroughRequestEnabled && !info.ChannelSetting.PassThroughBodyEnabled {
		return nil
	}
	model := info.UpstreamModelName
	if model == "" {
		model = info.OriginModelName
	}
	if info.ChannelOtherSettings.ParameterCapabilities.HasMediaTransforms(model) {
		return &relayparam.CapabilityViolationError{Model: model, Parameter: "parameter_capabilities", Reason: "media transforms require request body pass-through to be disabled"}
	}
	return nil
}

// HasMediaTransforms also controls body logging: never dump downloaded media.
func (info *RelayInfo) HasMediaTransforms() bool {
	if info == nil || info.ChannelMeta == nil {
		return false
	}
	model := info.UpstreamModelName
	if model == "" {
		model = info.OriginModelName
	}
	return info.ChannelOtherSettings.ParameterCapabilities.HasMediaTransforms(model)
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
