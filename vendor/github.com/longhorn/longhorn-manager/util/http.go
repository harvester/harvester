package util

import (
	"bytes"
	"io"
	"net/http"
	"strings"
)

func CopyReq(req *http.Request) *http.Request {
	r := *req
	buf, _ := io.ReadAll(r.Body)
	req.Body = io.NopCloser(bytes.NewBuffer(buf))
	r.Body = io.NopCloser(bytes.NewBuffer(buf))
	return &r
}

func GetSchemeFromURL(url string) string {
	schemeEndIndex := strings.Index(url, "://")
	if schemeEndIndex == -1 {
		return ""
	}

	return url[:schemeEndIndex]
}
