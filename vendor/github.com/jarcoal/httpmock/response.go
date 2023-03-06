package httpmock

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jarcoal/httpmock/internal"
)

// fromThenKeyType is used by Then().
type fromThenKeyType struct{}

var fromThenKey = fromThenKeyType{}

type suggestedInfo struct {
	kind      string
	suggested string
}

// suggestedMethodKeyType is used by NewNotFoundResponder().
type suggestedKeyType struct{}

var suggestedKey = suggestedKeyType{}

// Responder is a callback that receives an [*http.Request] and returns
// a mocked response.
type Responder func(*http.Request) (*http.Response, error)

func (r Responder) times(name string, n int, fn ...func(...any)) Responder {
	count := 0
	return func(req *http.Request) (*http.Response, error) {
		count++
		if count > n {
			err := internal.StackTracer{
				Err: fmt.Errorf("Responder not found for %s %s (coz %s and already called %d times)", req.Method, req.URL, name, count),
			}
			if len(fn) > 0 {
				err.CustomFn = fn[0]
			}
			return nil, err
		}
		return r(req)
	}
}

// Times returns a [Responder] callable n times before returning an
// error. If the [Responder] is called more than n times and fn is
// passed and non-nil, it acts as the fn parameter of
// [NewNotFoundResponder], allowing to dump the stack trace to
// localize the origin of the call.
//
//	import (
//	  "testing"
//	  "github.com/jarcoal/httpmock"
//	)
//	...
//	func TestMyApp(t *testing.T) {
//	  ...
//	  // This responder is callable 3 times, then an error is returned and
//	  // the stacktrace of the call logged using t.Log()
//	  httpmock.RegisterResponder("GET", "/foo/bar",
//	    httpmock.NewStringResponder(200, "{}").Times(3, t.Log),
//	  )
func (r Responder) Times(n int, fn ...func(...any)) Responder {
	return r.times("Times", n, fn...)
}

// Once returns a new [Responder] callable once before returning an
// error. If the [Responder] is called 2 or more times and fn is passed
// and non-nil, it acts as the fn parameter of [NewNotFoundResponder],
// allowing to dump the stack trace to localize the origin of the
// call.
//
//	import (
//	  "testing"
//	  "github.com/jarcoal/httpmock"
//	)
//	...
//	func TestMyApp(t *testing.T) {
//	  ...
//	  // This responder is callable only once, then an error is returned and
//	  // the stacktrace of the call logged using t.Log()
//	  httpmock.RegisterResponder("GET", "/foo/bar",
//	    httpmock.NewStringResponder(200, "{}").Once(t.Log),
//	  )
func (r Responder) Once(fn ...func(...any)) Responder {
	return r.times("Once", 1, fn...)
}

// Trace returns a new [Responder] that allows to easily trace the calls
// of the original [Responder] using fn. It can be used in conjunction
// with the testing package as in the example below with the help of
// [*testing.T.Log] method:
//
//	import (
//	  "testing"
//	  "github.com/jarcoal/httpmock"
//	)
//	...
//	func TestMyApp(t *testing.T) {
//	  ...
//	  httpmock.RegisterResponder("GET", "/foo/bar",
//	    httpmock.NewStringResponder(200, "{}").Trace(t.Log),
//	  )
func (r Responder) Trace(fn func(...any)) Responder {
	return func(req *http.Request) (*http.Response, error) {
		resp, err := r(req)
		return resp, internal.StackTracer{
			CustomFn: fn,
			Err:      err,
		}
	}
}

// Delay returns a new [Responder] that calls the original r Responder
// after a delay of d.
//
//	import (
//	  "testing"
//	  "time"
//	  "github.com/jarcoal/httpmock"
//	)
//	...
//	func TestMyApp(t *testing.T) {
//	  ...
//	  httpmock.RegisterResponder("GET", "/foo/bar",
//	    httpmock.NewStringResponder(200, "{}").Delay(100*time.Millisecond),
//	  )
func (r Responder) Delay(d time.Duration) Responder {
	return func(req *http.Request) (*http.Response, error) {
		time.Sleep(d)
		return r(req)
	}
}

