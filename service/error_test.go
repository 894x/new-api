package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResetStatusCode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		statusCode       int
		statusCodeConfig string
		expectedCode     int
	}{
		{
			name:             "map string value",
			statusCode:       429,
			statusCodeConfig: `{"429":"503"}`,
			expectedCode:     503,
		},
		{
			name:             "map int value",
			statusCode:       429,
			statusCodeConfig: `{"429":503}`,
			expectedCode:     503,
		},
		{
			name:             "skip invalid string value",
			statusCode:       429,
			statusCodeConfig: `{"429":"bad-code"}`,
			expectedCode:     429,
		},
		{
			name:             "skip status code 200",
			statusCode:       200,
			statusCodeConfig: `{"200":503}`,
			expectedCode:     200,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			newAPIError := &types.NewAPIError{
				StatusCode: tc.statusCode,
			}
			ResetStatusCode(newAPIError, tc.statusCodeConfig)
			require.Equal(t, tc.expectedCode, newAPIError.StatusCode)
		})
	}
}

func TestRelayErrorHandlerTruncatesInvalidJSONBodyInLog(t *testing.T) {
	withDebugEnabled(t, false)

	body := strings.Repeat("b", common.LocalLogContentLimit+256)
	var logBuffer bytes.Buffer

	common.LogWriterMu.Lock()
	oldWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &logBuffer
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultErrorWriter = oldWriter
		common.LogWriterMu.Unlock()
	})

	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	newAPIError := RelayErrorHandler(context.Background(), resp, false)

	require.NotNil(t, newAPIError)
	require.Equal(t, "bad response status code 500", newAPIError.Error())
	require.Contains(t, logBuffer.String(), "[truncated")
	require.Contains(t, logBuffer.String(), fmt.Sprintf("original_length=%d", len(body)))
	require.NotContains(t, logBuffer.String(), strings.Repeat("b", common.LocalLogContentLimit+1))
}

func TestRelayErrorHandlerKeepsStructuredErrorMessage(t *testing.T) {
	message := strings.Repeat("c", common.LocalLogContentLimit+256)
	body := `{"message":"` + message + `"}`
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	newAPIError := RelayErrorHandler(context.Background(), resp, false)

	require.NotNil(t, newAPIError)
	require.Equal(t, message, newAPIError.Error())
}

func TestRelayErrorHandlerKeepsOpenAIErrorMessage(t *testing.T) {
	message := strings.Repeat("d", common.LocalLogContentLimit+256)
	body := `{"error":{"message":"` + message + `","type":"server_error","code":"server_error"}}`
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	newAPIError := RelayErrorHandler(context.Background(), resp, false)

	require.NotNil(t, newAPIError)
	require.Equal(t, message, newAPIError.Error())
}

func TestOpenAIErrorForClientUnwrapsEmbeddedUpstreamErrorJSON(t *testing.T) {
	setHideErrorDetails(t, false)
	innerMessage := "Tool 25 function has invalid 'parameters' schema: None is not of type 'object'\n\n" +
		"Failed validating 'type' in metaschema['properties']['properties']:\n" +
		"    {'type': 'object', 'additionalProperties': {'$ref': '#'}, 'default': {}}\n\n" +
		"On schema['properties']:\n" +
		"    None"
	inner, err := common.Marshal(map[string]any{
		"object": "error", "message": innerMessage, "type": "BadRequestError", "param": nil, "code": 400,
	})
	require.NoError(t, err)
	outerMessage := "Prefill server error (400 Bad Request): " + string(inner) +
		" (request id: 202609220845438346456878268d9d62v45QYxO)"
	body, err := common.Marshal(map[string]any{
		"error": types.OpenAIError{Message: outerMessage, Type: "invalid_request_error"},
	})
	require.NoError(t, err)
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(bytes.NewReader(body)),
	}

	newAPIError := RelayErrorHandler(context.Background(), resp, false)
	require.NotNil(t, newAPIError)
	clientError := OpenAIErrorForClient(
		newErrorClientContext(common.RoleCommonUser, "request-json-error"),
		newAPIError,
	)

	assert.Equal(t, outerMessage, newAPIError.Error())
	assert.Equal(t, http.StatusBadRequest, newAPIError.StatusCode)
	clientBody, err := common.Marshal(map[string]any{"error": clientError})
	require.NoError(t, err)
	var actual struct {
		Error map[string]any `json:"error"`
	}
	require.NoError(t, common.Unmarshal(clientBody, &actual))
	assert.Equal(t, innerMessage, actual.Error["message"])
	assert.Equal(t, map[string]any{
		"type":                 "object",
		"additionalProperties": map[string]any{"$ref": "#"},
		"default":              map[string]any{},
	}, actual.Error["message_data"])
	assert.Equal(t, "BadRequestError", actual.Error["type"])
	assert.Equal(t, float64(400), actual.Error["code"])
	assert.Nil(t, actual.Error["param"])
}

