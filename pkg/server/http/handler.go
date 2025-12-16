package http

import (
	"errors"
	"net/http"

	"github.com/rancher/apiserver/pkg/apierror"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/harvester/harvester/pkg/util"
)

// ResponseBody only accepts object which can be marshalled to JSON.
// if you need to write some custom response, you can use ctx.RespWriter() to write directly to the response.
type ResponseBody interface{}

type HarvesterServerHandler interface {
	Do(ctx *Ctx) (ResponseBody, error)
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
		status := getHTTPStatusFromError(err)
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

func getHTTPStatusFromError(err error) int {
	var apiErr *apierror.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code.Status
	}

	var statusErr *apierrors.StatusError
	if errors.As(err, &statusErr) {
		// Handle Kubernetes StatusError types, which may be returned by Harvester webhooks and other K8s components
		return int(statusErr.ErrStatus.Code)
	}

	return http.StatusInternalServerError
}