var errThenDone = errors.New("ThenDone")

// similar is simple but a bit tricky. Here we consider two Responder
// are similar if they share the same function, but not necessarily
// the same environment. It is only used by Then below.
func (r Responder) similar(other Responder) bool {
	return reflect.ValueOf(r).Pointer() == reflect.ValueOf(other).Pointer()
}

// Then returns a new [Responder] that calls r on first invocation, then
// next on following ones, except when Then is chained, in this case
// next is called only once:
//
//	A := httpmock.NewStringResponder(200, "A")
//	B := httpmock.NewStringResponder(200, "B")
//	C := httpmock.NewStringResponder(200, "C")
//
//	httpmock.RegisterResponder("GET", "/pipo", A.Then(B).Then(C))
//
//	http.Get("http://foo.bar/pipo") // A is called
//	http.Get("http://foo.bar/pipo") // B is called
//	http.Get("http://foo.bar/pipo") // C is called
//	http.Get("http://foo.bar/pipo") // C is called, and so on
//
// A panic occurs if next is the result of another Then call (because
// allowing it could cause inextricable problems at runtime). Then
// calls can be chained, but cannot call each other by
// parameter. Example:
//
//	A.Then(B).Then(C) // is OK
//	A.Then(B.Then(C)) // panics as A.Then() parameter is another Then() call
//
// See also [ResponderFromMultipleResponses].
func (r Responder) Then(next Responder) (x Responder) {
	var done int
	var mu sync.Mutex
	x = func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()

		ctx := req.Context()
		thenCalledUs, _ := ctx.Value(fromThenKey).(bool)
		if !thenCalledUs {
			req = req.WithContext(context.WithValue(ctx, fromThenKey, true))
		}

		switch done {
		case 0:
			resp, err := r(req)
			if err != errThenDone {
				if !x.similar(r) { // r is NOT a Then
					done = 1
				}
				return resp, err
			}
			fallthrough

		case 1:
			done = 2 // next is NEVER a Then, as it is forbidden by design
			return next(req)
		}
		if thenCalledUs {
			return nil, errThenDone
		}
		return next(req)
	}

	if next.similar(x) {
		panic("Then() does not accept another Then() Responder as parameter")
	}
	return
}

// SetContentLength returns a new [Responder] based on r that ensures
// the returned [*http.Response] ContentLength field and
// Content-Length header are set to the right value.
//
// If r returns an [*http.Response] with a nil Body or equal to
// [http.NoBody], the length is always set to 0.
//
// If r returned response.Body implements:
//
//	Len() int
//
// then the length is set to the Body.Len() returned value. All
// httpmock generated bodies implement this method. Beware that
// [strings.Builder], [strings.Reader], [bytes.Buffer] and
// [bytes.Reader] types used with [io.NopCloser] do not implement
// Len() anymore.
//
// Otherwise, r returned response.Body is entirely copied into an
// internal buffer to get its length, then it is closed. The Body of
// the [*http.Response] returned by the [Responder] returned by
// SetContentLength can then be read again to return its content as
// usual. But keep in mind that each time this [Responder] is called,
// r is called first. So this one has to carefully handle its body: it
// is highly recommended to use [NewRespBodyFromString] or
// [NewRespBodyFromBytes] to set the body once (as
// [NewStringResponder] and [NewBytesResponder] do behind the scene),
// or to build the body each time r is called.
//
// The following calls are all correct:
//
//	responder = httpmock.NewStringResponder(200, "BODY").SetContentLength()
//	responder = httpmock.NewBytesResponder(200, []byte("BODY")).SetContentLength()
//	responder = ResponderFromResponse(&http.Response{
//	  // build a body once, but httpmock knows how to "rearm" it once read
//	  Body:          NewRespBodyFromString("BODY"),
//	  StatusCode:    200,
//	}).SetContentLength()
//	responder = httpmock.Responder(func(req *http.Request) (*http.Response, error) {
//	  // build a new body for each call
//	  return &http.Response{
//	    StatusCode: 200,
//	    Body:       io.NopCloser(strings.NewReader("BODY")),
//	  }, nil
//	}).SetContentLength()
//
// But the following is not correct:
//
//	responder = httpmock.ResponderFromResponse(&http.Response{
//	  StatusCode: 200,
//	  Body:       io.NopCloser(strings.NewReader("BODY")),
//	}).SetContentLength()
//
// it will only succeed for the first responder call. The following
// calls will deliver responses with an empty body, as it will already
// been read by the first call.
func (r Responder) SetContentLength() Responder {
	return func(req *http.Request) (*http.Response, error) {
		resp, err := r(req)
		if err != nil {
			return nil, err
		}
		nr := *resp
		switch nr.Body {
		case nil:
			nr.Body = http.NoBody
			fallthrough
		case http.NoBody:
			nr.ContentLength = 0
		default:
			bl, ok := nr.Body.(interface{ Len() int })
			if !ok {
				copyBody := &dummyReadCloser{orig: nr.Body}
				bl, nr.Body = copyBody, copyBody
			}
			nr.ContentLength = int64(bl.Len())
		}
		if nr.Header == nil {
			nr.Header = http.Header{}
		}
		nr.Header = nr.Header.Clone()
		nr.Header.Set("Content-Length", strconv.FormatInt(nr.ContentLength, 10))
		return &nr, nil
	}
}

