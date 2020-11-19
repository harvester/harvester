package decode

import (
	"gopkg.in/yaml.v2"
	"io"
)

// YAMLDecode yaml decoder core data structure
type YAMLDecode struct {
	obj interface{}
}

// NewYAMLDecode create a new yaml decoder
func NewYAMLDecode(obj interface{}) *YAMLDecode {
	if obj == nil {
		return nil
	}
	return &YAMLDecode{obj: obj}
}

// Decode yaml decoder
func (x *YAMLDecode) Decode(r io.Reader) error {
	decode := yaml.NewDecoder(r)
	return decode.Decode(x.obj)
}

// YAML yaml decoder
func YAML(r io.Reader, obj interface{}) error {
	decode := yaml.NewDecoder(r)
	return decode.Decode(obj)
}
