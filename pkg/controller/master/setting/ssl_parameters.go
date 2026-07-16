package setting

import (
	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

func (h *Handler) syncSSLParameters(setting *harvesterv1.Setting) error {
	// ssl parameters is deprecated in v1.9.x due to switch to traefik
	// no actual operation is performed if user configures ssl parameters setting
	return nil
}
