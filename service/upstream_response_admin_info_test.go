package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendUpstreamResponseAdminInfoCopiesCapturedIdentifiers(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(common.UpstreamResponseIdKey, "gen_upstream")
	c.Set(common.UpstreamResponseHeadersKey, map[string]string{
		"X-Request-Id": "request-secret",
		"X-Trace-Id":   "trace-secret",
	})
	adminInfo := map[string]interface{}{"use_channel": []int{9}}

	AppendUpstreamResponseAdminInfo(c, adminInfo)

	assert.Equal(t, "gen_upstream", adminInfo["upstream_response_id"])
	headers, ok := adminInfo["upstream_request_ids"].(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "request-secret", headers["X-Request-Id"])
	assert.Equal(t, "trace-secret", headers["X-Trace-Id"])
	assert.Equal(t, []int{9}, adminInfo["use_channel"])

	headers["X-Trace-Id"] = "changed"
	original, ok := common.GetContextKeyType[map[string]string](c, common.UpstreamResponseHeadersKey)
	require.True(t, ok)
	assert.Equal(t, "trace-secret", original["X-Trace-Id"])
}