// HeaderAdd returns a new [Responder] based on r that ensures the
// returned [*http.Response] includes h header. It adds each h entry
// to the header. It appends to any existing values associated with
// each h key. Each key is case insensitive; it is canonicalized by
// [http.CanonicalHeaderKey].
//
// See also [Responder.HeaderSet] and [Responder.SetContentLength].
func (r Responder) HeaderAdd(h http.Header) Responder {
	return func(req *http.Request) (*http.Response, error) {
		resp, err := r(req)
		if err != nil {
			return nil, err
		}
		nr := *resp
		if nr.Header == nil {
			nr.Header = make(http.Header, len(h))
		}
		nr.Header = nr.Header.Clone()
		for k, v := range h {
			k = http.CanonicalHeaderKey(k)
			if v == nil {
				if _, ok := nr.Header[k]; !ok {
					nr.Header[k] = nil
				}
				continue
			}
			nr.Header[k] = append(nr.Header[k], v...)
		}
		return &nr, nil
	}
}

// HeaderSet returns a new [Responder] based on r that ensures the
// returned [*http.Response] includes h header. It sets the header
// entries associated with each h key. It replaces any existing values
// associated each h key. Each key is case insensitive; it is
// canonicalized by [http.CanonicalHeaderKey].
//
// See also [Responder.HeaderAdd] and [Responder.SetContentLength].
func (r Responder) HeaderSet(h http.Header) Responder {
	return func(req *http.Request) (*http.Response, error) {
		resp, err := r(req)
		if err != nil {
			return nil, err
		}
		nr := *resp
		if nr.Header == nil {
			nr.Header = make(http.Header, len(h))
		}
		nr.Header = nr.Header.Clone()
		for k, v := range h {
			k = http.CanonicalHeaderKey(k)
			if v == nil {
				nr.Header[k] = nil
				continue
			}
			nr.Header[k] = append([]string(nil), v...)
		}
		return &nr, nil
	}
}

// ResponderFromResponse wraps an [*http.Response] in a [Responder].
//
// Be careful, except for responses generated by httpmock
// ([NewStringResponse] and [NewBytesResponse] functions) for which
// there is no problems, it is the caller responsibility to ensure the
// response body can be read several times and concurrently if needed,
// as it is shared among all [Responder] returned responses.
//
// For home-made responses, [NewRespBodyFromString] and
// [NewRespBodyFromBytes] functions can be used to produce response
// bodies that can be read several times and concurrently.
func ResponderFromResponse(resp *http.Response) Responder {
	return func(req *http.Request) (*http.Response, error) {
		res := *resp

		// Our stuff: generate a new io.ReadCloser instance sharing the same buffer
		if body, ok := resp.Body.(*dummyReadCloser); ok {
			res.Body = body.copy()
		}

		res.Request = req
		return &res, nil
	}
}

