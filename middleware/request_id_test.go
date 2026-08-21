package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestIdStartsTimingBeforeRequestBodyRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(RequestId())

	var snapshot common.RequestTimingSnapshot
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		_, err := common.GetRequestBody(c)
		require.NoError(t, err)
		timing := common.GetRequestTiming(c)
		require.NotNil(t, timing)
		snapshot = timing.Snapshot()
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"test"}`))
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusNoContent, recorder.Code)
	require.Positive(t, snapshot.RequestReceivedAtMs)
	require.Positive(t, snapshot.RequestBodyReadAtMs)
	assert.LessOrEqual(t, snapshot.RequestReceivedAtMs, snapshot.RequestBodyReadAtMs)
}
