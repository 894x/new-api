package common

import (
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	MaxUpstreamIdentifierBytes        = 512
	MaxUpstreamRequestIdentifierBytes = 128
)

func LimitUpstreamIdentifier(value string) string {
	return limitUTF8String(value, MaxUpstreamIdentifierBytes)
}

func LimitUpstreamRequestIdentifier(value string) string {
	return limitUTF8String(value, MaxUpstreamRequestIdentifierBytes)
}

func limitUTF8String(value string, maxBytes int) string {
	value = strings.ToValidUTF8(value, "�")
	if len(value) <= maxBytes {
		return value
	}

	const truncationMarker = "…"
	limit := maxBytes - len(truncationMarker)
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	return value[:limit] + truncationMarker
}

// CaptureUpstreamResponseID keeps the first top-level provider ID that will be
// replaced by the gateway-owned Chat Completions response ID.
func CaptureUpstreamResponseID(c *gin.Context, payload []byte, responseId string) {
	if c == nil || responseId == "" || c.GetString(UpstreamResponseIdKey) != "" {
		return
	}
	id := gjson.GetBytes(payload, "id")
	if !id.Exists() || id.String() == "" || id.String() == responseId {
		return
	}
	errorValue := gjson.GetBytes(payload, "error")
	if errorValue.Exists() && errorValue.Type != gjson.Null {
		if (errorValue.IsObject() && len(errorValue.Map()) > 0) ||
			(errorValue.IsArray() && len(errorValue.Array()) > 0) ||
			(!errorValue.IsObject() && !errorValue.IsArray() && errorValue.String() != "") {
			return
		}
	}
	c.Set(UpstreamResponseIdKey, LimitUpstreamIdentifier(id.String()))
}

// ReplaceTopLevelJSONID replaces only a JSON object's top-level id field.
func ReplaceTopLevelJSONID(payload []byte, responseId string) ([]byte, error) {
	if responseId == "" {
		return payload, nil
	}
	id := gjson.GetBytes(payload, "id")
	if id.Exists() && id.String() == responseId {
		return payload, nil
	}
	errorValue := gjson.GetBytes(payload, "error")
	if errorValue.Exists() && errorValue.Type != gjson.Null {
		if (errorValue.IsObject() && len(errorValue.Map()) > 0) ||
			(errorValue.IsArray() && len(errorValue.Array()) > 0) ||
			(!errorValue.IsObject() && !errorValue.IsArray() && errorValue.String() != "") {
			return payload, nil
		}
	}
	object := gjson.GetBytes(payload, "object").String()
	choicesExist := gjson.GetBytes(payload, "choices").Exists()
	if !id.Exists() && !choicesExist && object != "chat.completion" && object != "chat.completion.chunk" {
		return payload, nil
	}
	return sjson.SetBytes(payload, "id", responseId)
}
