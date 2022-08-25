package controller

const (
	EventReasonCreate         = "Create"
	EventReasonFailedCreating = "FailedCreating"
	EventReasonDelete         = "Delete"
	EventReasonFailedDeleting = "FailedDeleting"
	EventReasonStart          = "Start"
	EventReasonFailedStarting = "FailedStarting"
	EventReasonStop           = "Stop"
	EventReasonFailedStopping = "FailedStopping"
	EventReasonUpdate         = "Update"

	EventReasonRebuilt          = "Rebuilt"
	EventReasonRebuilding       = "Rebuilding"
	EventReasonFailedRebuilding = "FailedRebuilding"

	EventReasonVolumeCloneCompleted = "VolumeCloneCompleted"
	EventReasonVolumeCloneInitiated = "VolumeCloneInitiated"
	EventReasonVolumeCloneFailed    = "VolumeCloneFailed"

	EventReasonFailedStartingSnapshotPurge = "FailedStartingSnapshotPurge"
	EventReasonTimeoutSnapshotPurge        = "TimeoutSnapshotPurge"
	EventReasonFailedSnapshotPurge         = "FailedSnapshotPurge"

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
	EventReasonSyncing  = "Syncing"
)
