package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
)

const RequestCaptureStorageOptionKey = "RequestCaptureStorage"
const DefaultRequestCaptureStorageJSON = `{"retention_days":3,"max_gib":10}`

// Both fields share one option so readers never observe a partially saved pair.
type RequestCaptureStorageSettings struct {
	RetentionDays int `json:"retention_days"`
	MaxGiB        int `json:"max_gib"`
}

func ParseRequestCaptureStorageSettings(value string) (RequestCaptureStorageSettings, error) {
	var settings RequestCaptureStorageSettings
	if err := common.UnmarshalJsonStr(value, &settings); err != nil {
		return settings, fmt.Errorf("invalid request capture storage settings")
	}
	if settings.RetentionDays < 1 || settings.RetentionDays > 365 || settings.MaxGiB < 1 || settings.MaxGiB > 365 {
		return settings, fmt.Errorf("capture retention days and capacity GiB must be integers between 1 and 365")
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
		return RequestCaptureStorageSettings{RetentionDays: 3, MaxGiB: 10}
	}
	return settings
}
