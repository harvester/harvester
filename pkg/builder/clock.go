package builder

import (
	kubevirtv1 "kubevirt.io/api/core/v1"
)

// ClockOffsetUTC configures the UTC offset
// If an offset is specified, guest changes to the clock will be kept during reboots and are not
// reset.
func (v *VMBuilder) ClockOffsetUTC(seconds int) *VMBuilder {
	v.ensureClock()
	clock := v.VirtualMachine.Spec.Template.Spec.Domain.Clock
	clock.ClockOffset = kubevirtv1.ClockOffset{
		UTC: &kubevirtv1.ClockOffsetUTC{OffsetSeconds: new(seconds)},
	}
	return v
}

// ClockOffsetTimezone sets the guest clock to the specified timezone.
// Zone name follows the TZ environment variable format (e.g. 'America/New_York').
func (v *VMBuilder) ClockOffsetTimezone(timezone string) *VMBuilder {
	v.ensureClock()
	clock := v.VirtualMachine.Spec.Template.Spec.Domain.Clock
	clock.ClockOffset = kubevirtv1.ClockOffset{
		Timezone: new(kubevirtv1.ClockOffsetTimezone(timezone)),
	}
	return v
}

// Attach a Timer to the VM
// HPET (High Precision Event Timer) - multiple timers with periodic interrupts.
//
// Enabled set to false makes sure that the machine type or a preset can't add the timer.
// Defaults to true.
//
// Policy determines what happens when QEMU misses a deadline for injecting a tick to the guest.
// One of "delay", "catchup", "merge", "discard".
func (v *VMBuilder) ClockTimerHPET(enabled bool, policy string) *VMBuilder {
	v.ensureClockTimer()
	timer := v.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer
	timer.HPET = &kubevirtv1.HPETTimer{
		Enabled:    new(enabled),
		TickPolicy: kubevirtv1.HPETTickPolicy(policy),
	}
	return v
}

// Attach a Timer to the VM
// KVM 	(KVM clock) - lets guests read the host’s wall clock time (paravirtualized).
// For linux guests.
func (v *VMBuilder) ClockTimerKVM(enabled bool) *VMBuilder {
	v.ensureClockTimer()
	timer := v.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer
	timer.KVM = &kubevirtv1.KVMTimer{
		Enabled: new(enabled),
	}
	return v
}

// Attach a Timer to the VM
// PIT (Programmable Interval Timer) - a timer with periodic interrupts.
//
// Enabled set to false makes sure that the machine type or a preset can't add the timer.
// Defaults to true.
//
// Policy determines what happens when QEMU misses a deadline for injecting a tick to the guest.
// One of "delay", "catchup", "discard".
func (v *VMBuilder) ClockTimerPIT(enabled bool, policy string) *VMBuilder {
	v.ensureClockTimer()
	timer := v.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer
	timer.PIT = &kubevirtv1.PITTimer{
		Enabled:    new(enabled),
		TickPolicy: kubevirtv1.PITTickPolicy(policy),
	}
	return v
}

// Attach a Timer to the VM
// RTC (Real Time Clock) - a continuously running timer with periodic interrupts.
//
// Enabled set to false makes sure that the machine type or a preset can't add the timer.
// Defaults to true.
//
// Policy determines what happens when QEMU misses a deadline for injecting a tick to the guest.
// One of "delay", "catchup".
//
// Track the guest or the wall clock.
func (v *VMBuilder) ClockTimerRTC(enabled bool, policy, track string) *VMBuilder {
	v.ensureClockTimer()
	timer := v.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer
	timer.RTC = &kubevirtv1.RTCTimer{
		Enabled:    new(enabled),
		TickPolicy: kubevirtv1.RTCTickPolicy(policy),
		Track:      kubevirtv1.RTCTimerTrack(track),
	}
	return v
}

// Attach a Timer to the VM
// Hyperv (Hypervclock) - lets guests read the host’s wall clock time (paravirtualized).
// For windows guests.
func (v *VMBuilder) ClockTimerHyperV(enabled bool) *VMBuilder {
	v.ensureClockTimer()
	timer := v.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer
	timer.Hyperv = &kubevirtv1.HypervTimer{
		Enabled: new(enabled),
	}
	return v
}

// - - - Helpers

func (v *VMBuilder) ensureClock() {
	if v.VirtualMachine.Spec.Template.Spec.Domain.Clock == nil {
		v.VirtualMachine.Spec.Template.Spec.Domain.Clock = &kubevirtv1.Clock{}
	}
}

func (v *VMBuilder) ensureClockTimer() {
	v.ensureClock()
	if v.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer == nil {
		v.VirtualMachine.Spec.Template.Spec.Domain.Clock.Timer = &kubevirtv1.Timer{}
	}
}
