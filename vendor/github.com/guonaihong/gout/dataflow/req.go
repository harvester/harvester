package dataflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"text/template"

	"github.com/guonaihong/gout/core"
	"github.com/guonaihong/gout/debug"
	"github.com/guonaihong/gout/decode"
	"github.com/guonaihong/gout/encode"
	"github.com/guonaihong/gout/encoder"
	"github.com/guonaihong/gout/middler"
	"github.com/guonaihong/gout/setting"
)

// Req controls core data structure of http request
type Req struct {
	method   string
	url      string
	host     string
	userName *string
	password *string

	form    []interface{}
	wwwForm []interface{}

	// http body
	bodyEncoder encoder.Encoder
	bodyDecoder []decode.Decoder

	// http header
	headerEncode []interface{}
	// raw header
	rawHeader bool

	headerDecode interface{}

	// query
	queryEncode []interface{}

	httpCode *int
	g        *Gout

	callback func(*Context) error

	// cookie
	cookies []*http.Cookie

	ctxIndex int

	c   context.Context
	Err error

	reqModify []middler.RequestMiddler

	responseModify []middler.ResponseMiddler

	req *http.Request

	// 内嵌字段
	setting.Setting

	cancel context.CancelFunc
}

// Reset 重置 Req结构体
// req 结构布局说明，以decode为例
// body 可以支持text, json, yaml, xml，所以定义成接口形式
// headerDecode只有一个可能，就定义为具体类型。这里他们的decode实现也不一样
// 有没有必要，归一化成一种??? TODO:
func (r *Req) Reset() {
	if r.cancel != nil {
		r.cancel()
	}
	r.Setting.Reset()
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
		return strings.TrimPrefix(s, "?"), true
	}
	return "", false
}

func (r *Req) addDefDebug() {
	if r.bodyEncoder != nil {
		switch bodyType := r.bodyEncoder; bodyType.Name() {
		case "json":
			r.ReqBodyType = "json"
		case "xml":
			r.ReqBodyType = "xml"
		case "yaml":
			r.ReqBodyType = "yaml"
		}
	}
}