func TestOpenAIErrorForClientReturnsArbitraryUpstreamJSONObject(t *testing.T) {
	setHideErrorDetails(t, false)
	inner := `{"detail":{"reason":"bad \"schema\"","lines":[1,2]},"trace_id":"trace-123","status":400}`
	message := "Provider rejected request: " + inner
	err := types.WithOpenAIError(types.OpenAIError{Message: message, Type: "upstream_error", Code: "provider_failure"}, http.StatusBadRequest)

	clientError := OpenAIErrorForClient(
		newErrorClientContext(common.RoleCommonUser, "request-unknown-shape"),
		err,
	)

	clientBody, marshalErr := common.Marshal(map[string]any{"error": clientError})
	require.NoError(t, marshalErr)
	assert.JSONEq(t, `{"error":`+inner+`}`, string(clientBody))
}

func TestOpenAIErrorForClientDecodesQuotedJSONObject(t *testing.T) {
	setHideErrorDetails(t, false)
	inner := `{"issues":[{"field":"parameters","detail":"bad \"schema\""}],"retry":false}`
	quoted, marshalErr := common.Marshal(inner)
	require.NoError(t, marshalErr)
	err := types.WithOpenAIError(types.OpenAIError{
		Message: "Provider rejected request: " + string(quoted),
		Type:    "upstream_error",
	}, http.StatusBadRequest)

	clientError := OpenAIErrorForClient(
		newErrorClientContext(common.RoleCommonUser, "request-quoted"),
		err,
	)
	clientBody, marshalErr := common.Marshal(map[string]any{"error": clientError})
	require.NoError(t, marshalErr)
	assert.JSONEq(t, `{"error":`+inner+`}`, string(clientBody))
}

func TestOpenAIErrorForClientDecodesEscapedJSONFragment(t *testing.T) {
	setHideErrorDetails(t, false)
	inner := `{"issues":[{"field":"parameters","detail":"bad \"schema\""}],"retry":false}`
	quoted, marshalErr := common.Marshal(inner)
	require.NoError(t, marshalErr)
	escapedFragment := string(quoted[1 : len(quoted)-1])
	err := types.WithOpenAIError(types.OpenAIError{
		Message: "Provider rejected request: " + escapedFragment + " (request id: upstream-id)",
		Type:    "upstream_error",
	}, http.StatusBadRequest)

	clientError := OpenAIErrorForClient(
		newErrorClientContext(common.RoleCommonUser, "request-escaped"),
		err,
	)
	clientBody, marshalErr := common.Marshal(map[string]any{"error": clientError})
	require.NoError(t, marshalErr)
	assert.JSONEq(t, `{"error":`+inner+`}`, string(clientBody))
}

