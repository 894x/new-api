package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListSatisfiedChannelCandidatesPreservesAllEligiblePriorities(t *testing.T) {
	setupAssetChannelSelectTest(t)

	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			common.MemoryCacheEnabled = memoryCacheEnabled
			if memoryCacheEnabled {
				InitChannelCache()
			}

			candidates, err := ListSatisfiedChannelCandidatesWithFilters(
				"default",
				"asset-video-model",
				"/api/v3/contents/generations/tasks",
				"",
				nil,
			)
			require.NoError(t, err)
			assert.ElementsMatch(t, []SatisfiedChannelCandidate{
				{ChannelId: 4101, Priority: 100, Weight: 100},
				{ChannelId: 4102, Priority: 10, Weight: 100},
			}, candidates)

			candidates, err = ListSatisfiedChannelCandidatesWithFilters(
				"default",
				"asset-video-model",
				"/api/v3/contents/generations/tasks",
				"",
				map[int]struct{}{4102: {}},
			)
			require.NoError(t, err)
			assert.Equal(t, []SatisfiedChannelCandidate{
				{ChannelId: 4102, Priority: 10, Weight: 100},
			}, candidates)

			candidates, err = ListSatisfiedChannelCandidatesWithFilters(
				"default",
				"asset-video-model",
				"/api/v3/contents/generations/tasks",
				"",
				map[int]struct{}{},
			)
			require.NoError(t, err)
			assert.Empty(t, candidates)
		})
	}
}

func TestListSatisfiedChannelCandidatesAppliesVideoResolutionBeforePriority(t *testing.T) {
	db := setupAssetChannelSelectTest(t)

	var highPriorityChannel Channel
	require.NoError(t, db.First(&highPriorityChannel, "id = ?", 4101).Error)
	highPriorityChannel.SetOtherSettings(dto.ChannelOtherSettings{VideoCapabilities: &dto.VideoCapabilityConfig{
		Models: map[string]dto.VideoModelCapability{"asset-video-model": {Resolutions: []string{"720p"}}},
	}})
	require.NoError(t, db.Model(&highPriorityChannel).Update("settings", highPriorityChannel.OtherSettings).Error)

	var lowPriorityChannel Channel
	require.NoError(t, db.First(&lowPriorityChannel, "id = ?", 4102).Error)
	lowPriorityChannel.SetOtherSettings(dto.ChannelOtherSettings{VideoCapabilities: &dto.VideoCapabilityConfig{
		Models: map[string]dto.VideoModelCapability{"asset-video-model": {Resolutions: []string{"1080p"}}},
	}})
	require.NoError(t, db.Model(&lowPriorityChannel).Update("settings", lowPriorityChannel.OtherSettings).Error)

	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			common.MemoryCacheEnabled = memoryCacheEnabled
			if memoryCacheEnabled {
				InitChannelCache()
			}

			candidates, err := ListSatisfiedChannelCandidatesWithFilters(
				"default",
				"asset-video-model",
				"/v1/video/generations",
				"1080p",
				nil,
			)
			require.NoError(t, err)
			assert.Equal(t, []SatisfiedChannelCandidate{
				{ChannelId: 4102, Priority: 10, Weight: 100},
			}, candidates)
		})
	}
}

func TestListSatisfiedChannelCandidatesUsesNormalizedModelFallback(t *testing.T) {
	db := setupAssetChannelSelectTest(t)
	priority := int64(25)
	weight := uint(40)
	require.NoError(t, db.Create(&Channel{
		Id:       4103,
		Type:     constant.ChannelTypeOpenAI,
		Key:      "key-4103",
		Status:   common.ChannelStatusEnabled,
		Name:     "normalized-channel",
		Weight:   &weight,
		Models:   "gpt-4-gizmo-*",
		Group:    "default",
		Priority: &priority,
	}).Error)
	require.NoError(t, db.Create(&Ability{
		Group:     "default",
		Model:     "gpt-4-gizmo-*",
		ChannelId: 4103,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)

	for _, memoryCacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			common.MemoryCacheEnabled = memoryCacheEnabled
			if memoryCacheEnabled {
				InitChannelCache()
			}

			candidates, err := ListSatisfiedChannelCandidatesWithFilters(
				"default",
				"gpt-4-gizmo-customer-model",
				"/v1/chat/completions",
				"",
				nil,
			)
			require.NoError(t, err)
			assert.Equal(t, []SatisfiedChannelCandidate{
				{ChannelId: 4103, Priority: 25, Weight: 40},
			}, candidates)
		})
	}
}
