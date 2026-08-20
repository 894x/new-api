package helper

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

var ErrNullJSONStreamEvent = errors.New("upstream structured stream event is JSON null")

func IsNullJSONStreamEvent(data string) bool {
	return strings.EqualFold(strings.TrimSpace(data), "null")
}

// ResolveUpstreamStreamErrorStatus extracts an HTTP-like status from an
// already-identified upstream stream error envelope. Explicit client errors
// stay soft, while an unknown or malformed error envelope is treated as an
// upstream protocol failure.
func ResolveUpstreamStreamErrorStatus(data string, responseStatus int) int {
	var payload map[string]any
	if err := common.UnmarshalJsonStr(data, &payload); err == nil {
		for _, path := range [][]string{
			{"status_code"},
			{"status"},
			{"code"},
			{"error_code"},
			{"error", "status_code"},
			{"error", "status"},
			{"error", "code"},
			{"response", "status_code"},
			{"response", "status"},
			{"response", "error", "status_code"},
			{"response", "error", "status"},
			{"response", "error", "code"},
			{"Error", "Code"},
			{"ErrorMsg", "Code"},
			{"Response", "Error", "Code"},
			{"Response", "ErrorMsg", "Code"},
		} {
			if status, ok := streamErrorStatusAtPath(payload, path); ok {
				return status
			}
		}
		if errorsValue, ok := payload["errors"].([]any); ok {
			for _, value := range errorsValue {
				entry, ok := value.(map[string]any)
				if !ok {
					continue
				}
				for _, key := range []string{"status_code", "status", "code"} {
					if status, ok := streamErrorStatusAtPath(entry, []string{key}); ok {
						return status
					}
				}
			}
		}
	}
	if responseStatus >= http.StatusBadRequest && responseStatus <= 599 {
		return responseStatus
	}
	return http.StatusBadGateway
}

func streamErrorStatusAtPath(payload map[string]any, path []string) (int, bool) {
	var current any = payload
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return 0, false
		}
		current, ok = object[key]
		if !ok {
			return 0, false
		}
	}

	var status int
	switch value := current.(type) {
	case float64:
		if value != math.Trunc(value) {
			return 0, false
		}
		status = int(value)
	case string:
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return 0, false
		}
		status = parsed
	case int:
		status = value
	default:
		return 0, false
	}
	return status, status >= http.StatusBadRequest && status <= 599
}
