package builder

import (
	"testing"

	"github.com/stretchr/testify/assert"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

func TestClockOffsetUTC(t *testing.T) {
	builder := NewVMBuilder("test clock offset UTC")

	builder.ClockOffsetUTC(0)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.ClockOffset)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.ClockOffset.UTC)
	assert.Equal(t, 0, *builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.ClockOffset.UTC.OffsetSeconds)
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.ClockOffset.Timezone)
}

func TestClockOffsetTimezone(t *testing.T) {
	builder := NewVMBuilder("test clock offset timezone")

	builder.ClockOffsetTimezone("Europe/Berlin")
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.ClockOffset)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.ClockOffset.Timezone)
	assert.Equal(t, kubevirtv1.ClockOffsetTimezone("Europe/Berlin"), *builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.ClockOffset.Timezone)
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.ClockOffset.UTC)
}

func TestClockTimerHPET(t *testing.T) {
	builder := NewVMBuilder("test clock HPET timer")

	builder.ClockTimerHPET(true, "catchup")
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.HPET)
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.KVM)
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.PIT)
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.RTC)
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.HPET.Enabled)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.HPET.Enabled)
	assert.Equal(t, kubevirtv1.HPETTickPolicy("catchup"), builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.HPET.TickPolicy)
}

func TestClockTimerKVM(t *testing.T) {
	builder := NewVMBuilder("test clock KVM timer")

	builder.ClockTimerKVM(true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer)
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.HPET)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.KVM)
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.PIT)
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.RTC)
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.KVM.Enabled)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.KVM.Enabled)
}

func TestClockTimerPIT(t *testing.T) {
	builder := NewVMBuilder("test clock PIT timer")

	builder.ClockTimerPIT(true, "discard")
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer)
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.HPET)
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.KVM)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.PIT)
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.RTC)
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.PIT.Enabled)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.PIT.Enabled)
	assert.Equal(t, kubevirtv1.PITTickPolicy("discard"), builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.PIT.TickPolicy)
}

func TestClockTimerRTC(t *testing.T) {
	builder := NewVMBuilder("test clock RTC timer")

	builder.ClockTimerRTC(true, "discard", "wall")
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer)
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.HPET)
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.KVM)
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.PIT)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.RTC)
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.RTC.Enabled)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.RTC.Enabled)
	assert.Equal(t, kubevirtv1.RTCTickPolicy("discard"), builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.RTC.TickPolicy)
	assert.Equal(t, kubevirtv1.TrackWall, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.RTC.Track)
}

func TestClockTimerHyperV(t *testing.T) {
	builder := NewVMBuilder("test clock HyperV timer")

	builder.ClockTimerHyperV(true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer)
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.HPET)
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.KVM)
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.PIT)
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.RTC)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.Hyperv.Enabled)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.Hyperv.Enabled)
}

// Multiple timers can be attached to a single VM.
func TestClockTimerKVMAndRTCCombined(t *testing.T) {
	builder := NewVMBuilder("test clock KVM and RTC timers combined")

	builder.ClockTimerKVM(true).ClockTimerRTC(true, "delay", "guest")
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer)
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.HPET)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.KVM)
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.PIT)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.RTC)
	assert.Nil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.KVM.Enabled)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.KVM.Enabled)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.RTC.Enabled)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.RTC.Enabled)
	assert.Equal(t, kubevirtv1.RTCTickPolicy("delay"), builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.RTC.TickPolicy)
	assert.Equal(t, kubevirtv1.TrackGuest, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.RTC.Track)
}

func TestClockOffsetUTCCombined(t *testing.T) {
	builder := NewVMBuilder("test clock offset UTC and a timer")

	builder.ClockOffsetUTC(0).ClockTimerHyperV(true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.ClockOffset)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.ClockOffset.UTC)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.Hyperv)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.Hyperv.Enabled)
	assert.Equal(t, 0, *builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.ClockOffset.UTC.OffsetSeconds)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.Hyperv.Enabled)
}

func TestClockOffsetTimezoneCombined(t *testing.T) {
	builder := NewVMBuilder("test clock offset timezone and a timer")

	builder.ClockOffsetTimezone("Europe/Berlin").ClockTimerKVM(true)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.ClockOffset)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.ClockOffset.Timezone)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.KVM)
	assert.NotNil(t, builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.KVM.Enabled)
	assert.Equal(t, kubevirtv1.ClockOffsetTimezone("Europe/Berlin"), *builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.ClockOffset.Timezone)
	assert.Equal(t, true, *builder.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer.KVM.Enabled)
}
