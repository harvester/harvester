package gout

import (
	"bufio"
	"bytes"
	"github.com/guonaihong/gout/core"
	"github.com/guonaihong/gout/dataflow"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
)

type Import struct{}

func NewImport() *Import {
	return &Import{}
}

const rawTextSpace = "\r\n "

func (i *Import) RawText(text interface{}) *Text {
	var read io.Reader
	r := &Text{}

	out := dataflow.New()
	// TODO 函数
	r.SetGout(out)

	switch body := text.(type) {
	case string:
		body = strings.TrimLeft(body, rawTextSpace)
		read = strings.NewReader(body)
	case []byte:
		body = bytes.TrimLeft(body, rawTextSpace)
		read = bytes.NewReader(body)
	default:
		r.Err = core.ErrUnknownType
		return r
	}

	req, err := http.ReadRequest(bufio.NewReader(read))
	if err != nil {
		r.Err = err
		return r
	}

	// TODO 探索下能否支持https
	req.URL.Scheme = "http"
	req.URL.Host = req.Host
	req.RequestURI = ""

	if req.GetBody == nil {
		all, err := ioutil.ReadAll(req.Body)
		if err != nil {
			r.Err = err
			return r
		}

		req.GetBody = func() (io.ReadCloser, error) {
			return ioutil.NopCloser(bytes.NewReader(all)), nil
		}

		req.Body = ioutil.NopCloser(bytes.NewReader(all))
	}

	r.SetRequest(req)

	return r
}
