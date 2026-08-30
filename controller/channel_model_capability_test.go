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
	paramOverride := `{"operations":[{"mode":"set","path":"max_tokens","value":null}]}`
	channel := model.Channel{
		Id:            6203,
		Type:          1,
		Key:           "key",
		Status:        common.ChannelStatusEnabled,
		Name:          "capability-test",
		Models:        "model-a",
		Group:         "default",
		Priority:      &priority,
		Weight:        &weight,
		ParamOverride: &paramOverride,
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/model-capabilities?model=model-a", nil)

	GetModelChannelCapabilities(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"value":null`)
	var response struct {
		Success bool                           `json:"success"`
		Data    model.ModelChannelCapabilities `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, "model-a", response.Data.Model)
	require.Len(t, response.Data.Channels, 1)
	capability := response.Data.Channels[0]
	assert.Equal(t, channel.Id, capability.ChannelId)
	assert.True(t, capability.ParameterOverrideConfigured)
	assert.Equal(t, "operations", capability.ParameterOverrideMode)
	require.Len(t, capability.ParameterOverrideOperations, 1)
	assert.Equal(t, "max_tokens", capability.ParameterOverrideOperations[0].Path)
	assert.True(t, capability.ParameterOverrideOperations[0].ValueConfigured)
}
