package decode

import (
	"errors"
	"net/http"
	"net/textproto"
	"reflect"
)

var ErrWrongParam = errors.New("Wrong parameter")

type headerDecode struct{}

func (h *headerDecode) Decode(rsp *http.Response, obj interface{}) error {
	if obj == nil {
		return ErrWrongParam
	}

	// 如果是
	// http.Header
	// *http.Header
	// 等类型, 直接把rsp.Header的字段都拷贝下
	switch value := obj.(type) {
	case http.Header:
		for k, v := range rsp.Header {
			value[k] = v
		}
		return nil
	case *http.Header:
		*value = rsp.Header.Clone()
		return nil
	}

	return decode(headerSet(rsp.Header), obj, "header")
}

type headerSet map[string][]string

var _ setter = headerSet(nil)

func (h headerSet) Set(value reflect.Value, sf reflect.StructField, tagValue string) error {
	return setForm(h, value, sf, textproto.CanonicalMIMEHeaderKey(tagValue))
}