// ResponderFromMultipleResponses wraps an [*http.Response] list in a
// [Responder].
//
// Each response will be returned in the order of the provided list.
// If the [Responder] is called more than the size of the provided
// list, an error will be thrown.
//
// Be careful, except for responses generated by httpmock
// ([NewStringResponse] and [NewBytesResponse] functions) for which
// there is no problems, it is the caller responsibility to ensure the
// response body can be read several times and concurrently if needed,
// as it is shared among all [Responder] returned responses.
//
// For home-made responses, [NewRespBodyFromString] and
// [NewRespBodyFromBytes] functions can be used to produce response
// bodies that can be read several times and concurrently.
//
// If all responses have been returned and fn is passed and non-nil,
// it acts as the fn parameter of [NewNotFoundResponder], allowing to
// dump the stack trace to localize the origin of the call.
//
//	import (
//	  "github.com/jarcoal/httpmock"
//	  "testing"
//	)
//	...
//	func TestMyApp(t *testing.T) {
//	  ...
//	  // This responder is callable only once, then an error is returned and
//	  // the stacktrace of the call logged using t.Log()
//	  httpmock.RegisterResponder("GET", "/foo/bar",
//	    httpmock.ResponderFromMultipleResponses(
//	      []*http.Response{
//	        httpmock.NewStringResponse(200, `{"name":"bar"}`),
//	        httpmock.NewStringResponse(404, `{"mesg":"Not found"}`),
//	      },
//	      t.Log),
//	  )
//	}
//
// See also [Responder.Then].
func ResponderFromMultipleResponses(responses []*http.Response, fn ...func(...any)) Responder {
	responseIndex := 0
	mutex := sync.Mutex{}
	return func(req *http.Request) (*http.Response, error) {
		mutex.Lock()
		defer mutex.Unlock()
		defer func() { responseIndex++ }()
		if responseIndex >= len(responses) {
			err := internal.StackTracer{
				Err: fmt.Errorf("not enough responses provided: responder called %d time(s) but %d response(s) provided", responseIndex+1, len(responses)),
			}
			if len(fn) > 0 {
				err.CustomFn = fn[0]
			}
			return nil, err
		}
		res := *responses[responseIndex]
		// Our stuff: generate a new io.ReadCloser instance sharing the same buffer
		if body, ok := responses[responseIndex].Body.(*dummyReadCloser); ok {
			res.Body = body.copy()
		}

		res.Request = req
		return &res, nil
	}
}

// NewErrorResponder creates a [Responder] that returns an empty request and the
// given error. This can be used to e.g. imitate more deep http errors for the
// client.
func NewErrorResponder(err error) Responder {
	return func(req *http.Request) (*http.Response, error) {
		return nil, err
	}
}

// NewNotFoundResponder creates a [Responder] typically used in
// conjunction with [RegisterNoResponder] function and [testing]
// package, to be proactive when a [Responder] is not found. fn is
// called with a unique string parameter containing the name of the
// missing route and the stack trace to localize the origin of the
// call. If fn returns (= if it does not panic), the [Responder] returns
// an error of the form: "Responder not found for GET http://foo.bar/path".
// Note that fn can be nil.
//
// It is useful when writing tests to ensure that all routes have been
// mocked.
//
// Example of use:
//
//	import (
//	  "testing"
//	  "github.com/jarcoal/httpmock"
//	)
//	...
//	func TestMyApp(t *testing.T) {
//	   ...
//	   // Calls testing.Fatal with the name of Responder-less route and
//	   // the stack trace of the call.
//	   httpmock.RegisterNoResponder(httpmock.NewNotFoundResponder(t.Fatal))
//
// Will abort the current test and print something like:
//
//	transport_test.go:735: Called from net/http.Get()
//	      at /go/src/github.com/jarcoal/httpmock/transport_test.go:714
//	    github.com/jarcoal/httpmock.TestCheckStackTracer()
//	      at /go/src/testing/testing.go:865
//	    testing.tRunner()
//	      at /go/src/runtime/asm_amd64.s:1337
func NewNotFoundResponder(fn func(...any)) Responder {
	return func(req *http.Request) (*http.Response, error) {
		var extra string
		suggested, _ := req.Context().Value(suggestedKey).(*suggestedInfo)
		if suggested != nil {
			if suggested.kind == "matcher" {
				extra = fmt.Sprintf(` despite %s`, suggested.suggested)
			} else {
				extra = fmt.Sprintf(`, but one matches %s %q`, suggested.kind, suggested.suggested)
			}
		}
		return nil, internal.StackTracer{
			CustomFn: fn,
			Err:      fmt.Errorf("Responder not found for %s %s%s", req.Method, req.URL, extra),
		}
	}
}

