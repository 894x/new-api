package authz

const (
	ResourceUser = "user"

	ActionCreate      = "create"
	ActionBillingRead = "billing_read"
)

var (
	UserCreate      = Permission{Resource: ResourceUser, Action: ActionCreate}
	UserRead        = Permission{Resource: ResourceUser, Action: ActionRead}
	UserBillingRead = Permission{Resource: ResourceUser, Action: ActionBillingRead}
)

func init() {
	RegisterResource(ResourceDefinition{
		Resource: ResourceUser,
		LabelKey: "Managed User Access",
		Actions: []ActionDefinition{
			{
				Action:         ActionCreate,
				LabelKey:       "Create managed users",
				DescriptionKey: "Create ordinary users that are assigned to you.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionRead,
				LabelKey:       "View managed users",
				DescriptionKey: "View users that are assigned to you.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
			{
				Action:         ActionBillingRead,
				LabelKey:       "View managed billing",
				DescriptionKey: "View billing records for users that are assigned to you.",
				DefaultRoles:   []string{BuiltInRoleAdmin},
			},
		},
	})
}
