package util

import (
	"bytes"
	"io/ioutil"
	"net/http"
)

func CopyReq(req *http.Request) *http.Request {
	r := *req
	buf, _ := ioutil.ReadAll(r.Body)
	req.Body = ioutil.NopCloser(bytes.NewBuffer(buf))
	r.Body = ioutil.NopCloser(bytes.NewBuffer(buf))
	return &r
}
