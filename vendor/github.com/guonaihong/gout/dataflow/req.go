package dataflow

import (
	"bytes"
	"context"
	"fmt"
	"github.com/guonaihong/gout/core"
	"github.com/guonaihong/gout/decode"
	"github.com/guonaihong/gout/encode"
	api "github.com/guonaihong/gout/interface"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"
)

type Do interface {
	Do(*http.Request) (*http.Response, error)
}

// Req controls core data structure of http request
type Req struct {
	method string
	url    string
	host   string

	form    []interface{}
	wwwForm []interface{}

	// http body
	bodyEncoder encode.Encoder
	bodyDecoder decode.Decoder

	// http header
	headerEncode []interface{}
	headerDecode interface{}

	// query
	queryEncode []interface{}

	httpCode *int
	g        *Gout

	callback func(*Context) error

	//cookie
	cookies []*http.Cookie

	timeout time.Duration

	//自增id，主要给互斥API定优先级
	//对于互斥api，后面的会覆盖前面的
	index        int
	timeoutIndex int
	ctxIndex     int

	c   context.Context
	Err error

	reqModify []api.RequestMiddler
	req       *http.Request
}

// Reset 重置 Req结构体
// req 结构布局说明，以decode为例
// body 可以支持text, json, yaml, xml，所以定义成接口形式
// headerDecode只有一个可能，就定义为具体类型。这里他们的decode实现也不一样
// 有没有必要，归一化成一种??? TODO:
func (r *Req) Reset() {
	r.index = 0
	r.Err = nil
	r.cookies = nil
	r.form = nil
	r.wwwForm = nil
	r.bodyEncoder = nil
	r.bodyDecoder = nil
	r.httpCode = nil
	r.headerDecode = nil
	r.headerEncode = nil
	r.queryEncode = nil
	r.reqModify = nil
	r.c = nil
	r.req = nil
}

func isAndGetString(x interface{}) (string, bool) {
	p := reflect.ValueOf(x)

	for p.Kind() == reflect.Ptr {
		p = p.Elem()
	}

	if s, ok := core.GetString(p.Interface()); ok {
		if strings.HasPrefix(s, "?") {
			s = s[1:]
		}
		return s, true
	}
	return "", false
}

func (r *Req) addDefDebug() {
	if r.bodyEncoder != nil {
		switch bodyType := r.bodyEncoder.(encode.Encoder); bodyType.Name() {
		case "json":
			r.g.opt.ReqBodyType = "json"
		case "xml":
			r.g.opt.ReqBodyType = "xml"
		case "yaml":
			r.g.opt.ReqBodyType = "yaml"
		}
	}

}

func (r *Req) addContextType(req *http.Request) {
	if r.wwwForm != nil {
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	}

	if r.bodyEncoder != nil {
		switch bodyType := r.bodyEncoder.(encode.Encoder); bodyType.Name() {
		case "json":
			req.Header.Add("Content-Type", "application/json")
		case "xml":
			req.Header.Add("Content-Type", "application/xml")
		case "yaml":
			req.Header.Add("Content-Type", "application/x-yaml")
		}
	}

}

func (r *Req) selectRequest(body *bytes.Buffer) (req *http.Request, err error) {
	req = r.req

	defer func() {
		if err != nil {
			r.Err = err
			return
		}

		if len(r.method) > 0 {
			req.Method = r.method
		}

		if len(r.url) > 0 {
			req.URL, err = url.Parse(r.url)
			if err != nil {
				r.Err = err
				return
			}
		}

		if len(r.host) > 0 {
			urlStr := modifyURL(joinPaths("", r.host))
			URL, err := url.Parse(urlStr)
			if err != nil {
				r.Err = err
				return
			}

			if req.URL == nil {
				req.URL = URL
				r.Err = err
				return
			}

			req.URL.Scheme = URL.Scheme
			req.URL.Host = URL.Host

		}
	}()

	if req == nil {
		return http.NewRequest(r.method, r.url, body)
	}

	return
}

func (r *Req) encodeQuery() error {
	var query string
	q := encode.NewQueryEncode(nil)

	for _, queryEncode := range r.queryEncode {
		if qStr, ok := isAndGetString(queryEncode); ok {
			joiner := "&"
			if len(query) == 0 {
				joiner = ""
			}

			query += joiner + qStr
			continue
		}

		if err := encode.Encode(queryEncode, q); err != nil {
			return err
		}

	}

	joiner := "&"
	if len(query) == 0 {
		joiner = ""
	}

	query += joiner + q.End()
	if len(query) > 0 {
		r.url += "?" + query
	}

	return nil
}

func (r *Req) encodeForm(body *bytes.Buffer, f *encode.FormEncode) error {
	for _, body := range r.form {
		if err := encode.Encode(body, f); err != nil {
			return err
		}

	}
	return f.End()
}

func (r *Req) encodeWWWForm(body *bytes.Buffer) error {
	enc := encode.NewWWWFormEncode()
	for _, formData := range r.wwwForm {
		if err := enc.Encode(formData); err != nil {
			return err
		}
	}

	return enc.End(body)
}

