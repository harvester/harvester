package enjson

import (
	"bytes"
	"errors"
	"io"

	"github.com/guonaihong/gout/core"
	"github.com/guonaihong/gout/encoder"
	"github.com/guonaihong/gout/json"
)

var ErrNotJSON = errors.New("Not json data")

// JSONEncode json encoder structure
type JSONEncode struct {
	obj        interface{}
	escapeHTML bool
}

// NewJSONEncode create a new json encoder
func NewJSONEncode(obj interface{}, escapeHTML bool) encoder.Encoder {
	if obj == nil {
		return nil
	}

	return &JSONEncode{obj: obj, escapeHTML: escapeHTML}
}

func Marshal(obj interface{}, escapeHTML bool) (all []byte, err error) {

	if !escapeHTML {
		var buf bytes.Buffer
		encode := json.NewEncoder(&buf)
		encode.SetEscapeHTML(escapeHTML)
		err = encode.Encode(obj)
		if err != nil {
			return
		}
		// encode结束之后会自作聪明的加'\n'
		// 为了保持和json.Marshal一样的形为，手动删除最后一个'\n'
		all = buf.Bytes()
		if buf.Len() > 0 && buf.Bytes()[buf.Len()-1] == '\n' {
			all = buf.Bytes()[:buf.Len()-1]
		}
	} else {
		all, err = json.Marshal(obj)
		if err != nil {
			return
		}
	}
	return

}

// Encode json encoder
func (j *JSONEncode) Encode(w io.Writer) (err error) {
	if v, ok := core.GetBytes(j.obj); ok {
		if b := json.Valid(v); !b {
			return ErrNotJSON
		}
		_, err = w.Write(v)
		return err
	}

	var all []byte
	all, err = Marshal(j.obj, j.escapeHTML)
	if err != nil {
		return err
	}
	_, err = w.Write(all)
	return err
}

// Name json Encoder name
func (j *JSONEncode) Name() string {
	return "json"
}
