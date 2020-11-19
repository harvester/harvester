package encode

import (
	"github.com/guonaihong/gout/core"
	"io"
	"net/url"
	"reflect"
)

var _ Adder = (*WWWFormEncode)(nil)

// WWWFormEncode x-www-form-urlencoded encoder structure
type WWWFormEncode struct {
	values url.Values
}

// NewWWWFormEncode create a new x-www-form-urlencoded encoder
func NewWWWFormEncode() *WWWFormEncode {

	return &WWWFormEncode{values: make(url.Values)}
}

// Encode x-www-form-urlencoded encoder
func (we *WWWFormEncode) Encode(obj interface{}) (err error) {
	return Encode(obj, we)
}

// Add Encoder core function, used to set each key / value into the http x-www-form-urlencoded
// 这里value的设置暴露 reflect.Value和 reflect.StructField原因如下
// reflect.Value把value转成字符串
// reflect.StructField主要是可以在Add函数里面获取tag相关信息
func (we *WWWFormEncode) Add(key string, v reflect.Value, sf reflect.StructField) error {
	val := valToStr(v, sf)
	if len(val) == 0 {
		return nil
	}
	we.values.Add(key, val)
	return nil
}

func (we *WWWFormEncode) End(w io.Writer) error {
	_, err := w.Write(core.StringToBytes(we.values.Encode()))
	return err
}

// Name x-www-form-urlencoded Encoder name
func (we *WWWFormEncode) Name() string {
	return "www-form"
}