// Request Get the http.Request object
func (r *Req) Request() (req *http.Request, err error) {
	if r.Err != nil {
		return nil, r.Err
	}

	body := &bytes.Buffer{}

	// 如果同时传递调用SetWWWForm和SetJSON函数，默认json优先级别比较高
	if r.wwwForm != nil {
		if err := r.encodeWWWForm(body); err != nil {
			return nil, err
		}
	}

	// set http body
	if r.bodyEncoder != nil {
		body.Reset()
		if err := r.bodyEncoder.Encode(body); err != nil {
			return nil, err
		}
	}

	// set query header
	if r.queryEncode != nil {
		if err := r.encodeQuery(); err != nil {
			return nil, err
		}
	}

	var f *encode.FormEncode

	// TODO
	// 可以考虑和 bodyEncoder合并,
	// 头疼的是f.FormDataContentType如何合并，每个encoder都实现这个方法???
	if r.form != nil {

		f = encode.NewFormEncode(body)
		if err := r.encodeForm(body, f); err != nil {
			return nil, err
		}
	}

	req, err = r.selectRequest(body)
	if err != nil {
		return nil, err
	}
	// 放这个位置不会误删除SetForm的http header
	if r.headerEncode != nil {
		clearHeader(req.Header)
	}

	_ = r.GetContext()
	if r.c != nil {
		req = req.WithContext(r.c)
	}

	for _, c := range r.cookies {
		req.AddCookie(c)
	}

	if r.form != nil {
		req.Header.Add("Content-Type", f.FormDataContentType())
	}

	// set http header
	if r.headerEncode != nil {
		err = r.encodeHeader(req)
		if err != nil {
			return nil, err
		}
	}

	r.addDefDebug()
	r.addContextType(req)
	//运行请求中间件
	for _, reqModify := range r.reqModify {
		if err = reqModify.ModifyRequest(req); err != nil {
			return nil, err
		}
	}

	return req, nil
}

func (r *Req) encodeHeader(req *http.Request) (err error) {
	for _, h := range r.headerEncode {
		if h == nil {
			continue
		}

		err = encode.Encode(h, encode.NewHeaderEncode(req))
		if err != nil {
			return err
		}
	}

	return nil
}

func clearHeader(header http.Header) {
	for k := range header {
		delete(header, k)
	}
}

func (r *Req) GetContext() context.Context {
	if r.timeout > 0 && r.timeoutIndex > r.ctxIndex {
		r.c, _ = context.WithTimeout(context.Background(), r.timeout)
	}
	return r.c
}

func (r *Req) decode(req *http.Request, resp *http.Response, openDebug bool) (err error) {
	defer func() {
		if err == io.EOF {
			err = nil
		}
	}()

	if r.httpCode != nil {
		*r.httpCode = resp.StatusCode
	}

	if r.headerDecode != nil {
		err = decode.Header.Decode(resp, r.headerDecode)
		if err != nil {
			return err
		}
	}

	if openDebug {
		// This is code(output debug info) be placed here
		// all, err := ioutil.ReadAll(resp.Body)
		// respBody  = bytes.NewReader(all)
		if err = r.g.opt.resetBodyAndPrint(req, resp); err != nil {
			return err
		}
	}

	if r.bodyDecoder != nil {
		if err = r.bodyDecoder.Decode(resp.Body); err != nil {
			return err
		}
	}

	return nil
}

func (r *Req) getDataFlow() *DataFlow {
	return &r.g.DataFlow
}

const maxBodySlurpSize = 4 * (2 << 10) // 4KB
func clearBody(resp *http.Response) error {
	// 这里限制下io.Copy的大小
	_, err := io.CopyN(ioutil.Discard, resp.Body, maxBodySlurpSize)
	if err == io.EOF {
		err = nil
	}
	if err != nil {
		return err
	}
	return err
}

func (r *Req) Bind(req *http.Request, resp *http.Response) (err error) {

	if err = r.decode(req, resp, r.g.opt.Debug); err != nil {
		return err
	}

	if r.callback != nil {
		// 注意这里的r.callback使用了r.DataFlow的地址, r.callback和r.decode操作的是同一个的DataFlow
		// 执行r.callback只是装载解码器, 后面的r.decode才是真正的解码
		c := Context{Code: resp.StatusCode, DataFlow: r.getDataFlow()}
		if err := r.callback(&c); err != nil {
			return err
		}

		if err = r.decode(req, resp, false); err != nil {
			return err
		}
	}

	// 如果没有设置解码器
	if r.bodyDecoder == nil {
		return clearBody(resp)
	}

	return nil

}

func (r *Req) Client() *http.Client {
	if r.g == nil {
		return &DefaultClient
	}

	return r.g.Client
}

func (r *Req) getDebugOpt() *DebugOption {
	return &r.g.opt
}

func (r *Req) canTrace() bool {
	opt := r.getDebugOpt()
	return opt.Trace
}

// Do Send function
func (r *Req) Do() (err error) {
	if r.Err != nil {
		return r.Err
	}

	// reset  Req
	defer r.Reset()

	req, err := r.Request()
	if err != nil {
		return err
	}

	opt := r.getDebugOpt()

	//resp, err := r.Client().Do(req)
	//TODO r.Client() 返回Do接口
	resp, err := opt.startTrace(opt, r.canTrace(), req, r.Client())
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	return r.Bind(req, resp)
}

func modifyURL(url string) string {
	if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://") {
		return url
	}

	if strings.HasPrefix(url, ":") {
		return fmt.Sprintf("http://127.0.0.1%s", url)
	}

	if strings.HasPrefix(url, "/") {
		return fmt.Sprintf("http://127.0.0.1%s", url)
	}

	return fmt.Sprintf("http://%s", url)
}

func reqDef(method string, url string, g *Gout) Req {
	return Req{method: method, url: modifyURL(url), g: g}
}
