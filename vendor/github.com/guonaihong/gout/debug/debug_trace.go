package debug

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"reflect"
	"strings"
	"time"

	"github.com/guonaihong/gout/color"
	"github.com/guonaihong/gout/middler"
)

type TraceInfo struct {
	DnsDuration          time.Duration
	ConnDuration         time.Duration
	TLSDuration          time.Duration
	RequestDuration      time.Duration
	WaitResponseDuration time.Duration
	ResponseDuration     time.Duration
	TotalDuration        time.Duration
	w                    io.Writer
}

func (t *TraceInfo) StartTrace(opt *Options, needTrace bool, req *http.Request, do middler.Do) (*http.Response, error) {
	w := opt.Write
	var dnsStart, connStart, reqStart, tlsStart, waitResponseStart, respStart time.Time
	var dnsDuration, connDuration, reqDuration, tlsDuration, waitResponseDuration time.Duration
	var startNow time.Time

	if needTrace {
		startNow = time.Now()
		trace := &httptrace.ClientTrace{
			DNSStart: func(_ httptrace.DNSStartInfo) {
				dnsStart = time.Now()
			},

			DNSDone: func(dnsInfo httptrace.DNSDoneInfo) {
				dnsDuration = time.Since(dnsStart)
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
				tlsDuration = time.Since(tlsStart)
			},

			WroteRequest: func(_ httptrace.WroteRequestInfo) {
				now := time.Now()
				reqDuration = now.Sub(reqStart)
				waitResponseStart = now
			},

			GotFirstResponseByte: func() {
				waitResponseDuration = time.Since(waitResponseStart)
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
		t.WaitResponseDuration = waitResponseDuration
		t.ResponseDuration = time.Since(respStart)
		t.TotalDuration = time.Since(startNow)
		t.w = w
		// 格式化成json
		if opt.FormatTraceJSON {
			all, err := json.Marshal(t)
			if err != nil {
				return resp, err
			}
			if _, err := w.Write(all); err != nil {
				return resp, err
			}
		} else {
			t.output(opt)
		}
	}
	return resp, err
}

func (t *TraceInfo) output(opt *Options) {
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
