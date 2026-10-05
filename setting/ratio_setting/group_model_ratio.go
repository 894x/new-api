/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package ratio_setting

import (
	"fmt"
	"maps"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
)

const GroupGroupModelRatioOptionKey = "group_ratio_setting.group_group_model_ratio"

// GroupGroupModelRatios uses client-facing model IDs, never channel mappings or pricing aliases.
type GroupGroupModelRatios map[string]map[string]map[string]float64

func ParseGroupGroupModelRatios(raw string) (GroupGroupModelRatios, error) {
	var rawRatios map[string]map[string]map[string]*float64
	if err := common.UnmarshalJsonStr(raw, &rawRatios); err != nil {
		return nil, err
	}
	if rawRatios == nil {
		return nil, fmt.Errorf("model special ratios must be a JSON object")
	}
	ratios := make(GroupGroupModelRatios, len(rawRatios))
	for userGroup, groups := range rawRatios {
		if strings.TrimSpace(userGroup) == "" || userGroup != strings.TrimSpace(userGroup) || userGroup == "__proto__" || groups == nil {
			return nil, fmt.Errorf("invalid user group in model special ratios")
		}
		ratios[userGroup] = make(map[string]map[string]float64, len(groups))
		for usingGroup, models := range groups {
			if strings.TrimSpace(usingGroup) == "" || usingGroup != strings.TrimSpace(usingGroup) || usingGroup == "__proto__" || models == nil {
				return nil, fmt.Errorf("invalid billing group in model special ratios for %q", userGroup)
			}
			ratios[userGroup][usingGroup] = make(map[string]float64, len(models))
			for modelID, ratio := range models {
				if strings.TrimSpace(modelID) == "" || modelID != strings.TrimSpace(modelID) || modelID == "__proto__" || len(modelID) > 255 {
					return nil, fmt.Errorf("invalid public model ID in special ratios for %q/%q", userGroup, usingGroup)
				}
				if ratio == nil || math.IsNaN(*ratio) || math.IsInf(*ratio, 0) || *ratio < 0 {
					return nil, fmt.Errorf("model special ratio for %q/%q/%q must be finite and non-negative", userGroup, usingGroup, modelID)
				}
				ratios[userGroup][usingGroup][modelID] = *ratio
			}
		}
	}
	return ratios, nil
}

func UpdateGroupGroupModelRatioByJSONString(raw string) error {
	_, err := ParseGroupGroupModelRatios(raw)
	if err != nil {
		return err
	}
	store := GetGroupRatioSetting().GroupGroupModelRatio
	return types.LoadFromJsonString(store, raw)
}

func GroupGroupModelRatio2JSONString() string {
	return GetGroupRatioSetting().GroupGroupModelRatio.MarshalJSONString()
}

// GetUserGroupModelRatios returns a detached snapshot for one user group.
func GetUserGroupModelRatios(userGroup string) map[string]map[string]float64 {
	groups, _ := GetGroupRatioSetting().GroupGroupModelRatio.Get(userGroup)
	copy := make(map[string]map[string]float64, len(groups))
	for group, models := range groups {
		copy[group] = maps.Clone(models)
	}
	return copy
}

func GetGroupGroupModelRatio(userGroup, usingGroup, publicModelID string) (float64, bool) {
	groups, ok := GetGroupRatioSetting().GroupGroupModelRatio.Get(userGroup)
	if !ok {
		return 0, false
	}
	ratio, ok := groups[usingGroup][publicModelID]
	return ratio, ok
}
