package backup

type LonghornBackupMode string

const (
	LonghornBackupParameterBackupMode = "backup-mode"

	LonghornBackupModeFull        = LonghornBackupMode("full")
	LonghornBackupModeIncremental = LonghornBackupMode("incremental")
)
