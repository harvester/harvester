package builder

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFeatureACPI(t *testing.T) {
	builder := NewVMBuilder("test ACPI")
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)

	builder.FeatureACPI(true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.ACPI)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.ACPI.Enabled)

	builder.FeatureACPI(false)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.ACPI)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.ACPI.Enabled)
}

func TestFeatureAPIC(t *testing.T) {
	builder := NewVMBuilder("test APIC")
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)

	builder.FeatureAPIC(true, true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.APIC)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.APIC.FeatureState.Enabled)
	assert.Equal(t, true, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.APIC.EndOfInterrupt)

	builder.FeatureAPIC(true, false)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.APIC)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.APIC.FeatureState.Enabled)
	assert.Equal(t, false, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.APIC.EndOfInterrupt)

	builder.FeatureAPIC(false, true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.APIC)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.APIC.FeatureState.Enabled)
	assert.Equal(t, true, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.APIC.EndOfInterrupt)

	builder.FeatureAPIC(false, false)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.APIC)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.APIC.FeatureState.Enabled)
	assert.Equal(t, false, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.APIC.EndOfInterrupt)
}

func TestFeatureHyperVPassthrough(t *testing.T) {
	builder := NewVMBuilder("test HyperV Passthrough")
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)

	builder.FeatureHyperVPassthrough(true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.HypervPassthrough)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.HypervPassthrough.Enabled)

	builder.FeatureHyperVPassthrough(false)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.HypervPassthrough)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.HypervPassthrough.Enabled)
}

func TestFeatureHyperVRelaxed(t *testing.T) {
	builder := NewVMBuilder("test HyperV Relaxed")
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)

	builder.FeatureHyperVRelaxed(true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Relaxed)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Relaxed.Enabled)

	builder.FeatureHyperVRelaxed(false)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Relaxed)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Relaxed.Enabled)
}

func TestFeatureHyperVVAPIC(t *testing.T) {
	builder := NewVMBuilder("test HyperV VAPIC")
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)

	builder.FeatureHyperVVAPIC(true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.VAPIC)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.VAPIC.Enabled)

	builder.FeatureHyperVVAPIC(false)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.VAPIC)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.VAPIC.Enabled)
}

func TestFeatureHyperVSpinlocks(t *testing.T) {
	builder := NewVMBuilder("test HyperV Spinlocks")
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)

	builder.FeatureHyperVSpinlocks(true, uint32(8192))
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Spinlocks)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Spinlocks.FeatureState.Enabled)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Spinlocks.FeatureState.Enabled)
	assert.Equal(t, uint32(8192), *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Spinlocks.Retries)

	builder.FeatureHyperVSpinlocks(false, uint32(4096))
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Spinlocks)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Spinlocks.FeatureState.Enabled)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Spinlocks.FeatureState.Enabled)
	assert.Equal(t, uint32(4096), *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Spinlocks.Retries)
}

func TestFeatureHyperVVPIndex(t *testing.T) {
	builder := NewVMBuilder("test HyperV VPIndex")
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)

	builder.FeatureHyperVVPIndex(true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.VPIndex)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.VPIndex.Enabled)

	builder.FeatureHyperVVPIndex(false)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.VPIndex)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.VPIndex.Enabled)
}

func TestFeatureHyperVRuntime(t *testing.T) {
	builder := NewVMBuilder("test HyperV Runtime")
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)

	builder.FeatureHyperVRuntime(true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Runtime)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Runtime.Enabled)

	builder.FeatureHyperVRuntime(false)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Runtime)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Runtime.Enabled)
}

func TestFeatureHyperVSyNIC(t *testing.T) {
	builder := NewVMBuilder("test HyperV SyNIC")
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)

	builder.FeatureHyperVSyNIC(true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.SyNIC)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.SyNIC.Enabled)

	builder.FeatureHyperVSyNIC(false)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.SyNIC)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.SyNIC.Enabled)
}

func TestFeatureHyperVSyNICTimer(t *testing.T) {
	builder := NewVMBuilder("test HyperV SyNIC Timer")
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)

	builder.FeatureHyperVSyNICTimer(true, true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.SyNICTimer)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.SyNICTimer.Direct)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.SyNICTimer.FeatureState.Enabled)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.SyNICTimer.Direct.Enabled)

	builder.FeatureHyperVSyNICTimer(true, false)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.SyNICTimer)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.SyNICTimer.Direct)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.SyNICTimer.FeatureState.Enabled)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.SyNICTimer.Direct.Enabled)

	builder.FeatureHyperVSyNICTimer(false, true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.SyNICTimer)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.SyNICTimer.Direct)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.SyNICTimer.FeatureState.Enabled)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.SyNICTimer.Direct.Enabled)

	builder.FeatureHyperVSyNICTimer(false, false)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.SyNICTimer)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.SyNICTimer.Direct)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.SyNICTimer.FeatureState.Enabled)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.SyNICTimer.Direct.Enabled)
}

