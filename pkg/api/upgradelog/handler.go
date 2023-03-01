package upgradelog

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/wrangler/pkg/condition"
	ctlbatchv1 "github.com/rancher/wrangler/pkg/generated/controllers/batch/v1"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/name"
	"github.com/rancher/wrangler/pkg/schemas/validation"
	"github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlupgradelog "github.com/harvester/harvester/pkg/controller/master/upgradelog"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

const (
	archiveSuffix                = ".zip"
	defaultJobBackoffLimit int32 = 5
	logPackagingScript           = `
#!/usr/bin/env sh
set -ex

echo "start to package upgrade logs"

tmpdir=$(mktemp -d)
mkdir $tmpdir/logs

cd /archive/logs

for f in *.log
do
    cat $f | awk '{$1=$2=""; print $0}' | jq -r .message > $tmpdir/logs/$f
	if [ $? -eq 4 ]; then
	    echo "Incomplete JSON format log line, abort processing the file $f."
	    continue
	fi
done

cd $tmpdir
zip -r /archive/$ARCHIVE_NAME ./logs/
echo "done"
`
)

type Handler struct {
	httpClient       *http.Client
	jobClient        ctlbatchv1.JobClient
	podCache         ctlcorev1.PodCache
	upgradeCache     ctlharvesterv1.UpgradeCache
	upgradeLogCache  ctlharvesterv1.UpgradeLogCache
	upgradeLogClient ctlharvesterv1.UpgradeLogClient
}

func (h Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if err := h.doAction(rw, req); err != nil {
		status := http.StatusInternalServerError
		if e, ok := err.(*apierror.APIError); ok {
			status = e.Code.Status
		}
		rw.WriteHeader(status)
		_, _ = rw.Write([]byte(err.Error()))
		return
	}
}

func (h Handler) doAction(rw http.ResponseWriter, req *http.Request) error {
	vars := util.EncodeVars(mux.Vars(req))

	if req.Method == http.MethodGet {
		return h.doGet(vars["link"], rw, req)
	} else if req.Method == http.MethodPost {
		return h.doPost(vars["action"], rw, req)
	}

	return apierror.NewAPIError(validation.InvalidAction, fmt.Sprintf("Unsupported method %s", req.Method))
}

func (h Handler) doGet(link string, rw http.ResponseWriter, req *http.Request) error {
	switch link {
	case downloadArchiveLink:
		return h.downloadArchive(rw, req)
	default:
		return apierror.NewAPIError(validation.InvalidAction, fmt.Sprintf("Unsupported GET action %s", link))
	}
}

func (h Handler) doPost(action string, rw http.ResponseWriter, req *http.Request) error {
	switch action {
	case generateArchiveAction:
		return h.generateArchive(rw, req)
	default:
		return apierror.NewAPIError(validation.InvalidAction, fmt.Sprintf("Unsupported POST action %s", action))
	}
}

func (h Handler) downloadArchive(rw http.ResponseWriter, req *http.Request) error {
	vars := util.EncodeVars(mux.Vars(req))
	upgradeLogName := vars["name"]
	upgradeLogNamespace := vars["namespace"]
	archiveName := req.URL.Query().Get("archiveName")

	logrus.Infof("Retrieve the archive (%s) for the UpgradeLog (%s/%s)", archiveName, upgradeLogNamespace, upgradeLogName)

	upgradeLog, err := h.upgradeLogCache.Get(upgradeLogNamespace, upgradeLogName)
	if err != nil {
		return fmt.Errorf("failed to get the upgradelog resource (%s/%s): %w", upgradeLogNamespace, upgradeLogName, err)
	}

	isDownloadReady := checkConditionReady(upgradeLog.Status.Conditions, harvesterv1.DownloadReady)
	if !isDownloadReady {
		return fmt.Errorf("the archive (%s) of upgrade resource (%s/%s) is not ready yet", archiveName, upgradeLogNamespace, upgradeLogName)
	}

	// Crafting download request
	archiveFileName := fmt.Sprintf("%s%s", archiveName, archiveSuffix)
	downloadURL := fmt.Sprintf("http://%s.%s/%s", upgradeLog.Name, util.HarvesterSystemNamespaceName, archiveFileName)
	downloadReq, err := http.NewRequestWithContext(req.Context(), http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create the download request for the archive (%s): %w", archiveName, err)
	}

	// Get the archive from the log downloader
	downloadResp, err := h.doRetry(downloadReq)
	if err != nil {
		return fmt.Errorf("failed to send the download request for the archive (%s): %w", archiveName, err)
	}
	defer downloadResp.Body.Close()

	if downloadResp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed with unexpected http status code %d", downloadResp.StatusCode)
	}

	// TODO: set Content-Length with archive size
	rw.Header().Set("Content-Disposition", "attachment; filename="+archiveFileName)
	contentType := downloadResp.Header.Get("Content-Type")
	if contentType != "" {
		rw.Header().Set("Content-Type", contentType)
	}

	if _, err := io.Copy(rw, downloadResp.Body); err != nil {
		return fmt.Errorf("failed to copy the downloaded content to the target (%s), err: %w", archiveFileName, err)
	}

	return nil
}

