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

type ModelChannelCapability struct {
	ChannelModelRouting
	Groups                          []ModelChannelCapabilityGroup      `json:"groups"`
	UpstreamModel                   string                             `json:"upstream_model"`
	ModelMapped                     bool                               `json:"model_mapped"`
	EndpointTypes                   []string                           `json:"endpoint_types"`
	ParameterCapabilitiesConfigured bool                               `json:"parameter_capabilities_configured"`
	ParameterCapabilities           map[string]dto.ParameterCapability `json:"parameter_capabilities,omitempty"`
	VideoCapabilitiesConfigured     bool                               `json:"video_capabilities_configured"`
	VideoResolutions                []string                           `json:"video_resolutions"`
	ConfigurationError              string                             `json:"configuration_error,omitempty"`
}

type ModelChannelCapabilities struct {
	Model    string                   `json:"model"`
	Channels []ModelChannelCapability `json:"channels"`
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
			ChannelModelRouting: effectiveChannelModelRouting(channel, modelName, override),
			Groups:              groupsByChannel[channel.Id],
			UpstreamModel:       modelName,
			EndpointTypes:       make([]string, 0),
			VideoResolutions:    make([]string, 0),
		}

		upstreamModel, mapped, mappingErr := channel.ResolveUpstreamModelName(modelName)
		capability.UpstreamModel = upstreamModel
		capability.ModelMapped = mapped
		if mappingErr != nil {
			capability.ConfigurationError = mappingErr.Error()
		}

		settings := dto.ChannelOtherSettings{}
		if channel.OtherSettings != "" {
			if err := common.UnmarshalJsonStr(channel.OtherSettings, &settings); err != nil {
				capability.ConfigurationError = fmt.Sprintf("invalid channel settings: %v", err)
				result.Channels = append(result.Channels, capability)
				continue
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
