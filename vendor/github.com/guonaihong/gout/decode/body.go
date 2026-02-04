package decode

import (
	"fmt"
	"github.com/guonaihong/gout/core"
	"io"
	"io/ioutil"
	"reflect"
)

// BodyDecode body decoder structure
type BodyDecode struct {
	obj interface{}
}

// NewBodyDecode create a new body decoder
func NewBodyDecode(obj interface{}) *BodyDecode {
	if obj == nil {
		return nil
	}

	return &BodyDecode{obj: obj}
}

var convertBodyFunc = map[reflect.Kind]convert{
	reflect.Uint:    {bitSize: 0, cb: setUintField},
	reflect.Uint8:   {bitSize: 8, cb: setUintField},
	reflect.Uint16:  {bitSize: 16, cb: setUintField},
	reflect.Uint32:  {bitSize: 32, cb: setUintField},
	reflect.Uint64:  {bitSize: 64, cb: setUintField},
	reflect.Int:     {bitSize: 0, cb: setIntField},
	reflect.Int8:    {bitSize: 8, cb: setIntField},
	reflect.Int16:   {bitSize: 16, cb: setIntField},
	reflect.Int32:   {bitSize: 32, cb: setIntField},
	reflect.Int64:   {bitSize: 64, cb: setIntDurationField},
	reflect.Float32: {bitSize: 32, cb: setFloatField},
	reflect.Float64: {bitSize: 64, cb: setFloatField},
}

// Decode body decoder
func (b *BodyDecode) Decode(r io.Reader) error {
	return Body(r, b.obj)
}

// Body body decoder
func Body(r io.Reader, obj interface{}) error {
	if w, ok := obj.(io.Writer); ok {
		_, err := io.Copy(w, r)
		return err
	}

	all, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}

	value := core.LoopElem(reflect.ValueOf(obj))

	if value.Kind() == reflect.String {
		value.SetString(core.BytesToString(all))
		return nil
	}

	if _, ok := value.Interface().([]byte); ok {
		value.SetBytes(all)
		return nil
	}

	fn, ok := convertBodyFunc[value.Kind()]
	if ok {
		return fn.cb(core.BytesToString(all), fn.bitSize, emptyField, value)
	}

	return fmt.Errorf("type (%T) %s", value, core.ErrUnknownType)
}
