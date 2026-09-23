package common

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/jinzhu/copier"
)

func DeepCopy[T any](src *T) (*T, error) {
	if src == nil {
		return nil, fmt.Errorf("copy source cannot be nil")
	}
	var dst T
	err := copier.CopyWithOption(&dst, src, copier.Option{
		DeepCopy: true, IgnoreEmpty: true,
		Converters: []copier.TypeConverter{{
			SrcType: json.RawMessage{},
			DstType: json.RawMessage{},
			Fn: func(src any) (any, error) {
				// Copy raw JSON in bulk while retaining independent storage for
				// request mutation and retries, instead of reflecting over each byte.
				return json.RawMessage(bytes.Clone(src.(json.RawMessage))), nil
			},
		}, {
			SrcType: []byte{},
			DstType: []byte{},
			Fn: func(src any) (any, error) {
				return bytes.Clone(src.([]byte)), nil
			},
		}},
	})
	if err != nil {
		return nil, err
	}
	return &dst, nil
}
