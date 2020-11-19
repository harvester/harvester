package encode

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

// ErrUnsupported Unsupported type error returned
var ErrUnsupported = errors.New("Encode:Unsupported type")

var emptyField = reflect.StructField{}

// Adder interface
type Adder interface {
	Add(key string, v reflect.Value, sf reflect.StructField) error
	Name() string
}

// Encode core entry function
// in 的类型可以是
// struct
// map
// []string
func Encode(in interface{}, a Adder) error {
	v := reflect.ValueOf(in)

	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}

		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			if err := a.Add(valToStr(iter.Key(), emptyField), iter.Value(), emptyField); err != nil {
				return err
			}
		}
		return nil

	case reflect.Struct:
		if err := encode(v, emptyField, a); err != nil {
			return err
		}
		return nil

	case reflect.Slice, reflect.Array:
		if v.Len() == 0 {
			return nil
		}

		if v.Len()%2 != 0 {
			return fmt.Errorf("The %T length of the code must be even", v.Kind())
		}

		for i, l := 0, v.Len(); i < l; i += 2 {

			if err := a.Add(valToStr(v.Index(i), emptyField), v.Index(i+1), emptyField); err != nil {
				return err
			}
		}

		return nil
	}

	return ErrUnsupported
}

func parseTag(tag string) (string, tagOptions) {
	s := strings.Split(tag, ",")
	return s[0], s[1:]
}

func timeToStr(v reflect.Value, sf reflect.StructField) string {

	t := v.Interface().(time.Time)
	if t.IsZero() {
		return ""
	}

	timeFormat := sf.Tag.Get("time_format")
	if timeFormat == "" {
		timeFormat = time.RFC3339
	}

	switch tf := strings.ToLower(timeFormat); tf {
	case "unix", "unixnano":
		var tv int64
		if tf == "unix" {
			tv = t.Unix()
		} else {
			tv = t.UnixNano()
		}

		return strconv.FormatInt(tv, 10)
	}

	return t.Format(timeFormat)

}

func valToStr(v reflect.Value, sf reflect.StructField) string {
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}

	if v.Type() == timeType {
		return timeToStr(v, sf)
	}

	if v.IsZero() {
		return ""
	}

	if b, ok := v.Interface().([]byte); ok {
		return *(*string)(unsafe.Pointer(&b))
	}

	return fmt.Sprint(v.Interface())
}

func parseTagAndSet(val reflect.Value, sf reflect.StructField, a Adder) error {

	tagName := sf.Tag.Get(a.Name())
	tagName, opts := parseTag(tagName)

	if tagName == "" {
		tagName = sf.Name
	}

	if tagName == "" {
		return nil
	}

	if opts.Contains("omitempty") && valueIsEmpty(val) {
		return nil
	}

	return a.Add(tagName, val, sf)
}

func encode(val reflect.Value, sf reflect.StructField, a Adder) error {
	vKind := val.Kind()

	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return nil
		}

		return encode(val.Elem(), sf, a)
	}

	if vKind != reflect.Struct || !sf.Anonymous {
		if err := parseTagAndSet(val, sf, a); err != nil {
			return err
		}
	}

	if vKind == reflect.Struct {

		typ := val.Type()

		// TODO使用接口解耦具体类型
		if strings.HasSuffix(typ.Name(), "FormType") {
			return parseTagAndSet(val, sf, a)
		}

		for i := 0; i < typ.NumField(); i++ {

			sf := typ.Field(i)

			if sf.PkgPath != "" && !sf.Anonymous {
				continue
			}

			tag := sf.Tag.Get(a.Name())

			if tag == "-" {
				continue

			}

			if err := encode(val.Field(i), sf, a); err != nil {
				return err
			}
		}
	}

	return nil
}

type tagOptions []string

func (t tagOptions) Contains(tag string) bool {
	for _, v := range t {
		if tag == v {
			return true
		}
	}

	return false
}

var timeType = reflect.TypeOf(time.Time{})

func valueIsEmpty(v reflect.Value) bool {

	switch v.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Slice, reflect.Array, reflect.Map, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	case reflect.Invalid:
		return true
	}

	if v.Type() == timeType {
		return v.Interface().(time.Time).IsZero()
	}

	return false
}