func TestFeatureHyperVReset(t *testing.T) {
	builder := NewVMBuilder("test HyperV Reset")
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)

	builder.FeatureHyperVReset(true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Reset)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Reset.Enabled)

	builder.FeatureHyperVReset(false)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Reset)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Reset.Enabled)
}

func TestFeatureHyperVVendorID(t *testing.T) {
	builder := NewVMBuilder("test HyperV Vendor ID")
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)

	builder.FeatureHyperVVendorID(true, "foobar")
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.VendorID)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.VendorID.FeatureState.Enabled)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.VendorID.FeatureState.Enabled)
	assert.Equal(t, "foobar", builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.VendorID.VendorID)

	builder.FeatureHyperVVendorID(false, "barfoo")
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.VendorID)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.VendorID.FeatureState.Enabled)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.VendorID.FeatureState.Enabled)
	assert.Equal(t, "barfoo", builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.VendorID.VendorID)
}

func TestFeatureHyperVFrequencies(t *testing.T) {
	builder := NewVMBuilder("test HyperV Frequencies")
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)

	builder.FeatureHyperVFrequencies(true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Frequencies)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Frequencies.Enabled)

	builder.FeatureHyperVFrequencies(false)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Frequencies)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Frequencies.Enabled)
}

func TestFeatureHyperVReenlightenment(t *testing.T) {
	builder := NewVMBuilder("test HyperV Reenlightenment")
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)

	builder.FeatureHyperVReenlightenment(true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Reenlightenment)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Reenlightenment.Enabled)

	builder.FeatureHyperVReenlightenment(false)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Reenlightenment)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.Reenlightenment.Enabled)
}

func TestFeatureHyperVTLBFlush(t *testing.T) {
	builder := NewVMBuilder("test HyperV TLB Flush")
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)

	builder.FeatureHyperVTLBFlush(true, true, true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Direct)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Extended)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.FeatureState.Enabled)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Direct.Enabled)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Extended.Enabled)

	builder.FeatureHyperVTLBFlush(true, true, false)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Direct)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Extended)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.FeatureState.Enabled)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Direct.Enabled)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Extended.Enabled)

	builder.FeatureHyperVTLBFlush(true, false, true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Direct)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Extended)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.FeatureState.Enabled)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Direct.Enabled)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Extended.Enabled)

	builder.FeatureHyperVTLBFlush(true, false, false)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Direct)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Extended)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.FeatureState.Enabled)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Direct.Enabled)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Extended.Enabled)

	builder.FeatureHyperVTLBFlush(false, true, true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Direct)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Extended)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.FeatureState.Enabled)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Direct.Enabled)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Extended.Enabled)

	builder.FeatureHyperVTLBFlush(false, true, false)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Direct)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Extended)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.FeatureState.Enabled)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Direct.Enabled)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Extended.Enabled)

	builder.FeatureHyperVTLBFlush(false, false, true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Direct)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Extended)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.FeatureState.Enabled)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Direct.Enabled)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Extended.Enabled)

	builder.FeatureHyperVTLBFlush(false, false, false)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Direct)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Extended)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.FeatureState.Enabled)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Direct.Enabled)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.TLBFlush.Extended.Enabled)
}

func TestFeatureHyperVIPI(t *testing.T) {
	builder := NewVMBuilder("test HyperV IPI")
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)

	builder.FeatureHyperVIPI(true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.IPI)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.IPI.Enabled)

	builder.FeatureHyperVIPI(false)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.IPI)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.IPI.Enabled)
}

func TestFeatureHyperVEVMCS(t *testing.T) {
	builder := NewVMBuilder("test HyperV EVMCS")
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)

	builder.FeatureHyperVEVMCS(true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.EVMCS)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.EVMCS.Enabled)

	builder.FeatureHyperVEVMCS(false)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.EVMCS)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Hyperv.EVMCS.Enabled)
}

func TestFeatureSMM(t *testing.T) {
	builder := NewVMBuilder("test PVSpinlock")
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)

	builder.FeatureSMM(true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.SMM)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.SMM.Enabled)

	builder.FeatureSMM(false)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.SMM)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.SMM.Enabled)
}

func TestFeatureKVM(t *testing.T) {
	builder := NewVMBuilder("test KVM")
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)

	builder.FeatureKVM(true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.KVM)
	assert.Equal(t, true, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.KVM.Hidden)

	builder.FeatureKVM(false)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.KVM)
	assert.Equal(t, false, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.KVM.Hidden)
}

func TestFeaturePVSpinlock(t *testing.T) {
	builder := NewVMBuilder("test PVSpinlock")
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)

	builder.FeaturePVSpinlock(true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Pvspinlock)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Pvspinlock.Enabled)

	builder.FeaturePVSpinlock(false)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Pvspinlock)
	assert.Equal(t, false, *builder.VirtualMachine.Spec.Template.Spec.Domain.Features.Pvspinlock.Enabled)
}
