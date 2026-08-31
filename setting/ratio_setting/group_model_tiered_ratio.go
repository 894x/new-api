package ratio_setting

import (
	"fmt"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
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
	if err := checkGroupPricingConfiguration(GroupRatio2JSONString(), raw); err != nil {
		return err
	}
	policies, err := groupdiscount.ParsePoliciesJSON(raw)
	if err != nil {
		return err
	}
	return GetGroupRatioSetting().ModelTieredRatios.Replace(policies)
}

func CheckModelTieredRatios(raw string) error {
	return checkGroupPricingConfiguration(GroupRatio2JSONString(), raw)
}

func CheckGroupPricingConfiguration(groupRatiosJSON, modelTieredRatiosJSON string) error {
	return checkGroupPricingConfiguration(groupRatiosJSON, modelTieredRatiosJSON)
}

// UpdateGroupPricingConfiguration publishes a validated GroupRatio and tiered
// policy pair without exposing an intermediate orphan policy to readers. The
// temporary union may expose a newly added pricing group slightly early, but
// every observable policy group remains backed by GroupRatio throughout the
// replacement.
func UpdateGroupPricingConfiguration(groupRatiosJSON, modelTieredRatiosJSON string) error {
	groupRatios, err := parseGroupRatiosJSON(groupRatiosJSON)
	if err != nil {
		return err
	}
	policies, err := groupdiscount.ParsePoliciesJSON(modelTieredRatiosJSON)
	if err != nil {
		return err
	}
	if err := checkGroupPricingConfiguration(groupRatiosJSON, modelTieredRatiosJSON); err != nil {
		return err
	}

	groupRatioUnion := GetGroupRatioCopy()
	for group, ratio := range groupRatios {
		groupRatioUnion[group] = ratio
	}
	unionJSON, err := common.Marshal(groupRatioUnion)
	if err != nil {
		return err
	}
	if err := UpdateGroupRatioByJSONString(string(unionJSON)); err != nil {
		return err
	}
	if err := GetGroupRatioSetting().ModelTieredRatios.Replace(policies); err != nil {
		return err
	}
	return UpdateGroupRatioByJSONString(groupRatiosJSON)
}

// RecoverGroupPricingConfiguration disables all tiered policies before
// publishing a syntactically valid pricing-group map. Callers can separately
// retain invalid raw option values for administrative repair without exposing
// orphan policies to billing.
func RecoverGroupPricingConfiguration(groupRatiosJSON string) error {
	groupRatios, parseErr := parseGroupRatiosJSON(groupRatiosJSON)
	if err := GetGroupRatioSetting().ModelTieredRatios.Replace(groupdiscount.PolicyMap{}); err != nil {
		return err
	}
	if parseErr != nil {
		return parseErr
	}
	data, err := common.Marshal(groupRatios)
	if err != nil {
		return err
	}
	return UpdateGroupRatioByJSONString(string(data))
}

func checkGroupPricingConfiguration(groupRatiosJSON, modelTieredRatiosJSON string) error {
	groupRatios, err := parseGroupRatiosJSON(groupRatiosJSON)
	if err != nil {
		return err
	}
	policies, err := groupdiscount.ParsePoliciesJSON(modelTieredRatiosJSON)
	if err != nil {
		return err
	}

	unknownGroups := make([]string, 0)
	for group := range policies {
		if _, ok := groupRatios[group]; !ok {
			unknownGroups = append(unknownGroups, group)
		}
	}
	if len(unknownGroups) == 0 {
		return nil
	}
	sort.Strings(unknownGroups)
	return fmt.Errorf("model tiered ratio group %q is not configured in GroupRatio", unknownGroups[0])
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
