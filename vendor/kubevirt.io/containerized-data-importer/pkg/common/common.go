package common

import (
	"encoding/json"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
)

// Common types and constants used by the importer and controller.
// TODO: maybe the vm cloner can use these common values

const (
	// CDILabelKey provides a constant for CDI PVC labels
	CDILabelKey = "app"
	// CDILabelValue provides a constant  for CDI PVC label values
	CDILabelValue = "containerized-data-importer"
	// CDILabelSelector provides a constant to use for the selector to identify CDI objects in list
	CDILabelSelector = CDILabelKey + "=" + CDILabelValue

	// CDIComponentLabel can be added to all CDI resources
	CDIComponentLabel = "cdi.kubevirt.io"
	// CDIControllerName is the CDI controller name
	CDIControllerName = "cdi-controller"
	// CDIOperatorName is the CDI operator name
	CDIOperatorName = "cdi-operator"

	// CDIControllerResourceName is the CDI controller resource name
	CDIControllerResourceName = "cdi-deployment"
	// CDIApiServerResourceName is the CDI apiserver resource name
	CDIApiServerResourceName = "cdi-apiserver"
	// CDIUploadProxyResourceName is the CDI uploadproxy resource name
	CDIUploadProxyResourceName = "cdi-uploadproxy"
	// CDICronJobResourceName is the CDI cronjob resource name
	CDICronJobResourceName = "cdi-cronjob"

	// AppKubernetesPartOfLabel is the Kubernetes recommended part-of label
	AppKubernetesPartOfLabel = "app.kubernetes.io/part-of"
	// AppKubernetesVersionLabel is the Kubernetes recommended version label
	AppKubernetesVersionLabel = "app.kubernetes.io/version"
	// AppKubernetesManagedByLabel is the Kubernetes recommended managed-by label
	AppKubernetesManagedByLabel = "app.kubernetes.io/managed-by"
	// AppKubernetesComponentLabel is the Kubernetes recommended component label
	AppKubernetesComponentLabel = "app.kubernetes.io/component"

	// PrometheusLabelKey provides the label to indicate prometheus metrics are available in the pods.
	PrometheusLabelKey = "prometheus.cdi.kubevirt.io"
	// PrometheusLabelValue provides the label value which shouldn't be empty to avoid a prometheus WIP issue.
	PrometheusLabelValue = "true"
	// PrometheusServiceName is the name of the prometheus service created by the operator.
	PrometheusServiceName = "cdi-prometheus-metrics"
	// KubePersistentVolumeFillingUpSuppressLabelKey is the label name that helps suppress this alert for our PVCs
	KubePersistentVolumeFillingUpSuppressLabelKey = "alerts.k8s.io/KubePersistentVolumeFillingUp"
	// KubePersistentVolumeFillingUpSuppressLabelValue is the label value that helps suppress this alert for our PVCs
	KubePersistentVolumeFillingUpSuppressLabelValue = "disabled"

	// UploadTargetLabel has the UID of upload target PVC
	UploadTargetLabel = CDIComponentLabel + "/uploadTarget"

	// DataImportCronLabel has the name of the DataImportCron responsible for the labeled resource
	DataImportCronLabel = CDIComponentLabel + "/dataImportCron"
	// DataImportCronNsLabel has the namespace of the DataImportCron responsible for the labeled resource
	DataImportCronNsLabel = CDIComponentLabel + "/dataImportCronNs"
	// DataImportCronCleanupLabel tells whether to delete the resource when its DataImportCron is deleted
	DataImportCronCleanupLabel = DataImportCronLabel + ".cleanup"

	// PvcApplyStorageProfileLabel tells whether the PVC should be rendered by the mutating webhook based on StorageProfiles
	PvcApplyStorageProfileLabel = CDIComponentLabel + "/applyStorageProfile"

	// ImporterVolumePath provides a constant for the directory where the PV is mounted.
	ImporterVolumePath = "/data"
	// DiskImageName provides a constant for our importer/datastream_ginkgo_test and to build ImporterWritePath
	DiskImageName = "disk.img"
	// ImporterWritePath provides a constant for the cmd/cdi-importer/importer.go executable
	ImporterWritePath = ImporterVolumePath + "/" + DiskImageName
	// WriteBlockPath provides a constant for the path where the PV is mounted.
	WriteBlockPath = "/dev/cdi-block-volume"
	// NbdkitLogPath provides a constant for the path in which the nbdkit log messages are stored.
	NbdkitLogPath = "/tmp/nbdkit.log"
	// PodTerminationMessageFile is the name of the file to write the termination message to.
	PodTerminationMessageFile = "/dev/termination-log"
	// ImporterPodName provides a constant to use as a prefix for Pods created by CDI (controller only)
	ImporterPodName = "importer"
	// ImporterDataDir provides a constant for the controller pkg to use as a hardcoded path to where content is transferred to/from (controller only)
	ImporterDataDir = "/data"
	// ScratchDataDir provides a constant for the controller pkg to use as a hardcoded path to where scratch space is located.
	ScratchDataDir = "/scratch"
	// ImporterS3Host provides an S3 string used by importer/dataStream.go only
	ImporterS3Host = "s3.amazonaws.com"
	// ImporterCertDir is where the configmap containing certs will be mounted
	ImporterCertDir = "/certs"
	// DefaultPullPolicy imports k8s "IfNotPresent" string for the import_controller_gingko_test and the cdi-controller executable
	DefaultPullPolicy = string(v1.PullIfNotPresent)
	// ImportProxyConfigMapName provides the key for getting the name of the ConfigMap in the cdi namespace containing a CA certificate bundle
	ImportProxyConfigMapName = "trusted-ca-proxy-bundle-cm"
	// ImportProxyConfigMapKey provides the key name of the ConfigMap in the cdi namespace containing a CA certificate bundle
	ImportProxyConfigMapKey = "ca.crt"
	// ImporterProxyCertDir is where the configmap containing proxy certs will be mounted
	ImporterProxyCertDir = "/proxycerts/"

	// PullPolicy provides a constant to capture our env variable "PULL_POLICY" (only used by cmd/cdi-controller/controller.go)
	PullPolicy = "PULL_POLICY"
	// ImporterSource provides a constant to capture our env variable "IMPORTER_SOURCE"
	ImporterSource = "IMPORTER_SOURCE"
	// ImporterContentType provides a constant to capture our env variable "IMPORTER_CONTENTTYPE"
	ImporterContentType = "IMPORTER_CONTENTTYPE"
	// ImporterEndpoint provides a constant to capture our env variable "IMPORTER_ENDPOINT"
	ImporterEndpoint = "IMPORTER_ENDPOINT"
	// ImporterAccessKeyID provides a constant to capture our env variable "IMPORTER_ACCES_KEY_ID"
	ImporterAccessKeyID = "IMPORTER_ACCESS_KEY_ID"
	// ImporterSecretKey provides a constant to capture our env variable "IMPORTER_SECRET_KEY"
	ImporterSecretKey = "IMPORTER_SECRET_KEY"
	// ImporterImageSize provides a constant to capture our env variable "IMPORTER_IMAGE_SIZE"
	ImporterImageSize = "IMPORTER_IMAGE_SIZE"
	// ImporterCertDirVar provides a constant to capture our env variable "IMPORTER_CERT_DIR"
	ImporterCertDirVar = "IMPORTER_CERT_DIR"
	// InsecureTLSVar provides a constant to capture our env variable "INSECURE_TLS"
	InsecureTLSVar = "INSECURE_TLS"
	// CiphersTLSVar provides a constant to capture our env variable "TLS_CIPHERS"
	CiphersTLSVar = "TLS_CIPHERS"
	// MinVersionTLSVar provides a constant to capture our env variable "TLS_MIN_VERSION"
	MinVersionTLSVar = "TLS_MIN_VERSION"
	// ImporterDiskID provides a constant to capture our env variable "IMPORTER_DISK_ID"
	ImporterDiskID = "IMPORTER_DISK_ID"
	// ImporterUUID provides a constant to capture our env variable "IMPORTER_UUID"
	ImporterUUID = "IMPORTER_UUID"
	// ImporterPullMethod provides a constant to capture our env variable "IMPORTER_PULL_METHOD"
	ImporterPullMethod = "IMPORTER_PULL_METHOD"
	// ImporterReadyFile provides a constant to capture our env variable "IMPORTER_READY_FILE"
	ImporterReadyFile = "IMPORTER_READY_FILE"
	// ImporterDoneFile provides a constant to capture our env variable "IMPORTER_DONE_FILE"
	ImporterDoneFile = "IMPORTER_DONE_FILE"
	// ImporterBackingFile provides a constant to capture our env variable "IMPORTER_BACKING_FILE"
	ImporterBackingFile = "IMPORTER_BACKING_FILE"
	// ImporterThumbprint provides a constant to capture our env variable "IMPORTER_THUMBPRINT"
	ImporterThumbprint = "IMPORTER_THUMBPRINT"
	// ImporterCurrentCheckpoint provides a constant to capture our env variable "IMPORTER_CURRENT_CHECKPOINT"
	ImporterCurrentCheckpoint = "IMPORTER_CURRENT_CHECKPOINT"
	// ImporterPreviousCheckpoint provides a constant to capture our env variable "IMPORTER_PREVIOUS_CHECKPOINT"
	ImporterPreviousCheckpoint = "IMPORTER_PREVIOUS_CHECKPOINT"
	// ImporterFinalCheckpoint provides a constant to capture our env variable "IMPORTER_FINAL_CHECKPOINT"
	ImporterFinalCheckpoint = "IMPORTER_FINAL_CHECKPOINT"
	// CacheMode provides a constant to capture our env variable "CACHE_MODE"
	CacheMode = "CACHE_MODE"
	// CacheModeTryNone provides a constant to capture our env variable value for "CACHE_MODE" that tries O_DIRECT writing if target supports it
	CacheModeTryNone = "TRYNONE"
	// Preallocation provides a constant to capture out env variable "PREALLOCATION"
	Preallocation = "PREALLOCATION"
	// ImportProxyHTTP provides a constant to capture our env variable "http_proxy"
	ImportProxyHTTP = "http_proxy"
	// ImportProxyHTTPS provides a constant to capture our env variable "https_proxy"
	ImportProxyHTTPS = "https_proxy"
	// ImportProxyNoProxy provides a constant to capture our env variable "no_proxy"
	ImportProxyNoProxy = "no_proxy"
	// ImporterProxyCertDirVar provides a constant to capture our env variable "IMPORTER_PROXY_CERT_DIR"
	ImporterProxyCertDirVar = "IMPORTER_PROXY_CERT_DIR"
	// InstallerPartOfLabel provides a constant to capture our env variable "INSTALLER_PART_OF_LABEL"
	InstallerPartOfLabel = "INSTALLER_PART_OF_LABEL"
	// InstallerVersionLabel provides a constant to capture our env variable "INSTALLER_VERSION_LABEL"
	InstallerVersionLabel = "INSTALLER_VERSION_LABEL"
	// ImporterExtraHeader provides a constant to include extra HTTP headers, as the prefix to a format string
	ImporterExtraHeader = "IMPORTER_EXTRA_HEADER_"
	// ImporterSecretExtraHeadersDir is where the secrets containing extra HTTP headers will be mounted
	ImporterSecretExtraHeadersDir = "/extraheaders"

	// ImporterGoogleCredentialFileVar provides a constant to capture our env variable "GOOGLE_APPLICATION_CREDENTIALS"
	//nolint:gosec // This is not a real credential
	ImporterGoogleCredentialFileVar = "GOOGLE_APPLICATION_CREDENTIALS"
	// ImporterGoogleCredentialDir provides a constant to capture our secret mount Dir
	ImporterGoogleCredentialDir = "/google"
	// ImporterGoogleCredentialFile provides a constant to capture our credentials.json file
	//nolint:gosec // This is not the credential itself
	ImporterGoogleCredentialFile = "/google/credentials.json"

	// CloningLabelValue provides a constant to use as a label value for pod affinity (controller pkg only)
	CloningLabelValue = "host-assisted-cloning"
	// CloningTopologyKey  (controller pkg only)
	CloningTopologyKey = "kubernetes.io/hostname"
	// ClonerSourcePodName (controller pkg only)
	ClonerSourcePodName = "cdi-clone-source"
	// ClonerMountPath (controller pkg only)
	ClonerMountPath = "/var/run/cdi/clone/source"
	// ClonerSourcePodNameSuffix (controller pkg only)
	ClonerSourcePodNameSuffix = "-source-pod"

	// KubeVirtAnnKey is part of a kubevirt.io key.
	KubeVirtAnnKey = "kubevirt.io/"
	// CDIAnnKey is part of a kubevirt.io key.
	CDIAnnKey = "cdi.kubevirt.io/"

	// SmartClonerCDILabel is the label applied to resources created by the smart-clone controller
	SmartClonerCDILabel = "cdi-smart-clone"

	// CloneFromSnapshotFallbackPVCCDILabel is the label applied to the temp host assisted PVC used for fallback in cloning from volumesnapshot
	CloneFromSnapshotFallbackPVCCDILabel = "cdi-clone-from-snapshot-source-host-assisted-fallback-pvc"

	// UploadPodName (controller pkg only)
	UploadPodName = "cdi-upload"
	// UploadServerCDILabel is the label applied to upload server resources
	UploadServerCDILabel = "cdi-upload-server"
	// UploadServerPodname is name of the upload server pod container
	UploadServerPodname = UploadServerCDILabel
	// UploadServerDataDir is the destination directoryfor uploads
	UploadServerDataDir = ImporterDataDir
	// UploadServerServiceLabel is the label selector for upload server services
	UploadServerServiceLabel = "service"
	// UploadImageSize provides a constant to capture our env variable "UPLOAD_IMAGE_SIZE"
	UploadImageSize = "UPLOAD_IMAGE_SIZE"

	// FilesystemOverheadVar provides a constant to capture our env variable "FILESYSTEM_OVERHEAD"
	FilesystemOverheadVar = "FILESYSTEM_OVERHEAD"
	// DefaultGlobalOverhead is the amount of space reserved on Filesystem volumes by default
	DefaultGlobalOverhead = "0.055"

	// ConfigName is the name of default CDI Config
	ConfigName = "config"

	// OwnerUID provides the UID of the owner entity (either PVC or DV)
	OwnerUID = "OWNER_UID"

	// KeyAccess provides a constant to the accessKeyId label using in controller pkg and transport_test.go
	KeyAccess = "accessKeyId"
	// KeySecret provides a constant to the secretKey label using in controller pkg and transport_test.go
	KeySecret = "secretKey"

	// DefaultResyncPeriod sets a 10 minute resync period, used in the controller pkg and the controller cmd executable
	DefaultResyncPeriod = 10 * time.Minute

	// ScratchNameSuffix (controller pkg only)
	ScratchNameSuffix = "scratch"

	// UploadTokenIssuer is the JWT issuer of upload tokens
	UploadTokenIssuer = "cdi-apiserver"

	// CloneTokenIssuer is the JWT issuer for clone tokens
	CloneTokenIssuer = "cdi-apiserver"

	// ExtendedCloneTokenIssuer is the JWT issuer for clone tokens
	ExtendedCloneTokenIssuer = "cdi-deployment"

	// QemuSubGid is the gid used as the qemu group in fsGroup
	QemuSubGid = int64(107)

	// ControllerServiceAccountName is the name of the CDI controller service account
	ControllerServiceAccountName = "cdi-sa"

	// CronJobServiceAccountName is the name of the CDI cron job service account
	CronJobServiceAccountName = "cdi-cronjob"

	// VddkConfigMap is the name of the ConfigMap with a reference to the VDDK image
	VddkConfigMap = "v2v-vmware"
	// VddkConfigDataKey is the name of the ConfigMap key of the VDDK image reference
	VddkConfigDataKey = "vddk-init-image"
	// AwaitingVDDK is a Pending condition reason that indicates the PVC is waiting for a VDDK image
	AwaitingVDDK = "AwaitingVDDK"

	// UploadContentTypeHeader is the header upload clients may use to set the content type explicitly
	UploadContentTypeHeader = "x-cdi-content-type"

	// FilesystemCloneContentType is the content type when cloning a filesystem
	FilesystemCloneContentType = "filesystem-clone"

	// BlockdeviceClone is the content type when cloning a block device
	BlockdeviceClone = "blockdevice-clone"

	// UploadPathSync is the path to POST CDI uploads
	UploadPathSync = "/v1beta1/upload"

	// UploadPathAsync is the path to POST CDI uploads in async mode
	UploadPathAsync = "/v1beta1/upload-async"

	// UploadArchivePath is the path to POST CDI archive uploads
	UploadArchivePath = "/v1beta1/upload-archive"

	// UploadArchiveAlphaPath is the path to POST CDI alpha archive uploads
	UploadArchiveAlphaPath = "/v1alpha1/upload-archive"

	// UploadFormSync is the path to POST CDI uploads as form data
	UploadFormSync = "/v1beta1/upload-form"

	// UploadFormAsync is the path to POST CDI uploads as form data in async mode
	UploadFormAsync = "/v1beta1/upload-form-async"

	// PreallocationApplied is a string inserted into importer's/uploader's exit message
	PreallocationApplied = "Preallocation applied"

	// ScratchSpaceRequired is a string inserted into a pod exist message when scratch space is needed
	ScratchSpaceRequired = "scratch space required and none found"

	// SecretHeader is the key in a secret containing a sensitive extra header for HTTP data sources
	SecretHeader = "secretHeader"

	// UnusualRestartCountThreshold is the number of pod restarts that we consider unusual and would like to alert about
	UnusualRestartCountThreshold = 3

	// GenericError is a generic error string
	GenericError = "Error"

	// CDIControllerLeaderElectionHelperName is the name of the configmap that is used as a helper for controller leader election
	CDIControllerLeaderElectionHelperName = "cdi-controller-leader-election-helper"

	// ImagePullFailureText is the text of the ErrImagePullFailed error. We need it as a common constant because we're using
	// both to create and to later check the error in the termination text of the importer pod.
	ImagePullFailureText = "failed to pull image"
)

