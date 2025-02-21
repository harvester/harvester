package featuregates

import (
	"context"

	"github.com/pkg/errors"

	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"

	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"kubevirt.io/containerized-data-importer/pkg/common"
)

const (
	// HonorWaitForFirstConsumer - if enabled will not schedule worker pods on a storage with WaitForFirstConsumer binding mode
	HonorWaitForFirstConsumer = "HonorWaitForFirstConsumer"

	// DataVolumeClaimAdoption - if enabled will allow PVC to be adopted by a DataVolume
	// it is not an error if PVC of sam name exists before DataVolume is created
	DataVolumeClaimAdoption = "DataVolumeClaimAdoption"

	// WebhookPvcRendering - if enabled will deploy PVC mutating webhook for PVC rendering instead of the DV controller
	WebhookPvcRendering = "WebhookPvcRendering"
)

// FeatureGates is a util for determining whether an optional feature is enabled or not.
type FeatureGates interface {
	// HonorWaitForFirstConsumerEnabled - see the HonorWaitForFirstConsumer const
	HonorWaitForFirstConsumerEnabled() (bool, error)

	// ClaimAdoptionEnabled - see the DataVolumeClaimAdoption const
	ClaimAdoptionEnabled() (bool, error)

	// WebhookPvcRenderingEnabled - see the WebhookPvcRendering const
	WebhookPvcRenderingEnabled() (bool, error)
}

// CDIConfigFeatureGates is a util for determining whether an optional feature is enabled or not.
type CDIConfigFeatureGates struct {
	client client.Client
}

// NewFeatureGates creates a new instance of the feature gates
func NewFeatureGates(c client.Client) *CDIConfigFeatureGates {
	return &CDIConfigFeatureGates{client: c}
}

func (f *CDIConfigFeatureGates) isFeatureGateEnabled(featureGate string) (bool, error) {
	featureGates, err := f.getConfig()
	if err != nil {
		return false, errors.Wrap(err, "error getting CDIConfig")
	}

	for _, fg := range featureGates {
		if fg == featureGate {
			return true, nil
		}
	}
	return false, nil
}

func (f *CDIConfigFeatureGates) getConfig() ([]string, error) {
	config := &cdiv1.CDIConfig{}
	if err := f.client.Get(context.TODO(), types.NamespacedName{Name: common.ConfigName}, config); err != nil {
		return nil, err
	}

	return config.Spec.FeatureGates, nil
}

// HonorWaitForFirstConsumerEnabled - see the HonorWaitForFirstConsumer const
func (f *CDIConfigFeatureGates) HonorWaitForFirstConsumerEnabled() (bool, error) {
	return f.isFeatureGateEnabled(HonorWaitForFirstConsumer)
}

// ClaimAdoptionEnabled - see the DataVolumeClaimAdoption const
func (f *CDIConfigFeatureGates) ClaimAdoptionEnabled() (bool, error) {
	return f.isFeatureGateEnabled(DataVolumeClaimAdoption)
}

// WebhookPvcRenderingEnabled tells if webhook PVC rendering is enabled
func (f *CDIConfigFeatureGates) WebhookPvcRenderingEnabled() (bool, error) {
	return f.isFeatureGateEnabled(WebhookPvcRendering)
}

// IsWebhookPvcRenderingEnabled tells if webhook PVC rendering is enabled
func IsWebhookPvcRenderingEnabled(c client.Client) (bool, error) {
	gates := NewFeatureGates(c)
	return gates.WebhookPvcRenderingEnabled()
}
