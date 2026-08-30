package dto

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	ParameterCapabilitySelectorPattern = "pattern"
	ParameterCapabilitySelectorExact   = "exact"

	ParameterCapabilityActionReject = "reject"
	ParameterCapabilityActionDrop   = "drop"
	ParameterCapabilityActionClamp  = "clamp"

	maxParameterCapabilityRules      = 256
	maxParameterCapabilitiesPerScope = 128
)

var parameterCapabilityPathPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*(?:\.(?:[A-Za-z_][A-Za-z0-9_-]*|\*))*$`)

var billingSensitiveParameterPaths = map[string]struct{}{
	"max_tokens":                          {},
	"max_completion_tokens":               {},
	"max_output_tokens":                   {},
	"maxTokens":                           {},
	"maxCompletionTokens":                 {},
	"maxOutputTokens":                     {},
	"max_tokens_to_sample":                {},
	"maxTokensToSample":                   {},
	"generation_config.max_output_tokens": {},
	"generationConfig.maxOutputTokens":    {},
	"inferenceConfig.maxTokens":           {},
	"n":                                   {},
	"seconds":                             {},
	"duration":                            {},
}

// ParameterCapabilityConfig describes the request parameters accepted by a
// channel and its upstream models. Defaults apply to every model, while Rules
// add model-pattern and exact-model overrides.
type ParameterCapabilityConfig struct {
	Defaults map[string]ParameterCapability `json:"defaults,omitempty"`
	Rules    []ModelParameterCapabilityRule `json:"rules,omitempty"`
}

type ModelParameterCapabilityRule struct {
	Name       string                         `json:"name,omitempty"`
	Selector   ParameterCapabilitySelector    `json:"selector"`
	Parameters map[string]ParameterCapability `json:"parameters,omitempty"`
}

type ParameterCapabilitySelector struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type ParameterCapability struct {
	Supported              *bool    `json:"supported,omitempty"`
	Min                    *float64 `json:"min,omitempty"`
	Max                    *float64 `json:"max,omitempty"`
	AllowedValues          []string `json:"allowed_values,omitempty"`
	OnViolation            string   `json:"on_violation,omitempty"`
	ParticipateInSelection *bool    `json:"participate_in_selection,omitempty"`
}

func (c *ParameterCapabilityConfig) Validate() error {
	if c == nil {
		return nil
	}
	if err := validateParameterCapabilityMap(c.Defaults); err != nil {
		return fmt.Errorf("invalid parameter capability defaults: %w", err)
	}
	if len(c.Rules) > maxParameterCapabilityRules {
		return fmt.Errorf("too many parameter capability rules: %d", len(c.Rules))
	}
	for i, rule := range c.Rules {
		if err := rule.Selector.Validate(); err != nil {
			return fmt.Errorf("invalid parameter capability rule %d: %w", i+1, err)
		}
		if err := validateParameterCapabilityMap(rule.Parameters); err != nil {
			return fmt.Errorf("invalid parameter capability rule %d: %w", i+1, err)
		}
	}
	return nil
}

func (s ParameterCapabilitySelector) Validate() error {
	value := strings.TrimSpace(s.Value)
	if value == "" {
		return fmt.Errorf("model selector value is required")
	}
	switch s.Type {
	case ParameterCapabilitySelectorExact:
		return nil
	case ParameterCapabilitySelectorPattern:
		return nil
	default:
		return fmt.Errorf("unsupported model selector type %q", s.Type)
	}
}

func (s ParameterCapabilitySelector) Matches(model string) bool {
	value := strings.TrimSpace(s.Value)
	switch s.Type {
	case ParameterCapabilitySelectorExact:
		return model == value
	case ParameterCapabilitySelectorPattern:
		return matchModelCapabilityPattern(model, value)
	default:
		return false
	}
}

// Resolve returns the effective parameter constraints for an upstream model.
// Pattern rules are applied in declaration order, followed by exact rules.
func (c *ParameterCapabilityConfig) Resolve(model string) map[string]ParameterCapability {
	if c == nil {
		return nil
	}
	result := make(map[string]ParameterCapability, len(c.Defaults))
	mergeParameterCapabilityMap(result, c.Defaults)
	for _, rule := range c.Rules {
		if rule.Selector.Type == ParameterCapabilitySelectorPattern && rule.Selector.Matches(model) {
			mergeParameterCapabilityMap(result, rule.Parameters)
		}
	}
	for _, rule := range c.Rules {
		if rule.Selector.Type == ParameterCapabilitySelectorExact && rule.Selector.Matches(model) {
			mergeParameterCapabilityMap(result, rule.Parameters)
		}
	}
	return result
}

func (c *ParameterCapabilityConfig) HasSelectionConstraints() bool {
	if c == nil {
		return false
	}
	for _, capability := range c.Defaults {
		if capability.ParticipateInSelection != nil && *capability.ParticipateInSelection {
			return true
		}
	}
	for _, rule := range c.Rules {
		for _, capability := range rule.Parameters {
			if capability.ParticipateInSelection != nil && *capability.ParticipateInSelection {
				return true
			}
		}
	}
	return false
}

func validateParameterCapabilityMap(parameters map[string]ParameterCapability) error {
	if len(parameters) > maxParameterCapabilitiesPerScope {
		return fmt.Errorf("too many parameters in one scope: %d", len(parameters))
	}
	for path, capability := range parameters {
		if !parameterCapabilityPathPattern.MatchString(path) {
			return fmt.Errorf("invalid parameter path %q", path)
		}
		if capability.Min != nil && capability.Max != nil && *capability.Min > *capability.Max {
			return fmt.Errorf("parameter %s minimum cannot exceed maximum", path)
		}
		switch capability.OnViolation {
		case "", ParameterCapabilityActionReject, ParameterCapabilityActionDrop, ParameterCapabilityActionClamp:
		default:
			return fmt.Errorf("parameter %s has unsupported violation action %q", path, capability.OnViolation)
		}
		if capability.OnViolation == ParameterCapabilityActionClamp && capability.Min == nil && capability.Max == nil {
			return fmt.Errorf("parameter %s uses clamp without a numeric boundary", path)
		}
		if _, billingSensitive := billingSensitiveParameterPaths[path]; billingSensitive &&
			capability.OnViolation != "" && capability.OnViolation != ParameterCapabilityActionReject {
			return fmt.Errorf("billing-sensitive parameter %s must reject incompatible values", path)
		}
	}
	return nil
}

func mergeParameterCapabilityMap(target map[string]ParameterCapability, source map[string]ParameterCapability) {
	for path, override := range source {
		base := target[path]
		if override.Supported != nil {
			base.Supported = override.Supported
		}
		if override.Min != nil {
			base.Min = override.Min
		}
		if override.Max != nil {
			base.Max = override.Max
		}
		if len(override.AllowedValues) > 0 {
			base.AllowedValues = append([]string(nil), override.AllowedValues...)
		}
		if override.OnViolation != "" {
			base.OnViolation = override.OnViolation
		}
		if override.ParticipateInSelection != nil {
			base.ParticipateInSelection = override.ParticipateInSelection
		}
		target[path] = base
	}
}

func matchModelCapabilityPattern(model string, pattern string) bool {
	modelRunes := []rune(model)
	patternRunes := []rune(pattern)
	modelIndex, patternIndex := 0, 0
	starIndex, starMatchIndex := -1, 0

	for modelIndex < len(modelRunes) {
		if patternIndex < len(patternRunes) &&
			(patternRunes[patternIndex] == '?' || patternRunes[patternIndex] == modelRunes[modelIndex]) {
			modelIndex++
			patternIndex++
			continue
		}
		if patternIndex < len(patternRunes) && patternRunes[patternIndex] == '*' {
			starIndex = patternIndex
			starMatchIndex = modelIndex
			patternIndex++
			continue
		}
		if starIndex == -1 {
			return false
		}
		patternIndex = starIndex + 1
		starMatchIndex++
		modelIndex = starMatchIndex
	}

	for patternIndex < len(patternRunes) && patternRunes[patternIndex] == '*' {
		patternIndex++
	}
	return patternIndex == len(patternRunes)
}