func (r *Req) addContextType(req *http.Request) {
	if req.Header.Get("Content-Type") != "" {
		return
	}

	if r.wwwForm != nil {
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	}

	if r.bodyEncoder != nil {
		switch bodyType := r.bodyEncoder; bodyType.Name() {
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
			urlStr := modifyURL(cleanPaths(r.host))
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
	q := encode.NewQueryEncode(r.Setting)

	for _, queryEncode := range r.queryEncode {
		if queryEncode == nil {
			continue
		}

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
		if body == nil {
			continue
		}

		if err := encode.Encode(body, f); err != nil {
			return err
		}

	}
	return f.End()
}

func (r *Req) encodeWWWForm(body *bytes.Buffer) error {
	enc := encode.NewWWWFormEncode(r.Setting)
	for _, formData := range r.wwwForm {
		if formData == nil {
			continue
		}
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

	if r.form != nil && req.Header.Get("Content-Type") == "" {
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
	if !r.NoAutoContentType {
		r.addContextType(req)
	}

	if r.userName != nil && r.password != nil {
		req.SetBasicAuth(*r.userName, *r.password)
	}
	// 运行请求中间件
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

		err = encode.Encode(h, encode.NewHeaderEncode(req, r.rawHeader))
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

// retry模块需要context.Context，所以这里也返回context.Context
func (r *Req) GetContext() context.Context {
	if r.Timeout > 0 {
		if r.c == nil {
			r.c = context.TODO()
		}
		r.c, r.cancel = context.WithTimeout(r.c, r.Timeout)
	}
	return r.c
}

// TODO 优化代码，每个decode都有自己的指针偏移直接指向流，减少大body的内存使用
func (r *Req) decodeBody(req *http.Request, resp *http.Response) (err error) {
	if r.bodyDecoder != nil {
		// 当只有一个解码器时，直接在流上操作，避免读取整个响应体
		if len(r.bodyDecoder) == 1 {
			defer resp.Body.Close() // 确保在读取完成后关闭body
			return r.bodyDecoder[0].Decode(resp.Body)
		}

		// 当有多个解码器需要处理响应体时，才读取整个响应体到内存中
		all, err := ReadAll(resp)
		if err != nil {
			return err
		}
		resp.Body.Close() // 已经取走数据，直接关闭body

		for _, bodyDecoder := range r.bodyDecoder {
			if len(all) > 0 {
				resp.Body = ioutil.NopCloser(bytes.NewReader(all))
			}

			if err = bodyDecoder.Decode(resp.Body); err != nil {
				return err
			}
		}
	}

	return nil
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
		if err = decode.Header.Decode(resp, r.headerDecode); err != nil {
			return err
		}

		if err = valid.ValidateStruct(r.headerDecode); err != nil {
			return err
		}
	}

	if openDebug {
		// This is code(output debug info) be placed here
		// all, err := ioutil.ReadAll(resp.Body)
		// respBody  = bytes.NewReader(all)
		if err = r.ResetBodyAndPrint(req, resp); err != nil {
			return err
		}
	}
	// 运行响应中间件。放到debug打印后面，避免混淆请求返回内容
	for _, modify := range r.responseModify {
		err = modify.ModifyResponse(resp)
		if err != nil {
			return err
		}
	}

	return r.decodeBody(req, resp)
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
	if err = r.decode(req, resp, r.Setting.Debug); err != nil {
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

func (r *Req) getDebugOpt() *debug.Options {
	return &r.Setting.Options
}

func (r *Req) canTrace() bool {
	opt := r.getDebugOpt()
	return opt.Trace
}

// 使用chunked方式
func (r *Req) maybeUseChunked(req *http.Request) {
	if r.UseChunked {
		req.ContentLength = -1
	}
}

// getReqAndRsp 内部函数获取req和resp
func (r *Req) getReqAndRsp() (req *http.Request, rsp *http.Response, err error) {
	if r.Err != nil {
		return nil, nil, r.Err
	}

	req, err = r.Request()
	if err != nil {
		return
	}

	opt := r.getDebugOpt()

	// 如果调用Chunked()接口, 就使用chunked的数据包
	r.maybeUseChunked(req)

	// resp, err := r.Client().Do(req)
	// TODO r.Client() 返回Do接口
	rsp, err = opt.StartTrace(opt, r.canTrace(), req, r.Client())
	return
}

// Response 获取原始http.Response数据结构
func (r *Req) Response() (rsp *http.Response, err error) {
	_, rsp, err = r.getReqAndRsp()
	defer r.Reset()
	return
}

// Do Send function
func (r *Req) Do() (err error) {
	req, resp, err := r.getReqAndRsp()
	if resp != nil {
		defer resp.Body.Close()
	}
	// reset  Req
	defer r.Reset()

	if err != nil {
		return err
	}

	if err := r.Bind(req, resp); err != nil {
		return err
	}

	if r.bodyDecoder != nil {
		for _, bodyDecoder := range r.bodyDecoder {
			if err := valid.ValidateStruct(bodyDecoder.Value()); err != nil {
				return err
			}
		}
	}

	return nil
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

func reqDef(method string, url string, g *Gout, urlStruct ...interface{}) (Req, error) {
	if len(urlStruct) > 0 {
		var out strings.Builder
		tpl := template.Must(template.New(url).Parse(url))
		err := tpl.Execute(&out, urlStruct[0])
		if err != nil {
			return Req{}, err
		}
		url = out.String()
	}

	r := Req{method: method, url: modifyURL(url), g: g}

	r.Setting = GlobalSetting

	return r, nil
}

// ReadAll returns the whole response body as bytes.
// This is an optimized version of `io.ReadAll`.
func ReadAll(resp *http.Response) ([]byte, error) {
	if resp == nil {
		return nil, errors.New("response cannot be nil")
	}
	switch {
	case resp.ContentLength == 0:
		return []byte{}, nil
	// if we know the body length we can allocate the buffer only once
	case resp.ContentLength >= 0:
		body := make([]byte, resp.ContentLength)
		_, err := io.ReadFull(resp.Body, body)
		if err != nil {
			return nil, fmt.Errorf("failed to read the response body with a known length %d: %w", resp.ContentLength, err)
		}
		return body, nil

	default:
		// using `bytes.NewBuffer` + `io.Copy` is much faster than `io.ReadAll`
		// see https://github.com/elastic/beats/issues/36151#issuecomment-1931696767
		buf := bytes.NewBuffer(nil)
		_, err := io.Copy(buf, resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read the response body with unknown length: %w", err)
		}
		body := buf.Bytes()
		if body == nil {
			body = []byte{}
		}
		return body, nil
	}
}
