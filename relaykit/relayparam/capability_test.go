package relayparam

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyCapabilitiesPreservesExplicitZeroAndClampsOutOfRangeValue(t *testing.T) {
	supported := true
	min := 0.0
	max := 1.0
	config := &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{
		"temperature": {
			Supported:   &supported,
			Min:         &min,
			Max:         &max,
			OnViolation: dto.ParameterCapabilityActionClamp,
		},
	}}

	zero, changes, err := ApplyCapabilities([]byte(`{"temperature":0}`), config, "model-a")
	require.NoError(t, err)
	assert.JSONEq(t, `{"temperature":0}`, string(zero))
	assert.Empty(t, changes)

	clamped, changes, err := ApplyCapabilities([]byte(`{"temperature":1.5}`), config, "model-a")
	require.NoError(t, err)
	assert.JSONEq(t, `{"temperature":1}`, string(clamped))
	require.Len(t, changes, 1)
	assert.Equal(t, dto.ParameterCapabilityActionClamp, changes[0].Action)
}

func TestApplyCapabilitiesRejectsOrDropsUnsupportedParameter(t *testing.T) {
	disabled := false
	rejectConfig := &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{
		"top_k": {Supported: &disabled},
	}}

	_, _, err := ApplyCapabilities([]byte(`{"top_k":10}`), rejectConfig, "model-a")
	var violation *CapabilityViolationError
	require.ErrorAs(t, err, &violation)
	assert.Equal(t, "top_k", violation.Parameter)

	dropConfig := &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{
		"top_k": {Supported: &disabled, OnViolation: dto.ParameterCapabilityActionDrop},
	}}
	dropped, changes, err := ApplyCapabilities([]byte(`{"model":"model-a","top_k":10}`), dropConfig, "model-a")
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"model-a"}`, string(dropped))
	require.Len(t, changes, 1)
	assert.Equal(t, "top_k", changes[0].Parameter)
}

func TestApplyCapabilitiesLeavesMissingParameterAbsent(t *testing.T) {
	min := 0.0
	config := &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{
		"temperature": {Min: &min},
	}}

	result, changes, err := ApplyCapabilities([]byte(`{"model":"model-a"}`), config, "model-a")
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"model-a"}`, string(result))
	assert.Empty(t, changes)
}

func TestApplyCapabilitiesCanDropNonNumericValueForNumericConstraint(t *testing.T) {
	max := 1.0
	config := &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{
		"temperature": {Max: &max, OnViolation: dto.ParameterCapabilityActionDrop},
	}}

	result, changes, err := ApplyCapabilities([]byte(`{"model":"model-a","temperature":"high"}`), config, "model-a")
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"model-a"}`, string(result))
	require.Len(t, changes, 1)
	assert.Equal(t, dto.ParameterCapabilityActionDrop, changes[0].Action)
}
