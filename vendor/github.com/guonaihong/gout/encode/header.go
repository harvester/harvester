package encode

import (
	"net/http"
	"reflect"
)

var _ Adder = (*HeaderEncode)(nil)

// HeaderEncode http header encoder structure
type HeaderEncode struct {
	r         *http.Request
	rawHeader bool
}

// NewHeaderEncode create a new http header encoder
func NewHeaderEncode(req *http.Request, rawHeader bool) *HeaderEncode {
	return &HeaderEncode{r: req, rawHeader: rawHeader}
}

// Add Encoder core function, used to set each key / value into the http header
func (h *HeaderEncode) Add(key string, v reflect.Value, sf reflect.StructField) error {
	value := valToStr(v, sf)
	if !h.rawHeader {
		h.r.Header.Add(key, value)
	} else {

		h.r.Header[key] = append(h.r.Header[key], value)
	}
	return nil
}

// Name header Encoder name
func (h *HeaderEncode) Name() string {
	return "header"
}
