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
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateUserGroupConfigReplacesOnlyTheSelectedUserGroup(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}))

	originalDiscounts := ratio_setting.GroupGroupRatio2JSONString()
	originalRateLimits := setting.ModelRequestRateLimitGroup2JSONString()
	originalChannels := setting.GroupModelChannelGroupsJSON()
	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(originalDiscounts))
		require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(originalRateLimits))
		require.NoError(t, setting.UpdateGroupModelChannelGroups(originalChannels))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"other":{"default":0.95},"customer":{"old":0.8}}`))
	require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(`{"other":[1,1,1],"customer":[2,2,2]}`))
	require.NoError(t, setting.UpdateGroupModelChannelGroups(`{"other":{"model-x":["pool-x"]},"customer":{"old-model":["old-pool"]}}`))

	root := model.User{Username: "root-config-test", Role: common.RoleRootUser, Group: "default", Status: common.UserStatusEnabled, AffCode: "root-config-aff"}
	target := model.User{Username: "customer-config-test", Role: common.RoleCommonUser, Group: "customer", Status: common.UserStatusEnabled, AffCode: "customer-config-aff"}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&target).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(target.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/user/group-config", strings.NewReader(`{
  "group":"customer",
  "discounts":{"default":0.7,"vip":0.6},
  "rate_limit_enabled":true,
  "rate_limit":{"limits":[10,8,5000],"models":{"gpt-5":{"rpm":3,"tpm":1000}}},
  "model_channel_groups":{"gpt-5":["pool-a","pool-b"],"blocked":[]}
}`))
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
}
