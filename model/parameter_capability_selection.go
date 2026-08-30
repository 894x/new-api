package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayparam"
)

var ErrParameterCapabilityUnsupported = errors.New("request parameters are not supported by any eligible channel")

type ChannelSelectionFilters struct {
	RequestPath       string
	VideoResolution   string
	RequestBody       []byte
	AllowedChannelIds map[int]struct{}
}

type parameterCapabilityUnsupportedError struct {
	model string
	cause error
}

func (e *parameterCapabilityUnsupportedError) Error() string {
	if e == nil || e.cause == nil {
		return ErrParameterCapabilityUnsupported.Error()
	}
	return fmt.Sprintf("model %s has no compatible channel for the request parameters: %v", e.model, e.cause)
}

func (e *parameterCapabilityUnsupportedError) Unwrap() []error {
	if e == nil || e.cause == nil {
		return []error{ErrParameterCapabilityUnsupported}
	}
	return []error{ErrParameterCapabilityUnsupported, e.cause}
}

func (channel *Channel) SupportsSelectionParameters(requestModel string, requestBody []byte) (bool, error) {
	if channel == nil || len(requestBody) == 0 {
		return true, nil
	}
	config := channel.GetOtherSettings().ParameterCapabilities
	return supportsSelectionParameters(channel, config, requestModel, requestBody)
}

func supportsSelectionParameters(channel *Channel, config *dto.ParameterCapabilityConfig, requestModel string, requestBody []byte) (bool, error) {
	if channel == nil || config == nil || !config.HasSelectionConstraints() || len(requestBody) == 0 {
		return true, nil
	}
	upstreamModel, _, err := channel.ResolveUpstreamModelName(requestModel)
	if err != nil {
		return false, err
	}
	err = relayparam.CheckSelectionCapabilities(requestBody, config, upstreamModel)
	if err == nil {
		return true, nil
	}
	var violation *relayparam.CapabilityViolationError
	if errors.As(err, &violation) {
		return false, violation
	}
	return false, err
}

func newParameterCapabilityUnsupportedError(model string, cause error) error {
	return &parameterCapabilityUnsupportedError{model: model, cause: cause}
}
