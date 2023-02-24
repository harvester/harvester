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
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlsb "github.com/harvester/harvester/pkg/controller/master/supportbundle"
	"github.com/harvester/harvester/pkg/controller/master/supportbundle/types"
	"github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
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
			Timeout: 30 * time.Second,
		},
	}
}

func (h *DownloadHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	bundleName := util.EncodeVars(mux.Vars(r))["bundleName"]

	retainSb := false
	if retain, err := strconv.ParseBool(r.URL.Query().Get("retain")); err == nil {
		retainSb = retain
	}

	sb, err := h.supportBundleCache.Get(h.namespace, bundleName)
	if err != nil {
		util.ResponseError(rw, http.StatusBadRequest, errors.Wrap(err, "fail to get support bundle resource"))
		return
	}

	if sb.Status.State != types.StateReady || sb.Status.Filename == "" || sb.Status.Filesize == 0 {
		util.ResponseError(rw, http.StatusBadRequest, errors.New("support bundle is not ready"))
		return
	}

	managerPodIP, err := ctlsb.GetManagerPodIP(h.podCache, sb)
	if err != nil {
		util.ResponseError(rw, http.StatusBadRequest, errors.Wrap(err, "fail to get support bundle manager pod IP"))
		return
	}

	url := fmt.Sprintf("http://%s:8080/bundle", managerPodIP)
	req, err := http.NewRequestWithContext(h.context, http.MethodGet, url, nil)
	if err != nil {
		util.ResponseError(rw, http.StatusInternalServerError, err)
		return
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		util.ResponseError(rw, http.StatusInternalServerError, err)
		return
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
		util.ResponseErrorMsg(rw, http.StatusInternalServerError, msg)
		return
	}

	rw.Header().Set("Content-Length", fmt.Sprint(sb.Status.Filesize))
	rw.Header().Set("Content-Disposition", "attachment; filename="+sb.Status.Filename)
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" {
		rw.Header().Set("Content-Type", contentType)
	}

	if _, err := io.Copy(rw, resp.Body); err != nil {
		util.ResponseError(rw, http.StatusInternalServerError, err)
		return
	}

	if retainSb {
		return
	}

	logrus.Infof("delete support bundle %s", sb.Name)
	err = h.supportBundles.Delete(sb.Namespace, sb.Name, &metav1.DeleteOptions{})
	if err != nil {
		logrus.Errorf("fail to delete support bundle %s: %s", sb.Name, err)
	}
}
