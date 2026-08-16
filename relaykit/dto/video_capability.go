package dto

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	maxVideoCapabilityModels      = 512
	maxVideoModelResolutions      = 32
	maxVideoResolutionValueLength = 32
)

var (
	videoResolutionLabelPattern = regexp.MustCompile(`^[1-9][0-9]{2,3}p$|^[1-9][0-9]*k$`)
	videoResolutionSizePattern  = regexp.MustCompile(`^([1-9][0-9]{1,4})x([1-9][0-9]{1,4})$`)
)

type VideoCapabilityConfig struct {
	Models map[string]VideoModelCapability `json:"models,omitempty"`
}

type VideoModelCapability struct {
	Resolutions []string `json:"resolutions,omitempty"`
}

// SupportsResolution reports whether this channel can serve a public model at
// the requested resolution. Missing configuration, a missing model rule, or an
// omitted request resolution preserves the legacy wildcard behavior.
func (c *VideoCapabilityConfig) SupportsResolution(model string, resolution string) bool {
	if c == nil || resolution == "" {
		return true
	}
	capability, ok := c.Models[model]
	if !ok {
		return true
	}
	requested, err := NormalizeVideoResolution(resolution)
	if err != nil {
		return false
	}
	for _, allowed := range capability.Resolutions {
		normalized, normalizeErr := NormalizeVideoResolution(allowed)
		if normalizeErr == nil && normalized == requested {
			return true
		}
	}
	return false
}

func (c *VideoCapabilityConfig) Validate() error {
	if c == nil {
		return nil
	}
	if len(c.Models) > maxVideoCapabilityModels {
		return fmt.Errorf("too many video capability models: %d", len(c.Models))
	}
	for model, capability := range c.Models {
		if strings.TrimSpace(model) == "" {
			return fmt.Errorf("video capability model name cannot be empty")
		}
		if len(capability.Resolutions) == 0 {
			return fmt.Errorf("video capability model %q requires at least one resolution", model)
		}
		if len(capability.Resolutions) > maxVideoModelResolutions {
			return fmt.Errorf("video capability model %q has too many resolutions: %d", model, len(capability.Resolutions))
		}
		seen := make(map[string]struct{}, len(capability.Resolutions))
		for _, value := range capability.Resolutions {
			resolution, err := NormalizeVideoResolution(value)
			if err != nil {
				return fmt.Errorf("video capability model %q: %w", model, err)
			}
			if _, ok := seen[resolution]; ok {
				return fmt.Errorf("video capability model %q has duplicate resolution %q", model, resolution)
			}
			seen[resolution] = struct{}{}
		}
	}
	return nil
}

func NormalizeVideoResolution(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "*", "x")
	normalized = strings.ReplaceAll(normalized, " ", "")
	if normalized == "" || len(normalized) > maxVideoResolutionValueLength {
		return "", fmt.Errorf("invalid video resolution %q", value)
	}
	if videoResolutionLabelPattern.MatchString(normalized) {
		return normalized, nil
	}

	matches := videoResolutionSizePattern.FindStringSubmatch(normalized)
	if len(matches) != 3 {
		return "", fmt.Errorf("invalid video resolution %q", value)
	}
	width, _ := strconv.Atoi(matches[1])
	height, _ := strconv.Atoi(matches[2])
	longEdge, shortEdge := width, height
	if longEdge < shortEdge {
		longEdge, shortEdge = shortEdge, longEdge
	}
	switch {
	case longEdge == 640 && shortEdge == 360:
		return "360p", nil
	case longEdge == 854 && shortEdge == 480:
		return "480p", nil
	case longEdge == 960 && shortEdge == 540:
		return "540p", nil
	case longEdge == 1280 && shortEdge == 720:
		return "720p", nil
	case longEdge == 1920 && shortEdge == 1080:
		return "1080p", nil
	case longEdge == 3840 && shortEdge == 2160:
		return "4k", nil
	default:
		return fmt.Sprintf("%dx%d", longEdge, shortEdge), nil
	}
}
