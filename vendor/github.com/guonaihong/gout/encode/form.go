package encode

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/textproto"
	"reflect"
	"strings"

	"github.com/guonaihong/gout/core"
)

type formContent struct {
	fileName     string //filename
	contentType  string //Content-Type:Mime-Type
	data         []byte
	isFormFile   bool
	needOpenFile bool
}

var _ Adder = (*FormEncode)(nil)

// FormEncode form-data encoder structure
type FormEncode struct {
	*multipart.Writer
}

// NewFormEncode create a new form-data encoder
func NewFormEncode(b *bytes.Buffer) *FormEncode {
	return &FormEncode{Writer: multipart.NewWriter(b)}
}

func genFormContext(key string, val reflect.Value, sf reflect.StructField, fc *formContent) (err error) {

	formFile := sf.Tag.Get("form-file")
	if len(formFile) > 0 { //说明是结构体

		switch formFile {
		case "mem", "file", "true":
		default:
			return fmt.Errorf("Unsupported form-file value:%s", formFile)
		}
		// `form-file:"mem"`  从内存中读取
		fc.isFormFile = true

		// `form-file:"file"` 从文件中读取 `form-file:"true"` 也是从文件中读取 兼容老接口
		if formFile != "mem" {
			fc.needOpenFile = true
		}

		if err := genFormContextCore(key, val, sf, fc); err != nil {
			return err
		}

		return nil
	}

	return genFormContextCore(key, val, sf, fc)
}

func genFormContextCore(key string, val reflect.Value, sf reflect.StructField, fc *formContent) (err error) {
	var all []byte

	switch v := val.Interface().(type) {
	case core.FormMem:
		all = []byte(v) //这里是type的类型转换，没有任何内存拷贝
		fc.isFormFile = true
	case core.FormFile:
		all, err = ioutil.ReadFile(string(v))
		if err != nil {
			return err
		}
		if len(fc.fileName) == 0 {
			fc.fileName = string(v)
		}
		fc.isFormFile = true

	case core.FormType:
		if v.File == nil {
			return
		}
		fc.fileName = v.FileName
		fc.contentType = v.ContentType
		if err := genFormContext(key, reflect.ValueOf(v.File), sf, fc); err != nil {
			return err
		}
		return //已经得到data的值，直接返回
	case string:
		if fc.needOpenFile {
			all, err = ioutil.ReadFile(v)
			if err != nil {
				return err
			}
			if len(fc.fileName) == 0 {
				fc.fileName = v
			}
		} else {
			all = core.StringToBytes(v)
		}
	case []byte:
		if fc.needOpenFile {
			all, err = ioutil.ReadFile(core.BytesToString(v))
			if err != nil {
				return err
			}
			if len(fc.fileName) == 0 {
				fc.fileName = core.BytesToString(v)
			}
		} else {
			all = v
		}
	default:
		if val.Kind() == reflect.Interface {
			val = reflect.ValueOf(val.Interface())
		}

		switch t := val.Kind(); t {
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		case reflect.Float32, reflect.Float64:
		case reflect.String:
		default:
			return fmt.Errorf("unknown type gen form context:%T, kind:%v", v, val.Kind())
		}

		s := valToStr(val, emptyField)
		all = core.StringToBytes(s)
	}

	fc.data = all
	return nil
}

//下方为原函数附带的方法
var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

func escapeQuotes(s string) string {
	return quoteEscaper.Replace(s)
}

// CreateFormFile 重写原来net/http里面的CreateFormFile函数
func (f *FormEncode) CreateFormFile(fieldName, fileName, contentType string) (io.Writer, error) {

	if contentType == "" {
		contentType = "application/octet-stream"
	}

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="%s"; filename="%s"`,
			escapeQuotes(fieldName), escapeQuotes(fileName)))
	h.Set("Content-Type", contentType)
	return f.CreatePart(h)
}

// Add Encoder core function, used to set each key / value into the http form-data
func (f *FormEncode) Add(key string, v reflect.Value, sf reflect.StructField) (err error) {
	// 1.提取数据
	var fc formContent
	if err := genFormContext(key, v, sf, &fc); err != nil {
		return err
	}
	// 2.生成formdata格式数据

	return f.createForm(key, &fc)

}

func (f *FormEncode) createForm(key string, fc *formContent) error {
	if len(fc.data) == 0 {
		return nil
	}

	if !fc.isFormFile {
		part, err := f.CreateFormField(key)
		if err != nil {
			return err
		}
		_, err = part.Write(fc.data)
		return err
	}

	if len(fc.fileName) == 0 {
		fc.fileName = key
	}

	part, err := f.CreateFormFile(key, fc.fileName, fc.contentType)
	if err != nil {
		return err
	}
	_, err = part.Write(fc.data)
	return err
}

// End refresh data
func (f *FormEncode) End() error {
	return f.Close()
}

// Name form-data Encoder name
func (f *FormEncode) Name() string {
	return "form"
}
