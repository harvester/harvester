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
	AnnotationUpgradePatched       = prefix + "/upgrade-patched"
	AnnotationImageID              = prefix + "/imageId"
	AnnotationReservedMemory       = prefix + "/reservedMemory"
	AnnotationHash                 = prefix + "/hash"
	AnnotationRunStrategy          = prefix + "/vmRunStrategy"
	AnnotationSnapshotFreezeFS     = prefix + "/snapshotFreezeFS"
	AnnotationVMIEventRecords      = prefix + "/vmiEventRecords"
	LabelImageDisplayName          = prefix + "/imageDisplayName"
	LabelSetting                   = prefix + "/setting"
	LabelVMName                    = prefix + "/vmName"

	AnnotationStorageClassName          = prefix + "/storageClassName"
	AnnotationStorageProvisioner        = prefix + "/storageProvisioner"
	AnnotationIsDefaultStorageClassName = "storageclass.kubernetes.io/is-default-class"

	// AnnotationMigratingPrefix is used to store the migrating vm in the annotation of ResourceQuota
	// eg: harvesterhci.io/migrating-vm1: jsonOfResourceList, harvesterhci.io/migrating-vm2: jsonOfResourceList
	AnnotationMigratingPrefix = prefix + "/migrating-"

	// AnnotationInsufficientResourceQuota is indicated the resource is insufficient of Namespace
	AnnotationInsufficientResourceQuota = prefix + "/insufficient-resource-quota"

	AnnotationDefaultUserdataSecret = prefix + "/default-userdata-secret"

	// Add to rancher-monitoring addon to record grafana pv name
	AnnotationGrafanaPVName = prefix + "/grafana-pv-name"

	ContainerdRegistrySecretName = "harvester-containerd-registry"
	ContainerdRegistryFileName   = "registries.yaml"

	BackupTargetSecretName              = "harvester-backup-target-secret"
	InternalTLSSecretName               = "tls-rancher-internal"
	Rke2IngressNginxAppName             = "rke2-ingress-nginx"
	CattleSystemNamespaceName           = "cattle-system"
	CattleMonitoringSystemNamespace     = "cattle-monitoring-system"
	LonghornSystemNamespaceName         = "longhorn-system"
	LonghornDefaultManagerURL           = "http://longhorn-backend.longhorn-system:9500/v1"
	KubeSystemNamespace                 = "kube-system"
	FleetLocalNamespaceName             = "fleet-local"
	LocalClusterName                    = "local"
	HarvesterSystemNamespaceName        = "harvester-system"
	RancherLoggingName                  = "rancher-logging"
	RancherMonitoringPrometheus         = "rancher-monitoring-prometheus"
	RancherMonitoringAlertmanager       = "rancher-monitoring-alertmanager"
	RancherMonitoring                   = "rancher-monitoring"
	RancherMonitoringGrafana            = "rancher-monitoring-grafana"
	CattleLoggingSystemNamespaceName    = "cattle-logging-system"
	HarvesterUpgradeImageRepository     = "rancher/harvester-upgrade"
	HarvesterLonghornStorageClassName   = "harvester-longhorn"
	GrafanaPVCName                      = "rancher-monitoring-grafana"
	RancherMonitoringName               = "rancher-monitoring"
	CattleMonitoringSystemNamespaceName = "cattle-monitoring-system"
	HarvesterVMImportController         = "vm-import-controller-harvester-vm-import-controller"

	HTTPProxyEnv  = "HTTP_PROXY"
	HTTPSProxyEnv = "HTTPS_PROXY"
	NoProxyEnv    = "NO_PROXY"

	LonghornOptionBackingImageName = "backingImage"
	LonghornOptionMigratable       = "migratable"
	AddonValuesAnnotation          = "harvesterhci.io/addon-defaults"

	LabelUpgradeReadMessage          = prefix + "/read-message"
	LabelUpgradeState                = prefix + "/upgradeState"
	UpgradeStateLoggingInfraPrepared = "LoggingInfraPrepared"

	AnnotationArchiveName         = prefix + "/archiveName"
	LabelUpgradeLog               = prefix + "/upgradeLog"
	LabelUpgradeLogComponent      = prefix + "/upgradeLogComponent"
	UpgradeLogInfraComponent      = "infra"
	UpgradeLogShipperComponent    = "shipper"
	UpgradeLogAggregatorComponent = "aggregator"
	UpgradeLogDownloaderComponent = "downloader"
	UpgradeLogFlowComponent       = "clusterflow"
	UpgradeLogArchiveComponent    = "log-archive"
	UpgradeLogOperatorComponent   = "operator"
	UpgradeLogOutputComponent     = "clusteroutput"
	UpgradeLogPackagerComponent   = "packager"

	UpgradeNodeDrainTaintKey   = "kubevirt.io/drain"
	UpgradeNodeDrainTaintValue = "draining"

	FieldCattlePrefix             = "field.cattle.io"
	CattleAnnotationResourceQuota = FieldCattlePrefix + "/resourceQuota"

	ManagementCattlePrefix                   = "management.cattle.io"
	LabelManagementDefaultResourceQuota      = "resourcequota." + ManagementCattlePrefix + "/default-resource-quota"
	DefaultFleetControllerConfigMapName      = "fleet-controller"
	DefaultFleetControllerConfigMapNamespace = "cattle-fleet-system"
	RancherInternalCASetting                 = "internal-cacerts"
	RancherInternalServerURLSetting          = "internal-server-url"
	APIServerURLKey                          = "apiServerURL"
	APIServerCAKey                           = "apiServerCA"
)
