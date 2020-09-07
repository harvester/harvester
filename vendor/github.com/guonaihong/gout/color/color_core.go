package color

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"sort"
	"strconv"
	"strings"
)

// BodyType 区分body的类型
type BodyType int

const (
	// JSONType http body是json类型的
	JSONType BodyType = iota + 1
	// XMLType http body是xml类型的
	XMLType
	// YAMLType http body是yaml类型的
	YAMLType
	// TxtType http body是txt类型的
	TxtType
)

// 本文件来自github.com/TylerBrock/colorjson, 感谢TylerBrock
// TODO
// * 修复原来代码bug
// * 支持更多数据类型
// * 扩展原有功能以支持xml和yaml

const initialDepth = 0
const valueSep = ","
const null = "null"
const startMap = "{"
const endMap = "}"
const startArray = "["
const endArray = "]"

const emptyMap = startMap + endMap
const emptyArray = startArray + endArray

// Formatter 是颜色高亮核心结构体
type Formatter struct {
	KeyColor        *Color // 设置key的颜色
	StringColor     *Color // 设置string的颜色
	BoolColor       *Color // 设置bool的颜色
	NumberColor     *Color // 设置数字的颜色
	NullColor       *Color // 设置null的颜色
	StringMaxLength int
	Indent          int
	DisabledColor   bool
	RawStrings      bool

	r io.Reader
}

// NewFormatEncoder 着色json/yaml/xml构造函数
func NewFormatEncoder(r io.Reader, openColor bool, bodyType BodyType) *Formatter {
	// 如果颜色没打开，或者bodyType为txt
	if openColor == false || bodyType == TxtType {
		return nil
	}

	all, err := ioutil.ReadAll(r)
	if err != nil {
		return nil
	}

	var obj map[string]interface{}

	switch bodyType {
	case JSONType:
		err = json.Unmarshal(all, &obj)
		//todo xmlType and yamlType
	case XMLType:
	case YAMLType:
	}
	if err != nil {
		return nil
	}

	f := &Formatter{
		KeyColor:        New(true, FgWhite),
		StringColor:     New(true, FgGreen),
		BoolColor:       New(true, FgYellow),
		NumberColor:     New(true, FgCyan),
		NullColor:       New(true, FgMagenta),
		StringMaxLength: 0,
		DisabledColor:   false,
		Indent:          4,
		RawStrings:      false,
		r:               r,
	}

	all, _ = f.Marshal(obj)

	f.r = bytes.NewReader(all)
	return f
}

func (f *Formatter) sprintColor(c *Color, s string) string {
	if f.DisabledColor || c == nil {
		return fmt.Sprint(s)
	}
	return c.color(s)
}

func (f *Formatter) writeIndent(buf *bytes.Buffer, depth int) {
	buf.WriteString(strings.Repeat(" ", f.Indent*depth))
}

func (f *Formatter) writeObjSep(buf *bytes.Buffer) {
	if f.Indent != 0 {
		buf.WriteByte('\n')
	} else {
		buf.WriteByte(' ')
	}
}

// Marshal 给原始的结构化数据着色
func (f *Formatter) Marshal(jsonObj interface{}) ([]byte, error) {
	buffer := bytes.Buffer{}
	f.marshalValue(jsonObj, &buffer, initialDepth)
	return buffer.Bytes(), nil
}

func (f *Formatter) marshalMap(m map[string]interface{}, buf *bytes.Buffer, depth int) {
	remaining := len(m)

	if remaining == 0 {
		buf.WriteString(emptyMap)
		return
	}

	keys := make([]string, 0)
	for key := range m {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	buf.WriteString(startMap)
	f.writeObjSep(buf)

	for _, key := range keys {
		f.writeIndent(buf, depth+1)
		buf.WriteString(f.KeyColor.colorf(`"%s": `, key))
		f.marshalValue(m[key], buf, depth+1)
		remaining--
		if remaining != 0 {
			buf.WriteString(valueSep)
		}
		f.writeObjSep(buf)
	}
	f.writeIndent(buf, depth)
	buf.WriteString(endMap)
}

func (f *Formatter) marshalArray(a []interface{}, buf *bytes.Buffer, depth int) {
	if len(a) == 0 {
		buf.WriteString(emptyArray)
		return
	}

	buf.WriteString(startArray)
	f.writeObjSep(buf)

	for i, v := range a {
		f.writeIndent(buf, depth+1)
		f.marshalValue(v, buf, depth+1)
		if i < len(a)-1 {
			buf.WriteString(valueSep)
		}
		f.writeObjSep(buf)
	}
	f.writeIndent(buf, depth)
	buf.WriteString(endArray)
}

func (f *Formatter) marshalValue(val interface{}, buf *bytes.Buffer, depth int) {
	switch v := val.(type) {
	case map[string]interface{}:
		f.marshalMap(v, buf, depth)
	case []interface{}:
		f.marshalArray(v, buf, depth)
	case string:
		f.marshalString(v, buf)
	case float64:
		buf.WriteString(f.sprintColor(f.NumberColor, strconv.FormatFloat(v, 'f', -1, 64)))
	case bool:
		buf.WriteString(f.sprintColor(f.BoolColor, (strconv.FormatBool(v))))
	case nil:
		buf.WriteString(f.sprintColor(f.NullColor, null))
	}
}

func (f *Formatter) marshalString(str string, buf *bytes.Buffer) {
	if !f.RawStrings {
		strBytes, _ := json.Marshal(str)
		str = string(strBytes)
	}

	if f.StringMaxLength != 0 && len(str) >= f.StringMaxLength {
		str = fmt.Sprintf("%s...", str[0:f.StringMaxLength])
	}

	buf.WriteString(f.sprintColor(f.StringColor, str))
}

func (f *Formatter) Read(p []byte) (n int, err error) {
	return f.r.Read(p)
}
