package util

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

func ResponseBody(obj interface{}) []byte {
	respBody, err := json.Marshal(obj)
	if err != nil {
		return []byte(`{\"errors\":[\"Failed to parse response body\"]}`)
	}
	return respBody
}

func ResponseOKWithBody(rw http.ResponseWriter, obj interface{}) {
	rw.Header().Set("Content-type", "application/json")
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write(ResponseBody(obj))
}

func ResponseOK(rw http.ResponseWriter) {
	rw.WriteHeader(http.StatusOK)
}

func ResponseError(rw http.ResponseWriter, statusCode int, err error) {
	ResponseErrorMsg(rw, statusCode, err.Error())
}

func ResponseErrorMsg(rw http.ResponseWriter, statusCode int, errMsg string) {
	rw.WriteHeader(statusCode)
	_, _ = rw.Write(ResponseBody(v1beta1.ErrorResponse{Errors: []string{errMsg}}))
}

func removeNewLineInString(v string) string {
	escaped := strings.ReplaceAll(v, "\n", "")
	escaped = strings.ReplaceAll(escaped, "\r", "")
	return escaped
}

func EncodeVars(vars map[string]string) map[string]string {
	escapedVars := make(map[string]string)
	for k, v := range vars {
		escapedVars[k] = removeNewLineInString(v)
	}
	return escapedVars
}

// BuildHTTPClientWithCA returns an *http.Client whose transport is cloned from
// http.DefaultTransport (preserving proxy settings, timeouts, HTTP/2, etc.) with
// the TLS root CA pool extended by the additional PEM-encoded CA certificate(s)
// provided in additionalCA. If additionalCA is empty, http.DefaultClient is returned
// unchanged.
func BuildHTTPClientWithCA(additionalCA string) *http.Client {
	if additionalCA == "" {
		return http.DefaultClient
	}

	rootCAs, err := x509.SystemCertPool()
	if err != nil || rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}
	if ok := rootCAs.AppendCertsFromPEM([]byte(additionalCA)); !ok {
		logrus.Warn("BuildHTTPClientWithCA: failed to append additional CA certificates; falling back to system pool")
	}

	// Clone the default transport so that proxy, keep-alive, HTTP/2 and all
	// other settings are preserved; only the TLS root CAs are replaced.
	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		// Defensive fallback: should never happen in practice.
		return &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{RootCAs: rootCAs},
			},
		}
	}

	transport := defaultTransport.Clone()
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	}
	transport.TLSClientConfig.RootCAs = rootCAs

	return &http.Client{Transport: transport}
}
