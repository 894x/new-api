package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

type ModelChannelCapabilityGroup struct {
	Group   string `json:"group"`
	Enabled bool   `json:"enabled"`
}

type ModelChannelParameterOverrideCondition struct {
	Order          int    `json:"order"`
	Path           string `json:"path"`
	Mode           string `json:"mode"`
	Value          any    `json:"value"`
	Invert         bool   `json:"invert"`
	PassMissingKey bool   `json:"pass_missing_key"`
}

type ModelChannelParameterOverrideOperation struct {
	Order           int                                      `json:"order"`
	Description     string                                   `json:"description,omitempty"`
	Path            string                                   `json:"path,omitempty"`
	Mode            string                                   `json:"mode"`
	Value           any                                      `json:"value"`
	ValueConfigured bool                                     `json:"value_configured"`
	KeepOrigin      bool                                     `json:"keep_origin"`
	From            string                                   `json:"from,omitempty"`
	To              string                                   `json:"to,omitempty"`
	Conditions      []ModelChannelParameterOverrideCondition `json:"conditions"`
	Logic           string                                   `json:"logic"`
}

type ModelChannelCapability struct {
	ChannelModelRouting
	Groups                          []ModelChannelCapabilityGroup            `json:"groups"`
	UpstreamModel                   string                                   `json:"upstream_model"`
	ModelMapped                     bool                                     `json:"model_mapped"`
	EndpointTypes                   []string                                 `json:"endpoint_types"`
	ParameterCapabilitiesConfigured bool                                     `json:"parameter_capabilities_configured"`
	ParameterCapabilities           map[string]dto.ParameterCapability       `json:"parameter_capabilities,omitempty"`
	VideoCapabilitiesConfigured     bool                                     `json:"video_capabilities_configured"`
	VideoResolutions                []string                                 `json:"video_resolutions"`
	ParameterOverrideConfigured     bool                                     `json:"parameter_override_configured"`
	ParameterOverrideMode           string                                   `json:"parameter_override_mode"`
	ParameterOverrideLegacy         map[string]any                           `json:"parameter_override_legacy,omitempty"`
	ParameterOverrideOperations     []ModelChannelParameterOverrideOperation `json:"parameter_override_operations"`
	ConfigurationError              string                                   `json:"configuration_error,omitempty"`
}

type ModelChannelCapabilities struct {
	Model    string                   `json:"model"`
	Channels []ModelChannelCapability `json:"channels"`
}

const modelChannelCapabilityRedactedValue = "[REDACTED]"

func isDisplaySafeModelChannelOverridePath(path string) bool {
	switch strings.ToLower(strings.TrimSpace(path)) {
	case "model", "original_model", "upstream_model",
		"temperature", "top_p", "top_k", "min_p",
		"max_tokens", "max_completion_tokens", "max_output_tokens",
		"n", "seed", "stop", "presence_penalty", "frequency_penalty", "repetition_penalty",
		"logprobs", "top_logprobs", "service_tier", "verbosity", "modalities",
		"reasoning_effort", "reasoning.effort", "output_config.effort", "generationconfig.thinkingconfig.thinkinglevel",
		"size", "quality", "style", "resolution", "duration", "seconds",
		"user_group", "token_group", "using_group", "is_retry", "retry.is_retry", "last_error.code":
		return true
	default:
		return false
	}
}

func modelChannelCapabilityOverrideDisplayValue(path string, value any) any {
	if !isDisplaySafeModelChannelOverridePath(path) {
		return modelChannelCapabilityRedactedValue
	}
	switch typedValue := value.(type) {
	case nil, string, bool, float64:
		return value
	case []any:
		displayValue := make([]any, len(typedValue))
		for index, nestedValue := range typedValue {
			displayValue[index] = modelChannelCapabilityOverrideDisplayValue(path, nestedValue)
		}
		return displayValue
	default:
		return modelChannelCapabilityRedactedValue
	}
}

