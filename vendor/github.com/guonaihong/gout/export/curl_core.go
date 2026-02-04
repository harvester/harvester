package export

import (
	"fmt"
	"github.com/guonaihong/gout/core"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
)

type curl struct {
	Header []string
	Method string
	Data   string
	URL    string

	FormData []string
}

const boundary = "boundary="

func isExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	return false
}

func getFileName(fName string) string {
	count := 0
	fileName := fName
	for ; ; count++ {
		if isExists(fileName) {
			fileName = fmt.Sprintf("%s.%d", fName, count)
			continue
		}

		break
	}
	return fileName
}

func (c *curl) formData(req *http.Request) error {
	contentType := req.Header.Get("Content-Type")
	if strings.Index(contentType, "multipart/form-data") == -1 {
		return nil
	}
	req.Header.Del("Content-Type")

	c.Data = ""

	boundaryValue := ""

	if pos := strings.Index(contentType, "boundary="); pos == -1 {
		return nil
	} else {
		boundaryValue = strings.TrimSpace(contentType[pos+len(boundary):])
	}

	body, err := req.GetBody()
	if err != nil {
		return err
	}

	mr := multipart.NewReader(body, boundaryValue)
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		var buf strings.Builder

		buf.WriteString(p.FormName())
		buf.WriteByte('=')

		if p.FileName() != "" {

			fileName := getFileName(p.FileName())
			fileName = path.Base(fileName)
			fd, err := os.Create(fileName)
			if err != nil {
				return err
			}
			io.Copy(fd, p)

			buf.WriteString("@./")
			buf.WriteString(fileName)

			fd.Close()
		} else {
			io.Copy(&buf, p)
		}

		c.FormData = append(c.FormData, fmt.Sprintf("%q", buf.String()))
	}

	return nil
}

func (c *curl) header(req *http.Request) {
	header := make([]string, 0, len(req.Header))

	for k := range req.Header {
		header = append(header, k)
	}

	sort.Strings(header)

	for index, hKey := range header {
		hVal := req.Header[hKey]

		header[index] = fmt.Sprintf(`%s:%s`, hKey, strings.Join(hVal, ","))
		header[index] = fmt.Sprintf("%q", header[index])
	}

	c.Header = header
}

// GenCurl used to generate curl commands
func GenCurl(req *http.Request, long bool, w io.Writer) error {
	c := curl{}
	body, err := req.GetBody()
	if err != nil {
		return err
	}

	all, err := ioutil.ReadAll(body)
	if err != nil {
		return err
	}

	c.URL = fmt.Sprintf(`%q`, req.URL.String())
	c.Method = req.Method
	if len(all) > 0 {
		c.Data = fmt.Sprintf(`%q`, core.BytesToString(all))
	}
	c.formData(req)
	c.header(req)
	tp := newTemplate(long)
	return tp.Execute(w, c)
}
