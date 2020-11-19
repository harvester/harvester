package dataflow

import (
	"context"
	"errors"
	"fmt"
	"github.com/guonaihong/gout/decode"
	"github.com/guonaihong/gout/encode"
	api "github.com/guonaihong/gout/interface"
	"golang.org/x/net/proxy"
	"net"
	"net/http"
	"net/url"
	"time"
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
func (df *DataFlow) GET(url string) *DataFlow {
	df.Req = reqDef(get, joinPaths("", url), df.out)
	return df
}

// POST send HTTP POST method
func (df *DataFlow) POST(url string) *DataFlow {
	df.Req = reqDef(post, joinPaths("", url), df.out)
	return df
}

// PUT send HTTP PUT method
func (df *DataFlow) PUT(url string) *DataFlow {
	df.Req = reqDef(put, joinPaths("", url), df.out)
	return df
}

// DELETE send HTTP DELETE method
func (df *DataFlow) DELETE(url string) *DataFlow {
	df.Req = reqDef(delete2, joinPaths("", url), df.out)
	return df
}

// PATCH send HTTP PATCH method
func (df *DataFlow) PATCH(url string) *DataFlow {
	df.Req = reqDef(patch, joinPaths("", url), df.out)
	return df
}

// HEAD send HTTP HEAD method
func (df *DataFlow) HEAD(url string) *DataFlow {
	df.Req = reqDef(head, joinPaths("", url), df.out)
	return df
}

// OPTIONS send HTTP OPTIONS method
func (df *DataFlow) OPTIONS(url string) *DataFlow {
	df.Req = reqDef(options, joinPaths("", url), df.out)
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
func (df *DataFlow) SetURL(url string) *DataFlow {
	if df.Err != nil {
		return df
	}

	if df.Req.url == "" && df.Req.req == nil && df.Req.method == "" {
		df.Req = reqDef("", joinPaths("", url), df.out)
		return df
	}

	df.Req.url = modifyURL(joinPaths("", url))

	return df
}

func (df *DataFlow) SetRequest(req *http.Request) *DataFlow {
	df.req = req
	return df
}

// SetBody set the data to the http body, Support string/bytes/io.Reader
func (df *DataFlow) SetBody(obj interface{}) *DataFlow {
	if obj == nil {
		df.Err = errors.New("SetBody:the parameter is a nil pointer")
	}

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

// SetJSON send json to the http body, Support raw json(string, []byte)/struct/map types
func (df *DataFlow) SetJSON(obj interface{}) *DataFlow {
	df.out.opt.ReqBodyType = "json"
	df.Req.bodyEncoder = encode.NewJSONEncode(obj)
	return df
}

// SetXML send xml to the http body
func (df *DataFlow) SetXML(obj interface{}) *DataFlow {
	df.out.opt.ReqBodyType = "xml"
	df.Req.bodyEncoder = encode.NewXMLEncode(obj)
	return df
}

// SetYAML send yaml to the http body, Support struct,map types
func (df *DataFlow) SetYAML(obj interface{}) *DataFlow {
	df.out.opt.ReqBodyType = "yaml"
	df.Req.bodyEncoder = encode.NewYAMLEncode(obj)
	return df
}

func (df *DataFlow) initTransport() {
	if df.out.Client.Transport == nil {
		df.out.Client.Transport = &http.Transport{}
	}
}

func (df *DataFlow) getTransport() (*http.Transport, bool) {
	// 直接return df.out.Client.Transport.(*http.Transport) 等于下面的写法
	// ts := df.out.Client.Transport.(*http.Transport)
	// return ts 编译会报错
	ts, ok := df.out.Client.Transport.(*http.Transport)
	return ts, ok
}

// UnixSocket 函数会修改Transport, 请像对待全局变量一样对待UnixSocket
func (df *DataFlow) UnixSocket(path string) *DataFlow {

	df.initTransport()

	transport, ok := df.getTransport()
	if !ok {
		df.Req.Err = fmt.Errorf("UnixSocket:not found http.transport:%T", df.out.Client.Transport)
		return df
	}

	transport.Dial = func(proto, addr string) (conn net.Conn, err error) {
		return net.Dial("unix", path)
	}

	return df
}

// SetProxy 函数会修改Transport，请像对待全局变量一样对待SetProxy
func (df *DataFlow) SetProxy(proxyURL string) *DataFlow {
	proxy, err := url.Parse(modifyURL(proxyURL))
	if err != nil {
		df.Req.Err = err
		return df
	}

	df.initTransport()

	transport, ok := df.getTransport()
	if !ok {
		df.Req.Err = fmt.Errorf("SetProxy:not found http.transport:%T", df.out.Client.Transport)
		return df
	}

	transport.Proxy = http.ProxyURL(proxy)

	return df
}

// SetSOCKS5 函数会修改Transport,请像对待全局变量一样对待SetSOCKS5
func (df *DataFlow) SetSOCKS5(addr string) *DataFlow {
	dialer, err := proxy.SOCKS5("tcp", addr, nil, proxy.Direct)
	if err != nil {
		df.Req.Err = err
		return df
	}

	df.initTransport()

	transport, ok := df.getTransport()
	if !ok {
		df.Req.Err = fmt.Errorf("SetSOCKS5:not found http.transport:%T", df.out.Client.Transport)
		return df
	}

	transport.Dial = dialer.Dial
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
		df.Err = errors.New("BindBody:the parameter is a nil pointer")
	}

	df.Req.bodyDecoder = decode.NewBodyDecode(obj)
	return df
}

// BindJSON parse the json string in http body to obj.
// obj must be a pointer variable
func (df *DataFlow) BindJSON(obj interface{}) *DataFlow {
	df.out.opt.RspBodyType = "json"
	df.Req.bodyDecoder = decode.NewJSONDecode(obj)
	return df
}

// BindYAML parse the yaml string in http body to obj.
// obj must be a pointer variable
func (df *DataFlow) BindYAML(obj interface{}) *DataFlow {
	df.out.opt.RspBodyType = "yaml"
	df.Req.bodyDecoder = decode.NewYAMLDecode(obj)
	return df
}

// BindXML parse the xml string in http body to obj.
// obj must be a pointer variable
func (df *DataFlow) BindXML(obj interface{}) *DataFlow {
	df.out.opt.RspBodyType = "xml"
	df.Req.bodyDecoder = decode.NewXMLDecode(obj)
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

// SetTimeout set timeout, and WithContext are mutually exclusive functions
func (df *DataFlow) SetTimeout(d time.Duration) *DataFlow {
	df.Req.index++
	df.Req.timeoutIndex = df.Req.index
	df.Req.timeout = d
	return df
}

// WithContext set context, and SetTimeout are mutually exclusive functions
func (df *DataFlow) WithContext(c context.Context) *DataFlow {
	df.Req.index++
	df.Req.ctxIndex = df.Req.index
	df.Req.c = c
	return df
}

// Request middleware
func (df *DataFlow) RequestUse(reqModify ...api.RequestMiddler) *DataFlow {
	if len(reqModify) > 0 {
		df.reqModify = append(df.reqModify, reqModify...)
	}
	return df
}

// Debug start debug mode
func (df *DataFlow) Debug(d ...interface{}) *DataFlow {
	for _, v := range d {
		switch opt := v.(type) {
		case bool:
			if opt {
				defaultDebug(&df.out.opt)
			}
		case DebugOpt:
			opt.Apply(&df.out.opt)
		}
	}

	return df
}

func (df *DataFlow) IsDebug() bool {
	return df.out.opt.Debug
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
