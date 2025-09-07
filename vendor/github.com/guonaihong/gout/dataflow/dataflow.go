package dataflow

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"text/template"
	"time"

	"github.com/guonaihong/gout/debug"
	"github.com/guonaihong/gout/decode"
	"github.com/guonaihong/gout/encode"
	"github.com/guonaihong/gout/enjson"
	"github.com/guonaihong/gout/hcutil"
	"github.com/guonaihong/gout/middler"
	"github.com/guonaihong/gout/middleware/rsp/autodecodebody"
	"github.com/guonaihong/gout/setting"
)

const (
	get     = "GET"
	post    = "POST"
	put     = "PUT"
	delete2 = "DELETE"
	patch   = "PATCH"
	head    = "HEAD"
	options = "OPTIONS"
)

// DataFlow is the core data structure,
// including the encoder and decoder of http data
type DataFlow struct {
	Req
	out *Gout
}

// GET send HTTP GET method
func (df *DataFlow) GET(url string, urlStruct ...interface{}) *DataFlow {
	df.Req, df.Err = reqDef(get, cleanPaths(url), df.out, urlStruct...)
	return df
}

// POST send HTTP POST method
func (df *DataFlow) POST(url string, urlStruct ...interface{}) *DataFlow {
	df.Req, df.Err = reqDef(post, cleanPaths(url), df.out, urlStruct...)
	return df
}

// PUT send HTTP PUT method
func (df *DataFlow) PUT(url string, urlStruct ...interface{}) *DataFlow {
	df.Req, df.Err = reqDef(put, cleanPaths(url), df.out, urlStruct...)
	return df
}

// DELETE send HTTP DELETE method
func (df *DataFlow) DELETE(url string, urlStruct ...interface{}) *DataFlow {
	df.Req, df.Err = reqDef(delete2, cleanPaths(url), df.out, urlStruct...)
	return df
}

// PATCH send HTTP PATCH method
func (df *DataFlow) PATCH(url string, urlStruct ...interface{}) *DataFlow {
	df.Req, df.Err = reqDef(patch, cleanPaths(url), df.out, urlStruct...)
	return df
}

// HEAD send HTTP HEAD method
func (df *DataFlow) HEAD(url string, urlStruct ...interface{}) *DataFlow {
	df.Req, df.Err = reqDef(head, cleanPaths(url), df.out, urlStruct...)
	return df
}

// OPTIONS send HTTP OPTIONS method
func (df *DataFlow) OPTIONS(url string, urlStruct ...interface{}) *DataFlow {
	df.Req, df.Err = reqDef(options, cleanPaths(url), df.out, urlStruct...)
	return df
}

// SetSetting
func (df *DataFlow) SetSetting(s setting.Setting) *DataFlow {
	df.Req.Setting = s
	return df
}

// SetHost set host
func (df *DataFlow) SetHost(host string) *DataFlow {
	if df.Err != nil {
		return df
	}

	df.Req.host = host

	df.Req.g = df.out
	return df
}

// GetHost return value->host or host:port
func (df *DataFlow) GetHost() (string, error) {
	if df.req != nil {
		return df.req.URL.Host, nil
	}

	if len(df.host) > 0 {
		return df.host, nil
	}

	if len(df.url) > 0 {
		url, err := url.Parse(df.url)
		if err != nil {
			return "", err
		}
		return url.Host, nil
	}

	return "", errors.New("not url found")
}

// SetMethod set method
func (df *DataFlow) SetMethod(method string) *DataFlow {
	if df.Err != nil {
		return df
	}

	df.Req.method = method
	df.Req.g = df.out
	return df
}

// SetURL set url
func (df *DataFlow) SetURL(url string, urlStruct ...interface{}) *DataFlow {
	if df.Err != nil {
		return df
	}

	if len(urlStruct) > 0 {
		var out strings.Builder
		tpl := template.Must(template.New(url).Parse(url))
		err := tpl.Execute(&out, urlStruct[0])
		if err != nil {
			df.Err = err
			return df
		}
		url = out.String()
	}

	df.Req.url = modifyURL(cleanPaths(url))
	return df
}

func (df *DataFlow) SetRequest(req *http.Request) *DataFlow {
	df.req = req
	return df
}

// SetBody set the data to the http body, Support string/bytes/io.Reader
func (df *DataFlow) SetBody(obj interface{}) *DataFlow {
	df.Req.bodyEncoder = encode.NewBodyEncode(obj)
	return df
}

// SetForm send form data to the http body, Support struct/map/array/slice
func (df *DataFlow) SetForm(obj ...interface{}) *DataFlow {
	df.Req.form = append([]interface{}{}, obj...)
	return df
}

