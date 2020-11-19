package core

import (
	"errors"
	"net/http"
	"reflect"
	"unsafe"
)

// FormFile 用于formdata类型数据编码
// 从文件读取数据流
type FormFile string

// FormMem 用于formdata类型数据编码
// 从[]byte里面读取数据流
type FormMem []byte

// FormType 自定义formdata文件名和流的类型
type FormType struct {
	FileName    string      //filename
	ContentType string      //Content-Type:Mime-Type
	File        interface{} //FromFile | FromMem (这里就是您的从文件地址中读取和从内存中读取)
}

// H 是map[string]interface{} 简写
type H map[string]interface{}

// A是[]interface{} 简写
type A []interface{}

// ErrUnknownType 未知错误类型
var ErrUnknownType = errors.New("unknown type")

// LoopElem 不停地对指针解引用
func LoopElem(v reflect.Value) reflect.Value {
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return v
		}
		v = v.Elem()
	}

	return v
}

// BytesToString 没有内存开销的转换
func BytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// StringToBytes 没有内存开销的转换
func StringToBytes(s string) (b []byte) {
	bh := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	sh := *(*reflect.StringHeader)(unsafe.Pointer(&s))
	bh.Data = sh.Data
	bh.Len = sh.Len
	bh.Cap = sh.Len
	return b
}

// NewPtrVal 新建这个类型的指针变量并赋值
func NewPtrVal(defValue interface{}) interface{} {
	p := reflect.New(reflect.TypeOf(defValue))
	p.Elem().Set(reflect.ValueOf(defValue))
	return p.Interface()
}

func CloneRequest(r *http.Request) (*http.Request, error) {
	var err error

	r0 := &http.Request{}
	*r0 = *r

	r0.Header = make(http.Header, len(r.Header))

	for k, h := range r.Header {
		r0.Header[k] = append([]string(nil), h...)
	}

	r0.Body, err = r.GetBody()
	return r0, err
}

func GetBytes(v interface{}) (b []byte, ok bool) {
	switch d := v.(type) {
	case []byte:
		return d, true
	case string:
		return StringToBytes(d), true
	}
	return nil, false
}

func GetString(v interface{}) (s string, ok bool) {
	switch s := v.(type) {
	case []byte:
		return BytesToString(s), true
	case string:
		return s, true
	}
	return "", false
}
