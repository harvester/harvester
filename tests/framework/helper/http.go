package helper

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/guonaihong/gout"
	"github.com/guonaihong/gout/dataflow"
	"github.com/rancher/apiserver/pkg/types"
	"sigs.k8s.io/yaml"
)

func NewHTTPClient() *dataflow.Gout {
	return gout.
		New(&http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		})
}

func BuildAPIURL(version, resource string, httpsPort int) string {
	return fmt.Sprintf("https://localhost:%d/%s/%s", httpsPort, version, resource)
}

func BuildResourceURL(api, namespace, name string) string {
	return fmt.Sprintf("%s/%s/%s", api, namespace, name)
}

func GetResponse(url string) (respCode int, respBody []byte, err error) {
	err = NewHTTPClient().
		GET(url).
		BindBody(&respBody).
		Code(&respCode).
		Do()
	if err != nil {
		return
	}
	if respCode != http.StatusOK {
		return
	}
	return
}

func GetCollection(url string) (collection *types.GenericCollection, respCode int, respBody []byte, err error) {
	err = NewHTTPClient().
		GET(url).
		BindBody(&respBody).
		Code(&respCode).
		Do()
	if err != nil {
		return
	}
	if respCode != http.StatusOK {
		return
	}
	if err = json.Unmarshal(respBody, &collection); err != nil {
		return
	}
	return
}

func GetObject(url string, object interface{}) (respCode int, respBody []byte, err error) {
	err = NewHTTPClient().
		GET(url).
		BindBody(&respBody).
		Code(&respCode).
		Do()
	if err != nil {
		return
	}
	if respCode != http.StatusOK {
		return
	}
	if object != nil {
		err = json.Unmarshal(respBody, object)
	}
	return
}

func PutObject(url string, object interface{}) (respCode int, respBody []byte, err error) {
	err = NewHTTPClient().
		PUT(url).
		SetJSON(object).
		BindBody(&respBody).
		Code(&respCode).
		Do()
	return
}

func DeleteObject(url string) (respCode int, respBody []byte, err error) {
	err = NewHTTPClient().
		DELETE(url).
		BindBody(&respBody).
		Code(&respCode).
		Do()
	return
}

func PostObject(url string, object interface{}) (respCode int, respBody []byte, err error) {
	err = NewHTTPClient().
		POST(url).
		SetJSON(object).
		SetHeader().
		BindBody(&respBody).
		Code(&respCode).
		Do()
	return
}

func PostObjectByYAML(url string, object interface{}) (respCode int, respBody []byte, err error) {
	var yamlData []byte
	yamlData, err = yaml.Marshal(object)
	if err != nil {
		return
	}
	err = NewHTTPClient().
		POST(url).
		SetBody(yamlData).
		SetHeader(gout.H{"content-type": "application/yaml"}).
		BindBody(&respBody).
		Code(&respCode).
		Do()
	return
}

func PostAction(url string, action string) (respCode int, respBody []byte, err error) {
	err = NewHTTPClient().
		POST(fmt.Sprintf("%s?action=%s", url, action)).
		SetHeader().
		BindBody(&respBody).
		Code(&respCode).
		Do()
	return
}

func PostObjectAction(url string, object interface{}, action string) (respCode int, respBody []byte, err error) {
	err = NewHTTPClient().
		POST(fmt.Sprintf("%s?action=%s", url, action)).
		SetJSON(object).
		SetHeader().
		BindBody(&respBody).
		Code(&respCode).
		Do()
	return
}