// SetWWWForm send x-www-form-urlencoded to the http body, Support struct/map/array/slice types
func (df *DataFlow) SetWWWForm(obj ...interface{}) *DataFlow {
	df.Req.wwwForm = append([]interface{}{}, obj...)
	return df
}

// SetQuery send URL query string, Support string/[]byte/struct/map/slice types
func (df *DataFlow) SetQuery(obj ...interface{}) *DataFlow {
	df.Req.queryEncode = append([]interface{}{}, obj...)
	return df
}

// SetHeader send http header, Support struct/map/slice types
func (df *DataFlow) SetHeader(obj ...interface{}) *DataFlow {
	df.Req.headerEncode = append([]interface{}{}, obj...)
	return df
}

// SetHeader send http header, Support struct/map/slice types
func (df *DataFlow) SetHeaderRaw(obj ...interface{}) *DataFlow {
	df.Req.headerEncode = append([]interface{}{}, obj...)
	df.Req.rawHeader = true
	return df
}

// SetJSON send json to the http body, Support raw json(string, []byte)/struct/map types
func (df *DataFlow) SetJSON(obj interface{}) *DataFlow {
	df.ReqBodyType = "json"
	df.EscapeHTML = true
	df.Req.bodyEncoder = enjson.NewJSONEncode(obj, true)
	return df
}

// SetJSON send json to the http body, Support raw json(string, []byte)/struct/map types
// 与SetJSON的区一区别就是不转义HTML里面的标签
func (df *DataFlow) SetJSONNotEscape(obj interface{}) *DataFlow {
	df.ReqBodyType = "json"
	df.Req.bodyEncoder = enjson.NewJSONEncode(obj, false)
	return df
}

// SetXML send xml to the http body
func (df *DataFlow) SetXML(obj interface{}) *DataFlow {
	df.ReqBodyType = "xml"
	df.Req.bodyEncoder = encode.NewXMLEncode(obj)
	return df
}

// SetYAML send yaml to the http body, Support struct,map types
func (df *DataFlow) SetYAML(obj interface{}) *DataFlow {
	df.ReqBodyType = "yaml"
	df.Req.bodyEncoder = encode.NewYAMLEncode(obj)
	return df
}

// SetProtoBuf send yaml to the http body, Support struct types
// obj必须是结构体指针或者[]byte类型
func (df *DataFlow) SetProtoBuf(obj interface{}) *DataFlow {
	df.ReqBodyType = "protobuf"
	df.Req.bodyEncoder = encode.NewProtoBufEncode(obj)
	return df
}

// UnixSocket 函数会修改Transport, 请像对待全局变量一样对待UnixSocket
// 对于全局变量的解释可看下面的链接
// https://github.com/guonaihong/gout/issues/373
func (df *DataFlow) UnixSocket(path string) *DataFlow {
	df.Err = hcutil.UnixSocket(df.out.Client, path)
	return df
}

// SetProxy 函数会修改Transport，请像对待全局变量一样对待SetProxy
// 对于全局变量的解释可看下面的链接
// https://github.com/guonaihong/gout/issues/373
func (df *DataFlow) SetProxy(proxyURL string) *DataFlow {
	df.Err = hcutil.SetProxy(df.out.Client, proxyURL)
	return df
}

// SetSOCKS5 函数会修改Transport,请像对待全局变量一样对待SetSOCKS5
// 对于全局变量的解释可看下面的链接
// https://github.com/guonaihong/gout/issues/373
func (df *DataFlow) SetSOCKS5(addr string) *DataFlow {
	df.Err = hcutil.SetSOCKS5(df.out.Client, addr)
	return df
}

// SetCookies set cookies
func (df *DataFlow) SetCookies(c ...*http.Cookie) *DataFlow {
	df.Req.cookies = append(df.Req.cookies, c...)
	return df
}

// BindHeader parse http header to obj variable.
// obj must be a pointer variable
// Support string/int/float/slice ... types
func (df *DataFlow) BindHeader(obj interface{}) *DataFlow {
	df.Req.headerDecode = obj
	return df
}

// BindBody parse the variables in http body to obj.
// obj must be a pointer variable
func (df *DataFlow) BindBody(obj interface{}) *DataFlow {
	if obj == nil {
		return df
	}
	df.Req.bodyDecoder = append(df.Req.bodyDecoder, decode.NewBodyDecode(obj))
	return df
}

