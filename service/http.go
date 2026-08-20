package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

func CloseResponseBodyGracefully(httpResponse *http.Response) {
	if httpResponse == nil || httpResponse.Body == nil {
		return
	}
	err := httpResponse.Body.Close()
	if err != nil {
		common.SysError("failed to close response body: " + err.Error())
	}
}

func captureUpstreamResponseHeader(c *gin.Context, key string, values []string) {
	if c == nil || len(values) == 0 {
		return
	}
	canonicalKey := http.CanonicalHeaderKey(strings.TrimSpace(key))
	if canonicalKey == "" {
		return
	}
	captured, _ := common.GetContextKeyType[map[string]string](c, common.UpstreamResponseHeadersKey)
	if captured == nil {
		captured = make(map[string]string)
		c.Set(common.UpstreamResponseHeadersKey, captured)
	}
	captured[canonicalKey] = common.LimitUpstreamIdentifier(strings.Join(values, ", "))
}

func ResetUpstreamResponseMetadata(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(common.UpstreamRequestIdKey, "")
	c.Set(common.UpstreamResponseIdKey, "")
	c.Set(common.UpstreamResponseHeadersKey, make(map[string]string))
}

// CaptureUpstreamResponseHeaders preserves only the headers that the gateway
// blocks from client responses, so administrators can diagnose upstream calls.
func CaptureUpstreamResponseHeaders(c *gin.Context, headers http.Header) {
	if c == nil {
		return
	}
	for key, values := range headers {
		if strings.EqualFold(key, common.RequestIdKey) {
			if len(values) > 0 {
				c.Set(common.UpstreamRequestIdKey, common.LimitUpstreamRequestIdentifier(values[0]))
			}
			captureUpstreamResponseHeader(c, key, values)
			continue
		}
		if operation_setting.ShouldBlockUpstreamResponseHeader(key) {
			captureUpstreamResponseHeader(c, key, values)
		}
	}
}

// ShouldCopyUpstreamHeader checks whether a given upstream response header
// should be copied to the client response. Content-Length is managed separately;
// the gateway request ID and configured upstream-identifying headers are kept
// private and captured for administrator-only logs.
func ShouldCopyUpstreamHeader(c *gin.Context, k string, v []string) bool {
	if strings.EqualFold(k, "Content-Length") {
		return false
	}
	if strings.EqualFold(k, common.RequestIdKey) {
		if c != nil && len(v) > 0 {
			c.Set(common.UpstreamRequestIdKey, common.LimitUpstreamRequestIdentifier(v[0]))
		}
		captureUpstreamResponseHeader(c, k, v)
		return false
	}
	if operation_setting.ShouldBlockUpstreamResponseHeader(k) {
		captureUpstreamResponseHeader(c, k, v)
		return false
	}
	return true
}

func IOCopyBytesGracefully(c *gin.Context, src *http.Response, data []byte) {
	if c.Writer == nil {
		return
	}

	responseId := common.GetContextKeyString(c, constant.ContextKeyResponseId)
	common.CaptureUpstreamResponseID(c, data, responseId)
	normalizedData, err := common.ReplaceTopLevelJSONID(data, responseId)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("failed to normalize response id: %s", err.Error()))
		return
	}
	bodyChanged := !bytes.Equal(data, normalizedData)
	data = normalizedData

	body := io.NopCloser(bytes.NewBuffer(data))

	// We shouldn't set the header before we parse the response body, because the parse part may fail.
	// And then we will have to send an error response, but in this case, the header has already been set.
	// So the httpClient will be confused by the response.
	// For example, Postman will report error, and we cannot check the response at all.
	if src != nil {
		for k, v := range src.Header {
			if !ShouldCopyUpstreamHeader(c, k, v) {
				continue
			}
			if bodyChanged && (strings.EqualFold(k, "ETag") || strings.EqualFold(k, "Content-MD5") || strings.EqualFold(k, "Digest") || strings.EqualFold(k, "Content-Digest") || strings.EqualFold(k, "Repr-Digest")) {
				continue
			}
			c.Writer.Header().Set(k, v[0])
		}
	}

	// set Content-Length header manually BEFORE calling WriteHeader
	c.Writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))

	// Write header with status code (this sends the headers)
	if src != nil {
		c.Writer.WriteHeader(src.StatusCode)
	} else {
		c.Writer.WriteHeader(http.StatusOK)
	}

	_, err = io.Copy(c.Writer, body)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("failed to copy response body: %s", err.Error()))
	}
	c.Writer.Flush()
}
