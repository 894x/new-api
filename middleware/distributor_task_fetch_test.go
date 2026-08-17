package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestDistributeAllowsDoubaoVideoTaskFetchWithoutSelectingChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET(
		"/api/v3/contents/generations/tasks/:task_id",
		DoubaoVideoRequestConvert(),
		Distribute(),
		func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		},
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v3/contents/generations/tasks/task_public_id",
		nil,
	)

	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusNoContent, recorder.Code)
}
