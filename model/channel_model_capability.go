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

func isSensitiveModelChannelOverridePath(path string) bool {
	segments := strings.FieldsFunc(strings.ToLower(path), func(character rune) bool {
		switch character {
		case '.', '[', ']', '/', '\\':
			return true
		default:
			return false
		}
	})
	for _, segment := range segments {
		segment = strings.ReplaceAll(strings.TrimSpace(segment), "-", "_")
		switch segment {
		case "authorization", "proxy_authorization", "api_key", "apikey", "x_api_key", "x_goog_api_key",
			"cookie", "set_cookie", "password", "secret", "client_secret", "access_token", "refresh_token",
			"private_key", "credential", "credentials", "token":
			return true
		}
	}
	return false
}

func redactModelChannelCapabilityOverrideValue(value any) any {
	switch typedValue := value.(type) {
	case map[string]any:
		redacted := make(map[string]any, len(typedValue))
		mode, _ := typedValue["mode"].(string)
		path, _ := typedValue["path"].(string)
		redactConfiguredValue := strings.EqualFold(strings.TrimSpace(mode), "set_header") || isSensitiveModelChannelOverridePath(path)
		for key, nestedValue := range typedValue {
			switch {
			case isSensitiveModelChannelOverridePath(key):
				redacted[key] = modelChannelCapabilityRedactedValue
			case strings.EqualFold(strings.TrimSpace(key), "value") && redactConfiguredValue:
				redacted[key] = modelChannelCapabilityRedactedValue
			default:
				redacted[key] = redactModelChannelCapabilityOverrideValue(nestedValue)
			}
		}
		return redacted
	case []any:
		redacted := make([]any, len(typedValue))
		for index, nestedValue := range typedValue {
			redacted[index] = redactModelChannelCapabilityOverrideValue(nestedValue)
		}
		return redacted
	default:
		return value
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
						if logic, ok := operationMap["logic"].(string); ok {
							operation.Logic = logic
						}
						if value, exists := operationMap["value"]; exists {
							operation.Value = redactModelChannelCapabilityOverrideValue(value)
							if strings.EqualFold(strings.TrimSpace(operation.Mode), "set_header") || isSensitiveModelChannelOverridePath(operation.Path) {
								operation.Value = modelChannelCapabilityRedactedValue
							}
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
										Value: redactModelChannelCapabilityOverrideValue(conditions[path]),
									})
									if isSensitiveModelChannelOverridePath(trimmedPath) {
										operation.Conditions[len(operation.Conditions)-1].Value = modelChannelCapabilityRedactedValue
									}
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
									condition.Value = redactModelChannelCapabilityOverrideValue(conditionMap["value"])
									if isSensitiveModelChannelOverridePath(condition.Path) {
										condition.Value = modelChannelCapabilityRedactedValue
									}
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
							if isSensitiveModelChannelOverridePath(key) {
								capability.ParameterOverrideLegacy[key] = modelChannelCapabilityRedactedValue
							} else {
								capability.ParameterOverrideLegacy[key] = redactModelChannelCapabilityOverrideValue(value)
							}
						}
					}
				} else {
					capability.ParameterOverrideLegacy, _ = redactModelChannelCapabilityOverrideValue(paramOverride).(map[string]any)
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
