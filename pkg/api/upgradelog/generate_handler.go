package upgradelog

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	ctlbatchv1 "github.com/rancher/wrangler/pkg/generated/controllers/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/config"
	ctlupgradelog "github.com/harvester/harvester/pkg/controller/master/upgradelog"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

type GenerateHandler struct {
	context          context.Context
	jobClient        ctlbatchv1.JobClient
	upgradeCache     ctlharvesterv1.UpgradeCache
	upgradeLogClient ctlharvesterv1.UpgradeLogClient
	upgradeLogCache  ctlharvesterv1.UpgradeLogCache
}

func NewGenerateHandler(scaled *config.Scaled) *GenerateHandler {
	return &GenerateHandler{
		context:          scaled.Ctx,
		jobClient:        scaled.BatchFactory.Batch().V1().Job(),
		upgradeCache:     scaled.HarvesterFactory.Harvesterhci().V1beta1().Upgrade().Cache(),
		upgradeLogClient: scaled.HarvesterFactory.Harvesterhci().V1beta1().UpgradeLog(),
		upgradeLogCache:  scaled.HarvesterFactory.Harvesterhci().V1beta1().UpgradeLog().Cache(),
	}
}

func (h *GenerateHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	upgradeLogName := mux.Vars(r)["upgradeLogName"]
	upgradeLog, err := h.upgradeLogCache.Get(upgradeLogNamespace, upgradeLogName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			util.ResponseError(rw, http.StatusNotFound, err)
			return
		}
		util.ResponseError(rw, http.StatusInternalServerError, err)
		return
	}
	isUpgradeLogReady := false
	for _, condition := range upgradeLog.Status.Conditions {
		if condition.Type == harvesterv1.UpgradeLogReady && condition.Status == corev1.ConditionTrue {
			isUpgradeLogReady = true
			break
		}
	}
	if isUpgradeLogReady {
		// Get image version for log packager
		upgrade, err := h.upgradeCache.Get(upgradeLogNamespace, upgradeLog.Spec.Upgrade)
		if err != nil {
			if apierrors.IsNotFound(err) {
				util.ResponseError(rw, http.StatusNotFound, err)
				return
			}
			util.ResponseError(rw, http.StatusInternalServerError, err)
			return
		}
		imageVersion := upgrade.Status.PreviousVersion

		ts := time.Now().UTC()
		generatedTime := strings.Replace(ts.Format(time.RFC3339), ":", "-", -1)
		archiveName := fmt.Sprintf("%s-archive-%s", upgradeLog.Name, generatedTime)
		// TODO: update with the real size later
		archiveSize := int64(0)

		var component string
		if harvesterv1.UpgradeEnded.IsTrue(upgradeLog) {
			component = ctlupgradelog.DownloaderComponent
		} else {
			component = ctlupgradelog.AggregatorComponent
		}

		if _, err := h.jobClient.Create(ctlupgradelog.PrepareLogPackager(upgradeLog, imageVersion, archiveName, component)); err != nil {
			util.ResponseError(rw, http.StatusInternalServerError, errors.Wrap(err, "fail to create log packager job"))
			return
		}
		toUpdate := upgradeLog.DeepCopy()
		ctlupgradelog.SetUpgradeLogArchive(toUpdate, archiveName, archiveSize, generatedTime, false)
		if _, err := h.upgradeLogClient.Update(toUpdate); err != nil {
			util.ResponseError(rw, http.StatusInternalServerError, errors.Wrap(err, "fail to update upgradelog resource"))
			return
		}
		util.ResponseOKWithBody(rw, archiveName)
		return
	}
	util.ResponseError(rw, http.StatusNotAcceptable, fmt.Errorf("logging infra for upgradelog %s is not ready", upgradeLog.Name))
}
