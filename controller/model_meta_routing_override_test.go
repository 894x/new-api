package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelMetadataRenameAndDeleteDoNotMutateChannelRouting(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	priority := int64(1)
	weight := uint(2)
	channel := model.Channel{
		Id:       6201,
		Type:     1,
		Key:      "key",
		Status:   common.ChannelStatusEnabled,
		Name:     "metadata-independent-routing",
		Models:   "model-a",
		Group:    "default",
		Priority: &priority,
		Weight:   &weight,
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	overridePriority := int64(9)
	require.NoError(t, model.PatchChannelModelOverrides([]model.ChannelModelOverridePatch{
		{ChannelId: channel.Id, Model: "model-a", Priority: &overridePriority},
	}))
	metadata := model.Model{ModelName: "model-a", Status: 1, NameRule: model.NameRuleExact}
	require.NoError(t, metadata.Insert())

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/models",
		bytes.NewBufferString(`{"id":`+strconv.Itoa(metadata.Id)+`,"model_name":"model-renamed","status":1,"name_rule":0}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")
	UpdateModelMeta(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var updateResponse struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &updateResponse))
	assert.True(t, updateResponse.Success)

	recorder = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(metadata.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/models/"+strconv.Itoa(metadata.Id), nil)
	DeleteModelMeta(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var persistedChannel model.Channel
	require.NoError(t, db.First(&persistedChannel, channel.Id).Error)
	assert.Equal(t, "model-a", persistedChannel.Models)
	var ability model.Ability
	require.NoError(t, db.Where("channel_id = ? AND model = ?", channel.Id, "model-a").First(&ability).Error)
	var override model.ChannelModelOverride
	require.NoError(t, db.Where("channel_id = ? AND model = ?", channel.Id, "model-a").First(&override).Error)
	require.NotNil(t, override.Priority)
	assert.Equal(t, int64(9), *override.Priority)
	var renamedCount int64
	require.NoError(t, db.Model(&model.ChannelModelOverride{}).Where("model = ?", "model-renamed").Count(&renamedCount).Error)
	assert.Zero(t, renamedCount)
}
