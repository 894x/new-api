package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiniMaxVideoV2MiddlewareErrorKeepsRequestIDOutOfMessage(t *testing.T) {
	previousHideErrorDetails := operation_setting.ShouldHideErrorDetails()
	operation_setting.UpdateHideErrorDetails(false)
	t.Cleanup(func() { operation_setting.UpdateHideErrorDetails(previousHideErrorDetails) })

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set(common.RequestIdKey, "local-request-id")

	abortWithMiniMaxVideoV2Error(context, http.StatusBadRequest, "bad_request_error", "Invalid request body")

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	var payload struct {
		RequestID string `json:"request_id"`
		Error     struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	assert.Equal(t, "local-request-id", payload.RequestID)
	assert.Equal(t, "Invalid request body", payload.Error.Message)
}
