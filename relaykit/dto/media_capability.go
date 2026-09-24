package dto

import "fmt"

// Limits always describe decoded file bytes, not Base64 or JSON wire bytes.
// Pointer fields permit inherited values to be explicitly disabled.
type MediaFormatCapability struct {
	Supported     *bool  `json:"supported,omitempty"`
	MaxMediaBytes *int64 `json:"max_media_bytes,omitempty"`
}

type MediaConversions struct {
	URLToBase64 *bool `json:"url_to_base64,omitempty"`
	Base64ToURL *bool `json:"base64_to_url,omitempty"`
}

type MediaCapability struct {
	Kind        string                           `json:"kind,omitempty"`
	Formats     map[string]MediaFormatCapability `json:"formats,omitempty"`
	Conversions MediaConversions                 `json:"conversions,omitempty"`
}

func (m *MediaCapability) Validate() error {
	if m.Kind != "" && m.Kind != "video" {
		return fmt.Errorf("unsupported media kind %q", m.Kind)
	}
	for name, format := range m.Formats {
		if name != "url" && name != "base64" {
			return fmt.Errorf("unsupported media format %q", name)
		}
		if format.MaxMediaBytes != nil && (*format.MaxMediaBytes <= 0 || *format.MaxMediaBytes > 1<<40) {
			return fmt.Errorf("max_media_bytes must be between 1 and 1099511627776")
		}
	}
	return nil
}

// MergeMediaCapability does not mutate configuration maps shared by channel caches.
func MergeMediaCapability(base, override *MediaCapability) *MediaCapability {
	if base == nil && override == nil {
		return nil
	}
	result := &MediaCapability{Formats: make(map[string]MediaFormatCapability)}
	for _, source := range []*MediaCapability{base, override} {
		if source == nil {
			continue
		}
		if source.Kind != "" {
			result.Kind = source.Kind
		}
		for name, format := range source.Formats {
			current := result.Formats[name]
			if format.Supported != nil {
				current.Supported = format.Supported
			}
			if format.MaxMediaBytes != nil {
				current.MaxMediaBytes = format.MaxMediaBytes
			}
			result.Formats[name] = current
		}
		if source.Conversions.URLToBase64 != nil {
			result.Conversions.URLToBase64 = source.Conversions.URLToBase64
		}
		if source.Conversions.Base64ToURL != nil {
			result.Conversions.Base64ToURL = source.Conversions.Base64ToURL
		}
	}
	return result
}
