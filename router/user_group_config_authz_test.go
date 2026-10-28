package router

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerGroupConfigurationAuthorization(t *testing.T) {
	f := setupManagedUserE2E(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}, &model.Channel{}, &model.Ability{}, &model.ChannelModelOverride{}, &model.UserChannelRoutingOverride{}, &model.UserGroupChannelRoutingOverride{}, &model.AuditLog{}))
	versionQuery := "SELECT VERSION()"
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		versionQuery = "SELECT sqlite_version()"
	}
	var databaseVersion string
	require.NoError(t, model.DB.Raw(versionQuery).Scan(&databaseVersion).Error)
	t.Logf("Database: %s %s", common.MainDatabaseType(), databaseVersion)
	f.managerA.Role = common.RoleAdminUser
	require.NoError(t, model.DB.Model(&f.managerA).Update("role", f.managerA.Role).Error)
	f.unprivileged.Group = "customer-authz"
	require.NoError(t, model.DB.Model(&f.unprivileged).Update("group", f.unprivileged.Group).Error)
	peer := model.User{Username: "group-config-peer", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "group-config-peer"}
	require.NoError(t, model.DB.Create(&peer).Error)
	shared := model.User{Username: "group-config-shared", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: f.unprivileged.Group, AffCode: "group-config-shared"}
	require.NoError(t, model.DB.Create(&shared).Error)
	channel := model.Channel{Name: "customer-group-channel", Key: "unused", Models: "gpt-5", Group: "default", Status: common.ChannelStatusEnabled}
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "gpt-5", ChannelId: channel.Id, Enabled: true}).Error)

	oldDiscounts := ratio_setting.GroupGroupRatio2JSONString()
	oldModelDiscounts := ratio_setting.GroupGroupModelRatio2JSONString()
	oldRates := setting.ModelRequestRateLimitGroup2JSONString()
	oldChannels := setting.GroupModelChannelGroupsJSON()
	oldSpecialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.MarshalJSONString()
	common.OptionMapRWMutex.Lock()
	oldOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(oldDiscounts))
		require.NoError(t, ratio_setting.UpdateGroupGroupModelRatioByJSONString(oldModelDiscounts))
		require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(oldRates))
		require.NoError(t, setting.UpdateGroupModelChannelGroups(oldChannels))
		require.NoError(t, types.LoadFromJsonString(ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup, oldSpecialGroups))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptions
		common.OptionMapRWMutex.Unlock()
	})
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupModelRatioByJSONString(`{}`))
	require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(`{}`))
	require.NoError(t, setting.UpdateGroupModelChannelGroups(`{}`))
	require.NoError(t, types.LoadFromJsonString(ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup, `{}`))

	base := fmt.Sprintf("/api/user/%d", f.unprivileged.Id)
	endpoints := []struct{ method, path string }{
		{http.MethodGet, "/group-config"},
		{http.MethodGet, "/channel-routing-overrides"},
		{http.MethodPost, "/group-config/preview"},
		{http.MethodPut, "/group-config"},
		{http.MethodPatch, "/channel-routing-overrides"},
		{http.MethodPost, "/group-config/dedicated"},
	}
	catalogStatus, catalogResponse := f.request(t, http.MethodGet, "/api/authz/catalog", f.root.Id, nil)
	require.Equal(t, http.StatusOK, catalogStatus, "%v", catalogResponse)
	resources := catalogResponse["data"].(map[string]any)["resources"].([]any)
	foundResource := false
	for _, entry := range resources {
		resource := entry.(map[string]any)
		if resource["resource"] != authz.ResourceUserGroupConfig {
			continue
		}
		foundResource = true
		assert.Equal(t, "Customer group configuration", resource["label_key"])
		actions := []string{}
		for _, entry := range resource["actions"].([]any) {
			actions = append(actions, entry.(map[string]any)["action"].(string))
		}
		assert.ElementsMatch(t, []string{authz.ActionRead, authz.ActionWrite}, actions)
	}
	require.True(t, foundResource, "the permission editor catalog must include customer group configuration")
	for _, endpoint := range endpoints {
		status, response := f.request(t, endpoint.method, base+endpoint.path, f.managerA.Id, nil)
		assert.Equal(t, http.StatusForbidden, status, "%s %s: %v", endpoint.method, endpoint.path, response)
	}
	assert.False(t, authz.Can(f.managerA.Id, f.managerA.Role, authz.UserGroupConfigRead), "administrators must receive an explicit grant")
	assert.False(t, authz.Can(f.managerA.Id, f.managerA.Role, authz.UserGroupConfigWrite))

	setCustomerGroupPermissions(t, f, f.managerA, false, true)
	for _, endpoint := range endpoints {
		status, _ := f.request(t, endpoint.method, base+endpoint.path, f.managerA.Id, nil)
		assert.Equal(t, http.StatusForbidden, status, "write alone cannot access %s %s", endpoint.method, endpoint.path)
	}
	setCustomerGroupPermissions(t, f, f.managerA, true, false)
	status, response := f.request(t, http.MethodGet, base+"/group-config", f.managerA.Id, nil)
	require.Equal(t, http.StatusOK, status, "%v", response)
	require.Equal(t, true, response["success"], "%v", response)
	config := response["data"].(map[string]any)
	assert.EqualValues(t, 2, config["group_user_count"], "the explicitly granted resource includes shared group configuration")
	status, response = f.request(t, http.MethodGet, base+"/channel-routing-overrides?model=gpt-5&routing_group=default", f.managerA.Id, nil)
	require.Equal(t, http.StatusOK, status, "%v", response)
	require.Equal(t, true, response["success"], "%v", response)
	routing := response["data"].(map[string]any)
	status, response = f.request(t, http.MethodPost, base+"/group-config/preview", f.managerA.Id, map[string]any{"group": f.unprivileged.Group, "model_channel_groups": map[string][]string{"gpt-5": {"default"}}})
	require.Equal(t, http.StatusOK, status, "%v", response)
	assert.Equal(t, true, response["success"])
	for _, endpoint := range endpoints[3:] {
		status, _ := f.request(t, endpoint.method, base+endpoint.path, f.managerA.Id, nil)
		assert.Equal(t, http.StatusForbidden, status, "read only cannot mutate %s", endpoint.path)
	}

	setCustomerGroupPermissions(t, f, f.managerA, true, true)
	for _, target := range []model.User{peer, f.root} {
		for _, endpoint := range endpoints {
			status, _ := f.request(t, endpoint.method, fmt.Sprintf("/api/user/%d%s", target.Id, endpoint.path), f.managerA.Id, nil)
			assert.Equal(t, http.StatusForbidden, status, "admin cannot target role %d at %s %s", target.Role, endpoint.method, endpoint.path)
		}
	}
	// Even policies injected outside the ordinary-user permission editor cannot
	// promote an ordinary caller into this group-wide administrator capability.
	require.NoError(t, authz.SetUserPermissions(f.managerB.Id, authz.PermissionsMap{authz.ResourceUserGroupConfig: {authz.ActionRead: true, authz.ActionWrite: true}}))
	for _, endpoint := range endpoints {
		status, _ := f.request(t, endpoint.method, base+endpoint.path, f.managerB.Id, nil)
		assert.Equal(t, http.StatusForbidden, status, "ordinary caller cannot access %s %s", endpoint.method, endpoint.path)
	}

	status, response = f.request(t, http.MethodPut, base+"/group-config", f.managerA.Id, map[string]any{
		"group": f.unprivileged.Group, "revision": config["revision"], "discounts": map[string]float64{"default": 0.75},
		"model_discounts":    map[string]map[string]float64{"default": {"gpt-5": 0.6}},
		"rate_limit_enabled": true, "rate_limit": map[string]any{"limits": [3]int{20, 10, 5000}, "models": map[string]any{}},
		"model_channel_groups": map[string][]string{"gpt-5": {"default"}},
	})
	require.Equal(t, http.StatusOK, status, "%v", response)
	require.Equal(t, true, response["success"], "%v", response)
	config = response["data"].(map[string]any)
	assert.JSONEq(t, `{"customer-authz":{"default":0.75}}`, ratio_setting.GroupGroupRatio2JSONString())
	var option model.Option
	require.NoError(t, model.DB.Where(&model.Option{Key: "GroupGroupRatio"}).First(&option).Error)
	assert.JSONEq(t, `{"customer-authz":{"default":0.75}}`, option.Value)
	status, response = f.request(t, http.MethodGet, fmt.Sprintf("/api/user/%d/group-config", shared.Id), f.managerA.Id, nil)
	require.Equal(t, http.StatusOK, status, "%v", response)
	assert.Equal(t, map[string]any{"default": 0.75}, response["data"].(map[string]any)["discounts"], "a shared-group grant intentionally applies to all members")
	status, response = f.request(t, http.MethodPatch, base+"/channel-routing-overrides", f.managerA.Id, map[string]any{
		"model": "gpt-5", "routing_group": "default", "revision": routing["revision"],
		"overrides": []map[string]any{{"channel_id": channel.Id, "priority_override": 9, "enabled": true}},
	})
	require.Equal(t, http.StatusOK, status, "%v", response)
	require.Equal(t, true, response["success"], "%v", response)
	overrides, err := model.ListUserGroupChannelRoutingOverrides(f.unprivileged.Id, "gpt-5")
	require.NoError(t, err)
	require.Len(t, overrides, 1)
	require.NotNil(t, overrides[0].Priority)
	assert.EqualValues(t, 9, *overrides[0].Priority)

	status, response = f.request(t, http.MethodPost, base+"/group-config/dedicated", f.managerA.Id, map[string]any{"group": f.unprivileged.Group, "revision": config["revision"]})
	require.Equal(t, http.StatusOK, status, "%v", response)
	require.Equal(t, true, response["success"], "%v", response)
	assert.Equal(t, true, response["data"].(map[string]any)["is_dedicated_group"])
	updated, err := model.GetUserById(f.unprivileged.Id, false)
	require.NoError(t, err)
	assert.Equal(t, "企业客户-"+f.unprivileged.Username, updated.Group)
	var auditEvents []model.AuditLog
	require.NoError(t, model.DB.Where("user_id = ? AND action IN ?", f.managerA.Id, []string{"user.group_config.update", "user.channel_routing.update", "user.group_config.create_dedicated"}).Find(&auditEvents).Error)
	require.Len(t, auditEvents, 3)
	for _, event := range auditEvents {
		assert.Equal(t, common.RoleAdminUser, event.ActorRole)
		assert.True(t, event.Success)
		require.NotNil(t, event.Other.Op)
		encodedParams, err := common.Marshal(event.Other.Op.Params)
		require.NoError(t, err)
		var params struct {
			TargetUserID int `json:"target_user_id"`
		}
		require.NoError(t, common.Unmarshal(encodedParams, &params))
		assert.Equal(t, f.unprivileged.Id, params.TargetUserID)
	}

	setCustomerGroupPermissions(t, f, f.managerA, false, false)
	for _, endpoint := range endpoints {
		status, _ := f.request(t, endpoint.method, base+endpoint.path, f.managerA.Id, nil)
		assert.Equal(t, http.StatusForbidden, status, "revocation applies to the existing session at %s %s", endpoint.method, endpoint.path)
	}
	status, response = f.request(t, http.MethodGet, base+"/group-config", f.root.Id, nil)
	require.Equal(t, http.StatusOK, status, "%v", response)
	require.Equal(t, true, response["success"], "%v", response)
	config = response["data"].(map[string]any)
	status, response = f.request(t, http.MethodGet, base+"/channel-routing-overrides?model=gpt-5&routing_group=default", f.root.Id, nil)
	require.Equal(t, http.StatusOK, status, "%v", response)
	require.Equal(t, true, response["success"], "%v", response)
	routing = response["data"].(map[string]any)
	status, response = f.request(t, http.MethodPost, base+"/group-config/preview", f.root.Id, map[string]any{"group": updated.Group, "model_channel_groups": map[string][]string{"gpt-5": {"default"}}})
	require.Equal(t, http.StatusOK, status, "%v", response)
	require.Equal(t, true, response["success"], "%v", response)
	status, response = f.request(t, http.MethodPut, base+"/group-config", f.root.Id, config)
	require.Equal(t, http.StatusOK, status, "%v", response)
	require.Equal(t, true, response["success"], "%v", response)
	config = response["data"].(map[string]any)
	status, response = f.request(t, http.MethodPatch, base+"/channel-routing-overrides", f.root.Id, map[string]any{
		"model": "gpt-5", "routing_group": "default", "revision": routing["revision"],
		"overrides": []map[string]any{{"channel_id": channel.Id, "priority_override": 0, "enabled": true}},
	})
	require.Equal(t, http.StatusOK, status, "%v", response)
	require.Equal(t, true, response["success"], "%v", response)
	status, response = f.request(t, http.MethodPost, base+"/group-config/dedicated", f.root.Id, map[string]any{"group": updated.Group, "revision": config["revision"]})
	require.Equal(t, http.StatusOK, status, "%v", response)
	assert.Equal(t, true, response["success"])
	status, response = f.request(t, http.MethodGet, fmt.Sprintf("/api/user/%d/group-config", peer.Id), f.root.Id, nil)
	assert.Equal(t, http.StatusOK, status, "%v", response)
	assert.Equal(t, true, response["success"])
}

func setCustomerGroupPermissions(t *testing.T, f *managedUserE2EFixture, target model.User, read, write bool) {
	t.Helper()
	status, response := f.request(t, http.MethodPut, "/api/user/", f.root.Id, map[string]any{
		"id": target.Id, "username": target.Username, "display_name": target.DisplayName, "role": target.Role, "group": target.Group,
		"admin_permissions": map[string]map[string]bool{authz.ResourceUserGroupConfig: {authz.ActionRead: read, authz.ActionWrite: write}},
	})
	require.Equal(t, http.StatusOK, status, "%v", response)
	require.Equal(t, true, response["success"], "%v", response)
	status, response = f.request(t, http.MethodGet, "/api/user/self", target.Id, nil)
	require.Equal(t, http.StatusOK, status, "%v", response)
	permissions := response["data"].(map[string]any)["permissions"].(map[string]any)["admin_permissions"].(map[string]any)
	assert.Equal(t, map[string]any{authz.ActionRead: read, authz.ActionWrite: write}, permissions[authz.ResourceUserGroupConfig])
}
