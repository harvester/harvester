package util

const (
	prefix                         = "harvesterhci.io"
	RemovedPVCsAnnotationKey       = prefix + "/removedPersistentVolumeClaims"
	AnnotationMigrationTarget      = prefix + "/migrationTargetNodeName"
	AnnotationMigrationUID         = prefix + "/migrationUID"
	AnnotationMigrationState       = prefix + "/migrationState"
	AnnotationTimestamp            = prefix + "/timestamp"
	AnnotationVolumeClaimTemplates = prefix + "/volumeClaimTemplates"
	AnnotationImageID              = prefix + "/imageId"
	AnnotationHash                 = prefix + "/hash"
	AnnotationSettingFromConfig    = prefix + "/preconfigValue"

	BackupTargetSecretName      = "harvester-backup-target-secret"
	LonghornSystemNamespaceName = "longhorn-system"
)
