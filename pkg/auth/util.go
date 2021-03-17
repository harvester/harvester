package auth

import (
	"github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester/pkg/settings"
)

func IsRancherAuthMode() bool {
	return settings.AuthenticationMode.Get() == string(v1alpha1.Rancher)
}
