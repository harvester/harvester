package encode

import (
	"bytes"
	"encoding/xml"
	"errors"
	"github.com/guonaihong/gout/core"
	"io"
)

var ErrNotXML = errors.New("Not xml data")

// XMLEncode xml encoder structure
type XMLEncode struct {
	obj interface{}
}

// NewXMLEncode create a new xml encoder
func NewXMLEncode(obj interface{}) *XMLEncode {
	if obj == nil {
		return nil
	}

	return &XMLEncode{obj: obj}
}

// Encode xml encoder
func (x *XMLEncode) Encode(w io.Writer) (err error) {
	if v, ok := core.GetBytes(x.obj); ok {
		if b := XMLValid(v); !b {
			return ErrNotXML
		}

		_, err = w.Write(v)
		return err
	}

	encode := xml.NewEncoder(w)
	return encode.Encode(x.obj)
}

// Name xml Encoder name
func (x *XMLEncode) Name() string {
	return "xml"
}

func XMLValid(b []byte) bool {
	dec := xml.NewDecoder(bytes.NewBuffer(b))
	for {
		_, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return false
		}
	}

	return true
}