// NewStringResponse creates an [*http.Response] with a body based on
// the given string.  Also accepts an HTTP status code.
//
// To pass the content of an existing file as body use [File] as in:
//
//	httpmock.NewStringResponse(200, httpmock.File("body.txt").String())
func NewStringResponse(status int, body string) *http.Response {
	return &http.Response{
		Status:        strconv.Itoa(status),
		StatusCode:    status,
		Body:          NewRespBodyFromString(body),
		Header:        http.Header{},
		ContentLength: -1,
	}
}

// NewStringResponder creates a [Responder] from a given body (as a
// string) and status code.
//
// To pass the content of an existing file as body use [File] as in:
//
//	httpmock.NewStringResponder(200, httpmock.File("body.txt").String())
func NewStringResponder(status int, body string) Responder {
	return ResponderFromResponse(NewStringResponse(status, body))
}

// NewBytesResponse creates an [*http.Response] with a body based on the
// given bytes.  Also accepts an HTTP status code.
//
// To pass the content of an existing file as body use [File] as in:
//
//	httpmock.NewBytesResponse(200, httpmock.File("body.raw").Bytes())
func NewBytesResponse(status int, body []byte) *http.Response {
	return &http.Response{
		Status:        strconv.Itoa(status),
		StatusCode:    status,
		Body:          NewRespBodyFromBytes(body),
		Header:        http.Header{},
		ContentLength: -1,
	}
}

// NewBytesResponder creates a [Responder] from a given body (as a byte
// slice) and status code.
//
// To pass the content of an existing file as body use [File] as in:
//
//	httpmock.NewBytesResponder(200, httpmock.File("body.raw").Bytes())
func NewBytesResponder(status int, body []byte) Responder {
	return ResponderFromResponse(NewBytesResponse(status, body))
}

// NewJsonResponse creates an [*http.Response] with a body that is a
// JSON encoded representation of the given any.  Also accepts
// an HTTP status code.
//
// To pass the content of an existing file as body use [File] as in:
//
//	httpmock.NewJsonResponse(200, httpmock.File("body.json"))
func NewJsonResponse(status int, body any) (*http.Response, error) { // nolint: revive
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	response := NewBytesResponse(status, encoded)
	response.Header.Set("Content-Type", "application/json")
	return response, nil
}

// NewJsonResponder creates a [Responder] from a given body (as an
// any that is encoded to JSON) and status code.
//
// To pass the content of an existing file as body use [File] as in:
//
//	httpmock.NewJsonResponder(200, httpmock.File("body.json"))
func NewJsonResponder(status int, body any) (Responder, error) { // nolint: revive
	resp, err := NewJsonResponse(status, body)
	if err != nil {
		return nil, err
	}
	return ResponderFromResponse(resp), nil
}

// NewJsonResponderOrPanic is like [NewJsonResponder] but panics in
// case of error.
//
// It simplifies the call of [RegisterResponder], avoiding the use of a
// temporary variable and an error check, and so can be used as
// [NewStringResponder] or [NewBytesResponder] in such context:
//
//	httpmock.RegisterResponder(
//	  "GET",
//	  "/test/path",
//	  httpmock.NewJsonResponderOrPanic(200, &MyBody),
//	)
//
// To pass the content of an existing file as body use [File] as in:
//
//	httpmock.NewJsonResponderOrPanic(200, httpmock.File("body.json"))
func NewJsonResponderOrPanic(status int, body any) Responder { // nolint: revive
	responder, err := NewJsonResponder(status, body)
	if err != nil {
		panic(err)
	}
	return responder
}

