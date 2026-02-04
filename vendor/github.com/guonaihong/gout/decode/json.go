package decode

import (
	"encoding/json"
	"io"
)

// JSONDecode json decoder core data structure
type JSONDecode struct {
	obj interface{}
}

// NewJSONDecode create a new json decoder
func NewJSONDecode(obj interface{}) *JSONDecode {
	if obj == nil {
		return nil
	}
	return &JSONDecode{obj: obj}
}

// Decode json decoder
func (j *JSONDecode) Decode(r io.Reader) error {
	decode := json.NewDecoder(r)
	return decode.Decode(j.obj)
}

// JSON json decoder
func JSON(r io.Reader, obj interface{}) error {
	decode := json.NewDecoder(r)
	return decode.Decode(obj)
}
