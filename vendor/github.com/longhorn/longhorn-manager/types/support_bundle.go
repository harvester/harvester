package types

import (
	"time"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	SupportBundleNameFmt = "support-bundle-%v"

	SupportBundleManagerApp      = "support-bundle-manager"
	SupportBundleManagerLabelKey = "rancher/supportbundle"

	SupportBundleURLPort        = 8080
	SupportBundleURLStatusFmt   = "http://%s:%v/status"
	SupportBundleURLDownloadFmt = "http://%s:%v/bundle"

	SupportBundleDownloadTimeout = 24 * time.Hour
)

func IsSupportBundleControllerDeleting(supportBundle *longhorn.SupportBundle) bool {
	switch supportBundle.Status.State {
	case longhorn.SupportBundleStatePurging,
		longhorn.SupportBundleStateDeleting,
		longhorn.SupportBundleStateReplaced:
		return true
	}
	return false
}
