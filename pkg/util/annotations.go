package util

const (
	prefix                          = "harvester.cattle.io"
	RemovedDataVolumesAnnotationKey = prefix + "/removedDataVolumes"
	AnnotationMigrationTarget       = prefix + "/migrationTargetNodeName"
	AnnotationMigrationUID          = prefix + "/migrationUID"
	AnnotationMigrationState        = prefix + "/migrationState"
	AnnotationTimestamp             = prefix + "/timestamp"
)
