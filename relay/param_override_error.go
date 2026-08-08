package relay

import (
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
)

func newAPIErrorFromParamOverride(err error) *types.NewAPIError {
	if fixedErr, ok := relaycommon.AsParamOverrideReturnError(err); ok {
		return relaycommon.NewAPIErrorFromParamOverride(fixedErr)
	}
	return types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid, types.ErrOptionWithSkipRetry())
}

func newAPIErrorFromRequestPolicy(err error) *types.NewAPIError {
	if capabilityErr, ok := relaycommon.AsParameterCapabilityViolation(err); ok {
		return relaycommon.NewAPIErrorFromParameterCapability(capabilityErr)
	}
	return newAPIErrorFromParamOverride(err)
}
