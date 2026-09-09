package middleware

import (
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"net/http"
)

// RequestCapture runs after authentication and before distribution/rewrites.
func RequestCapture() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		switch c.Request.URL.Path {
		case "/v1/chat/completions", "/v1/completions", "/v1/responses", "/v1/responses/compact", "/v1/messages":
		default:
			c.Next()
			return
		}
		session := service.BeginRequestCapture(c)
		if session == nil {
			c.Next()
			return
		}
		session.CaptureClientRequest(c)
		finishResponse := session.CaptureClientResponse(c)
		completed := false
		defer func() {
			finishResponse(completed)
			session.Finish()
		}()
		c.Next()
		completed = true
	}
}
