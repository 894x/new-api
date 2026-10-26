package common

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// RewriteResponseModel projects protocol model declarations to the client's
// original model ID. Call only at the downstream write boundary, after upstream
// model observations and usage have been captured. Content and tool arguments
// are never traversed, and model-free events retain their original shape.
func RewriteResponseModel(c *gin.Context, payload []byte) ([]byte, error) {
	if c == nil {
		return payload, nil
	}
	model := GetContextKeyString(c, constant.ContextKeyOriginalModel)
	if model == "" {
		return payload, nil
	}
	for _, path := range []string{"model", "response.model", "message.model", "modelVersion"} {
		declared := gjson.GetBytes(payload, path)
		if declared.Type != gjson.String || declared.Str == model {
			continue
		}
		var err error
		payload, err = sjson.SetBytes(payload, path, model)
		if err != nil {
			return nil, err
		}
	}
	return payload, nil
}
