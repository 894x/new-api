package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/dynamic_routing_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupDynamicRoutingOptionControllerTest(t *testing.T) {
	t.Helper()

	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousOptionMap := common.OptionMap
	previousRedisEnabled := common.RedisEnabled
	originalSetting := dynamic_routing_setting.GetSetting()

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.Log{}))
	model.DB = db
	model.LOG_DB = db
	common.OptionMap = map[string]string{}
	common.RedisEnabled = false

	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.OptionMap = previousOptionMap
		common.RedisEnabled = previousRedisEnabled
		require.NoError(t, dynamic_routing_setting.ReplaceAndSync(originalSetting))
	})
}

func TestUpdateDynamicRoutingOptionsPublishesOneCompleteSetting(t *testing.T) {
	setupDynamicRoutingOptionControllerTest(t)

	next := dynamic_routing_setting.GetSetting()
	next.Enabled = true
	next.MaxSamples = 80
	next.MinSamples = 4
	next.ProbeFraction = 0.02
	next.Aggressiveness = 0.95
	body, err := common.Marshal(next)
	require.NoError(t, err)
	before := dynamic_routing_setting.GetSnapshot()

	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPut, "/api/option/dynamic_routing", strings.NewReader(string(body)))

	UpdateDynamicRoutingOptions(context)

	assert.Equal(t, http.StatusOK, response.Code)
	var payload struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	assert.True(t, payload.Success)
	assert.Equal(t, before.Version+1, dynamic_routing_setting.GetSnapshot().Version)
	assert.Equal(t, next, dynamic_routing_setting.GetSetting())

	var count int64
	require.NoError(t, model.DB.Model(&model.Option{}).
		Where("key LIKE ?", dynamic_routing_setting.OptionPrefix+"%").
		Count(&count).Error)
	assert.Equal(t, int64(len(dynamic_routing_setting.ToOptionValues(next))), count)
}

func TestUpdateDynamicRoutingOptionsRejectsPartialInvalidBodyWithoutPublishing(t *testing.T) {
	setupDynamicRoutingOptionControllerTest(t)
	before := dynamic_routing_setting.GetSnapshot()

	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPut, "/api/option/dynamic_routing", strings.NewReader(`{"enabled":true}`))

	UpdateDynamicRoutingOptions(context)

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Equal(t, before, dynamic_routing_setting.GetSnapshot())
	var count int64
	require.NoError(t, model.DB.Model(&model.Option{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestUpdateDynamicRoutingOptionsRequiresEveryFieldWhenDisabled(t *testing.T) {
	setupDynamicRoutingOptionControllerTest(t)
	before := dynamic_routing_setting.GetSnapshot()

	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/option/dynamic_routing",
		strings.NewReader(`{"enabled":false,"max_samples":60,"max_age_seconds":90,"min_samples":3}`),
	)

	UpdateDynamicRoutingOptions(context)

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Equal(t, before, dynamic_routing_setting.GetSnapshot())
	var count int64
	require.NoError(t, model.DB.Model(&model.Option{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestUpdateOptionRejectsSingleDynamicRoutingField(t *testing.T) {
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/option/",
		strings.NewReader(`{"key":"dynamic_routing_setting.enabled","value":true}`),
	)

	UpdateOption(context)

	assert.Equal(t, http.StatusBadRequest, response.Code)
	var payload struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	assert.False(t, payload.Success)
	assert.Contains(t, payload.Message, "atomically")
}
