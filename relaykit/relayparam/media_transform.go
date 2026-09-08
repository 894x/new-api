package relayparam

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// MediaTransformer is supplied by the host. The standalone module never fetches URLs.
// It returns a MIME-qualified base64 data URI, with errors safe for client responses.
type MediaTransformer func(url string, mediaType string) (string, error)

// ApplyMediaTransforms changes only configured upstream fields. URL objects keep
// their siblings (detail, fps, etc.); input_audio uses raw base64 plus format.
func ApplyMediaTransforms(data []byte, config *dto.ParameterCapabilityConfig, model string, transform MediaTransformer) ([]byte, []CapabilityChange, error) {
	capabilities := config.Resolve(model)
	paths := make([]string, 0)
	for path, capability := range capabilities {
		if capability.HasMediaTransform() && (capability.Supported == nil || *capability.Supported) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	result := data
	var changes []CapabilityChange
	for _, path := range paths {
		capability := capabilities[path]
		// Validate the resolved rule too: constraints may come from another scope.
		if err := (&dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{path: capability}}).Validate(); err != nil {
			return nil, changes, newCapabilityViolation(model, path, "", "invalid media transform constraints")
		}
		mediaType := strings.TrimSuffix(capability.Transform, "_url_to_base64")
		matches, err := ResolveJSONPaths(result, path, false)
		if err != nil {
			return nil, changes, err
		}
		for _, match := range matches {
			value := gjson.GetBytes(result, match)
			valuePath := match
			audioPath := ""
			if mediaType == "audio" && strings.HasSuffix(match, ".input_audio") {
				audioPath = match
				valuePath = match + ".data"
				value = gjson.GetBytes(result, valuePath)
			} else if mediaType == "audio" && strings.HasSuffix(match, ".input_audio.data") {
				audioPath = strings.TrimSuffix(match, ".data")
			} else if value.IsObject() {
				valuePath = match + ".url"
				value = gjson.GetBytes(result, valuePath)
			}
			if !value.Exists() {
				continue
			}
			if value.Type != gjson.String {
				return nil, changes, newCapabilityViolation(model, match, "", "media source must be a URL string")
			}
			source := value.String()
			if strings.HasPrefix(source, "data:") {
				continue
			}
			parsed, err := url.Parse(source)
			// Bare base64 audio data is already in the upstream format.
			if audioPath != "" && err == nil && parsed.Scheme == "" {
				continue
			}
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
				return nil, changes, newCapabilityViolation(model, match, "", "media source must use HTTP or HTTPS without credentials")
			}
			if transform == nil {
				return nil, changes, newCapabilityViolation(model, match, "", "media download handler is unavailable")
			}
			encoded, err := transform(source, mediaType)
			if err != nil {
				return nil, changes, newCapabilityViolation(model, match, "", fmt.Sprintf("media conversion failed: %s", err))
			}
			if audioPath != "" {
				header, payload, ok := strings.Cut(encoded, ",")
				format := ""
				switch header {
				case "data:audio/mpeg;base64":
					format = "mp3"
				case "data:audio/wav;base64", "data:audio/wave;base64", "data:audio/x-wav;base64":
					format = "wav"
				}
				if !ok || format == "" {
					return nil, changes, newCapabilityViolation(model, match, "", "input_audio requires MP3 or WAV media")
				}
				declared := gjson.GetBytes(result, audioPath+".format").String()
				if declared != "" && declared != format {
					return nil, changes, newCapabilityViolation(model, match, "", "input_audio format does not match downloaded media")
				}
				result, err = sjson.SetBytes(result, audioPath+".format", format)
				if err != nil {
					return nil, changes, err
				}
				encoded = payload
			}
			if int64(len(result))-int64(len(value.Raw))+int64(len(encoded))+16 > 96<<20 {
				return nil, changes, newCapabilityViolation(model, match, "", "converted media request exceeds 96 MiB")
			}
			result, err = sjson.SetBytes(result, valuePath, encoded)
			if err != nil {
				return nil, changes, err
			}
			changes = append(changes, CapabilityChange{Parameter: valuePath, Action: capability.Transform})
		}
	}
	return result, changes, nil
}
