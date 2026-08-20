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
	defaultRPM := int64(60)
	defaultTPM := int64(6000)
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
		RPM:      &defaultRPM,
		TPM:      &defaultTPM,
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
		bytes.NewBufferString(`{"overrides":[{"model":"model-a","priority_override":0,"weight_override":null,"rpm_override":0,"tpm_override":null}]}`),
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
	require.NotNil(t, response.Data[0].RPMOverride)
	assert.Equal(t, int64(0), *response.Data[0].RPMOverride)
	assert.Nil(t, response.Data[0].TPMOverride)
	assert.Equal(t, int64(0), response.Data[0].EffectiveRPM)
	assert.Equal(t, int64(6000), response.Data[0].EffectiveTPM)
	var auditLog model.Log
	require.NoError(t, db.Order("id DESC").First(&auditLog).Error)
	assert.Contains(t, auditLog.Other, `"action":"channel.model_routing_override"`)
}

func TestPatchChannelModelRoutingOverridesPreservesNewCapacityFieldsForLegacyClients(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	priority := int64(4)
	weight := uint(8)
	defaultRPM := int64(60)
	defaultTPM := int64(6000)
	channel := model.Channel{
		Id:       6105,
		Type:     1,
		Key:      "key",
		Status:   common.ChannelStatusEnabled,
		Name:     "routing-test",
		Models:   "model-a",
		Group:    "default",
		Priority: &priority,
		Weight:   &weight,
		RPM:      &defaultRPM,
		TPM:      &defaultTPM,
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	modelRPM := int64(30)
	modelTPM := int64(3000)
	require.NoError(t, model.PatchChannelModelOverrides([]model.ChannelModelOverridePatch{{
		ChannelId: channel.Id,
		Model:     "model-a",
		RPM:       &modelRPM,
		TPM:       &modelTPM,
	}}))

	patch := func(body string) []model.ChannelModelRouting {
		t.Helper()
		gin.SetMode(gin.TestMode)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Params = gin.Params{{Key: "id", Value: "6105"}}
		ctx.Request = httptest.NewRequest(
			http.MethodPatch,
			"/api/channel/6105/model-routing-overrides",
			bytes.NewBufferString(body),
		)
		ctx.Request.Header.Set("Content-Type", "application/json")
		PatchChannelModelRoutingOverrides(ctx)
		require.Equal(t, http.StatusOK, recorder.Code)
		var response struct {
			Success bool                        `json:"success"`
			Data    []model.ChannelModelRouting `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		require.True(t, response.Success)
		return response.Data
	}

	routings := patch(`{"overrides":[{"model":"model-a","priority_override":0,"weight_override":null}]}`)
	require.Len(t, routings, 1)
	require.NotNil(t, routings[0].RPMOverride)
	assert.Equal(t, int64(30), *routings[0].RPMOverride)
	require.NotNil(t, routings[0].TPMOverride)
	assert.Equal(t, int64(3000), *routings[0].TPMOverride)

	routings = patch(`{"overrides":[{"model":"model-a","priority_override":0,"weight_override":null,"rpm_override":null,"tpm_override":null}]}`)
	require.Len(t, routings, 1)
	assert.Nil(t, routings[0].RPMOverride)
	assert.Nil(t, routings[0].TPMOverride)
	assert.Equal(t, int64(60), routings[0].EffectiveRPM)
	assert.Equal(t, int64(6000), routings[0].EffectiveTPM)
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
