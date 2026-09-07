package model

import (
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type ChannelModelMatrixFilter struct {
	Model           string `form:"model" binding:"max=255"`
	Channel         string `form:"channel" binding:"max=255"`
	Group           string `form:"group" binding:"max=255"`
	Status          string `form:"status" binding:"oneof=enabled all"`
	ModelPage       int    `form:"model_page" binding:"gte=1"`
	ModelPageSize   int    `form:"model_page_size" binding:"gte=1,lte=100"`
	ChannelPage     int    `form:"channel_page" binding:"gte=1"`
	ChannelPageSize int    `form:"channel_page_size" binding:"gte=1,lte=50"`
}

type ChannelModelMatrixChannel struct {
	ID     int      `json:"id"`
	Name   string   `json:"name"`
	Type   int      `json:"type"`
	Status int      `json:"status"`
	Groups []string `json:"groups"`
}

type ChannelModelMatrixCell struct {
	ChannelModelRouting
	UpstreamModel string `json:"upstream_model"`
	MappingError  bool   `json:"mapping_error"`
}

type ChannelModelMatrix struct {
	Models          []string                    `json:"models"`
	Channels        []ChannelModelMatrixChannel `json:"channels"`
	Cells           []ChannelModelMatrixCell    `json:"cells"`
	Groups          []string                    `json:"groups"`
	ModelTotal      int                         `json:"model_total"`
	ChannelTotal    int                         `json:"channel_total"`
	ModelPage       int                         `json:"model_page"`
	ModelPageSize   int                         `json:"model_page_size"`
	ChannelPage     int                         `json:"channel_page"`
	ChannelPageSize int                         `json:"channel_page_size"`
}

// ListChannelModelMatrix derives rows from configured public model names, even
// when no metadata or sparse override exists. Only the visible cross-section's
// overrides are fetched; channel credentials and other settings are never read.
func ListChannelModelMatrix(filter ChannelModelMatrixFilter) (*ChannelModelMatrix, error) {
	var channels []*Channel
	if err := DB.Select([]string{"id", "name", "type", "status", "models", "group", "priority", "weight", "rpm", "tpm", "model_mapping"}).
		Order("id").Find(&channels).Error; err != nil {
		return nil, err
	}
	result := &ChannelModelMatrix{
		Models: []string{}, Channels: []ChannelModelMatrixChannel{}, Cells: []ChannelModelMatrixCell{}, Groups: []string{},
		ModelPageSize: min(max(filter.ModelPageSize, 1), 100), ChannelPageSize: min(max(filter.ChannelPageSize, 1), 50),
	}
	modelSearch := strings.ToLower(strings.TrimSpace(filter.Model))
	channelSearch := strings.ToLower(strings.TrimSpace(filter.Channel))
	groupFilter := strings.TrimSpace(filter.Group)
	groups := make(map[string]struct{})
	models := make(map[string]struct{})
	channelModels := make(map[int][]string)
	filteredChannels := make([]*Channel, 0, len(channels))
	for _, channel := range channels {
		channelGroups := channel.GetGroups()
		for _, group := range channelGroups {
			if group != "" {
				groups[group] = struct{}{}
			}
		}
		if filter.Status != "all" && channel.Status != common.ChannelStatusEnabled {
			continue
		}
		if groupFilter != "" && !slices.Contains(channelGroups, groupFilter) {
			continue
		}
		if channelSearch != "" && !strings.Contains(strings.ToLower(channel.Name), channelSearch) && strconv.Itoa(channel.Id) != channelSearch {
			continue
		}
		filteredChannels = append(filteredChannels, channel)
		channelModels[channel.Id] = normalizeChannelModels(channel)
		for _, name := range channelModels[channel.Id] {
			if strings.Contains(strings.ToLower(name), modelSearch) {
				models[name] = struct{}{}
			}
		}
	}
	for group := range groups {
		result.Groups = append(result.Groups, group)
	}
	sort.Strings(result.Groups)
	for name := range models {
		result.Models = append(result.Models, name)
	}
	sort.Strings(result.Models)
	result.ModelTotal, result.ChannelTotal = len(result.Models), len(filteredChannels)
	result.ModelPage = min(max(filter.ModelPage, 1), max(1, (result.ModelTotal+result.ModelPageSize-1)/result.ModelPageSize))
	result.ChannelPage = min(max(filter.ChannelPage, 1), max(1, (result.ChannelTotal+result.ChannelPageSize-1)/result.ChannelPageSize))
	modelStart := (result.ModelPage - 1) * result.ModelPageSize
	channelStart := (result.ChannelPage - 1) * result.ChannelPageSize
	result.Models = result.Models[modelStart:min(modelStart+result.ModelPageSize, result.ModelTotal)]
	visibleChannels := filteredChannels[channelStart:min(channelStart+result.ChannelPageSize, result.ChannelTotal)]
	channelIDs := make([]int, 0, len(visibleChannels))
	for _, channel := range visibleChannels {
		channelIDs = append(channelIDs, channel.Id)
		result.Channels = append(result.Channels, ChannelModelMatrixChannel{
			ID: channel.Id, Name: channel.Name, Type: channel.Type, Status: channel.Status, Groups: channel.GetGroups(),
		})
	}
	if len(channelIDs) == 0 || len(result.Models) == 0 {
		return result, nil
	}
	var overrides []ChannelModelOverride
	if err := DB.Where("channel_id IN ? AND model IN ?", channelIDs, result.Models).Find(&overrides).Error; err != nil {
		return nil, err
	}
	type pair struct {
		channelID int
		model     string
	}
	overrideByPair := make(map[pair]*ChannelModelOverride, len(overrides))
	for i := range overrides {
		override := &overrides[i]
		overrideByPair[pair{override.ChannelId, override.Model}] = override
	}
	for _, channel := range visibleChannels {
		for _, name := range result.Models {
			if !slices.Contains(channelModels[channel.Id], name) {
				continue
			}
			upstream, _, err := channel.ResolveUpstreamModelName(name)
			result.Cells = append(result.Cells, ChannelModelMatrixCell{
				ChannelModelRouting: effectiveChannelModelRouting(channel, name, overrideByPair[pair{channel.Id, name}]),
				UpstreamModel:       upstream, MappingError: err != nil,
			})
		}
	}
	return result, nil
}
