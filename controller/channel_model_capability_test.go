package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetModelChannelCapabilitiesReturnsReadOnlyAggregate(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	priority := int64(4)
	weight := uint(8)
	channel := model.Channel{
		Id:       6203,
		Type:     1,
		Key:      "key",
		Status:   common.ChannelStatusEnabled,
		Name:     "capability-test",
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
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/model-capabilities?model=model-a", nil)

	GetModelChannelCapabilities(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool                           `json:"success"`
		Data    model.ModelChannelCapabilities `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, "model-a", response.Data.Model)
	require.Len(t, response.Data.Channels, 1)
	assert.Equal(t, channel.Id, response.Data.Channels[0].ChannelId)
}
