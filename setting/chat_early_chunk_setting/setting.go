package chat_early_chunk_setting

import (
	"fmt"
	"math"
	"slices"
)

// Empty modes inherit. Pool channel overrides belong to this pool, never to
// the shared channel configuration.
type Pool struct {
	Mode     string         `json:"mode,omitempty"`
	Channels map[int]string `json:"channels,omitempty"`
}

func ValidMode(mode string, inherit bool) bool {
	return inherit && mode == "" || slices.Contains([]string{"off", "observe", "enabled"}, mode)
}

func (p *Pool) Validate() error {
	if p == nil {
		return nil
	}
	if !ValidMode(p.Mode, true) || len(p.Channels) > 1000 {
		return fmt.Errorf("invalid early chunk pool configuration")
	}
	for id, mode := range p.Channels {
		if id <= 0 || !ValidMode(mode, true) {
			return fmt.Errorf("invalid early chunk channel override")
		}
	}
	return nil
}

func (p *Pool) Resolve(base string, channelID int) string {
	if p == nil {
		return base
	}
	if p.Mode != "" {
		base = p.Mode
	}
	if mode := p.Channels[channelID]; mode != "" {
		base = mode
	}
	return base
}

type Config struct {
	Mode           string  `json:"mode"`
	WindowStartMS  int     `json:"window_start_ms"`
	WindowEndMS    int     `json:"window_end_ms"`
	TargetGapMS    int     `json:"target_gap_ms"`
	MinSamples     int     `json:"min_samples"`
	MinProbability float64 `json:"min_probability"`
	MaxAgeSeconds  int     `json:"max_age_seconds"`
}

func Default() Config {
	return Config{Mode: "off", WindowStartMS: 4500, WindowEndMS: 5500, TargetGapMS: 1000, MinSamples: 100, MinProbability: .95, MaxAgeSeconds: 900}
}

func (c Config) Validate() error {
	if !ValidMode(c.Mode, false) || c.WindowStartMS < 0 || c.WindowEndMS <= c.WindowStartMS || c.WindowEndMS > 30000 || c.TargetGapMS < 1 || c.TargetGapMS > 10000 || c.MinSamples < 20 || c.MinSamples > 256 || math.IsNaN(c.MinProbability) || c.MinProbability < .5 || c.MinProbability > 1 || c.MaxAgeSeconds < 60 || c.MaxAgeSeconds > 86400 {
		return fmt.Errorf("invalid early chunk prediction settings")
	}
	return nil
}
