package setting

import (
	"fmt"
	"math"
	"strconv"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

var ModelRequestRateLimitEnabled = false
var ModelRequestRateLimitDurationMinutes = 1
var ModelRequestRateLimitCount = 0
var ModelRequestRateLimitSuccessCount = 1000
var ModelRequestRateLimitTPM = 0
var ModelRequestRateLimitGroup = map[string][3]int{}
var ModelRequestRateLimitMutex sync.RWMutex

func ModelRequestRateLimitGroup2JSONString() string {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	jsonBytes, err := common.Marshal(ModelRequestRateLimitGroup)
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

	limits, found := ModelRequestRateLimitGroup[group]
	if !found {
		return 0, 0, 0, false
	}
	return limits[0], limits[1], limits[2], true
}

func CheckModelRequestRateLimitGroup(jsonStr string) error {
	_, err := parseModelRequestRateLimitGroups(jsonStr)
	return err
}

func parseModelRequestRateLimitGroups(jsonStr string) (map[string][3]int, error) {
	rawGroups := make(map[string][]int)
	err := common.UnmarshalJsonStr(jsonStr, &rawGroups)
	if err != nil {
		return nil, err
	}
	if rawGroups == nil {
		return nil, fmt.Errorf("group rate limits must be a JSON object")
	}

	groups := make(map[string][3]int, len(rawGroups))
	for group, rawLimits := range rawGroups {
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
		groups[group] = limits
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
