package writer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rancher/apiserver/pkg/types"
)

const (
	JSURL          = "https://releases.rancher.com/api-ui/%API_UI_VERSION%/ui.min.js"
	CSSURL         = "https://releases.rancher.com/api-ui/%API_UI_VERSION%/ui.min.css"
	DefaultVersion = "1.1.11"
)

var (
	start = `
<!DOCTYPE html>
<!-- If you are reading this, there is a good chance you would prefer sending an
"Accept: application/json" header and receiving actual JSON responses. -->
<link rel="stylesheet" type="text/css" href="%CSSURL%" />
<script src="%JSURL%"></script>
<script>
var user = "admin";
var curlUser='${CATTLE_ACCESS_KEY}:${CATTLE_SECRET_KEY}';
var schemas=%SCHEMAS%;
var data =
`
	end = []byte(`</script>
`)
)

type StringGetter func() string

type HTMLResponseWriter struct {
	EncodingResponseWriter
	CSSURL       StringGetter
	JSURL        StringGetter
	APIUIVersion StringGetter
}

func (h *HTMLResponseWriter) start(apiOp *types.APIRequest, code int) {
	AddCommonResponseHeader(apiOp)
	apiOp.Response.Header().Set("content-type", "text/html")
	apiOp.Response.WriteHeader(code)
}

func (h *HTMLResponseWriter) Write(apiOp *types.APIRequest, code int, obj types.APIObject) {
	h.write(apiOp, code, obj)
}

func (h *HTMLResponseWriter) WriteList(apiOp *types.APIRequest, code int, list types.APIObjectList) {
	h.write(apiOp, code, list)
}

func (h *HTMLResponseWriter) write(apiOp *types.APIRequest, code int, obj interface{}) {
	h.start(apiOp, code)
	schemaSchema := apiOp.Schemas.Schemas["schema"]
	headerString := start
	if schemaSchema != nil {
		headerString = strings.Replace(headerString, "%SCHEMAS%", jsonEncodeURL(apiOp.URLBuilder.Collection(schemaSchema)), 1)
	}
	var jsurl, cssurl string
	if h.CSSURL != nil && h.JSURL != nil && h.CSSURL() != "" && h.JSURL() != "" {
		jsurl = h.JSURL()
		cssurl = h.CSSURL()
	} else if h.APIUIVersion != nil && h.APIUIVersion() != "" {
		jsurl = strings.Replace(JSURL, "%API_UI_VERSION%", h.APIUIVersion(), 1)
		cssurl = strings.Replace(CSSURL, "%API_UI_VERSION%", h.APIUIVersion(), 1)
	} else {
		jsurl = strings.Replace(JSURL, "%API_UI_VERSION%", DefaultVersion, 1)
		cssurl = strings.Replace(CSSURL, "%API_UI_VERSION%", DefaultVersion, 1)
	}

	// jsurl and cssurl are added to the document as attributes not entities which requires special encoding.
	jsurl, _ = encodeAttribute(jsurl)
	cssurl, _ = encodeAttribute(cssurl)

	headerString = strings.Replace(headerString, "%JSURL%", jsurl, 1)
	headerString = strings.Replace(headerString, "%CSSURL%", cssurl, 1)

	apiOp.Response.Write([]byte(headerString))
	if apiObj, ok := obj.(types.APIObject); ok {
		h.Body(apiOp, apiOp.Response, apiObj)
	} else if list, ok := obj.(types.APIObjectList); ok {
		h.BodyList(apiOp, apiOp.Response, list)
	}
	if schemaSchema != nil {
		apiOp.Response.Write(end)
	}
}

func jsonEncodeURL(str string) string {
	data, _ := json.Marshal(str)
	return string(data)
}

// encodeAttribute encodes all characters with the HTML Entity &#xHH; format, including spaces, where HH represents the hexadecimal value of the character in Unicode.
// For example, A becomes &#x41;. All alphanumeric characters (letters A to Z, a to z, and digits 0 to 9) remain unencoded.
// more info: https://cheatsheetseries.owasp.org/cheatsheets/Cross_Site_Scripting_Prevention_Cheat_Sheet.html#output-encoding-rules-summary
func encodeAttribute(raw string) (string, error) {
	var builder strings.Builder
	for _, r := range raw {
		if ('A' <= r && r <= 'Z') || ('a' <= r && r <= 'z') || ('0' <= r && r <= '9') {
			_, err := builder.WriteRune(r)
			if err != nil {
				return "", fmt.Errorf("failed to write: %w", err)
			}
		} else {
			// encode non-alphanumeric rune to hex.
			_, err := fmt.Fprintf(&builder, "&#x%X;", r)
			if err != nil {
				return "", fmt.Errorf("failed to write: %w", err)
			}
		}
	}
	return builder.String(), nil
}
