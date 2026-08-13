package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPatchChannelModelRoutingOverridesAcceptsExplicitZeroAndNull(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	priority := int64(4)
	weight := uint(8)
	channel := model.Channel{
		Id:       6101,
		Type:     1,
		Key:      "key",
		Status:   common.ChannelStatusEnabled,
		Name:     "routing-test",
		Models:   "model-a",
		Group:    "default",
		Priority: &priority,
		Weight:   &weight,
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "6101"}}
	ctx.Request = httptest.NewRequest(
		http.MethodPatch,
		"/api/channel/6101/model-routing-overrides",
		bytes.NewBufferString(`{"overrides":[{"model":"model-a","priority_override":0,"weight_override":null}]}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")

	PatchChannelModelRoutingOverrides(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool                        `json:"success"`
		Data    []model.ChannelModelRouting `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	require.Len(t, response.Data, 1)
	require.NotNil(t, response.Data[0].PriorityOverride)
	assert.Equal(t, int64(0), *response.Data[0].PriorityOverride)
	assert.Nil(t, response.Data[0].WeightOverride)
	assert.Equal(t, int64(0), response.Data[0].EffectivePriority)
	assert.Equal(t, uint(8), response.Data[0].EffectiveWeight)
	var auditLog model.Log
	require.NoError(t, db.Order("id DESC").First(&auditLog).Error)
	assert.Contains(t, auditLog.Other, `"action":"channel.model_routing_override"`)
}

func TestPatchModelChannelRoutingOverridesAppliesExactModelAcrossChannels(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	priority := int64(4)
	weight := uint(8)
	for _, channelId := range []int{6102, 6103} {
		channel := model.Channel{
			Id:       channelId,
			Type:     1,
			Key:      "key",
			Status:   common.ChannelStatusEnabled,
			Name:     "routing-test",
			Models:   "model-a,model-b",
			Group:    "default",
			Priority: &priority,
			Weight:   &weight,
		}
		require.NoError(t, db.Create(&channel).Error)
		require.NoError(t, channel.AddAbilities(nil))
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPatch,
		"/api/channel/model-routing-overrides?model=model-a",
		bytes.NewBufferString(`{"overrides":[{"channel_id":6102,"priority_override":0,"weight_override":null},{"channel_id":6103,"model":"model-a","priority_override":null,"weight_override":0}]}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")
	PatchModelChannelRoutingOverrides(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool                        `json:"success"`
		Data    []model.ChannelModelRouting `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	require.Len(t, response.Data, 2)
	for _, routing := range response.Data {
		assert.Equal(t, "model-a", routing.Model)
	}
	var modelBOverrideCount int64
	require.NoError(t, db.Model(&model.ChannelModelOverride{}).Where("model = ?", "model-b").Count(&modelBOverrideCount).Error)
	assert.Zero(t, modelBOverrideCount)
	var auditCount int64
	require.NoError(t, db.Model(&model.Log{}).Where("other LIKE ?", `%model.channel_routing_override%`).Count(&auditCount).Error)
	assert.Equal(t, int64(1), auditCount)
}

func TestPatchModelChannelRoutingOverridesRejectsMismatchedModelWithoutWrites(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	priority := int64(4)
	weight := uint(8)
	channel := model.Channel{
		Id:       6104,
		Type:     1,
		Key:      "key",
		Status:   common.ChannelStatusEnabled,
		Name:     "routing-test",
		Models:   "model-a,model-b",
		Group:    "default",
		Priority: &priority,
		Weight:   &weight,
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPatch,
		"/api/channel/model-routing-overrides?model=model-a",
		bytes.NewBufferString(`{"overrides":[{"channel_id":6104,"model":"model-b","priority_override":0,"weight_override":null}]}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")
	PatchModelChannelRoutingOverrides(ctx)

	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	var count int64
	require.NoError(t, db.Model(&model.ChannelModelOverride{}).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, db.Model(&model.Log{}).Count(&count).Error)
	assert.Zero(t, count)
}
