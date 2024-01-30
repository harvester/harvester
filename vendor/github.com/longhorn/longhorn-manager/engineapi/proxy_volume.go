package engineapi

import (
	"fmt"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

func (p *Proxy) VolumeGet(e *longhorn.Engine) (volume *Volume, err error) {
	recv, err := p.grpcClient.VolumeGet(string(e.Spec.BackendStoreDriver), e.Name, e.Spec.VolumeName, p.DirectToURL(e))
	if err != nil {
		return nil, err
	}

	return (*Volume)(recv), nil
}

func (p *Proxy) VolumeExpand(e *longhorn.Engine) (err error) {
	return p.grpcClient.VolumeExpand(string(e.Spec.BackendStoreDriver), e.Name, e.Spec.VolumeName, p.DirectToURL(e),
		e.Spec.VolumeSize)
}

func (p *Proxy) VolumeFrontendStart(e *longhorn.Engine) (err error) {
	frontendName, err := GetEngineInstanceFrontend(e.Spec.BackendStoreDriver, e.Spec.Frontend)
	if err != nil {
		return err
	}

	if frontendName == "" {
		return fmt.Errorf("cannot start empty frontend")
	}

	return p.grpcClient.VolumeFrontendStart(string(e.Spec.BackendStoreDriver), e.Name, e.Spec.VolumeName,
		p.DirectToURL(e), frontendName)
}

func (p *Proxy) VolumeFrontendShutdown(e *longhorn.Engine) (err error) {
	return p.grpcClient.VolumeFrontendShutdown(string(e.Spec.BackendStoreDriver), e.Name, e.Spec.VolumeName,
		p.DirectToURL(e))
}

func (p *Proxy) VolumeUnmapMarkSnapChainRemovedSet(e *longhorn.Engine) error {
	return p.grpcClient.VolumeUnmapMarkSnapChainRemovedSet(string(e.Spec.BackendStoreDriver), e.Name, e.Spec.VolumeName,
		p.DirectToURL(e), e.Spec.UnmapMarkSnapChainRemovedEnabled)
}