// BindJSON parse the json string in http body to obj.
// obj must be a pointer variable
func (df *DataFlow) BindJSON(obj interface{}) *DataFlow {
	if obj == nil {
		return df
	}
	df.out.RspBodyType = "json"
	df.Req.bodyDecoder = append(df.Req.bodyDecoder, decode.NewJSONDecode(obj))
	return df
}

// BindYAML parse the yaml string in http body to obj.
// obj must be a pointer variable
func (df *DataFlow) BindYAML(obj interface{}) *DataFlow {
	if obj == nil {
		return df
	}
	df.RspBodyType = "yaml"
	df.Req.bodyDecoder = append(df.Req.bodyDecoder, decode.NewYAMLDecode(obj))
	return df
}

// BindXML parse the xml string in http body to obj.
// obj must be a pointer variable
func (df *DataFlow) BindXML(obj interface{}) *DataFlow {
	if obj == nil {
		return df
	}
	df.RspBodyType = "xml"
	df.Req.bodyDecoder = append(df.Req.bodyDecoder, decode.NewXMLDecode(obj))
	return df
}

// BindDecoder allow user parse data by their own decoder
func (df *DataFlow) BindDecoder(decode decode.Decoder) *DataFlow {
	df.Req.bodyDecoder = append(df.Req.bodyDecoder, decode)
	return df
}

// Code parse the http code into the variable httpCode
func (df *DataFlow) Code(httpCode *int) *DataFlow {
	df.Req.httpCode = httpCode
	return df
}

// Callback parse the http body into obj according to the condition (json or string)
func (df *DataFlow) Callback(cb func(*Context) error) *DataFlow {
	df.Req.callback = cb
	return df
}

// Chunked
func (df *DataFlow) Chunked() *DataFlow {
	df.Req.Chunked()
	return df
}

// SetTimeout set timeout, and WithContext are mutually exclusive functions
func (df *DataFlow) SetTimeout(d time.Duration) *DataFlow {
	df.Req.SetTimeout(d)
	return df
}

// WithContext set context, and SetTimeout are mutually exclusive functions
func (df *DataFlow) WithContext(c context.Context) *DataFlow {
	df.Req.c = c
	return df
}

// SetBasicAuth
func (df *DataFlow) SetBasicAuth(username, password string) *DataFlow {
	df.Req.userName = &username
	df.Req.password = &password
	return df
}

// Request middleware
func (df *DataFlow) RequestUse(reqModify ...middler.RequestMiddler) *DataFlow {
	if len(reqModify) > 0 {
		df.reqModify = append(df.reqModify, reqModify...)
	}
	return df
}

// Response middleware
func (df *DataFlow) ResponseUse(responseModify ...middler.ResponseMiddler) *DataFlow {
	if len(responseModify) > 0 {
		df.responseModify = append(df.responseModify, responseModify...)
	}
	return df
}

// Debug start debug mode
func (df *DataFlow) Debug(d ...interface{}) *DataFlow {
	for _, v := range d {
		switch opt := v.(type) {
		case bool:
			if opt {
				debug.DefaultDebug(&df.Setting.Options)
			}
		case debug.Apply:
			opt.Apply(&df.Setting.Options)
		}
	}

	return df
}

// https://github.com/guonaihong/gout/issues/264
// When calling SetWWWForm(), the Content-Type header will be added automatically,
// and calling NoAutoContentType() will not add an HTTP header
//
// SetWWWForm "Content-Type", "application/x-www-form-urlencoded"
// SetJSON "Content-Type", "application/json"
func (df *DataFlow) NoAutoContentType() *DataFlow {
	df.Req.NoAutoContentType = true
	return df
}

// https://github.com/guonaihong/gout/issues/343
// content-encoding会指定response body的压缩方法，支持常用的压缩，gzip, deflate, br等
func (df *DataFlow) AutoDecodeBody() *DataFlow {
	return df.ResponseUse(middler.WithResponseMiddlerFunc(autodecodebody.AutoDecodeBody))
}

func (df *DataFlow) IsDebug() bool {
	return df.Setting.Debug
}

// Do send function
func (df *DataFlow) Do() (err error) {
	return df.Req.Do()
}

// Filter filter function, use this function to turn on the filter function
func (df *DataFlow) Filter() *filter {
	return &filter{df: df}
}

// F filter function, use this function to turn on the filter function
func (df *DataFlow) F() *filter {
	return df.Filter()
}

// Export filter function, use this function to turn on the filter function
func (df *DataFlow) Export() *export {
	return &export{df: df}
}

// E filter function, use this function to turn on the filter function
func (df *DataFlow) E() *export {
	return df.Export()
}

func (df *DataFlow) SetGout(out *Gout) {
	df.out = out
	df.Req.g = out
}
