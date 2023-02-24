package util

import (
	"encoding/json"
	"net/http"
	"strings"

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
	escaped := strings.Replace(v, "\n", "", -1)
	escaped = strings.Replace(escaped, "\r", "", -1)
	return escaped
}

func EncodeVars(vars map[string]string) map[string]string {
	escapedVars := make(map[string]string)
	for k, v := range vars {
		escapedVars[k] = removeNewLineInString(v)
	}
	return escapedVars
}
