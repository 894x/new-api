package relayparam

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParameterPresenceConstraints(t *testing.T) {
	for _, tc := range []struct {
		name, path, constraints, payload, wantPath, wantReason string
	}{
		{"missing required field", "value", `"required":true`, `{}`, "value", "parameter is required"},
		{"optional missing field", "value", `"non_empty":true`, `{}`, "", ""},
		{"required null is present", "value", `"required":true`, `{"value":null}`, "", ""},
		{"null is empty", "value", `"non_empty":true`, `{"value":null}`, "value", "value must not be empty"},
		{"blank string is empty", "value", `"non_empty":true`, `{"value":" \t\u0085"}`, "value", "value must not be empty"},
		{"empty object", "value", `"non_empty":true`, `{"value":{}}`, "value", "value must not be empty"},
		{"empty array", "value", `"non_empty":true`, `{"value":[]}`, "value", "value must not be empty"},
		{"zero is not empty", "value", `"non_empty":true`, `{"value":0}`, "", ""},
		{"false is not empty", "value", `"non_empty":true`, `{"value":false}`, "", ""},
		{"nonempty object", "value", `"non_empty":true`, `{"value":{"url":"video"}}`, "", ""},
		{"missing video URL", "messages.*.content.*.video_url.url", `"required":true,"non_empty":true`, `{"messages":[{"content":[{"type":"text","text":"hello"},{"type":"video_url","video_url":{}}]}]}`, "messages.0.content.1.video_url.url", "parameter is required"},
		{"blank video URL", "messages.*.content.*.video_url.url", `"required":true,"non_empty":true`, `{"messages":[{"content":[{"video_url":{"url":" "}}]}]}`, "messages.0.content.0.video_url.url", "value must not be empty"},
		{"text request has no video parent", "messages.*.content.*.video_url.url", `"required":true,"non_empty":true`, `{"messages":[{"content":[{"type":"text","text":"hello"}]}]}`, "", ""},
		{"empty wildcard collection", "messages.*.content.*.video_url.url", `"required":true`, `{"messages":[]}`, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var config dto.ParameterCapabilityConfig
			require.NoError(t, kitutil.Unmarshal([]byte(`{"defaults":{"`+tc.path+`":{`+tc.constraints+`,"participate_in_selection":true}}}`), &config))
			require.NoError(t, config.Validate())
			payload := []byte(tc.payload)
			selectionErr := CheckSelectionCapabilities(payload, &config, "model-a")
			result, _, forwardingErr := ApplyCapabilities(payload, &config, "model-a")
			assert.Equal(t, tc.payload, string(payload), "selection must not mutate the request")
			for _, err := range []error{selectionErr, forwardingErr} {
				if tc.wantReason == "" {
					require.NoError(t, err)
					continue
				}
				var violation *CapabilityViolationError
				require.ErrorAs(t, err, &violation)
				assert.Equal(t, tc.wantPath, violation.Parameter)
				assert.Equal(t, tc.wantReason, violation.Reason)
			}
			if tc.wantReason == "" {
				assert.JSONEq(t, tc.payload, string(result))
			}
		})
	}
}

func TestParameterPresenceOverridesAndSelectionOptOut(t *testing.T) {
	var config dto.ParameterCapabilityConfig
	require.NoError(t, kitutil.Unmarshal([]byte(`{"defaults":{"value":{"required":true,"non_empty":true,"participate_in_selection":false}},"rules":[{"selector":{"type":"exact","value":"optional"},"parameters":{"value":{"required":false,"non_empty":false}}}]}`), &config))
	require.NoError(t, config.Validate())
	require.NoError(t, CheckSelectionCapabilities([]byte(`{}`), &config, "model-a"))
	_, _, err := ApplyCapabilities([]byte(`{}`), &config, "model-a")
	require.ErrorContains(t, err, "parameter is required")
	result, _, err := ApplyCapabilities([]byte(`{}`), &config, "optional")
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(result))
	result, _, err = ApplyCapabilities([]byte(`{"value":null}`), &config, "optional")
	require.NoError(t, err)
	assert.JSONEq(t, `{"value":null}`, string(result))
}

func TestParameterPresenceRejectsIncompatibleConfiguration(t *testing.T) {
	for _, configJSON := range []string{
		`{"defaults":{"value":{"required":true,"on_violation":"drop"}}}`,
		`{"defaults":{"value":{"non_empty":true,"on_violation":"clamp","min":0}}}`,
		`{"defaults":{"$request.body_size_bytes":{"required":true}}}`,
	} {
		var config dto.ParameterCapabilityConfig
		require.NoError(t, kitutil.Unmarshal([]byte(configJSON), &config))
		require.Error(t, config.Validate(), configJSON)
	}
}

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

func TestRequestBodySizeCapabilityFiltersSelectionWithoutMutatingPayload(t *testing.T) {
	max := 1024.0
	participates := true
	config := &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{
		dto.ParameterCapabilityRequestBodySizeBytes: {
			Max:                    &max,
			ParticipateInSelection: &participates,
		},
		"tools.*": {ParticipateInSelection: &participates},
	}}
	payload := []byte(`{"model":"model-a"}`)
	maxBoundarySize := int64(1024)
	oversizedBody := int64(1025)

	require.NoError(t, CheckSelectionCapabilitiesWithBodySize(nil, &maxBoundarySize, config, "model-a"))
	require.NoError(t, CheckSelectionCapabilitiesWithBodySize(nil, nil, config, "model-a"))
	err := CheckSelectionCapabilitiesWithBodySize(nil, &oversizedBody, config, "model-a")
	var violation *CapabilityViolationError
	require.ErrorAs(t, err, &violation)
	assert.Equal(t, dto.ParameterCapabilityRequestBodySizeBytes, violation.Parameter)
	assert.Equal(t, "1025", violation.Value)

	result, changes, err := ApplyCapabilities(payload, config, "model-a")
	require.NoError(t, err)
	assert.Equal(t, payload, result)
	assert.Empty(t, changes)
}
