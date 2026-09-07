package setting

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

var ModelRequestRateLimitEnabled = false
var ModelRequestRateLimitDurationMinutes = 1
var ModelRequestRateLimitCount = 0
var ModelRequestRateLimitSuccessCount = 1000
var ModelRequestRateLimitTPM = 0

// Nil model fields inherit the group; explicit zero disables that limit.
type ModelRateLimit struct {
	RPM *int `json:"rpm,omitempty"`
	TPM *int `json:"tpm,omitempty"`
}

type GroupRateLimit struct {
	Limits [3]int                    `json:"limits"`
	Models map[string]ModelRateLimit `json:"models,omitempty"`
}

var ModelRequestRateLimitGroup = map[string]GroupRateLimit{}
var ModelRequestRateLimitMutex sync.RWMutex

func ModelRequestRateLimitGroup2JSONString() string {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	groups := make(map[string]any, len(ModelRequestRateLimitGroup))
	for group, config := range ModelRequestRateLimitGroup {
		if len(config.Models) == 0 {
			groups[group] = config.Limits
		} else {
			groups[group] = config
		}
	}
	jsonBytes, err := common.Marshal(groups)
	if err != nil {
		common.SysLog("error marshalling model request rate limits: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateModelRequestRateLimitGroupByJSONString(jsonStr string) error {
	groups, err := parseModelRequestRateLimitGroups(jsonStr)
	if err != nil {
		return err
	}

	ModelRequestRateLimitMutex.Lock()
	defer ModelRequestRateLimitMutex.Unlock()
	ModelRequestRateLimitGroup = groups
	return nil
}

func GetGroupRateLimit(group string) (totalCount, successCount, tpm int, found bool) {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	if ModelRequestRateLimitGroup == nil {
		return 0, 0, 0, false
	}

	config, found := ModelRequestRateLimitGroup[group]
	if !found {
		return 0, 0, 0, false
	}
	return config.Limits[0], config.Limits[1], config.Limits[2], true
}

// ResolveGroupModelRateLimit preserves the legacy request window and success
// limit when RPM is inherited. An explicit RPM counts all requests in 60 seconds.
func ResolveGroupModelRateLimit(group, modelName string) (total, success, tpm int, duration int64) {
	total, success, tpm = ModelRequestRateLimitCount, ModelRequestRateLimitSuccessCount, ModelRequestRateLimitTPM
	duration = int64(ModelRequestRateLimitDurationMinutes) * 60
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()
	if config, found := ModelRequestRateLimitGroup[group]; found {
		total, success, tpm = config.Limits[0], config.Limits[1], config.Limits[2]
		if limits, found := config.Models[modelName]; found {
			if limits.RPM != nil {
				total, success, duration = *limits.RPM, 0, 60
			}
			if limits.TPM != nil {
				tpm = *limits.TPM
			}
		}
	}
	return
}

func CheckModelRequestRateLimitGroup(jsonStr string) error {
	_, err := parseModelRequestRateLimitGroups(jsonStr)
	return err
}

func parseModelRequestRateLimitGroups(jsonStr string) (map[string]GroupRateLimit, error) {
	rawGroups := make(map[string]json.RawMessage)
	err := common.UnmarshalJsonStr(jsonStr, &rawGroups)
	if err != nil {
		return nil, err
	}
	if rawGroups == nil {
		return nil, fmt.Errorf("group rate limits must be a JSON object")
	}

	groups := make(map[string]GroupRateLimit, len(rawGroups))
	for group, raw := range rawGroups {
		var rawLimits []int
		var rawModels map[string]json.RawMessage
		if strings.HasPrefix(strings.TrimSpace(string(raw)), "{") {
			var fields map[string]json.RawMessage
			if err := common.Unmarshal(raw, &fields); err != nil {
				return nil, err
			}
			for field := range fields {
				if field != "limits" && field != "models" {
					return nil, fmt.Errorf("group %s has unknown rate limit field %s", group, field)
				}
			}
			if err := common.Unmarshal(fields["limits"], &rawLimits); err != nil {
				return nil, fmt.Errorf("group %s limits: %w", group, err)
			}
			if raw, exists := fields["models"]; exists {
				if err := common.Unmarshal(raw, &rawModels); err != nil {
					return nil, fmt.Errorf("group %s models: %w", group, err)
				}
				if rawModels == nil {
					return nil, fmt.Errorf("group %s models must be a JSON object", group)
				}
			}
		} else if err := common.Unmarshal(raw, &rawLimits); err != nil {
			return nil, fmt.Errorf("group %s limits: %w", group, err)
		}
		if len(rawLimits) != 2 && len(rawLimits) != 3 {
			return nil, fmt.Errorf("group %s rate limits must contain 2 or 3 values", group)
		}
		limits := [3]int{rawLimits[0], rawLimits[1], 0}
		if len(rawLimits) == 3 {
			limits[2] = rawLimits[2]
		}
		if limits[0] < 0 || limits[1] < 1 || limits[2] < 0 {
			return nil, fmt.Errorf("group %s has invalid rate limit values: [%d, %d, %d]", group, limits[0], limits[1], limits[2])
		}
		if limits[0] > math.MaxInt32 || limits[1] > math.MaxInt32 || limits[2] > math.MaxInt32 {
			return nil, fmt.Errorf("group %s [%d, %d, %d] has max rate limits value 2147483647", group, limits[0], limits[1], limits[2])
		}
		models := make(map[string]ModelRateLimit, len(rawModels))
		for modelName, raw := range rawModels {
			if strings.TrimSpace(modelName) == "" {
				return nil, fmt.Errorf("group %s model name must not be empty", group)
			}
			var fields map[string]*int
			if err := common.Unmarshal(raw, &fields); err != nil {
				return nil, fmt.Errorf("group %s model %s: %w", group, modelName, err)
			}
			if fields == nil {
				return nil, fmt.Errorf("group %s model %s limits must be a JSON object", group, modelName)
			}
			for field, value := range fields {
				if field != "rpm" && field != "tpm" {
					return nil, fmt.Errorf("group %s model %s has unknown field %s", group, modelName, field)
				}
				if value != nil && (*value < 0 || *value > math.MaxInt32) {
					return nil, fmt.Errorf("group %s model %s %s must be between 0 and %d", group, modelName, field, math.MaxInt32)
				}
			}
			models[modelName] = ModelRateLimit{RPM: fields["rpm"], TPM: fields["tpm"]}
		}
		groups[group] = GroupRateLimit{Limits: limits, Models: models}
	}
	return groups, nil
}

func ValidateModelRequestRateLimitTPM(value string) error {
	tpm, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return fmt.Errorf("TPM must be an integer between 0 and %d", math.MaxInt32)
	}
	if tpm < 0 {
		return fmt.Errorf("TPM must be an integer between 0 and %d", math.MaxInt32)
	}
	return nil
}
