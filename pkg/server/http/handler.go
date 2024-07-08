package http

import (
	"errors"
	"net/http"

	"github.com/rancher/apiserver/pkg/apierror"

	"github.com/harvester/harvester/pkg/util"
)

type HarvesterServerHandler interface {
	Do(ctx *Ctx) (interface{}, error)
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
	ctx := newDefaultHarvesterServerCtx(rw, req)
	resp, err := handler.httpHandler.Do(ctx)
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
		rw.WriteHeader(ctx.StatusCode())
		return
	}

	util.ResponseOKWithBody(rw, resp)
}
