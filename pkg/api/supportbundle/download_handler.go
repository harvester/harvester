package supportbundle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/rancher/apiserver/pkg/apierror"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/v3/pkg/schemas/validation"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlsb "github.com/harvester/harvester/pkg/controller/master/supportbundle"
	"github.com/harvester/harvester/pkg/controller/master/supportbundle/types"
	"github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	harvesterServer "github.com/harvester/harvester/pkg/server/http"
	"github.com/harvester/harvester/pkg/util"
)

type DownloadHandler struct {
	context            context.Context
	namespace          string
	supportBundles     v1beta1.SupportBundleClient
	supportBundleCache v1beta1.SupportBundleCache
	podCache           ctlcorev1.PodCache

	httpClient *http.Client
}

func NewDownloadHandler(scaled *config.Scaled, namespace string) *DownloadHandler {
	return &DownloadHandler{
		context:            scaled.Ctx,
		namespace:          namespace,
		supportBundles:     scaled.HarvesterFactory.Harvesterhci().V1beta1().SupportBundle(),
		supportBundleCache: scaled.HarvesterFactory.Harvesterhci().V1beta1().SupportBundle().Cache(),
		podCache:           scaled.CoreFactory.Core().V1().Pod().Cache(),
		httpClient: &http.Client{
			Timeout: 24 * time.Hour,
		},
	}
}

func (h *DownloadHandler) Do(ctx *harvesterServer.Ctx) (harvesterServer.ResponseBody, error) {
	r, rw := ctx.Req(), ctx.RespWriter()

	bundleName := util.EncodeVars(mux.Vars(r))["bundleName"]

	retainSb := false
	if retain, err := strconv.ParseBool(r.URL.Query().Get("retain")); err == nil {
		retainSb = retain
	}

	sb, err := h.supportBundleCache.Get(h.namespace, bundleName)
	if err != nil {
		return nil, apierror.NewAPIError(validation.InvalidBodyContent, errors.Wrap(err, "fail to get support bundle resource").Error())
	}

	if sb.Status.State != types.StateReady || sb.Status.Filename == "" || sb.Status.Filesize == 0 {
		return nil, apierror.NewAPIError(validation.InvalidBodyContent, "support bundle is not ready")
	}

	managerPodIP, err := ctlsb.GetManagerPodIP(h.podCache, sb)
	if err != nil {
		return nil, apierror.NewAPIError(validation.InvalidBodyContent, errors.Wrap(err, "fail to get support bundle manager pod IP").Error())
	}

	url := fmt.Sprintf("http://%s:8080/bundle", managerPodIP)
	req, err := http.NewRequestWithContext(h.context, http.MethodGet, url, nil)
	if err != nil {
		return nil, apierror.NewAPIError(validation.ServerError, errors.Wrap(err, "fail to create request to manager").Error())
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, apierror.NewAPIError(validation.ServerError, errors.Wrap(err, "fail to send request to manager").Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("Unexpected status code %d from manager.", resp.StatusCode)
		body, err := ioutil.ReadAll(resp.Body)
		if err == nil {
			var errResp harvesterv1.ErrorResponse
			if e := json.Unmarshal(body, &errResp); e == nil {
				msg = fmt.Sprintf("%s %v", msg, errResp.Errors)
			}
		}
		return nil, apierror.NewAPIError(validation.ServerError, msg)
	}

	rw.Header().Set("Content-Length", fmt.Sprint(sb.Status.Filesize))
	rw.Header().Set("Content-Disposition", "attachment; filename="+sb.Status.Filename)
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" {
		rw.Header().Set("Content-Type", contentType)
	}

	if _, err := io.Copy(rw, resp.Body); err != nil {
		return nil, apierror.NewAPIError(validation.ServerError, errors.Wrap(err, "fail to copy response body to response writer").Error())
	}

	if retainSb {
		return nil, nil
	}

	logrus.Infof("delete support bundle %s", sb.Name)
	err = h.supportBundles.Delete(sb.Namespace, sb.Name, &metav1.DeleteOptions{})
	if err != nil {
		logrus.Errorf("fail to delete support bundle %s: %s", sb.Name, err)
	}
	return nil, nil
}
