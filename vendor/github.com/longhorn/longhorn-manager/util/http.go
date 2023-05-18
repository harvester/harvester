package util

import (
	"bytes"
	"io"
	"net/http"
)

func CopyReq(req *http.Request) *http.Request {
	r := *req
	buf, _ := io.ReadAll(r.Body)
	req.Body = io.NopCloser(bytes.NewBuffer(buf))
	r.Body = io.NopCloser(bytes.NewBuffer(buf))
	return &r
}