// NewXmlResponse creates an [*http.Response] with a body that is an
// XML encoded representation of the given any.  Also accepts an HTTP
// status code.
//
// To pass the content of an existing file as body use [File] as in:
//
//	httpmock.NewXmlResponse(200, httpmock.File("body.xml"))
func NewXmlResponse(status int, body any) (*http.Response, error) { // nolint: revive
	var (
		encoded []byte
		err     error
	)
	if f, ok := body.(File); ok {
		encoded, err = f.bytes()
	} else {
		encoded, err = xml.Marshal(body)
	}
	if err != nil {
		return nil, err
	}
	response := NewBytesResponse(status, encoded)
	response.Header.Set("Content-Type", "application/xml")
	return response, nil
}

// NewXmlResponder creates a [Responder] from a given body (as an
// any that is encoded to XML) and status code.
//
// To pass the content of an existing file as body use [File] as in:
//
//	httpmock.NewXmlResponder(200, httpmock.File("body.xml"))
func NewXmlResponder(status int, body any) (Responder, error) { // nolint: revive
	resp, err := NewXmlResponse(status, body)
	if err != nil {
		return nil, err
	}
	return ResponderFromResponse(resp), nil
}

// NewXmlResponderOrPanic is like [NewXmlResponder] but panics in case
// of error.
//
// It simplifies the call of [RegisterResponder], avoiding the use of a
// temporary variable and an error check, and so can be used as
// [NewStringResponder] or [NewBytesResponder] in such context:
//
//	httpmock.RegisterResponder(
//	  "GET",
//	  "/test/path",
//	  httpmock.NewXmlResponderOrPanic(200, &MyBody),
//	)
//
// To pass the content of an existing file as body use [File] as in:
//
//	httpmock.NewXmlResponderOrPanic(200, httpmock.File("body.xml"))
func NewXmlResponderOrPanic(status int, body any) Responder { // nolint: revive
	responder, err := NewXmlResponder(status, body)
	if err != nil {
		panic(err)
	}
	return responder
}

// NewRespBodyFromString creates an [io.ReadCloser] from a string that
// is suitable for use as an HTTP response body.
//
// To pass the content of an existing file as body use [File] as in:
//
//	httpmock.NewRespBodyFromString(httpmock.File("body.txt").String())
func NewRespBodyFromString(body string) io.ReadCloser {
	return &dummyReadCloser{orig: body}
}

// NewRespBodyFromBytes creates an [io.ReadCloser] from a byte slice
// that is suitable for use as an HTTP response body.
//
// To pass the content of an existing file as body use [File] as in:
//
//	httpmock.NewRespBodyFromBytes(httpmock.File("body.txt").Bytes())
func NewRespBodyFromBytes(body []byte) io.ReadCloser {
	return &dummyReadCloser{orig: body}
}

type lenReadSeeker interface {
	io.ReadSeeker
	Len() int
}

type dummyReadCloser struct {
	orig any           // string or []byte
	body lenReadSeeker // instanciated on demand from orig
}

// copy returns a new instance resetting d.body to nil.
func (d *dummyReadCloser) copy() *dummyReadCloser {
	return &dummyReadCloser{orig: d.orig}
}

// setup ensures d.body is correctly initialized.
func (d *dummyReadCloser) setup() {
	if d.body == nil {
		switch body := d.orig.(type) {
		case string:
			d.body = strings.NewReader(body)
		case []byte:
			d.body = bytes.NewReader(body)
		case io.ReadCloser:
			var buf bytes.Buffer
			io.Copy(&buf, body) //nolint: errcheck
			body.Close()
			d.body = bytes.NewReader(buf.Bytes())
		}
	}
}

func (d *dummyReadCloser) Read(p []byte) (n int, err error) {
	d.setup()
	return d.body.Read(p)
}

func (d *dummyReadCloser) Close() error {
	d.setup()
	d.body.Seek(0, io.SeekEnd) // nolint: errcheck
	return nil
}

func (d *dummyReadCloser) Len() int {
	d.setup()
	return d.body.Len()
}
