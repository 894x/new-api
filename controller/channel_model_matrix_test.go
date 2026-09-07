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

func TestGetChannelModelMatrixDefaultsAndQueryValidation(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.Channel{Id: 6201, Type: 1, Name: "Enabled", Status: common.ChannelStatusEnabled, Models: "public-model", Group: "default", Key: "private-key"}).Error)
	require.NoError(t, db.Create(&model.Channel{Id: 6202, Type: 1, Name: "Disabled", Status: common.ChannelStatusManuallyDisabled, Models: "disabled-model", Group: "default"}).Error)
	for _, tt := range []struct {
		name     string
		query    string
		success  bool
		channels int
	}{
		{"defaults", "", true, 1},
		{"all channels", "?status=all", true, 2},
		{"channel id filter", "?channel=6201", true, 1},
		{"oversized model page", "?model_page_size=101", false, 0},
		{"oversized channel page", "?channel_page_size=51", false, 0},
		{"invalid page", "?model_page=0", false, 0},
		{"invalid status", "?status=unknown", false, 0},
		{"malformed page", "?channel_page=x", false, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/model-matrix"+tt.query, nil)
			GetChannelModelMatrix(ctx)
			var response struct {
				Success bool                     `json:"success"`
				Data    model.ChannelModelMatrix `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.Equal(t, tt.success, response.Success)
			if tt.success {
				assert.Len(t, response.Data.Channels, tt.channels)
				assert.Equal(t, 25, response.Data.ModelPageSize)
				assert.Equal(t, 10, response.Data.ChannelPageSize)
			}
			assert.NotContains(t, recorder.Body.String(), "private-key")
		})
	}
}
