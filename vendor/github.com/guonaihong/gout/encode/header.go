package encode

import (
	"net/http"
	"reflect"
)

var _ Adder = (*HeaderEncode)(nil)

// HeaderEncode http header encoder structure
type HeaderEncode struct {
	r *http.Request
}

// NewHeaderEncode create a new http header encoder
func NewHeaderEncode(req *http.Request) *HeaderEncode {
	return &HeaderEncode{r: req}
}

// Add Encoder core function, used to set each key / value into the http header
func (h *HeaderEncode) Add(key string, v reflect.Value, sf reflect.StructField) error {
	h.r.Header.Add(key, valToStr(v, sf))
	return nil
}

// Name header Encoder name
func (h *HeaderEncode) Name() string {
	return "header"
}
