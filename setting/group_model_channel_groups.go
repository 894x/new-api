package setting

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

const GroupModelChannelGroupsOptionKey = "GroupModelChannelGroups"

// GroupModelChannelGroups maps user group -> public model -> channel pool
// groups. Their channels form a union, intersected with the token's candidates.
// Missing models are unrestricted; an empty list denies the model.
type GroupModelChannelGroups map[string]map[string][]string

var groupModelChannelGroups = struct {
	sync.RWMutex
	policies GroupModelChannelGroups
}{policies: GroupModelChannelGroups{}}

func ParseGroupModelChannelGroups(value string) (GroupModelChannelGroups, error) {
	var policies GroupModelChannelGroups
	if err := common.UnmarshalJsonStr(value, &policies); err != nil {
		return nil, err
	}
	if policies == nil {
		return nil, fmt.Errorf("channel group policies must be an object")
	}
	for userGroup, models := range policies {
		if userGroup == "" || strings.TrimSpace(userGroup) != userGroup || userGroup == "auto" || userGroup == "__proto__" || models == nil {
			return nil, fmt.Errorf("invalid user group policy: %q", userGroup)
		}
		for modelName, groups := range models {
			if modelName == "" || strings.TrimSpace(modelName) != modelName || modelName == "__proto__" || groups == nil {
				return nil, fmt.Errorf("model %q in group %q must have an array of channel groups", modelName, userGroup)
			}
			seen := make(map[string]bool, len(groups))
			for _, group := range groups {
				if group == "" || strings.TrimSpace(group) != group || group == "auto" || group == "__proto__" || seen[group] {
					return nil, fmt.Errorf("invalid or duplicate channel group %q for model %q", group, modelName)
				}
				seen[group] = true
			}
		}
	}
	return policies, nil
}

func UpdateGroupModelChannelGroups(value string) error {
	policies, err := ParseGroupModelChannelGroups(value)
	if err != nil {
		return err
	}
	groupModelChannelGroups.Lock()
	defer groupModelChannelGroups.Unlock()
	groupModelChannelGroups.policies = policies
	return nil
}

func GroupModelChannelGroupsJSON() string {
	groupModelChannelGroups.RLock()
	defer groupModelChannelGroups.RUnlock()
	value, _ := common.Marshal(groupModelChannelGroups.policies)
	return string(value)
}

func GetGroupModelChannelGroups(userGroup, modelName string) ([]string, bool) {
	groupModelChannelGroups.RLock()
	defer groupModelChannelGroups.RUnlock()
	groups, configured := groupModelChannelGroups.policies[userGroup][modelName]
	return slices.Clone(groups), configured
}
