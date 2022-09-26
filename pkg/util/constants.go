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
	AnnotationRunStrategy          = prefix + "/vmRunStrategy"
	LabelImageDisplayName          = prefix + "/imageDisplayName"

	AnnotationStorageClassName          = prefix + "/storageClassName"
	AnnotationStorageProvisioner        = prefix + "/storageProvisioner"
	AnnotationIsDefaultStorageClassName = "storageclass.kubernetes.io/is-default-class"

	ContainerdRegistrySecretName = "harvester-containerd-registry"
	ContainerdRegistryFileName   = "registries.yaml"

	BackupTargetSecretName      = "harvester-backup-target-secret"
	InternalTLSSecretName       = "tls-rancher-internal"
	Rke2IngressNginxAppName     = "rke2-ingress-nginx"
	CattleSystemNamespaceName   = "cattle-system"
	LonghornSystemNamespaceName = "longhorn-system"
	LonghornDefaultManagerURL   = "http://longhorn-backend.longhorn-system:9500/v1"
	KubeSystemNamespace         = "kube-system"
	FleetLocalNamespaceName     = "fleet-local"
	LocalClusterName            = "local"

	HTTPProxyEnv  = "HTTP_PROXY"
	HTTPSProxyEnv = "HTTPS_PROXY"
	NoProxyEnv    = "NO_PROXY"

	LonghornOptionBackingImageName = "backingImage"
	LonghornOptionMigratable       = "migratable"
	AddonValuesAnnotation          = "harvesterhci.io/addon-defaults"
)
