package settings

import (
	"fmt"
	"strings"

	kvfeaturegate "kubevirt.io/kubevirt/pkg/virt-config/featuregate"
)

// KubeVirtFeatureGatesDenyList contains KubeVirt feature gates that must not be
// enabled through the kubevirt-feature-gates setting because they conflict
// with how Harvester deploys and runs KubeVirt.
//   - ExternalNetResourceInjection: virt-controller stops reading
//     NetworkAttachmentDefinitions, which breaks Harvester VM networks.
//   - Root: runs virt-launcher as root.
//   - ConfigurableHypervisor: hypervisors other than KVM are not supported.
//   - CrossArchitectureVirtualization: emulated guests of another CPU
//     architecture are not supported.
var KubeVirtFeatureGatesDenyList = map[string]struct{}{
	"ExternalNetResourceInjection":    {},
	"Root":                            {},
	"ConfigurableHypervisor":          {},
	"CrossArchitectureVirtualization": {},
}

// splitKubeVirtFeatureGates splits a comma-separated kubevirt-feature-gates
// value into trimmed, non-empty gate names, keeping their order.
func splitKubeVirtFeatureGates(value string) []string {
	names := []string{}
	for _, raw := range strings.Split(value, ",") {
		if name := strings.TrimSpace(raw); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// checkKubeVirtFeatureGate returns why a gate cannot be enabled through the
// setting, or nil when it is an Alpha or Beta gate of the vendored KubeVirt.
func checkKubeVirtFeatureGate(name string) error {
	if _, ok := KubeVirtFeatureGatesDenyList[name]; ok {
		return fmt.Errorf("feature gate %q is not supported by Harvester", name)
	}
	// KubeVirt marks PersistentReservation as GA, but the pr-helper is only
	// deployed when spec.configuration.persistentReservationConfiguration is
	// enabled (or the gate name is listed literally), so it is not on by
	// default. It is configured through its own field, not as a feature gate.
	if name == kvfeaturegate.PersistentReservation {
		return fmt.Errorf("feature gate %q is not managed by this setting, it is enabled through spec.configuration.persistentReservationConfiguration of the KubeVirt CR", name)
	}
	info := kvfeaturegate.FeatureGateInfo(name)
	if info == nil {
		return fmt.Errorf("feature gate %q is not a known KubeVirt feature gate", name)
	}
	switch info.State {
	case kvfeaturegate.Alpha, kvfeaturegate.Beta:
		return nil
	case kvfeaturegate.GA:
		return fmt.Errorf("feature gate %q is generally available in KubeVirt and is always enabled, please remove it", name)
	default:
		return fmt.Errorf("feature gate %q is %s in KubeVirt and cannot be enabled, please remove it", name, strings.ToLower(string(info.State)))
	}
}

// ValidateKubeVirtFeatureGates validates a new kubevirt-feature-gates value
// against the previous one. Every gate that is added must be an Alpha or Beta
// gate of the vendored KubeVirt version, and no gate can be listed twice.
//
// Gates that were already in the previous value are not checked again, except
// against the deny list and PersistentReservation. A gate that was Beta when
// it was set can be GA, deprecated, discontinued, or removed after a Harvester
// upgrade bumps KubeVirt, and that must not block later edits of the setting.
// Such gates are ignored by KubeVirtFeatureGatesToApply.
func ValidateKubeVirtFeatureGates(oldValue, newValue string) error {
	previous := map[string]struct{}{}
	for _, name := range splitKubeVirtFeatureGates(oldValue) {
		previous[name] = struct{}{}
	}

	seen := map[string]struct{}{}
	for _, name := range splitKubeVirtFeatureGates(newValue) {
		if _, ok := seen[name]; ok {
			return fmt.Errorf("feature gate %q is listed more than once", name)
		}
		seen[name] = struct{}{}

		err := checkKubeVirtFeatureGate(name)
		if err == nil {
			continue
		}
		_, denied := KubeVirtFeatureGatesDenyList[name]
		if _, carried := previous[name]; carried && !denied && name != kvfeaturegate.PersistentReservation {
			continue
		}
		return err
	}
	return nil
}

// KubeVirtFeatureGatesToApply returns the gates of a kubevirt-feature-gates
// value that must be written to the KubeVirt CR: Alpha and Beta gates of the
// vendored KubeVirt version, plus deprecated gates (they still work when listed),
// without duplicates. Gates that cannot be enabled are returned separately in
// ignored. This happens when a gate became GA (it is always on) or discontinued,
// or was removed in the KubeVirt version shipped by a newer Harvester release.
func KubeVirtFeatureGatesToApply(value string) (gates []string, ignored map[string]error) {
	gates = []string{}
	ignored = map[string]error{}
	seen := map[string]struct{}{}
	for _, name := range splitKubeVirtFeatureGates(value) {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		if err := checkKubeVirtFeatureGate(name); err != nil {
			// A deprecated gate still works when it is listed, so keep a gate
			// that was set earlier and became deprecated after an upgrade.
			// Adding it again is rejected by ValidateKubeVirtFeatureGates.
			if info := kvfeaturegate.FeatureGateInfo(name); info == nil || info.State != kvfeaturegate.Deprecated ||
				name == kvfeaturegate.PersistentReservation {
				ignored[name] = err
				continue
			}
			if _, denied := KubeVirtFeatureGatesDenyList[name]; denied {
				ignored[name] = err
				continue
			}
		}
		gates = append(gates, name)
	}
	return gates, ignored
}