func (h Handler) generateArchive(rw http.ResponseWriter, req *http.Request) error {
	vars := util.EncodeVars(mux.Vars(req))
	upgradeLogName := vars["name"]
	upgradeLogNamespace := vars["namespace"]

	logrus.Infof("Generate an archive for the UpgradeLog (%s/%s)", upgradeLogNamespace, upgradeLogName)

	upgradeLog, err := h.upgradeLogCache.Get(upgradeLogNamespace, upgradeLogName)
	if err != nil {
		return fmt.Errorf("failed to get the upgradelog resource (%s/%s): %w", upgradeLogNamespace, upgradeLogName, err)
	}

	isUpgradeLogReady := checkConditionReady(upgradeLog.Status.Conditions, harvesterv1.UpgradeLogReady)
	if !isUpgradeLogReady {
		return fmt.Errorf("the logging infrastructure for the upgradelog resource (%s/%s) is not ready yet", upgradeLogNamespace, upgradeLogName)
	}

	// Get image version for log packager
	upgradeName := upgradeLog.Spec.UpgradeName
	upgrade, err := h.upgradeCache.Get(util.HarvesterSystemNamespaceName, upgradeName)
	if err != nil {
		return fmt.Errorf("failed to get the upgrade resource (%s/%s): %w", util.HarvesterSystemNamespaceName, upgradeName, err)
	}
	imageVersion := upgrade.Status.PreviousVersion

	ts := time.Now().UTC()
	generatedTime := strings.Replace(ts.Format(time.RFC3339), ":", "-", -1)
	archiveName := fmt.Sprintf("%s-archive-%s", upgradeLog.Name, generatedTime)
	// TODO: update with the real size later
	archiveSize := int64(0)

	// To decide the PodAffinity for the log packager Pod
	// 1. If the logging infrastructure has been torn down, the packager Pod should be with the downloader
	// 2. If the logging infrastructure still exists, the packager Pod should be with the aggregator (fluentd)
	var component string
	if harvesterv1.UpgradeEnded.IsTrue(upgradeLog) {
		component = util.UpgradeLogDownloaderComponent
	} else {
		component = util.UpgradeLogAggregatorComponent
	}

	if _, err := h.jobClient.Create(prepareLogPackager(upgradeLog, imageVersion, archiveName, component)); err != nil {
		return fmt.Errorf("failed to create log packager job for the upgradelog resource (%s/%s): %w", upgradeLogNamespace, upgradeLogName, err)
	}
	toUpdate := upgradeLog.DeepCopy()
	ctlupgradelog.SetUpgradeLogArchive(toUpdate, archiveName, archiveSize, generatedTime, false)
	if _, err := h.upgradeLogClient.Update(toUpdate); err != nil {
		return fmt.Errorf("failed to update the upgradelog resource (%s/%s): %w", upgradeLogNamespace, upgradeLogName, err)
	}
	util.ResponseOKWithBody(rw, archiveName)

	return nil
}

func (h Handler) doRetry(req *http.Request) (*http.Response, error) {
	const retry = 15
	var (
		err  error
		resp *http.Response
	)

	for i := 0; i < retry; i++ {
		resp, err = h.httpClient.Do(req)
		if err == nil {
			return resp, nil
		}
		time.Sleep(2 * time.Second)
	}

	// return the last error
	return nil, err
}

func checkConditionReady(conditions []harvesterv1.Condition, targetCondition condition.Cond) bool {
	isReady := false
	for _, condition := range conditions {
		if condition.Type == targetCondition && condition.Status == corev1.ConditionTrue {
			isReady = true
			break
		}
	}
	return isReady
}

func prepareLogPackager(upgradeLog *harvesterv1.UpgradeLog, imageVersion, archiveName, component string) *batchv1.Job {
	backoffLimit := defaultJobBackoffLimit
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				util.AnnotationArchiveName: archiveName,
			},
			Labels: map[string]string{
				util.LabelUpgradeLog:          upgradeLog.Name,
				util.LabelUpgradeLogComponent: util.UpgradeLogPackagerComponent,
			},
			GenerateName: name.SafeConcatName(upgradeLog.Name, util.UpgradeLogPackagerComponent) + "-",
			Namespace:    util.HarvesterSystemNamespaceName,
			OwnerReferences: []metav1.OwnerReference{
				{
					Name:       upgradeLog.Name,
					Kind:       upgradeLog.Kind,
					UID:        upgradeLog.UID,
					APIVersion: upgradeLog.APIVersion,
				},
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						util.LabelUpgradeLog:          upgradeLog.Name,
						util.LabelUpgradeLogComponent: util.UpgradeLogPackagerComponent,
					},
				},
				Spec: corev1.PodSpec{
					Affinity: &corev1.Affinity{
						PodAffinity: &corev1.PodAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											util.LabelUpgradeLog:          upgradeLog.Name,
											util.LabelUpgradeLogComponent: component,
										},
									},
									Namespaces: []string{
										util.HarvesterSystemNamespaceName,
									},
									TopologyKey: "kubernetes.io/hostname",
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "packager",
							Image: fmt.Sprintf("%s:%s", util.HarvesterUpgradeImageRepository, imageVersion),
							Command: []string{
								"sh", "-c", logPackagingScript,
							},
							Env: []corev1.EnvVar{
								{
									Name:  "ARCHIVE_NAME",
									Value: archiveName,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "log-archive",
									MountPath: "/archive",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "log-archive",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: name.SafeConcatName(upgradeLog.Name, util.UpgradeLogArchiveComponent),
								},
							},
						},
					},
					Tolerations: []corev1.Toleration{
						{
							Key:      "kubevirt.io/drain",
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoSchedule,
						},
						{
							Key:      "node.kubernetes.io/unschedulable",
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoSchedule,
						},
					},
					RestartPolicy: corev1.RestartPolicyOnFailure,
				},
			},
			BackoffLimit: &backoffLimit,
		},
	}
}
