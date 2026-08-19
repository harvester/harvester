package builder

import (
	"k8s.io/utils/ptr"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

// - - - VM Features - - -

// enable/disable ACPI feature for VM.
// See also:
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_features
func (v *VMBuilder) FeatureACPI(enabled bool) *VMBuilder {
	v.ensureFeatures()
	features := v.VirtualMachine.Spec.Template.Spec.Domain.Features
	features.ACPI = kubevirtv1.FeatureState{Enabled: ptr.To(enabled)}
	return v
}

// enable/disable APIC feature for VM.
// See also:
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_features
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_featureapic
func (v *VMBuilder) FeatureAPIC(enabled, endOfInterrupt bool) *VMBuilder {
	v.ensureFeatures()
	features := v.VirtualMachine.Spec.Template.Spec.Domain.Features
	features.APIC = &kubevirtv1.FeatureAPIC{
		FeatureState: kubevirtv1.FeatureState{
			Enabled: ptr.To(enabled),
		},
		EndOfInterrupt: endOfInterrupt,
	}
	return v
}

// enable/disable HyperV Passthrough feature for VM.
// See also:
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_features
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_hypervpassthrough
func (v *VMBuilder) FeatureHyperVPassthrough(enabled bool) *VMBuilder {
	v.ensureFeatures()
	features := v.VirtualMachine.Spec.Template.Spec.Domain.Features
	features.HypervPassthrough = &kubevirtv1.HyperVPassthrough{Enabled: ptr.To(enabled)}
	return v
}

// enable/disable HyperV Relaxed feature for VM.
// See also:
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_features
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_featurehyperv
func (v *VMBuilder) FeatureHyperVRelaxed(enabled bool) *VMBuilder {
	v.ensureFeatureHyperV()
	hyperv := v.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv
	hyperv.Relaxed = &kubevirtv1.FeatureState{Enabled: ptr.To(enabled)}
	return v
}

// enable/disable HyperV VAPIC feature for VM.
// See also:
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_features
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_featurehyperv
func (v *VMBuilder) FeatureHyperVVAPIC(enabled bool) *VMBuilder {
	v.ensureFeatureHyperV()
	hyperv := v.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv
	hyperv.VAPIC = &kubevirtv1.FeatureState{Enabled: ptr.To(enabled)}
	return v
}

// enable/disable and configure HyperV Spinlocks feature for VM.
// See also:
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_features
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_featurehyperv
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_featurespinlocks
func (v *VMBuilder) FeatureHyperVSpinlocks(enabled bool, retries uint32) *VMBuilder {
	v.ensureFeatureHyperV()
	hyperv := v.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv
	hyperv.Spinlocks = &kubevirtv1.FeatureSpinlocks{
		FeatureState: kubevirtv1.FeatureState{
			Enabled: ptr.To(enabled),
		},
		Retries: ptr.To(max(uint32(4096), retries)), // Retries value must be at least 4096
	}
	return v
}

// enable/disable HyperV VP Index feature for VM.
// See also:
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_features
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_featurehyperv
func (v *VMBuilder) FeatureHyperVVPIndex(enabled bool) *VMBuilder {
	v.ensureFeatureHyperV()
	hyperv := v.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv
	hyperv.VPIndex = &kubevirtv1.FeatureState{Enabled: ptr.To(enabled)}
	return v
}

// enable/disable HyperV Runtime feature for VM.
// See also:
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_features
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_featurehyperv
func (v *VMBuilder) FeatureHyperVRuntime(enabled bool) *VMBuilder {
	v.ensureFeatureHyperV()
	hyperv := v.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv
	hyperv.Runtime = &kubevirtv1.FeatureState{Enabled: ptr.To(enabled)}
	return v
}

// enable/disable HyperV SyNIC feature for VM.
// See also:
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_features
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_featurehyperv
func (v *VMBuilder) FeatureHyperVSyNIC(enabled bool) *VMBuilder {
	v.ensureFeatureHyperV()
	hyperv := v.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv
	hyperv.SyNIC = &kubevirtv1.FeatureState{Enabled: ptr.To(enabled)}
	return v
}

// enable/disable and configure HyperV SyNIC Timer feature for VM.
// See also:
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_features
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_featurehyperv
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_synictimer
func (v *VMBuilder) FeatureHyperVSyNICTimer(enabled, direct bool) *VMBuilder {
	v.ensureFeatureHyperV()
	hyperv := v.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv
	hyperv.SyNICTimer = &kubevirtv1.SyNICTimer{
		FeatureState: kubevirtv1.FeatureState{
			Enabled: ptr.To(enabled),
		},
		Direct: &kubevirtv1.FeatureState{
			Enabled: ptr.To(direct),
		},
	}
	return v
}

// enable/disable HyperV Reset feature for VM.
// See also:
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_features
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_featurehyperv
func (v *VMBuilder) FeatureHyperVReset(enabled bool) *VMBuilder {
	v.ensureFeatureHyperV()
	hyperv := v.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv
	hyperv.Reset = &kubevirtv1.FeatureState{Enabled: ptr.To(enabled)}
	return v
}

// enable/disable and configure HyperV Vendor ID feature for VM.
// See also:
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_features
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_featurehyperv
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_featurevendorid
func (v *VMBuilder) FeatureHyperVVendorID(enabled bool, vendorid string) *VMBuilder {
	v.ensureFeatureHyperV()
	hyperv := v.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv
	hyperv.VendorID = &kubevirtv1.FeatureVendorID{
		FeatureState: kubevirtv1.FeatureState{
			Enabled: ptr.To(enabled),
		},
		VendorID: vendorid,
	}
	return v
}

// enable/disable HyperV Frequencies feature for VM.
// See also:
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_features
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_featurehyperv
func (v *VMBuilder) FeatureHyperVFrequencies(enabled bool) *VMBuilder {
	v.ensureFeatureHyperV()
	hyperv := v.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv
	hyperv.Frequencies = &kubevirtv1.FeatureState{Enabled: ptr.To(enabled)}
	return v
}

// enable/disable HyperV Reenlightenment feature for VM.
// See also:
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_features
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_featurehyperv
func (v *VMBuilder) FeatureHyperVReenlightenment(enabled bool) *VMBuilder {
	v.ensureFeatureHyperV()
	hyperv := v.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv
	hyperv.Reenlightenment = &kubevirtv1.FeatureState{Enabled: ptr.To(enabled)}
	return v
}

// enable/disable and configure HyperV TLB Flush feature for VM.
// See also:
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_features
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_featurehyperv
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_tlbflush
func (v *VMBuilder) FeatureHyperVTLBFlush(enabled, direct, extended bool) *VMBuilder {
	v.ensureFeatureHyperV()
	hyperv := v.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv
	hyperv.TLBFlush = &kubevirtv1.TLBFlush{
		FeatureState: kubevirtv1.FeatureState{
			Enabled: ptr.To(enabled),
		},
		Direct: &kubevirtv1.FeatureState{
			Enabled: ptr.To(direct),
		},
		Extended: &kubevirtv1.FeatureState{
			Enabled: ptr.To(extended),
		},
	}
	return v
}

// enable/disable HyperV IPI feature for VM.
// See also:
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_features
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_featurehyperv
func (v *VMBuilder) FeatureHyperVIPI(enabled bool) *VMBuilder {
	v.ensureFeatureHyperV()
	hyperv := v.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv
	hyperv.IPI = &kubevirtv1.FeatureState{Enabled: ptr.To(enabled)}
	return v
}

// enable/disable HyperV EVMCS feature for VM.
// See also:
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_features
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_featurehyperv
func (v *VMBuilder) FeatureHyperVEVMCS(enabled bool) *VMBuilder {
	v.ensureFeatureHyperV()
	hyperv := v.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv
	hyperv.EVMCS = &kubevirtv1.FeatureState{Enabled: ptr.To(enabled)}
	return v
}

// enable/disable SMM feature for VM.
// See also:
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_features
func (v *VMBuilder) FeatureSMM(enabled bool) *VMBuilder {
	v.ensureFeatures()
	features := v.VirtualMachine.Spec.Template.Spec.Domain.Features
	features.SMM = &kubevirtv1.FeatureState{Enabled: ptr.To(enabled)}
	return v
}

// enable/disable KVM feature for VM.
// See also:
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_features
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_featurekvm
func (v *VMBuilder) FeatureKVM(hidden bool) *VMBuilder {
	v.ensureFeatures()
	features := v.VirtualMachine.Spec.Template.Spec.Domain.Features
	features.KVM = &kubevirtv1.FeatureKVM{Hidden: hidden}
	return v
}

// enable/disable paravirtualized spinlocks feature for VM.
// See also:
// https://kubevirt.io/api-reference/v1.8.3/definitions.html#_v1_features
func (v *VMBuilder) FeaturePVSpinlock(enabled bool) *VMBuilder {
	v.ensureFeatures()
	features := v.VirtualMachine.Spec.Template.Spec.Domain.Features
	features.Pvspinlock = &kubevirtv1.FeatureState{Enabled: ptr.To(enabled)}
	return v
}

// - - - helper functions - - -

// ensureFeatures creates an empty kubevirtv1.Features struct if necessary
func (v *VMBuilder) ensureFeatures() {
	if v.VirtualMachine.Spec.Template.Spec.Domain.Features == nil {
		v.VirtualMachine.Spec.Template.Spec.Domain.Features = &kubevirtv1.Features{}
	}
}

// ensureFeatureHyperV creates an empty kubevirtv1.Features struct if necessary and populates the
// Hyerpv property of that struct with en empty kubevirtv1.FeatureHyperv struct if necessary
func (v *VMBuilder) ensureFeatureHyperV() {
	v.ensureFeatures()
	if v.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv == nil {
		v.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv = &kubevirtv1.FeatureHyperv{}
	}
}
