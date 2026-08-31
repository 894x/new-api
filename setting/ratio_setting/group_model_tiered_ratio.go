package ratio_setting

import (
	"time"

	"github.com/QuantumNous/new-api/pkg/groupdiscount"
)

const ModelTieredRatiosOptionKey = "group_ratio_setting.model_tiered_ratios"

func ModelTieredRatios2JSONString() string {
	data, err := GetGroupRatioSetting().ModelTieredRatios.MarshalJSON()
	if err != nil {
		return "{}"
	}
	return string(data)
}

func UpdateModelTieredRatiosByJSONString(raw string) error {
	policies, err := groupdiscount.ParsePoliciesJSON(raw)
	if err != nil {
		return err
	}
	return GetGroupRatioSetting().ModelTieredRatios.Replace(policies)
}

func CheckModelTieredRatios(raw string) error {
	return groupdiscount.ValidatePoliciesJSON(raw)
}

// CaptureModelTieredDiscountResolver freezes all group policies that an
// admission-time request may encounter during same-group or auto-group retry.
func CaptureModelTieredDiscountResolver(userGroup, originModel string, requestAt time.Time) *groupdiscount.FrozenResolver {
	contractUsingGroups := map[string]struct{}{}
	if contracts, ok := groupGroupRatioMap.Get(userGroup); ok {
		for usingGroup := range contracts {
			contractUsingGroups[usingGroup] = struct{}{}
		}
	}
	return GetGroupRatioSetting().ModelTieredRatios.FreezeResolver(
		contractUsingGroups,
		originModel,
		requestAt,
	)
}

// ResolveModelTieredDiscount freezes the applicable group/model policy for a
// request. A user-group -> using-group contract remains authoritative and
// suppresses dynamic discounting, preserving existing GroupGroupRatio behavior.
func ResolveModelTieredDiscount(userGroup, usingGroup, originModel string, requestAt time.Time) (groupdiscount.Snapshot, bool, error) {
	if _, hasContractRatio := GetGroupGroupRatio(userGroup, usingGroup); hasContractRatio {
		return groupdiscount.Snapshot{}, false, nil
	}
	policy, matchedModel, progressBasis, ok := GetGroupRatioSetting().ModelTieredRatios.ResolveWithBasis(usingGroup, originModel)
	if !ok {
		return groupdiscount.Snapshot{}, false, nil
	}
	return groupdiscount.FreezePolicyWithBasis(policy, progressBasis, usingGroup, originModel, matchedModel, requestAt)
}
