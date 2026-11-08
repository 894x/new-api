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
	LeadRatio      float64 `json:"lead_ratio"`
	MinLeadMS      int     `json:"min_lead_ms"`
	MaxLeadMS      int     `json:"max_lead_ms"`
	EarliestMS     int     `json:"earliest_ms"`
	MinSamples     int     `json:"min_samples"`
	MinProbability float64 `json:"min_probability"`
	MaxAgeSeconds  int     `json:"max_age_seconds"`
}

func Default() Config {
	return Config{Mode: "off", LeadRatio: .15, MinLeadMS: 1000, MaxLeadMS: 3000, EarliestMS: 1000, MinSamples: 100, MinProbability: .95, MaxAgeSeconds: 900}
}

func (c Config) Validate() error {
	if !ValidMode(c.Mode, false) || math.IsNaN(c.LeadRatio) || c.LeadRatio < .01 || c.LeadRatio > 1 || c.MinLeadMS < 1 || c.MaxLeadMS < c.MinLeadMS || c.MaxLeadMS > 30000 || c.EarliestMS < 0 || c.EarliestMS > 30000 || c.MinSamples < 20 || c.MinSamples > 256 || math.IsNaN(c.MinProbability) || c.MinProbability < .5 || c.MinProbability > 1 || c.MaxAgeSeconds < 60 || c.MaxAgeSeconds > 86400 {
		return fmt.Errorf("invalid early chunk prediction settings")
	}
	return nil
}
