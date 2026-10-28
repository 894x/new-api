package authz

const ResourceUserGroupConfig = "user_group_config"

var (
	UserGroupConfigRead  = Permission{Resource: ResourceUserGroupConfig, Action: ActionRead}
	UserGroupConfigWrite = Permission{Resource: ResourceUserGroupConfig, Action: ActionWrite}
)

func init() {
	RegisterResource(ResourceDefinition{
		Resource: ResourceUserGroupConfig,
		LabelKey: "Customer group configuration",
		Actions: []ActionDefinition{
			{
				Action:         ActionRead,
				LabelKey:       "View customer group configuration",
				DescriptionKey: "View customer group discounts, rate limits, channel policies, and user channel priorities.",
			},
			{
				Action:         ActionWrite,
				LabelKey:       "Edit customer group configuration",
				DescriptionKey: "Edit customer group discounts, rate limits, channel policies, and user channel priorities. Changes to a shared group affect every user in that group. Requires view permission.",
			},
		},
	})
}
