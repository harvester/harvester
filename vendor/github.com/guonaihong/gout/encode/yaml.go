package encode

import (
	"errors"
	"github.com/guonaihong/gout/core"
	"gopkg.in/yaml.v2"
	"io"
)

var ErrNotYAML = errors.New("Not yaml data")

// YAMLEncode yaml encoder structure
type YAMLEncode struct {
	obj interface{}
}

// NewYAMLEncode create a new yaml encoder
func NewYAMLEncode(obj interface{}) *YAMLEncode {
	if obj == nil {
		return nil
	}

	return &YAMLEncode{obj: obj}
}

// Encode yaml encoder
func (y *YAMLEncode) Encode(w io.Writer) (err error) {
	if v, ok := core.GetBytes(y.obj); ok {
		if b := YAMLValid(v); !b {
			return ErrNotYAML
		}
		_, err = w.Write(v)
		return err
	}
	encode := yaml.NewEncoder(w)
	return encode.Encode(y.obj)
}

// Name yaml Encoder name
func (y *YAMLEncode) Name() string {
	return "yaml"
}

func YAMLValid(b []byte) bool {
	var m map[string]interface{}

	err := yaml.Unmarshal(b, &m)
	if err != nil {
		return false
	}

	return true
}
