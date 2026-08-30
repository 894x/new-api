package relayparam

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type CapabilityViolationError struct {
	Model     string
	Parameter string
	Value     string
	Reason    string
}

func (e *CapabilityViolationError) Error() string {
	if e == nil {
		return "parameter capability violation"
	}
	return fmt.Sprintf("parameter %s is incompatible with the selected model: %s", e.Parameter, e.Reason)
}

type CapabilityChange struct {
	Parameter string `json:"parameter"`
	Action    string `json:"action"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
}

// CheckSelectionCapabilities checks only constraints explicitly enabled for
// channel selection. It never applies drop or clamp actions and therefore does
// not mutate the client request used by later routing candidates.
func CheckSelectionCapabilities(data []byte, config *dto.ParameterCapabilityConfig, model string) error {
	if !config.HasSelectionConstraints() {
		return nil
	}
	capabilities := config.Resolve(model)
	if len(capabilities) == 0 {
		return nil
	}

	paths := make([]string, 0, len(capabilities))
	for path, capability := range capabilities {
		if capability.ParticipateInSelection != nil && *capability.ParticipateInSelection {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)

	for _, path := range paths {
		capability := capabilities[path]
		resolvedPaths, err := ResolveJSONPaths(data, path, false)
		if err != nil {
			return err
		}
		for _, resolvedPath := range resolvedPaths {
			value := gjson.GetBytes(data, resolvedPath)
			if !value.Exists() {
				continue
			}
			if capability.Supported != nil && !*capability.Supported {
				return newCapabilityViolation(model, resolvedPath, value.Raw, "parameter is not supported")
			}
			if capability.Min != nil || capability.Max != nil {
				if value.Type != gjson.Number {
					return newCapabilityViolation(model, resolvedPath, value.Raw, "value must be a number")
				}
				number := value.Float()
				if capability.Min != nil && number < *capability.Min {
					return newCapabilityViolation(model, resolvedPath, value.Raw, fmt.Sprintf("value must be greater than or equal to %v", *capability.Min))
				}
				if capability.Max != nil && number > *capability.Max {
					return newCapabilityViolation(model, resolvedPath, value.Raw, fmt.Sprintf("value must be less than or equal to %v", *capability.Max))
				}
			}
			if len(capability.AllowedValues) > 0 && !containsAllowedValue(capability.AllowedValues, value.String()) {
				return newCapabilityViolation(model, resolvedPath, value.Raw, fmt.Sprintf("value must be one of: %s", strings.Join(capability.AllowedValues, ", ")))
			}
		}
	}
	return nil
}

func newCapabilityViolation(model string, parameter string, value string, reason string) *CapabilityViolationError {
	return &CapabilityViolationError{Model: model, Parameter: parameter, Value: value, Reason: reason}
}

// ApplyCapabilities validates and normalizes a converted upstream request.
// Missing parameters are left untouched so omission remains distinct from an
// explicitly supplied zero, false, or empty value.
func ApplyCapabilities(data []byte, config *dto.ParameterCapabilityConfig, model string) ([]byte, []CapabilityChange, error) {
	capabilities := config.Resolve(model)
	if len(capabilities) == 0 {
		return data, nil, nil
	}

	paths := make([]string, 0, len(capabilities))
	for path := range capabilities {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	result := data
	changes := make([]CapabilityChange, 0)
	for _, path := range paths {
		capability := capabilities[path]
		action := capability.OnViolation
		if action == "" {
			action = dto.ParameterCapabilityActionReject
		}
		resolvedPaths, err := ResolveJSONPaths(result, path, false)
		if err != nil {
			return nil, changes, err
		}
		if action == dto.ParameterCapabilityActionDrop {
			slices.Reverse(resolvedPaths)
		}
		for _, resolvedPath := range resolvedPaths {
			value := gjson.GetBytes(result, resolvedPath)
			if !value.Exists() {
				continue
			}

			if capability.Supported != nil && !*capability.Supported {
				var err error
				result, changes, err = handleViolation(result, changes, model, resolvedPath, value.Raw, "parameter is not supported", action, nil)
				if err != nil {
					return nil, changes, err
				}
				continue
			}

			if capability.Min != nil || capability.Max != nil {
				if value.Type != gjson.Number {
					var err error
					result, changes, err = handleViolation(result, changes, model, resolvedPath, value.Raw, "value must be a number", action, nil)
					if err != nil {
						return nil, changes, err
					}
					continue
				}
				number := value.Float()
				clamped := number
				reason := ""
				if capability.Min != nil && number < *capability.Min {
					clamped = *capability.Min
					reason = fmt.Sprintf("value must be greater than or equal to %v", *capability.Min)
				}
				if capability.Max != nil && number > *capability.Max {
					clamped = *capability.Max
					reason = fmt.Sprintf("value must be less than or equal to %v", *capability.Max)
				}
				if reason != "" {
					var err error
					result, changes, err = handleViolation(result, changes, model, resolvedPath, value.Raw, reason, action, &clamped)
					if err != nil {
						return nil, changes, err
					}
					continue
				}
			}

			if len(capability.AllowedValues) > 0 && !containsAllowedValue(capability.AllowedValues, value.String()) {
				reason := fmt.Sprintf("value must be one of: %s", strings.Join(capability.AllowedValues, ", "))
				var err error
				result, changes, err = handleViolation(result, changes, model, resolvedPath, value.Raw, reason, action, nil)
				if err != nil {
					return nil, changes, err
				}
			}
		}
	}
	return result, changes, nil
}

func handleViolation(
	data []byte,
	changes []CapabilityChange,
	model string,
	parameter string,
	from string,
	reason string,
	action string,
	clampedValue *float64,
) ([]byte, []CapabilityChange, error) {
	switch action {
	case dto.ParameterCapabilityActionDrop:
		updated, err := sjson.DeleteBytes(data, parameter)
		if err != nil {
			return nil, changes, err
		}
		return updated, append(changes, CapabilityChange{Parameter: parameter, Action: action}), nil
	case dto.ParameterCapabilityActionClamp:
		if clampedValue != nil {
			updated, err := sjson.SetBytes(data, parameter, *clampedValue)
			if err != nil {
				return nil, changes, err
			}
			return updated, append(changes, CapabilityChange{
				Parameter: parameter,
				Action:    action,
				From:      from,
				To:        fmt.Sprintf("%v", *clampedValue),
			}), nil
		}
		fallthrough
	default:
		return nil, changes, &CapabilityViolationError{
			Model: model, Parameter: parameter, Value: from, Reason: reason,
		}
	}
}

func containsAllowedValue(allowed []string, value string) bool {
	for _, candidate := range allowed {
		if candidate == value {
			return true
		}
	}
	return false
}
