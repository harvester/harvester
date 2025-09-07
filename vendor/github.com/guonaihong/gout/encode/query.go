package encode

import (
	"net/url"
	"reflect"

	"github.com/guonaihong/gout/setting"
)

var _ Adder = (*QueryEncode)(nil)

// QueryEncode URL query encoder structure
type QueryEncode struct {
	values url.Values
	setting.Setting
}

// NewQueryEncode create a new URL query  encoder
func NewQueryEncode(s setting.Setting) *QueryEncode {
	return &QueryEncode{values: make(url.Values), Setting: s}
}

// Add Encoder core function, used to set each key / value into the http URL query
func (q *QueryEncode) Add(key string, v reflect.Value, sf reflect.StructField) error {
	val := valToStr(v, sf)
	if !q.NotIgnoreEmpty && len(val) == 0 {
		return nil
	}
	q.values.Add(key, val)
	return nil
}

// End URL query structured data into strings
func (q *QueryEncode) End() string {
	return q.values.Encode()
}

// Name URL query Encoder name
func (q *QueryEncode) Name() string {
	return "query"
}
