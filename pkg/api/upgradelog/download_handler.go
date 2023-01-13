package upgradelog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlupgradelog "github.com/harvester/harvester/pkg/controller/master/upgradelog"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

const (
	upgradeLogNamespace = "harvester-system"
	archiveSuffix       = ".tar.gz"
)

type DownloadHandler struct {
	context          context.Context
	podCache         ctlcorev1.PodCache
	upgradeLogClient ctlharvesterv1.UpgradeLogClient
	upgradeLogCache  ctlharvesterv1.UpgradeLogCache
	httpClient       *http.Client
}

func NewDownloadHandler(scaled *config.Scaled) *DownloadHandler {
	return &DownloadHandler{
		context:          scaled.Ctx,
		podCache:         scaled.CoreFactory.Core().V1().Pod().Cache(),
		upgradeLogClient: scaled.HarvesterFactory.Harvesterhci().V1beta1().UpgradeLog(),
		upgradeLogCache:  scaled.HarvesterFactory.Harvesterhci().V1beta1().UpgradeLog().Cache(),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (h *DownloadHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	upgradeLogName := mux.Vars(r)["upgradeLogName"]
	archiveName := mux.Vars(r)["archiveName"]

	upgradeLog, err := h.upgradeLogCache.Get(upgradeLogNamespace, upgradeLogName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			util.ResponseError(rw, http.StatusNotFound, errors.Wrap(err, "fail to get the upgradelog resource"))
			return
		}
		util.ResponseError(rw, http.StatusInternalServerError, errors.Wrap(err, "fail to get the upgradelog resource"))
		return
	}
	isDownloadReady := false
	for _, condition := range upgradeLog.Status.Conditions {
		if condition.Type == harvesterv1.DownloadReady && condition.Status == corev1.ConditionTrue {
			isDownloadReady = true
			break
		}
	}
	if isDownloadReady {
		downloaderPodIP, err := ctlupgradelog.GetDownloaderPodIP(h.podCache, upgradeLog)
		if err != nil {
			util.ResponseError(rw, http.StatusBadRequest, errors.Wrap(err, "fail to get the downloader pod IP"))
			return
		}

		url := fmt.Sprintf("http://%s/%s", downloaderPodIP, fmt.Sprintf("%s%s", archiveName, archiveSuffix))
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
			msg := fmt.Sprintf("Unexpected status code %d from downloader.", resp.StatusCode)
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

		// TODO: set Content-Length with archive size
		rw.Header().Set("Content-Disposition", "attachment; filename="+fmt.Sprintf("%s%s", archiveName, archiveSuffix))
		contentType := resp.Header.Get("Content-Type")
		if contentType != "" {
			rw.Header().Set("Content-Type", contentType)
		}

		if _, err := io.Copy(rw, resp.Body); err != nil {
			util.ResponseError(rw, http.StatusInternalServerError, err)
			return
		}
	} else {
		util.ResponseError(rw, http.StatusNotAcceptable, errors.New("log archive is not ready"))
		return
	}
}
