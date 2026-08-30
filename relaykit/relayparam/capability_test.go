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

func TestApplyCapabilitiesRejectsUnsupportedValueMatchedThroughArrayWildcards(t *testing.T) {
	disabled := false
	config := &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{
		"messages.*.content.*.image_url": {
			Supported: &disabled,
		},
	}}
	request := []byte(`{
		"messages": [
			{"role":"system","content":"describe the image"},
			{"role":"user","content":[
				{"type":"text","text":"what is this?"},
				{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="}}
			]}
		]
	}`)

	_, _, err := ApplyCapabilities(request, config, "vision-disabled-model")
	var violation *CapabilityViolationError
	require.ErrorAs(t, err, &violation)
	assert.Equal(t, "messages.1.content.1.image_url", violation.Parameter)
	assert.Equal(t, "parameter is not supported", violation.Reason)
}

func TestCheckSelectionCapabilitiesRejectsParticipatingWildcardConstraintWithoutMutatingRequest(t *testing.T) {
	disabled := false
	participates := true
	config := &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{
		"messages.*.content.*.image_url": {
			Supported:              &disabled,
			OnViolation:            dto.ParameterCapabilityActionDrop,
			ParticipateInSelection: &participates,
		},
	}}
	request := []byte(`{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="}}]}]}`)
	original := append([]byte(nil), request...)

	err := CheckSelectionCapabilities(request, config, "vision-disabled-model")
	var violation *CapabilityViolationError
	require.ErrorAs(t, err, &violation)
	assert.Equal(t, "messages.0.content.0.image_url", violation.Parameter)
	assert.Equal(t, original, request)
}

func TestCheckSelectionCapabilitiesIgnoresNonParticipatingAndMissingParameters(t *testing.T) {
	disabled := false
	participates := false
	config := &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{
		"messages.*.content.*.image_url": {
			Supported:              &disabled,
			ParticipateInSelection: &participates,
		},
		"tools": {Supported: &disabled},
	}}

	require.NoError(t, CheckSelectionCapabilities(
		[]byte(`{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]}]}`),
		config,
		"model-a",
	))

	participates = true
	require.NoError(t, CheckSelectionCapabilities([]byte(`{"messages":[{"role":"user","content":"hello"}]}`), config, "model-a"))
}

func TestApplyCapabilitiesDropsEveryArrayEntryMatchedByWildcard(t *testing.T) {
	disabled := false
	config := &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{
		"tools.*": {Supported: &disabled, OnViolation: dto.ParameterCapabilityActionDrop},
	}}

	result, changes, err := ApplyCapabilities(
		[]byte(`{"tools":[{"name":"a"},{"name":"b"},{"name":"c"}]}`),
		config,
		"model-a",
	)

	require.NoError(t, err)
	assert.JSONEq(t, `{"tools":[]}`, string(result))
	assert.Len(t, changes, 3)
}