// ProxyPaths are all supported paths
var ProxyPaths = append(
	append(SyncUploadPaths, AsyncUploadPaths...),
	append(SyncUploadFormPaths, AsyncUploadFormPaths...)...,
)

// SyncUploadPaths are paths to POST CDI uploads
var SyncUploadPaths = []string{
	UploadPathSync,
	"/v1alpha1/upload",
}

// AsyncUploadPaths are paths to POST CDI uploads in async mode
var AsyncUploadPaths = []string{
	UploadPathAsync,
	"/v1alpha1/upload-async",
}

// ArchiveUploadPaths are paths to POST CDI uploads of archive
var ArchiveUploadPaths = []string{
	UploadArchivePath,
	UploadArchiveAlphaPath,
}

// SyncUploadFormPaths are paths to POST CDI uploads as form data
var SyncUploadFormPaths = []string{
	UploadFormSync,
	"/v1alpha1/upload-form",
}

// AsyncUploadFormPaths are paths to POST CDI uploads as form data in async mode
var AsyncUploadFormPaths = []string{
	UploadFormAsync,
	"/v1alpha1/upload-form-async",
}

// VddkInfo holds VDDK version and connection information returned by an importer pod
type VddkInfo struct {
	Version string
	Host    string
}

// TerminationMessage contains data to be serialized and used as the termination message of the importer.
type TerminationMessage struct {
	ScratchSpaceRequired *bool             `json:"scratchSpaceRequired,omitempty"`
	PreallocationApplied *bool             `json:"preallocationApplied,omitempty"`
	DeadlinePassed       *bool             `json:"deadlinePassed,omitempty"`
	VddkInfo             *VddkInfo         `json:"vddkInfo,omitempty"`
	Labels               map[string]string `json:"labels,omitempty"`
	Message              *string           `json:"message,omitempty"`
}

func (it *TerminationMessage) String() (string, error) {
	msg, err := json.Marshal(it)
	if err != nil {
		return "", err
	}

	// Messages longer than 4096 are truncated by kubelet
	if length := len(msg); length > 4096 {
		return "", fmt.Errorf("Termination message length %d exceeds maximum length of 4096 bytes", length)
	}

	return string(msg), nil
}

// ServerInfo contains data to be serialized and used as the body of responses to the info endpoint of the containerimage-server.
type ServerInfo struct {
	Env []string `json:"env,omitempty"`
}
