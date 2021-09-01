// shared types for support bundle controller and manager
package types

import "time"

const (
	StateNone       = ""
	StateGenerating = "generating"
	StateError      = "error"
	StateReady      = "ready"

	// labels
	SupportBundleLabelKey = "rancher/supportbundle"
	DrainKey              = "kubevirt.io/drain"

	AppManager = "support-bundle-manager"
	AppAgent   = "support-bundle-agent"

	BundleCreationTimeout = 8 * time.Minute
	NodeBundleWaitTimeout = "5m"
)

type ManagerStatus struct {
	// phase to collect bundle
	Phase string

	// fail to collect bundle
	Error bool

	// error message
	ErrorMessage string

	// progress of the bundle collecting. 0 - 100.
	Progress int

	// bundle filename
	Filename string

	// bundle filesize
	Filesize int64
}
