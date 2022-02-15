package util

const (
	prefix                         = "harvesterhci.io"
	RemovedPVCsAnnotationKey       = prefix + "/removedPersistentVolumeClaims"
	AdditionalCASecretName         = "harvester-additional-ca"
	AdditionalCAFileName           = "additional-ca.pem"
	AnnotationMigrationTarget      = prefix + "/migrationTargetNodeName"
	AnnotationMigrationUID         = prefix + "/migrationUID"
	AnnotationMigrationState       = prefix + "/migrationState"
	AnnotationTimestamp            = prefix + "/timestamp"
	AnnotationVolumeClaimTemplates = prefix + "/volumeClaimTemplates"
	AnnotationImageID              = prefix + "/imageId"
	AnnotationReservedMemory       = prefix + "/reservedMemory"
	AnnotationHash                 = prefix + "/hash"
	// used in VM and PVC annotations, represents Harvester volume abstracted status based on CSI driver (e.g. LH) volume status
	// when this annotation is not existing or the value is empty, it means the volume is OK, other values mean !OK
	AnnotationVolumeStatus = prefix + "/volume-status"
	// used in PVC annotations, represents the previous owner VM of the PVC, PVC is just detached from it
	// controller removes this annotation quickly
	// NOTE: this annotation only exists for a very short time
	AnnotationDetachedVM = prefix + "/detached-vm"

	BackupTargetSecretName      = "harvester-backup-target-secret"
	InternalTLSSecretName       = "tls-rancher-internal"
	Rke2IngressNginxAppName     = "rke2-ingress-nginx"
	CattleSystemNamespaceName   = "cattle-system"
	LonghornSystemNamespaceName = "longhorn-system"
	KubeSystemNamespace         = "kube-system"

	HTTPProxyEnv  = "HTTP_PROXY"
	HTTPSProxyEnv = "HTTPS_PROXY"
	NoProxyEnv    = "NO_PROXY"
)
