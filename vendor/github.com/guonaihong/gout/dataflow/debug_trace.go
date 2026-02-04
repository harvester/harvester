package dataflow

import (
	"crypto/tls"
	"fmt"
	"github.com/guonaihong/gout/color"
	"io"
	"net/http"
	"net/http/httptrace"
	"os"
	"reflect"
	"strings"
	"time"
)

func Trace() DebugOpt {
	return DebugFunc(func(o *DebugOption) {
		o.Color = true
		o.Trace = true
		o.Write = os.Stdout
	})
}

type TraceInfo struct {
	DnsDuration         time.Duration
	ConnDuration        time.Duration
	TLSDuration         time.Duration
	RequestDuration     time.Duration
	WaitResponeDuration time.Duration
	ResponseDuration    time.Duration
	TotalDuration       time.Duration
	w                   io.Writer
}

func (t *TraceInfo) startTrace(opt *DebugOption, needTrace bool, req *http.Request, do Do) (*http.Response, error) {
	w := opt.Write
	var dnsStart, connStart, reqStart, tlsStart, waitResponseStart, respStart time.Time
	var dnsDuration, connDuration, reqDuration, tlsDuration, waitResponeDuration time.Duration
	var startNow time.Time

	if needTrace {
		startNow = time.Now()
		trace := &httptrace.ClientTrace{

			DNSStart: func(_ httptrace.DNSStartInfo) {
				dnsStart = time.Now()
			},

			DNSDone: func(dnsInfo httptrace.DNSDoneInfo) {
				dnsDuration = time.Now().Sub(dnsStart)
			},

			GetConn: func(hostPort string) {
				connStart = time.Now()
			},

			GotConn: func(connInfo httptrace.GotConnInfo) {
				now := time.Now()
				connDuration = now.Sub(connStart)
				reqStart = now
			},

			TLSHandshakeStart: func() {
				tlsStart = time.Now()
			},

			TLSHandshakeDone: func(tls.ConnectionState, error) {
				tlsDuration = time.Now().Sub(tlsStart)
			},

			WroteRequest: func(_ httptrace.WroteRequestInfo) {
				now := time.Now()
				reqDuration = now.Sub(reqStart)
				waitResponseStart = now
			},

			GotFirstResponseByte: func() {
				waitResponeDuration = time.Now().Sub(waitResponseStart)
				respStart = time.Now()
			},
		}

		req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	}

	resp, err := do.Do(req)

	if needTrace {
		t.DnsDuration = dnsDuration
		t.ConnDuration = connDuration
		t.TLSDuration = tlsDuration
		t.RequestDuration = reqDuration
		t.WaitResponeDuration = waitResponeDuration
		t.ResponseDuration = time.Now().Sub(respStart)
		t.TotalDuration = time.Now().Sub(startNow)
		t.w = w
		t.output(opt)
	}
	return resp, err
}

func (t *TraceInfo) output(opt *DebugOption) {
	v := reflect.ValueOf(t)
	v = v.Elem()

	maxLen := 0
	typ := v.Type()
	for i := 0; i < v.NumField(); i++ {
		sf := typ.Field(i)
		if sf.PkgPath != "" {
			continue
		}

		if maxLen < len(sf.Name) {
			maxLen = len(sf.Name)
		}
	}

	cl := color.New(opt.Color)
	title := strings.Repeat("=", maxLen)
	space4 := "    "
	fmt.Fprintf(t.w, "%s %s: %s\n", title, cl.Sbluef("Trace Info(S)"), title)
	for i := 0; i < v.NumField(); i++ {
		sf := typ.Field(i)
		if sf.PkgPath != "" {
			continue
		}

		name := sf.Name
		d := v.Field(i).Interface().(time.Duration).String()
		fmt.Fprintf(t.w, "%s %s%s : %s\n", space4, cl.Spurplef(name), strings.Repeat(" ", maxLen+2-len(name)), cl.Sbluef(d))
	}

	fmt.Fprintf(t.w, "%s %s: %s\n", title, cl.Sbluef("Trace Info(E)"), title)
	fmt.Fprintf(t.w, "\n")
}
