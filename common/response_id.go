package common

import (
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

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