func TestOpenAIErrorForClientEmbeddedMessageDictionaryCases(t *testing.T) {
	setHideErrorDetails(t, false)
	cases := []struct {
		name     string
		inner    string
		expected string
	}{
		{
			name:  "nested values and braces inside quoted text",
			inner: `{"message":"schema: {'note': 'brace } and { inside', 'flags': [true, false], 'count': 2}","vendor":"another-channel"}`,
			expected: `{"error":{"message":"schema: {'note': 'brace } and { inside', 'flags': [true, false], 'count': 2}","vendor":"another-channel",` +
				`"message_data":{"note":"brace } and { inside","flags":[true,false],"count":2}}}`,
		},
		{
			name:  "python null and boolean literals",
			inner: `{"message":"schema: {'value': None, 'ok': True, 'disabled': False, 'literal': 'None'}"}`,
			expected: `{"error":{"message":"schema: {'value': None, 'ok': True, 'disabled': False, 'literal': 'None'}",` +
				`"message_data":{"value":null,"ok":true,"disabled":false,"literal":"None"}}}`,
		},
		{
			name:     "malformed dictionary keeps inner error",
			inner:    `{"message":"schema: {'invalid': [1, 2}","vendor":"another-channel"}`,
			expected: `{"error":{"message":"schema: {'invalid': [1, 2}","vendor":"another-channel"}}`,
		},
		{
			name:     "malformed dictionary does not promote valid nested map",
			inner:    `{"message":"schema: {'outer': {'inner': 1}, 'broken': }","vendor":"another-channel"}`,
			expected: `{"error":{"message":"schema: {'outer': {'inner': 1}, 'broken': }","vendor":"another-channel"}}`,
		},
		{
			name:     "plain braces are not treated as a dictionary",
			inner:    `{"message":"template {placeholder} is invalid","vendor":"another-channel"}`,
			expected: `{"error":{"message":"template {placeholder} is invalid","vendor":"another-channel"}}`,
		},
		{
			name:     "existing message data is preserved",
			inner:    `{"message":"schema: {'value': 2}","message_data":{"original":true}}`,
			expected: `{"error":{"message":"schema: {'value': 2}","message_data":{"original":true}}}`,
		},
		{
			name:     "different provider schema without message",
			inner:    `{"detail":{"issues":[{"path":"input","expected":null}]},"retryable":false}`,
			expected: `{"error":{"detail":{"issues":[{"path":"input","expected":null}]},"retryable":false}}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := types.WithOpenAIError(types.OpenAIError{
				Message: "Provider error: " + tc.inner + " (request id: upstream-id)",
				Type:    "upstream_error",
			}, http.StatusBadRequest)
			clientError := OpenAIErrorForClient(
				newErrorClientContext(common.RoleCommonUser, "request-cases"),
				err,
			)
			clientBody, marshalErr := common.Marshal(map[string]any{"error": clientError})
			require.NoError(t, marshalErr)
			assert.JSONEq(t, tc.expected, string(clientBody))
		})
	}
}

func TestOpenAIErrorForClientKeepsMalformedEmbeddedJSONInMessage(t *testing.T) {
	setHideErrorDetails(t, false)
	malformedWithNested := `{"detail":{"inner":"valid"},"broken": }`
	quoted, marshalErr := common.Marshal(malformedWithNested)
	require.NoError(t, marshalErr)
	cases := []struct {
		name    string
		message string
	}{
		{name: "incomplete object", message: `Provider rejected request: {"detail":`},
		{name: "malformed object with valid nested object", message: "Provider rejected request: " + malformedWithNested},
		{name: "malformed escaped object with valid nested object", message: "Provider rejected request: " + string(quoted[1:len(quoted)-1])},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := types.WithOpenAIError(types.OpenAIError{Message: tc.message, Type: "upstream_error"}, http.StatusBadRequest)
			clientError := OpenAIErrorForClient(
				newErrorClientContext(common.RoleCommonUser, "request-malformed"),
				err,
			)

			assert.Equal(t, tc.message+" (request id: request-malformed)", clientError.Message)
			assert.Empty(t, clientError.RawError)
			clientBody, marshalErr := common.Marshal(map[string]any{"error": clientError})
			require.NoError(t, marshalErr)
			var actual struct {
				Error types.OpenAIError `json:"error"`
			}
			require.NoError(t, common.Unmarshal(clientBody, &actual))
			assert.Equal(t, "upstream_error", actual.Error.Type)
			assert.Equal(t, clientError.Message, actual.Error.Message)
			assert.Nil(t, actual.Error.Code)
		})
	}
}

func TestOpenAIErrorForClientDoesNotExposeEmbeddedJSONWhenDetailsAreHidden(t *testing.T) {
	setHideErrorDetails(t, true)
	message := `Provider rejected request: {"detail":{"secret":"provider-private"}}`
	err := types.WithOpenAIError(types.OpenAIError{Message: message, Type: "upstream_error"}, http.StatusBadRequest)

	clientError := OpenAIErrorForClient(
		newErrorClientContext(common.RoleCommonUser, "request-hidden"),
		err,
	)
	clientBody, marshalErr := common.Marshal(map[string]any{"error": clientError})
	require.NoError(t, marshalErr)
	assert.NotContains(t, string(clientBody), "provider-private")
	assert.NotContains(t, string(clientBody), "detail")
	assert.Contains(t, string(clientBody), "request-hidden")
}

func TestRelayErrorHandlerResponseBodyUsesConfiguredClientReplacement(t *testing.T) {
	setHideErrorDetails(t, true)
	setErrorResponseReplacementRules(t, []operation_setting.ErrorResponseReplacementRule{{
		StatusCode:  http.StatusInternalServerError,
		Match:       "Moonshot AI",
		Replacement: "rhzs",
	}})
	body := `{"error":{"message":"服务处理请求时发生内部错误，请稍后重试。若持续出现该问题，请联系 Moonshot AI 技术支持。","type":"server_error","code":"server_error"}}`
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	newAPIError := RelayErrorHandler(context.Background(), resp, false)
	require.NotNil(t, newAPIError)
	clientError := OpenAIErrorForClient(
		newErrorClientContext(common.RoleCommonUser, "request-moonshot"),
		newAPIError,
	)

	assert.Contains(t, newAPIError.Error(), "Moonshot AI 技术支持")
	assert.Contains(t, clientError.Message, "请联系 rhzs 技术支持")
	assert.Contains(t, clientError.Message, "request-moonshot")
	assert.NotContains(t, clientError.Message, "Moonshot AI")
	assert.Equal(t, http.StatusInternalServerError, newAPIError.StatusCode)
}

func TestRelayErrorHandlerKeepsInvalidJSONBodyInDebugLog(t *testing.T) {
	withDebugEnabled(t, true)

	body := strings.Repeat("e", common.LocalLogContentLimit+256)
	var logBuffer bytes.Buffer

	common.LogWriterMu.Lock()
	oldWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &logBuffer
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultErrorWriter = oldWriter
		common.LogWriterMu.Unlock()
	})

	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	newAPIError := RelayErrorHandler(context.Background(), resp, false)

	require.NotNil(t, newAPIError)
	require.NotContains(t, logBuffer.String(), "[truncated")
	require.Contains(t, logBuffer.String(), body)
}

func withDebugEnabled(t *testing.T, enabled bool) {
	t.Helper()

	oldDebug := common.DebugEnabled
	common.DebugEnabled = enabled
	t.Cleanup(func() {
		common.DebugEnabled = oldDebug
	})
}
