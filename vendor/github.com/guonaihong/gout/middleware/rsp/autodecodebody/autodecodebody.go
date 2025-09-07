package autodecodebody

import (
	"bytes"
	"compress/zlib"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/andybalholm/brotli"
)

func AutoDecodeBody(rsp *http.Response) (err error) {
	encoding := rsp.Header.Get("Content-Encoding")

	encoding = strings.ToLower(encoding)
	var out bytes.Buffer
	var rc io.ReadCloser
	switch encoding {
	//case "gzip": // net/http包里面已经做了gzip自动解码
	//rc, err = gzip.NewReader(rsp.Body)
	case "compress":
		// compress 是一种浏览器基本不使用的压缩格式，暂不考虑支持
		return errors.New("gout:There is currently no plan to support the compress format")
	case "deflate":
		rc, err = zlib.NewReader(rsp.Body)
	case "br":
		rc = ioutil.NopCloser(brotli.NewReader(rsp.Body))
	default:
		return nil
	}

	if err != nil {
		return err
	}

	if _, err = io.Copy(&out, rc); err != nil {
		return err
	}

	if err = rc.Close(); err != nil {
		return err
	}
	rsp.Body = ioutil.NopCloser(&out)
	return nil
}