func redactModelChannelCapabilityOverrideConfig(value any) any {
	switch typedValue := value.(type) {
	case map[string]any:
		redacted := make(map[string]any, len(typedValue))
		mode, _ := typedValue["mode"].(string)
		path, _ := typedValue["path"].(string)
		_, hasMode := typedValue["mode"]
		_, hasValue := typedValue["value"]
		if strings.TrimSpace(path) != "" && (hasMode || hasValue) {
			for key, nestedValue := range typedValue {
				normalizedKey := strings.ToLower(strings.TrimSpace(key))
				switch {
				case normalizedKey == "value":
					redacted[key] = modelChannelCapabilityOverrideDisplayValue(path, nestedValue)
				case (normalizedKey == "from" || normalizedKey == "to") &&
					(strings.EqualFold(strings.TrimSpace(mode), "replace") || strings.EqualFold(strings.TrimSpace(mode), "regex_replace")):
					redacted[key] = modelChannelCapabilityOverrideDisplayValue(path, nestedValue)
				case normalizedKey == "conditions":
					redacted[key] = redactModelChannelCapabilityOverrideConfig(nestedValue)
				case normalizedKey == "mode", normalizedKey == "path", normalizedKey == "description",
					normalizedKey == "keep_origin", normalizedKey == "logic", normalizedKey == "invert",
					normalizedKey == "pass_missing_key", normalizedKey == "order",
					normalizedKey == "from", normalizedKey == "to":
					redacted[key] = nestedValue
				default:
					redacted[key] = modelChannelCapabilityRedactedValue
				}
			}
			return redacted
		}
		for key, nestedValue := range typedValue {
			if strings.EqualFold(strings.TrimSpace(key), "operations") {
				redacted[key] = redactModelChannelCapabilityOverrideConfig(nestedValue)
			} else {
				redacted[key] = modelChannelCapabilityOverrideDisplayValue(key, nestedValue)
			}
		}
		return redacted
	case []any:
		redacted := make([]any, len(typedValue))
		for index, nestedValue := range typedValue {
			switch nestedValue.(type) {
			case map[string]any, []any:
				redacted[index] = redactModelChannelCapabilityOverrideConfig(nestedValue)
			default:
				redacted[index] = modelChannelCapabilityRedactedValue
			}
		}
		return redacted
	default:
		return modelChannelCapabilityRedactedValue
	}
}

func (channel *Channel) ResolveUpstreamModelName(modelName string) (string, bool, error) {
	modelName = strings.TrimSpace(modelName)
	mappingJSON := channel.GetModelMapping()
	if mappingJSON == "" || mappingJSON == "{}" {
		return modelName, false, nil
	}

	mapping := make(map[string]string)
	if err := common.UnmarshalJsonStr(mappingJSON, &mapping); err != nil {
		return modelName, false, fmt.Errorf("invalid model mapping: %w", err)
	}
	currentModel := modelName
	mapped := false
	visited := map[string]struct{}{currentModel: {}}
	for {
		nextModel, exists := mapping[currentModel]
		if !exists || nextModel == "" {
			return currentModel, mapped, nil
		}
		if _, seen := visited[nextModel]; seen {
			if nextModel == currentModel {
				if currentModel == modelName {
					return modelName, false, nil
				}
				return currentModel, true, nil
			}
			return currentModel, mapped, errors.New("model mapping contains cycle")
		}
		visited[nextModel] = struct{}{}
		currentModel = nextModel
		mapped = true
	}
}

