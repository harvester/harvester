package http

import (
	"net/http"

	"github.com/rancher/apiserver/pkg/apierror"

	"github.com/harvester/harvester/pkg/util"
)

type HarvesterServerHandler interface {
	Do(_ http.ResponseWriter, r *http.Request) (interface{}, error)
}

type harvesterServerHandler struct {
	httpHandler HarvesterServerHandler
}

func NewHandler(httpHandler HarvesterServerHandler) http.Handler {
	return &harvesterServerHandler{
		httpHandler: httpHandler,
	}
}

func (handler *harvesterServerHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	resp, err := handler.httpHandler.Do(rw, req)
	if err != nil {
		status := http.StatusInternalServerError
		if e, ok := err.(*apierror.APIError); ok {
			status = e.Code.Status
		}
		rw.WriteHeader(status)
		_, _ = rw.Write([]byte(err.Error()))
		return
	}

	if resp == nil {
		rw.WriteHeader(http.StatusNoContent)
		return
	}

	util.ResponseOKWithBody(rw, resp)
}
