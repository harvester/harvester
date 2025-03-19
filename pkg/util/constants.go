package util

const (
	prefix                              = "harvesterhci.io"
	RemovedPVCsAnnotationKey            = prefix + "/removedPersistentVolumeClaims"
	AdditionalCASecretName              = "harvester-additional-ca"
	AdditionalCAFileName                = "additional-ca.pem"
	AnnotationMigrationTarget           = prefix + "/migrationTargetNodeName"
	AnnotationMigrationUID              = prefix + "/migrationUID"
	AnnotationMigrationState            = prefix + "/migrationState"
	AnnotationTimestamp                 = prefix + "/timestamp"
	AnnotationVolumeClaimTemplates      = prefix + "/volumeClaimTemplates"
	AnnotationUpgradePatched            = prefix + "/upgrade-patched"
	AnnotationImageID                   = prefix + "/imageId"
	AnnotationReservedMemory            = prefix + "/reservedMemory"
	AnnotationHash                      = prefix + "/hash"
	AnnotationRunStrategy               = prefix + "/vmRunStrategy"
	AnnotationSnapshotFreezeFS          = prefix + "/snapshotFreezeFS"
	AnnotationSnapshotRevise            = prefix + "/snapRevise"
	AnnotationSVMBackupID               = prefix + "/svmbackupId"
	AnnotationSVMBackupSkipCronCheck    = prefix + "/svmbackupSkipCronCheck"
	AnnotationGoldenImage               = prefix + "/goldenImage"
	AnnotationVolForVM                  = prefix + "/volumeForVirtualMachine"
	LabelImageDisplayName               = prefix + "/imageDisplayName"
	LabelSetting                        = prefix + "/setting"
	LabelVMName                         = prefix + "/vmName"
	LabelSVMBackupUID                   = prefix + "/svmbackupUID"
	LabelSVMBackupTimestamp             = prefix + "/svmbackupTimestamp"
	LabelVMCreator                      = prefix + "/creator"
	LabelNodeNameKey                    = "kubevirt.io/nodeName"
	AnnotationStorageClassName          = prefix + "/storageClassName"
	AnnotationStorageProvisioner        = prefix + "/storageProvisioner"
	AnnotationIsDefaultStorageClassName = "storageclass.kubernetes.io/is-default-class"
	AnnotationLastRefreshTime           = prefix + "/lastRefreshTime"

	AnnotationSkipRancherLoggingAddonWebhookCheck = prefix + "/skipRancherLoggingAddonWebhookCheck"

	// AnnotationSkipResourceQuotaAutoScaling is used to disable to resourcequota auto scaling
	AnnotationSkipResourceQuotaAutoScaling = prefix + "/skipResourceQuotaAutoScaling"

	// AnnotationMigratingNamePrefix is used to store the migrating vm in the annotation of ResourceQuota
	// eg: harvesterhci.io/migrating-vm1: jsonOfResourceList, harvesterhci.io/migrating-vm2: jsonOfResourceList
	// will be removed after v1.5.0
	AnnotationMigratingNamePrefix = prefix + "/migrating-"

	// replace AnnotationMigratingNamePrefix from v1.5.0
	AnnotationMigratingUIDPrefix = prefix + "/migratingUID-"

	// AnnotationMigratingPrefix is replaced by AnnotationMigratingNamePrefix, and is kept for compatibility
	AnnotationMigratingPrefix = AnnotationMigratingNamePrefix

	// AnnotationInsufficientResourceQuota is indicated the resource is insufficient of Namespace
	AnnotationInsufficientResourceQuota = prefix + "/insufficient-resource-quota"

	AnnotationDefaultUserdataSecret = prefix + "/default-userdata-secret"

	// Add to rancher-monitoring addon to record grafana pv name
	AnnotationGrafanaPVName = prefix + "/grafana-pv-name"

	// Add to harvester-longhorn storageclass to protect it
	// For any storageclass created & protected by controller, the controller can utilize this annotation
	AnnotationIsReservedStorageClass = prefix + "/is-reserved-storageclass"

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
	GrafanaPVCName                      = "rancher-monitoring-grafana"
	RancherMonitoringName               = "rancher-monitoring"
	CattleMonitoringSystemNamespaceName = "cattle-monitoring-system"
	HarvesterVMImportController         = "vm-import-controller-harvester-vm-import-controller"
	// kubevirt create a CRD object automatically: type kubevirt, name kubevirt, namespace: harvester-system
	// this object stores all kubevirt related configuration
	KubeVirtObjectName = "kubevirt"

	HTTPProxyEnv  = "HTTP_PROXY"
	HTTPSProxyEnv = "HTTPS_PROXY"
	NoProxyEnv    = "NO_PROXY"

	LonghornOptionBackingImageName           = "backingImage"
	LonghornOptionBackingImageDataSourceName = "backingImageDataSourceType"
	LonghornOptionMigratable                 = "migratable"
	LonghornOptionEncrypted                  = "encrypted"
	AddonValuesAnnotation                    = "harvesterhci.io/addon-defaults"

	// CDI constants
	CSIProvisionerLVM      = "lvm.driver.harvesterhci.io"
	CSIProvisionerLonghorn = "driver.longhorn.io"
	KindVolumeImportSource = "VolumeImportSource"
	ImportSourceFSBlank    = "filesystem-blank-source"
	LVMTopologyNodeKey     = "topology.lvm.csi/node"

	// CSI constants
	CSIProvisionerSecretNameKey      = "csi.storage.k8s.io/provisioner-secret-name"
	CSIProvisionerSecretNamespaceKey = "csi.storage.k8s.io/provisioner-secret-namespace"
	CSINodePublishSecretNameKey      = "csi.storage.k8s.io/node-publish-secret-name"
	CSINodePublishSecretNamespaceKey = "csi.storage.k8s.io/node-publish-secret-namespace"
	CSINodeStageSecretNameKey        = "csi.storage.k8s.io/node-stage-secret-name"
	CSINodeStageSecretNamespaceKey   = "csi.storage.k8s.io/node-stage-secret-namespace"

	LabelUpgradeReadMessage          = prefix + "/read-message"
	LabelUpgradeState                = prefix + "/upgradeState"
	UpgradeStateLoggingInfraPrepared = "LoggingInfraPrepared"

	AnnotationArchiveName                 = prefix + "/archiveName"
	LabelUpgradeLog                       = prefix + "/upgradeLog"
	LabelUpgradeLogComponent              = prefix + "/upgradeLogComponent"
	UpgradeLogInfraComponent              = "infra"
	UpgradeLogFluentbitAgentComponent     = "agent"
	UpgradeLogShipperComponent            = "shipper"
	UpgradeLogAggregatorComponent         = "aggregator"
	UpgradeLogDownloaderComponent         = "downloader"
	UpgradeLogFlowComponent               = "clusterflow"
	UpgradeLogArchiveComponent            = "log-archive"
	UpgradeLogOperatorComponent           = "operator"
	UpgradeLogOutputComponent             = "clusteroutput"
	UpgradeLogPackagerComponent           = "packager"
	AnnotationUpgradeLogLogArchiveAltName = prefix + "/logArchiveAltName"
	UpgradeLogLoggingOperatorSource       = prefix + "/loggingOperatorSource" // the source of logging operator: addon or managedchart

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

	RKEControlPlaneRoleLabel = "rke.cattle.io/control-plane-role"

	LabelMaintainModeStrategy                          = prefix + "/maintain-mode-strategy"
	AnnotationMaintainModeStrategyNodeName             = prefix + "/maintain-mode-strategy-node-name"
	MaintainModeStrategyMigrate                        = "Migrate"
	MaintainModeStrategyShutdownAndRestartAfterEnable  = "ShutdownAndRestartAfterEnable"
	MaintainModeStrategyShutdownAndRestartAfterDisable = "ShutdownAndRestartAfterDisable"
	MaintainModeStrategyShutdown                       = "Shutdown"

	// s3 backup target constants
	AWSAccessKey       = "AWS_ACCESS_KEY_ID"
	AWSSecretKey       = "AWS_SECRET_ACCESS_KEY"
	AWSEndpoints       = "AWS_ENDPOINTS"
	AWSCERT            = "AWS_CERT"
	VirtualHostedStyle = "VIRTUAL_HOSTED_STYLE"

	DefaultResourceQuotaName = "default-resource-quota"

	AnnotationCPUManagerUpdateStatus = prefix + "/cpu-manager-update-status"
	LabelCPUManagerUpdateNode        = prefix + "/cpu-manager-update-node"
	LabelCPUManagerUpdatePolicy      = prefix + "/cpu-manager-update-policy"
	LabelCPUManagerExitCode          = prefix + "/cpu-manager-exit-code"

	VClusterNamespace          = "rancher-vcluster"
	LablelVClusterAppNameKey   = "app"
	LablelVClusterAppNameValue = "vcluster"

	StorageClassHarvesterLonghorn = "harvester-longhorn" // the initial & default storageclass
	HarvesterChartReleaseName     = "harvester"          // the release name

	// copied from helm pkg/action/validate.go
	HelmReleaseNameAnnotation      = "meta.helm.sh/release-name"
	HelmReleaseNamespaceAnnotation = "meta.helm.sh/release-namespace"

	// moved from nodedrain_controller for public usage
	ContainerDiskOrCDRomKey             = "CDRomOrContainerDiskPresent"
	NodeSchedulingRequirementsNotMetKey = "NodeSchedulingRequirementsNotMet"
	MaintainModeStrategyKey             = "MaintainModeStrategy"
	LastHealthyReplicaKey               = "LastHealthyReplica"

	// moved from storage_network for public usage
	StorageNetworkAnnotation        = "storage-network.settings.harvesterhci.io"
	ReplicaStorageNetworkAnnotation = StorageNetworkAnnotation + "/replica"
	PausedStorageNetworkAnnotation  = StorageNetworkAnnotation + "/paused"
	HashStorageNetworkAnnotation    = StorageNetworkAnnotation + "/hash"
	NadStorageNetworkAnnotation     = StorageNetworkAnnotation + "/net-attach-def"
	OldNadStorageNetworkAnnotation  = StorageNetworkAnnotation + "/old-net-attach-def"

	HashStorageNetworkLabel = HashStorageNetworkAnnotation

	StorageNetworkNetAttachDefPrefix    = "storagenetwork-"
	StorageNetworkNetAttachDefNamespace = HarvesterSystemNamespaceName
)
