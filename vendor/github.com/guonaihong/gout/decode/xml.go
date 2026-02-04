package decode

import (
	"encoding/xml"
	"io"
)

// XMLDecode xml decoder core data structure
type XMLDecode struct {
	obj interface{}
}

// NewXMLDecode create a new xml decoder
func NewXMLDecode(obj interface{}) *XMLDecode {
	if obj == nil {
		return nil
	}
	return &XMLDecode{obj: obj}
}

// Decode xml decoder
func (x *XMLDecode) Decode(r io.Reader) error {
	decode := xml.NewDecoder(r)
	return decode.Decode(x.obj)
}

// XML xml decoder
func XML(r io.Reader, obj interface{}) error {
	decode := xml.NewDecoder(r)
	return decode.Decode(obj)
}
