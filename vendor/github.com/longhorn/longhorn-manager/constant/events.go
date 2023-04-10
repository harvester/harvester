package constant

const (
	EventReasonCreate            = "Create"
	EventReasonFailedCreatingFmt = "FailedCreating: %v %v"
	EventReasonCreated           = "Created"
	EventReasonDelete            = "Delete"
	EventReasonDeleting          = "Deleting"
	EventReasonFailedDeleting    = "FailedDeleting"
	EventReasonStart             = "Start"
	EventReasonFailedStarting    = "FailedStarting"
	EventReasonStop              = "Stop"
	EventReasonFailedStopping    = "FailedStopping"
	EventReasonUpdate            = "Update"

	EventReasonRebuilt          = "Rebuilt"
	EventReasonRebuilding       = "Rebuilding"
	EventReasonFailedRebuilding = "FailedRebuilding"

	EventReasonVolumeCloneCompleted = "VolumeCloneCompleted"
	EventReasonVolumeCloneInitiated = "VolumeCloneInitiated"
	EventReasonVolumeCloneFailed    = "VolumeCloneFailed"

	EventReasonFailedStartingSnapshotPurge = "FailedStartingSnapshotPurge"
	EventReasonTimeoutSnapshotPurge        = "TimeoutSnapshotPurge"
	EventReasonFailedSnapshotPurge         = "FailedSnapshotPurge"

	EventReasonRestored      = "Restored"
	EventReasonRestoredFmt   = "Restored %v"
	EventReasonFailedRestore = "FailedRestore"

	EventReasonFailedExpansion    = "FailedExpansion"
	EventReasonSucceededExpansion = "SucceededExpansion"
	EventReasonCanceledExpansion  = "CanceledExpansion"

	EventReasonAttached       = "Attached"
	EventReasonDetached       = "Detached"
	EventReasonHealthy        = "Healthy"
	EventReasonFaulted        = "Faulted"
	EventReasonDegraded       = "Degraded"
	EventReasonOrphaned       = "Orphaned"
	EventReasonUnknown        = "Unknown"
	EventReasonFailedEviction = "FailedEviction"

	EventReasonDetachedUnexpectly = "DetachedUnexpectly"
	EventReasonRemount            = "Remount"
	EventReasonAutoSalvaged       = "AutoSalvaged"

	EventReasonFetching = "Fetching"
	EventReasonFetched  = "Fetched"

	EventReasonSyncing = "Syncing"
	EventReasonSynced  = "Synced"

	EventReasonFailedSnapshotDataIntegrityCheck = "FailedSnapshotDataIntegrityCheck"

	EventReasonFailed   = "Failed"
	EventReasonReady    = "Ready"
	EventReasonUploaded = "Uploaded"

	EventReasonRolloutSkippedFmt = "RolloutSkipped: %v %v"
)
