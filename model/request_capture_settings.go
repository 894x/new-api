package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
)

const RequestCaptureStorageOptionKey = "RequestCaptureStorage"
const DefaultRequestCaptureStorageJSON = `{"retention_days":3,"max_gib":10,"max_concurrency":8,"only_non_200":false}`
const MaxRequestCaptureConcurrency = 1024

// Settings share one option so a request observes one consistent policy.
type RequestCaptureStorageSettings struct {
	RetentionDays  int  `json:"retention_days"`
	MaxGiB         int  `json:"max_gib"`
	MaxConcurrency int  `json:"max_concurrency"`
	OnlyNon200     bool `json:"only_non_200"`
}

func ParseRequestCaptureStorageSettings(value string) (RequestCaptureStorageSettings, error) {
	// Existing persisted settings predate concurrency and filtering controls.
	settings := RequestCaptureStorageSettings{MaxConcurrency: 8}
	if err := common.UnmarshalJsonStr(value, &settings); err != nil {
		return settings, fmt.Errorf("invalid request capture storage settings")
	}
	invalidRetention := settings.RetentionDays < 1 || settings.RetentionDays > 365
	invalidCapacity := settings.MaxGiB < 1 || settings.MaxGiB > 365
	if invalidRetention || invalidCapacity {
		return settings, fmt.Errorf("capture retention days and capacity GiB must be integers between 1 and 365")
	}
	if settings.MaxConcurrency < 1 || settings.MaxConcurrency > MaxRequestCaptureConcurrency {
		return settings, fmt.Errorf("capture concurrency must be an integer between 1 and %d", MaxRequestCaptureConcurrency)
	}
	return settings, nil
}

func GetRequestCaptureStorageSettings() RequestCaptureStorageSettings {
	common.OptionMapRWMutex.RLock()
	value := common.OptionMap[RequestCaptureStorageOptionKey]
	common.OptionMapRWMutex.RUnlock()
	if value == "" {
		value = DefaultRequestCaptureStorageJSON
	}
	settings, err := ParseRequestCaptureStorageSettings(value)
	if err != nil {
		// All publication paths validate before mutation. This guard also keeps
		// storage bounded when embedded callers initialize OptionMap themselves.
		common.SysError("invalid request capture storage settings; using defaults")
		return RequestCaptureStorageSettings{RetentionDays: 3, MaxGiB: 10, MaxConcurrency: 8}
	}
	return settings
}
