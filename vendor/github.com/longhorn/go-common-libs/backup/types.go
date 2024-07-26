package backup

type LonghornBackupMode string

const (
	LonghornBackupParameterBackupMode = "backup-mode"

	LonghornBackupModeFull        = LonghornBackupMode("full")
	LonghornBackupModeIncremental = LonghornBackupMode("incremental")
)

const (
	LonghornBackupBackingImageParameterSecret          = "secret"
	LonghornBackupBackingImageParameterSecretNamespace = "secret-namespace"
)