func ListModelChannelCapabilities(modelName string) (ModelChannelCapabilities, error) {
	modelName = strings.TrimSpace(modelName)
	result := ModelChannelCapabilities{
		Model:    modelName,
		Channels: make([]ModelChannelCapability, 0),
	}
	if modelName == "" {
		return result, errors.New("model cannot be empty")
	}

	var abilities []Ability
	if err := DB.Where("model = ?", modelName).Find(&abilities).Error; err != nil {
		return result, err
	}
	if len(abilities) == 0 {
		return result, nil
	}

	channelIDs := make([]int, 0)
	seenChannelIDs := make(map[int]struct{})
	groupsByChannel := make(map[int][]ModelChannelCapabilityGroup)
	for _, ability := range abilities {
		groupsByChannel[ability.ChannelId] = append(groupsByChannel[ability.ChannelId], ModelChannelCapabilityGroup{
			Group:   ability.Group,
			Enabled: ability.Enabled,
		})
		if _, exists := seenChannelIDs[ability.ChannelId]; exists {
			continue
		}
		seenChannelIDs[ability.ChannelId] = struct{}{}
		channelIDs = append(channelIDs, ability.ChannelId)
	}
	for channelID := range groupsByChannel {
		sort.Slice(groupsByChannel[channelID], func(i, j int) bool {
			return groupsByChannel[channelID][i].Group < groupsByChannel[channelID][j].Group
		})
	}

	var channels []*Channel
	if err := DB.Where("id IN ?", channelIDs).Find(&channels).Error; err != nil {
		return result, err
	}
	var overrides []ChannelModelOverride
	if err := DB.Where("channel_id IN ? AND model = ?", channelIDs, modelName).Find(&overrides).Error; err != nil {
		return result, err
	}
	overridesByChannel := make(map[int]ChannelModelOverride, len(overrides))
	for _, override := range overrides {
		overridesByChannel[override.ChannelId] = override
	}

	for _, channel := range channels {
		var override *ChannelModelOverride
		if current, exists := overridesByChannel[channel.Id]; exists {
			override = &current
		}
		capability := ModelChannelCapability{
			ChannelModelRouting:         effectiveChannelModelRouting(channel, modelName, override),
			Groups:                      groupsByChannel[channel.Id],
			UpstreamModel:               modelName,
			EndpointTypes:               make([]string, 0),
			VideoResolutions:            make([]string, 0),
			ParameterOverrideMode:       "none",
			ParameterOverrideOperations: make([]ModelChannelParameterOverrideOperation, 0),
		}
		configurationErrors := make([]string, 0, 2)

		upstreamModel, mapped, mappingErr := channel.ResolveUpstreamModelName(modelName)
		capability.UpstreamModel = upstreamModel
		capability.ModelMapped = mapped
		if mappingErr != nil {
			configurationErrors = append(configurationErrors, mappingErr.Error())
		}

		if channel.ParamOverride != nil && strings.TrimSpace(*channel.ParamOverride) != "" {
			paramOverride := make(map[string]any)
			if err := common.UnmarshalJsonStr(*channel.ParamOverride, &paramOverride); err != nil {
				configurationErrors = append(configurationErrors, fmt.Sprintf("invalid parameter override: %v", err))
			} else if len(paramOverride) > 0 {
				capability.ParameterOverrideConfigured = true
				operationsValue, hasOperationsKey := paramOverride["operations"]
				operations, operationsFormat := operationsValue.([]any)
				if hasOperationsKey && operationsFormat {
					parsedOperations := make([]ModelChannelParameterOverrideOperation, 0, len(operations))
					for operationIndex, operationValue := range operations {
						operationMap, ok := operationValue.(map[string]any)
						if !ok {
							operationsFormat = false
							break
						}
						mode, ok := operationMap["mode"].(string)
						if !ok {
							operationsFormat = false
							break
						}

						operation := ModelChannelParameterOverrideOperation{
							Order:      operationIndex + 1,
							Mode:       mode,
							Conditions: make([]ModelChannelParameterOverrideCondition, 0),
							Logic:      "OR",
						}
						operation.Description, _ = operationMap["description"].(string)
						operation.Path, _ = operationMap["path"].(string)
						operation.KeepOrigin, _ = operationMap["keep_origin"].(bool)
						operation.From, _ = operationMap["from"].(string)
						operation.To, _ = operationMap["to"].(string)
						if strings.EqualFold(strings.TrimSpace(operation.Mode), "replace") || strings.EqualFold(strings.TrimSpace(operation.Mode), "regex_replace") {
							if _, exists := operationMap["from"]; exists {
								operation.From, _ = modelChannelCapabilityOverrideDisplayValue(operation.Path, operation.From).(string)
							}
							if _, exists := operationMap["to"]; exists {
								operation.To, _ = modelChannelCapabilityOverrideDisplayValue(operation.Path, operation.To).(string)
							}
						}
						if logic, ok := operationMap["logic"].(string); ok {
							operation.Logic = logic
						}
						if value, exists := operationMap["value"]; exists {
							operation.Value = modelChannelCapabilityOverrideDisplayValue(operation.Path, value)
							operation.ValueConfigured = true
						}

						if conditionsValue, exists := operationMap["conditions"]; exists {
							switch conditions := conditionsValue.(type) {
							case map[string]any:
								conditionPaths := make([]string, 0, len(conditions))
								for path := range conditions {
									conditionPaths = append(conditionPaths, path)
								}
								sort.Strings(conditionPaths)
								for _, path := range conditionPaths {
									trimmedPath := strings.TrimSpace(path)
									if trimmedPath == "" {
										continue
									}
									operation.Conditions = append(operation.Conditions, ModelChannelParameterOverrideCondition{
										Path:  trimmedPath,
										Mode:  "full",
										Value: modelChannelCapabilityOverrideDisplayValue(trimmedPath, conditions[path]),
									})
								}
								if len(operation.Conditions) == 0 {
									operationsFormat = false
								}
							case []any:
								for conditionIndex, conditionValue := range conditions {
									conditionMap, ok := conditionValue.(map[string]any)
									if !ok {
										operationsFormat = false
										break
									}
									path, _ := conditionMap["path"].(string)
									conditionMode, _ := conditionMap["mode"].(string)
									if strings.TrimSpace(path) == "" || strings.TrimSpace(conditionMode) == "" {
										operationsFormat = false
										break
									}
									condition := ModelChannelParameterOverrideCondition{
										Order: conditionIndex + 1,
										Path:  path,
										Mode:  conditionMode,
									}
									condition.Value = modelChannelCapabilityOverrideDisplayValue(condition.Path, conditionMap["value"])
									condition.Invert, _ = conditionMap["invert"].(bool)
									condition.PassMissingKey, _ = conditionMap["pass_missing_key"].(bool)
									operation.Conditions = append(operation.Conditions, condition)
								}
							default:
								operationsFormat = false
							}
						}
						if !operationsFormat {
							break
						}
						for conditionIndex := range operation.Conditions {
							operation.Conditions[conditionIndex].Order = conditionIndex + 1
						}
						parsedOperations = append(parsedOperations, operation)
					}
					if operationsFormat {
						capability.ParameterOverrideOperations = parsedOperations
					}
				}

				if hasOperationsKey && operationsFormat {
					capability.ParameterOverrideLegacy = make(map[string]any)
					for key, value := range paramOverride {
						if !strings.EqualFold(strings.TrimSpace(key), "operations") {
							capability.ParameterOverrideLegacy[key] = modelChannelCapabilityOverrideDisplayValue(key, value)
						}
					}
				} else {
					capability.ParameterOverrideLegacy, _ = redactModelChannelCapabilityOverrideConfig(paramOverride).(map[string]any)
				}

				hasLegacy := len(capability.ParameterOverrideLegacy) > 0
				switch {
				case hasLegacy && operationsFormat:
					capability.ParameterOverrideMode = "mixed"
				case operationsFormat:
					capability.ParameterOverrideMode = "operations"
				case hasLegacy:
					capability.ParameterOverrideMode = "legacy"
				}
			}
		}

		settings := dto.ChannelOtherSettings{}
		if channel.OtherSettings != "" {
			if err := common.UnmarshalJsonStr(channel.OtherSettings, &settings); err != nil {
				configurationErrors = append(configurationErrors, fmt.Sprintf("invalid channel settings: %v", err))
			}
		}

		endpointTypes := common.GetEndpointTypesByChannelType(channel.Type, modelName)
		if channel.Type == constant.ChannelTypeAdvancedCustom && settings.AdvancedCustom != nil {
			endpointTypes = settings.AdvancedCustom.SupportedEndpointTypesForModel(modelName)
		}
		for _, endpointType := range endpointTypes {
			capability.EndpointTypes = append(capability.EndpointTypes, string(endpointType))
		}

		if settings.ParameterCapabilities != nil && mappingErr == nil {
			capability.ParameterCapabilities = settings.ParameterCapabilities.Resolve(upstreamModel)
			capability.ParameterCapabilitiesConfigured = len(capability.ParameterCapabilities) > 0
		}
		if settings.VideoCapabilities != nil {
			if videoCapability, exists := settings.VideoCapabilities.Models[modelName]; exists {
				capability.VideoCapabilitiesConfigured = true
				capability.VideoResolutions = append(capability.VideoResolutions, videoCapability.Resolutions...)
			}
		}
		capability.ConfigurationError = strings.Join(configurationErrors, "; ")
		result.Channels = append(result.Channels, capability)
	}

	sort.Slice(result.Channels, func(i, j int) bool {
		if result.Channels[i].EffectivePriority != result.Channels[j].EffectivePriority {
			return result.Channels[i].EffectivePriority > result.Channels[j].EffectivePriority
		}
		return result.Channels[i].ChannelId < result.Channels[j].ChannelId
	})
	return result, nil
}
