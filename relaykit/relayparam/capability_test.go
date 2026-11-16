package relayparam

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type parameterCapabilityValidationCase struct {
	name     string
	messages int
	body     []byte
}

func parameterCapabilityValidationCases() []parameterCapabilityValidationCase {
	var cases []parameterCapabilityValidationCase
	for _, shape := range []string{"text", "mixed", "nested_tools", "base64"} {
		counts := []int{1, 16, 64, 256}
		if shape == "base64" {
			counts = []int{1, 8, 32}
		}
		for _, count := range counts {
			messages := make([]any, 0, count)
			for i := range count {
				message := map[string]any{"role": "user", "content": fmt.Sprintf("message %d: ", i) + strings.Repeat("context ", 64)}
				switch shape {
				case "mixed":
					switch i % 4 {
					case 0:
						message["content"] = []any{
							map[string]any{"type": "text", "text": strings.Repeat("quoted \"text\" \n", 128)},
							map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://media.example.test/image.png"}},
							map[string]any{"type": "video_url", "video_url": map[string]any{"url": "https://media.example.test/video.mp4"}},
						}
					case 1:
						message["role"], message["content"] = "assistant", nil
						message["tool_calls"] = []any{map[string]any{"id": fmt.Sprintf("call-%d", i), "type": "function", "function": map[string]any{"name": "inspect", "arguments": `{"nested":{"value":0,"enabled":false,"items":[null,{},[]]}}`}}}
					case 2:
						message["role"], message["tool_call_id"] = "tool", fmt.Sprintf("call-%d", i-1)
					case 3:
						message["content"] = []any{map[string]any{"type": "text", "text": "unicode: 中文 / 日本語 / 😀"}}
					}
				case "base64":
					message["content"] = []any{
						map[string]any{"type": "text", "text": "inspect this image"},
						map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + strings.Repeat("AAAA", 16*1024)}},
					}
				}
				messages = append(messages, message)
			}
			payload := map[string]any{"model": "validation-model", "messages": messages, "temperature": 0, "stream": false}
			if shape == "nested_tools" {
				properties := make(map[string]any)
				for _, key := range []string{"normal", "a.b", "*", "#", "@this", "quote\"key", "back\\slash", "中文"} {
					properties[key] = map[string]any{"type": "object", "description": "outer", "properties": map[string]any{
						"nested.key": map[string]any{"type": "string", "description": strings.Repeat("schema description ", 64)},
						"optional":   map[string]any{"type": []string{"string", "null"}, "description": "optional input"},
					}}
				}
				payload["tools"] = []any{
					map[string]any{"type": "function", "function": map[string]any{"name": "inspect", "parameters": map[string]any{"type": "object", "properties": properties}}},
					map[string]any{"type": "function", "function": map[string]any{"name": "empty", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}}},
				}
				payload["metadata"] = map[string]any{"groups": []any{map[string]any{"a.b": map[string]any{"value": 0}, "#": map[string]any{"value": 3}}, map[string]any{}}}
			}
			body, err := kitutil.Marshal(payload)
			if err != nil {
				panic(err)
			}
			cases = append(cases, parameterCapabilityValidationCase{fmt.Sprintf("%s/messages_%d", shape, count), count, body})
		}
	}
	return cases
}

func parameterCapabilityValidationConfig() *dto.ParameterCapabilityConfig {
	participates, supported, nonempty := true, true, true
	zero, one, ten := 0.0, 1.0, 10.0
	return &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{
		"messages.*.content.*.image_url.url":                                {Supported: &supported, NonEmpty: &nonempty, ParticipateInSelection: &participates},
		"messages.*.content.*.video_url.url":                                {Supported: &supported, NonEmpty: &nonempty, ParticipateInSelection: &participates},
		"tools.*.function.parameters.properties.*.description":              {NonEmpty: &nonempty, ParticipateInSelection: &participates},
		"tools.*.function.parameters.properties.*.properties.*.description": {NonEmpty: &nonempty, ParticipateInSelection: &participates},
		"metadata.groups.*.*.value":                                         {Min: &zero, Max: &ten, ParticipateInSelection: &participates},
		"temperature":                                                       {Min: &zero, Max: &one, ParticipateInSelection: &participates},
		"stream":                                                            {AllowedValues: []string{"false", "true"}, ParticipateInSelection: &participates},
	}}
}

func TestParameterCapabilityComplexRequestMatrix(t *testing.T) {
	config := parameterCapabilityValidationConfig()
	require.NoError(t, config.Validate())
	for _, tc := range parameterCapabilityValidationCases() {
		t.Run(tc.name, func(t *testing.T) {
			original := append([]byte(nil), tc.body...)
			checker := NewSelectionCapabilityChecker(tc.body, nil)
			for _, model := range []string{"validation-model", "another-candidate", "retry-candidate"} {
				require.NoError(t, checker.Check(config, model))
			}
			result, changes, err := ApplyCapabilities(tc.body, config, "validation-model")
			require.NoError(t, err)
			assert.Empty(t, changes)
			assert.Equal(t, original, result, "a valid request must remain byte-for-byte unchanged")
			assert.Equal(t, original, tc.body, "selection must not mutate the original request")
			// A fresh candidate with an incompatible numeric bound must still be
			// rejected after the same body passed other candidates repeatedly.
			minimum, participates := 0.5, true
			reject := &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{
				"temperature": {Min: &minimum, ParticipateInSelection: &participates},
			}}
			var violation *CapabilityViolationError
			require.ErrorAs(t, checker.Check(reject, "incompatible"), &violation)
			assert.Equal(t, "temperature", violation.Parameter)
			assert.Equal(t, "value must be greater than or equal to 0.5", violation.Reason)
		})
	}
}

