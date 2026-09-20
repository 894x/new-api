package relayparam

import (
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type MediaInfo struct {
	Format string
	Bytes  int64 // -1 means unknown; URL headers are only declared sizes.
	Exact  bool
}

// A source may report an inaccurate size. A different channel can still accept
// the now-known bytes; distinguish that from malformed input or storage failures.
var ErrActualMediaSize = errors.New("downloaded video exceeds the channel media size limit")

// MediaProcessor owns request-scoped I/O and caching in the host application.
type MediaProcessor interface {
	Inspect(source string) (MediaInfo, error)
	Convert(source, target string, limit int64) (string, error)
}

type MediaInput struct {
	Path   string
	Source string
	Info   MediaInfo
}

// MediaDescriber lets hosts cache extraction as well as size inspection. Large
// Base64 fields must not be re-parsed/hashed for every candidate channel.
type MediaDescriber interface {
	Describe(data []byte, path string) ([]MediaInput, error)
}

func DescribeMediaInputs(data []byte, path string, processor MediaProcessor) ([]MediaInput, error) {
	matches, err := ResolveJSONPaths(data, path, false)
	if err != nil {
		return nil, err
	}
	inputs := make([]MediaInput, 0, len(matches))
	for _, match := range matches {
		value := gjson.GetBytes(data, match)
		if value.IsObject() {
			match += ".url"
			value = gjson.GetBytes(data, match)
		}
		if value.Type != gjson.String || value.String() == "" {
			return nil, fmt.Errorf("video source must be a non-empty string")
		}
		if processor == nil {
			return nil, fmt.Errorf("video media processor is unavailable")
		}
		info, err := processor.Inspect(value.String())
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, MediaInput{Path: match, Source: value.String(), Info: info})
	}
	return inputs, nil
}

type MediaDeliveryPlan struct {
	Path   string
	Source string
	Target string
	Info   MediaInfo
	Limit  int64
}

// Base64VideoInfo validates without allocating a decoded copy of the video.
func Base64VideoInfo(source string) (MediaInfo, error) {
	header, payload, ok := strings.Cut(source, ",")
	if !ok || !strings.HasPrefix(header, "data:video/") || !strings.HasSuffix(header, ";base64") || payload == "" {
		return MediaInfo{}, fmt.Errorf("video Base64 must be a non-empty video data URI")
	}
	n := len(payload)
	padding := 0
	for padding < n && payload[n-padding-1] == '=' {
		padding++
	}
	if padding > 2 || (padding > 0 && n%4 != 0) || n%4 == 1 {
		return MediaInfo{}, fmt.Errorf("invalid video Base64 padding")
	}
	for i := 0; i < n-padding; i++ {
		c := payload[i]
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '+' || c == '/') {
			return MediaInfo{}, fmt.Errorf("invalid video Base64 character")
		}
	}
	encoding := base64.StdEncoding.Strict()
	if padding == 0 {
		encoding = base64.RawStdEncoding.Strict()
	}
	start := (n - 1) / 4 * 4
	var tail [3]byte
	if _, err := encoding.Decode(tail[:], []byte(payload[start:])); err != nil {
		return MediaInfo{}, fmt.Errorf("invalid video Base64 ending")
	}
	return MediaInfo{Format: "base64", Bytes: int64(n)*3/4 - int64(padding), Exact: true}, nil
}

// PlanMediaDelivery is side-effect free except for the host's cached Inspect.
// The returned tier is 0 for direct delivery, 1 when any video needs conversion.
func PlanMediaDelivery(data []byte, config *dto.ParameterCapabilityConfig, model string, processor MediaProcessor, selectionOnly bool) ([]MediaDeliveryPlan, int, error) {
	capabilities := config.Resolve(model)
	paths := make([]string, 0)
	for path, capability := range capabilities {
		if capability.Media != nil && (!selectionOnly || capability.ParticipateInSelection != nil && *capability.ParticipateInSelection) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	plans := make([]MediaDeliveryPlan, 0)
	tier := 0
	seen := make(map[string]bool)
	for _, path := range paths {
		capability := capabilities[path]
		if err := (&dto.ParameterCapabilityConfig{Defaults: map[string]dto.ParameterCapability{path: capability}}).Validate(); err != nil {
			return nil, 0, newCapabilityViolation(model, path, "", "invalid resolved video media capability")
		}
		var inputs []MediaInput
		var err error
		if describer, ok := processor.(MediaDescriber); ok {
			inputs, err = describer.Describe(data, path)
		} else {
			inputs, err = DescribeMediaInputs(data, path, processor)
		}
		if err != nil {
			return nil, 0, newCapabilityViolation(model, path, "", err.Error())
		}
		for _, input := range inputs {
			match, info := input.Path, input.Info
			if seen[match] {
				return nil, 0, newCapabilityViolation(model, match, "", "overlapping video media capability paths")
			}
			seen[match] = true
			if capability.Supported != nil && !*capability.Supported {
				return nil, 0, newCapabilityViolation(model, match, "", "video input is not supported")
			}
			target := info.Format
			format := capability.Media.Formats[target]
			fits := format.Supported != nil && *format.Supported && (format.MaxMediaBytes == nil || info.Bytes < 0 || info.Bytes <= *format.MaxMediaBytes)
			if !fits {
				allowed := capability.Media.Conversions.URLToBase64
				target = "base64"
				if info.Format == "base64" {
					target = "url"
					allowed = capability.Media.Conversions.Base64ToURL
				}
				format = capability.Media.Formats[target]
				if allowed == nil || !*allowed || format.Supported == nil || !*format.Supported || (format.MaxMediaBytes != nil && info.Bytes >= 0 && info.Bytes > *format.MaxMediaBytes) {
					return nil, 0, newCapabilityViolation(model, match, "", "video format or file size is not supported by this channel")
				}
				tier = 1
			}
			limit := int64(0)
			if format.MaxMediaBytes != nil {
				limit = *format.MaxMediaBytes
			}
			plans = append(plans, MediaDeliveryPlan{Path: match, Source: input.Source, Target: target, Info: info, Limit: limit})
		}
	}
	return plans, tier, nil
}

func ApplyMediaDelivery(data []byte, config *dto.ParameterCapabilityConfig, model string, processor MediaProcessor) ([]byte, []CapabilityChange, error) {
	plans, _, err := PlanMediaDelivery(data, config, model, processor, false)
	if err != nil {
		return nil, nil, err
	}
	if len(plans) > 0 && len(data) > 96<<20 {
		return nil, nil, newCapabilityViolation(model, "video_url", "", "video request exceeds 96 MiB")
	}
	result := data
	var changes []CapabilityChange
	for _, plan := range plans {
		if plan.Info.Format == plan.Target {
			continue
		}
		value, err := processor.Convert(plan.Source, plan.Target, plan.Limit)
		if err != nil {
			violation := newCapabilityViolation(model, plan.Path, "", err.Error())
			violation.Retryable = errors.Is(err, ErrActualMediaSize)
			return nil, changes, violation
		}
		if int64(len(result))-int64(len(plan.Source))+int64(len(value))+16 > 96<<20 {
			return nil, changes, newCapabilityViolation(model, plan.Path, "", "converted media request exceeds 96 MiB")
		}
		result, err = sjson.SetBytes(result, plan.Path, value)
		if err != nil {
			return nil, changes, err
		}
		changes = append(changes, CapabilityChange{Parameter: plan.Path, Action: plan.Info.Format + "_to_" + plan.Target})
	}
	return result, changes, nil
}
