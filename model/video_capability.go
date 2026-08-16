package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/relaykit/dto"
)

var ErrVideoResolutionUnsupported = errors.New("video resolution is not supported by any available channel")

func newVideoResolutionUnsupportedError(model string, resolution string) error {
	return fmt.Errorf("%w: model %s, resolution %s", ErrVideoResolutionUnsupported, model, resolution)
}

func (channel *Channel) SupportsVideoResolution(model string, resolution string) bool {
	if channel == nil || resolution == "" {
		return channel != nil
	}
	return channel.GetOtherSettings().VideoCapabilities.SupportsResolution(model, resolution)
}

func filterChannelIDsByVideoResolution(channelIDs []int, configs map[int]*dto.VideoCapabilityConfig, model string, resolution string) []int {
	if resolution == "" || len(channelIDs) == 0 {
		return channelIDs
	}
	filtered := make([]int, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		if configs[channelID].SupportsResolution(model, resolution) {
			filtered = append(filtered, channelID)
		}
	}
	return filtered
}
