package supportbundle

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlsb "github.com/harvester/harvester/pkg/controller/master/supportbundle"
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
	bundleName := mux.Vars(r)["bundleName"]

	retainSb := false
	if retain, err := strconv.ParseBool(r.URL.Query().Get("retain")); err == nil {
		retainSb = retain
	}

	sb, err := h.supportBundleCache.Get(h.namespace, bundleName)
	if err != nil {
		util.ResponseError(rw, http.StatusBadRequest, errors.Wrap(err, "fail to get support bundle resource"))
		return
	}

	if sb.Status.State != ctlsb.StateReady || sb.Status.Filename == "" || sb.Status.Filesize == 0 {
		util.ResponseError(rw, http.StatusBadRequest, errors.New("support bundle is not ready"))
		return
	}

	managerPodIP, err := h.getManagerPodIP(sb)
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
		util.ResponseErrorMsg(rw, http.StatusInternalServerError, fmt.Sprintf("unexpected status code %d", resp.StatusCode))
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

func (h *DownloadHandler) getManagerPodIP(sb *harvesterv1.SupportBundle) (string, error) {
	sets := labels.Set{
		"app":                       ctlsb.AppManager,
		ctlsb.SupportBundleLabelKey: sb.Name,
	}

	pods, err := h.podCache.List(sb.Namespace, sets.AsSelector())
	if err != nil {
		return "", err

	}
	if len(pods) != 1 {
		return "", errors.New("more than one manager pods are found")
	}
	return pods[0].Status.PodIP, nil
}
