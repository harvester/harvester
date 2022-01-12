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
	AnnotationHash                 = prefix + "/hash"
	AnnotationRemovePVC            = prefix + "/removePVC"

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
