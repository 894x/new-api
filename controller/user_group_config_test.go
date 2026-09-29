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
package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateUserGroupConfigReplacesOnlyTheSelectedUserGroup(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.Ability{}))
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCacheEnabled })

	originalDiscounts := ratio_setting.GroupGroupRatio2JSONString()
	originalRateLimits := setting.ModelRequestRateLimitGroup2JSONString()
	originalChannels := setting.GroupModelChannelGroupsJSON()
	originalSpecialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.MarshalJSONString()
	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(originalDiscounts))
		require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(originalRateLimits))
		require.NoError(t, setting.UpdateGroupModelChannelGroups(originalChannels))
		require.NoError(t, types.LoadFromJsonString(ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup, originalSpecialGroups))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"other":{"default":0.95},"customer":{"old":0.8}}`))
	require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(`{"other":[1,1,1],"customer":[2,2,2]}`))
	require.NoError(t, setting.UpdateGroupModelChannelGroups(`{"other":{"model-x":["pool-x"]},"customer":{"old-model":["old-pool"]}}`))
	require.NoError(t, types.LoadFromJsonString(ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup, `{"customer":{"+:vip":"VIP"}}`))

	root := model.User{Username: "root-config-test", Role: common.RoleRootUser, Group: "default", Status: common.UserStatusEnabled, AffCode: "root-config-aff"}
	target := model.User{Username: "customer-config-test", Role: common.RoleCommonUser, Group: "customer", Status: common.UserStatusEnabled, AuthVersion: 1, AffCode: "customer-config-aff"}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&target).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "pool-a", Model: "gpt-5", ChannelId: 101, Enabled: true},
		{Group: "pool-b", Model: "gpt-5", ChannelId: 101, Enabled: true},
		{Group: "pool-b", Model: "gpt-5", ChannelId: 102, Enabled: true},
		{Group: "pool-b", Model: "gpt-5", ChannelId: 103, Enabled: false},
	}).Error)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.UserSession{
		SID: "customer-config-session", UserID: target.Id, Version: 1, UserAuthVersion: 1,
		Status: model.UserSessionStatusActive, RefreshHash: "refresh-hash", LoginMethod: "password",
		LastActiveAt: now, ExpiresAt: now + 3600,
	}).Error)

	gin.SetMode(gin.TestMode)
	previewRecorder := httptest.NewRecorder()
	previewContext, _ := gin.CreateTestContext(previewRecorder)
	previewContext.Params = gin.Params{{Key: "id", Value: strconv.Itoa(target.Id)}}
	previewContext.Request = httptest.NewRequest(http.MethodPost, "/api/user/group-config/preview", strings.NewReader(`{"group":"customer","model_channel_groups":{"gpt-5":["pool-a","pool-b"],"blocked":[]}}`))
	PreviewUserGroupChannels(previewContext)
	assert.Equal(t, http.StatusOK, previewRecorder.Code)
	assert.JSONEq(t, `{"success":true,"message":"","data":{"gpt-5":{"candidate_count":2},"blocked":{"candidate_count":0}}}`, previewRecorder.Body.String())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(target.Id)}}
	initial, err := buildUserGroupConfig(target.Id)
	require.NoError(t, err)
	assert.Equal(t, "企业客户-"+target.Username, initial.DedicatedGroupName)
	assert.False(t, initial.IsDedicatedGroup)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/user/group-config", strings.NewReader(fmt.Sprintf(`{
  "group":"customer",
  "revision":%q,
  "discounts":{"default":0.7,"vip":0.6},
  "rate_limit_enabled":true,
  "rate_limit":{"limits":[10,8,5000],"models":{"gpt-5":{"rpm":3,"tpm":1000}}},
  "model_channel_groups":{"gpt-5":["pool-a","pool-b"],"blocked":[]}
}`, initial.Revision)))
	ctx.Set("id", root.Id)
	ctx.Set("role", common.RoleRootUser)

	UpdateUserGroupConfig(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.JSONEq(t, `{"other":{"default":0.95},"customer":{"default":0.7,"vip":0.6}}`, ratio_setting.GroupGroupRatio2JSONString())
	assert.JSONEq(t, `{"other":[1,1,1],"customer":{"limits":[10,8,5000],"models":{"gpt-5":{"rpm":3,"tpm":1000}}}}`, setting.ModelRequestRateLimitGroup2JSONString())
	assert.JSONEq(t, `{"other":{"model-x":["pool-x"]},"customer":{"gpt-5":["pool-a","pool-b"],"blocked":[]}}`, setting.GroupModelChannelGroupsJSON())

	var options []model.Option
	require.NoError(t, db.Where("key IN ?", []string{"GroupGroupRatio", "ModelRequestRateLimitGroup", setting.GroupModelChannelGroupsOptionKey}).Find(&options).Error)
	require.Len(t, options, 3)
	stored := make(map[string]string, len(options))
	for _, option := range options {
		stored[option.Key] = option.Value
	}
	assert.JSONEq(t, ratio_setting.GroupGroupRatio2JSONString(), stored["GroupGroupRatio"])
	assert.JSONEq(t, setting.ModelRequestRateLimitGroup2JSONString(), stored["ModelRequestRateLimitGroup"])
	assert.JSONEq(t, setting.GroupModelChannelGroupsJSON(), stored[setting.GroupModelChannelGroupsOptionKey])

	configured, err := buildUserGroupConfig(target.Id)
	require.NoError(t, err)
	stalePolicyRecorder := httptest.NewRecorder()
	stalePolicyContext, _ := gin.CreateTestContext(stalePolicyRecorder)
	stalePolicyContext.Params = gin.Params{{Key: "id", Value: strconv.Itoa(target.Id)}}
	stalePolicyContext.Request = httptest.NewRequest(http.MethodPut, "/api/user/group-config", strings.NewReader(fmt.Sprintf(`{"group":"customer","revision":%q,"discounts":{},"rate_limit_enabled":false,"rate_limit":{"limits":[0,1,0]},"model_channel_groups":{}}`, initial.Revision)))
	UpdateUserGroupConfig(stalePolicyContext)
	assert.Equal(t, http.StatusConflict, stalePolicyRecorder.Code)
	assert.JSONEq(t, `{"other":{"default":0.95},"customer":{"default":0.7,"vip":0.6}}`, ratio_setting.GroupGroupRatio2JSONString())

	previousAuthVersion := target.AuthVersion
	createdRecorder := httptest.NewRecorder()
	createdContext, _ := gin.CreateTestContext(createdRecorder)
	createdContext.Params = gin.Params{{Key: "id", Value: strconv.Itoa(target.Id)}}
	createdContext.Request = httptest.NewRequest(http.MethodPost, "/api/user/group-config/dedicated", strings.NewReader(fmt.Sprintf(`{"group":"customer","revision":%q}`, configured.Revision)))
	createdContext.Set("id", root.Id)
	createdContext.Set("role", common.RoleRootUser)

	CreateDedicatedUserGroup(createdContext)

	assert.Equal(t, http.StatusOK, createdRecorder.Code, createdRecorder.Body.String())
	updatedUser, err := model.GetUserById(target.Id, false)
	require.NoError(t, err)
	assert.Equal(t, "企业客户-"+target.Username, updatedUser.Group)
	createdConfig, err := buildUserGroupConfig(target.Id)
	require.NoError(t, err)
	assert.True(t, createdConfig.IsDedicatedGroup)
	assert.Greater(t, updatedUser.AuthVersion, previousAuthVersion)
	var session model.UserSession
	require.NoError(t, db.First(&session, "sid = ?", "customer-config-session").Error)
	assert.Equal(t, model.UserSessionStatusRevoked, session.Status)
	assert.JSONEq(t, `{"other":{"default":0.95},"customer":{"default":0.7,"vip":0.6},"企业客户-customer-config-test":{"default":0.7,"vip":0.6}}`, ratio_setting.GroupGroupRatio2JSONString())
	assert.JSONEq(t, `{"other":[1,1,1],"customer":{"limits":[10,8,5000],"models":{"gpt-5":{"rpm":3,"tpm":1000}}},"企业客户-customer-config-test":{"limits":[10,8,5000],"models":{"gpt-5":{"rpm":3,"tpm":1000}}}}`, setting.ModelRequestRateLimitGroup2JSONString())
	assert.JSONEq(t, `{"other":{"model-x":["pool-x"]},"customer":{"gpt-5":["pool-a","pool-b"],"blocked":[]},"企业客户-customer-config-test":{"gpt-5":["pool-a","pool-b"],"blocked":[]}}`, setting.GroupModelChannelGroupsJSON())
	assert.JSONEq(t, `{"customer":{"+:vip":"VIP"},"企业客户-customer-config-test":{"+:vip":"VIP"}}`, ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.MarshalJSONString())
	assert.False(t, ratio_setting.ContainsGroupRatio(updatedUser.Group))
	assert.Equal(t, 0.7, service.GetUserGroupRatio(updatedUser.Group, "default"))
	rpm, success, tpm, windowSeconds := setting.ResolveGroupModelRateLimit(updatedUser.Group, "gpt-5")
	assert.Equal(t, 3, rpm)
	assert.Zero(t, success)
	assert.Equal(t, 1000, tpm)
	assert.Equal(t, int64(60), windowSeconds)
	allowedChannels, err := service.GroupModelAllowedChannelIDs(updatedUser.Group, "gpt-5")
	require.NoError(t, err)
	assert.Equal(t, map[int]struct{}{101: {}, 102: {}}, allowedChannels)
	blockedChannels, err := service.GroupModelAllowedChannelIDs(updatedUser.Group, "blocked")
	require.NoError(t, err)
	assert.Empty(t, blockedChannels)
	assert.NotNil(t, blockedChannels, "an empty policy must deny the model")

	staleRecorder := httptest.NewRecorder()
	staleContext, _ := gin.CreateTestContext(staleRecorder)
	staleContext.Params = gin.Params{{Key: "id", Value: strconv.Itoa(target.Id)}}
	staleContext.Request = httptest.NewRequest(http.MethodPut, "/api/user/group-config", strings.NewReader(fmt.Sprintf(`{"group":"customer","revision":%q,"discounts":{},"rate_limit_enabled":false,"rate_limit":{"limits":[0,1,0]},"model_channel_groups":{}}`, configured.Revision)))
	UpdateUserGroupConfig(staleContext)
	assert.Equal(t, http.StatusConflict, staleRecorder.Code)

	rootConfig, err := buildUserGroupConfig(root.Id)
	require.NoError(t, err)
	rootRecorder := httptest.NewRecorder()
	rootContext, _ := gin.CreateTestContext(rootRecorder)
	rootContext.Params = gin.Params{{Key: "id", Value: strconv.Itoa(root.Id)}}
	rootContext.Request = httptest.NewRequest(http.MethodPost, "/api/user/group-config/dedicated", strings.NewReader(fmt.Sprintf(`{"group":"default","revision":%q}`, rootConfig.Revision)))
	CreateDedicatedUserGroup(rootContext)
	assert.Equal(t, http.StatusBadRequest, rootRecorder.Code)
}
