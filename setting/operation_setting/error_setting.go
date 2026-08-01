package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

type ErrorSetting struct {
	HideErrorDetails bool `json:"hide_error_details"`
}

var errorSetting = ErrorSetting{
	HideErrorDetails: true,
}

func init() {
	config.GlobalConfig.Register("error_setting", &errorSetting)
}

func GetErrorSetting() *ErrorSetting {
	return &errorSetting
}

func ShouldHideErrorDetails() bool {
	return errorSetting.HideErrorDetails
}