func BenchmarkParameterCapabilityRequestMatrix(b *testing.B) {
	config := parameterCapabilityValidationConfig()
	for _, tc := range parameterCapabilityValidationCases() {
		for _, candidates := range []int{1, 8} {
			b.Run(fmt.Sprintf("%s/candidates_%d", tc.name, candidates), func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(tc.body)))
				for b.Loop() {
					checker := NewSelectionCapabilityChecker(tc.body, nil)
					for range candidates {
						if err := checker.Check(config, "validation-model"); err != nil {
							b.Fatal(err)
						}
					}
				}
				b.ReportMetric(float64(len(tc.body)), "body-B")
			})
		}
	}
}

func TestSelectionCapabilityCheckerReevaluatesCandidateRules(t *testing.T) {
	participates, supported, required := true, true, false
	body := []byte(`{"messages":[{"content":[{"type":"image_url","image_url":{"url":"image"}},{"type":"text","text":"hello"}]}],"temperature":0,"stream":false}`)
	checker := NewSelectionCapabilityChecker(body, nil)
	config := &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{
		"messages.*.content.*.image_url": {Supported: &supported, ParticipateInSelection: &participates},
		"messages.*.content.*.missing":   {Required: &required, ParticipateInSelection: &participates},
	}}
	require.NoError(t, checker.Check(config, "vision"))
	supported = false
	var violation *CapabilityViolationError
	require.ErrorAs(t, checker.Check(config, "text-only"), &violation)
	assert.Equal(t, "messages.0.content.0.image_url", violation.Parameter)
	// Reusing path observations must never reuse a previous channel's decision.
	supported, required = true, true
	require.ErrorAs(t, checker.Check(config, "vision"), &violation)
	assert.Equal(t, "messages.0.content.0.missing", violation.Parameter)
	required = false
	require.NoError(t, checker.Check(config, "vision"))
	assert.JSONEq(t, `{"messages":[{"content":[{"type":"image_url","image_url":{"url":"image"}},{"type":"text","text":"hello"}]}],"temperature":0,"stream":false}`, string(body))

	other := NewSelectionCapabilityChecker([]byte(`{"messages":[{"content":"text only"}]}`), nil)
	supported = false
	require.NoError(t, other.Check(config, "text-only"), "another request must not inherit image observations")
}

func TestApplyCapabilitiesRefreshesObservationsAfterRewrites(t *testing.T) {
	disabled, participates := false, true
	limit := 1.0
	config := &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{
		"items.*.obsolete": {Supported: &disabled, OnViolation: dto.ParameterCapabilityActionDrop, ParticipateInSelection: &participates},
		"items.*.value":    {Max: &limit, OnViolation: dto.ParameterCapabilityActionClamp},
	}}
	result, changes, err := ApplyCapabilities([]byte(`{"items":[{"obsolete":true,"value":3},{"obsolete":false,"value":0}]}`), config, "model")
	require.NoError(t, err)
	assert.JSONEq(t, `{"items":[{"value":1},{"value":0}]}`, string(result))
	assert.Len(t, changes, 3)
	// Check the rewritten payload independently, including a previously dropped path.
	require.NoError(t, CheckSelectionCapabilities(result, config, "model"))
}

func TestSelectionCapabilityCheckerPreservesDuplicateObjectKeyLookups(t *testing.T) {
	disabled, participates := false, true
	config := &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{
		"items.*.image_url": {Supported: &disabled, ParticipateInSelection: &participates},
	}}
	for _, tc := range []struct {
		body          string
		wantViolation bool
	}{
		{`{"items":{"same":{"image_url":"image"},"same":{}}}`, true},
		{`{"items":{"same":{},"same":{"image_url":"image"}}}`, true},
		{`{"items":{"same":{},"same":{}}}`, false},
	} {
		err := NewSelectionCapabilityChecker([]byte(tc.body), nil).Check(config, "model")
		if tc.wantViolation {
			require.ErrorContains(t, err, "parameter is not supported")
		} else {
			require.NoError(t, err)
		}
	}
}

func TestSelectionCapabilityCheckerTracksExactRequestBodyAndSize(t *testing.T) {
	body := []byte(`{"messages":[]}`)
	size := int64(33)
	checker := NewSelectionCapabilityChecker(body, &size)
	assert.True(t, checker.Matches(body, &size))
	assert.False(t, checker.Matches(append([]byte(nil), body...), &size), "a replacement body needs new observations")
	assert.False(t, checker.Matches(body, nil), "unknown and exact request sizes are different")
	size = 32
	assert.False(t, checker.Matches(body, &size), "changed size metadata needs a new checker")
	limit, participates := 32.0, true
	config := &dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{
		dto.ParameterCapabilityRequestBodySizeBytes: {Max: &limit, ParticipateInSelection: &participates},
	}}
	var violation *CapabilityViolationError
	require.ErrorAs(t, checker.Check(config, "model"), &violation)
	assert.Equal(t, "33", violation.Value, "the checker owns its original size metadata")
	require.NoError(t, NewSelectionCapabilityChecker(nil, nil).Check(config, "model"))
}

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
