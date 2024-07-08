package http

import (
	"errors"
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
		var e *apierror.APIError
		if errors.As(err, &e) {
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

	if resp == util.EmptyResponseBody {
		util.ResponseOK(rw)
		return
	}

	util.ResponseOKWithBody(rw, resp)
}
