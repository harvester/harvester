package decode

import (
	"net/http"
	"net/textproto"
	"reflect"
)

type headerDecode struct{}

func (h *headerDecode) Decode(rsp *http.Response, obj interface{}) error {
	return decode(headerSet(rsp.Header), obj, "header")
}

type headerSet map[string][]string

var _ setter = headerSet(nil)

func (h headerSet) Set(value reflect.Value, sf reflect.StructField, tagValue string) error {
	return setForm(h, value, sf, textproto.CanonicalMIMEHeaderKey(tagValue))
}
