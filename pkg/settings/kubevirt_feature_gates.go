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

// ParseKubeVirtFeatureGates parses the kubevirt-feature-gates setting value, a
// comma-separated list of KubeVirt feature gate names, and validates every name
// against the feature gate registry of the vendored KubeVirt version.
// Only Alpha and Beta gates can be enabled: GA gates are always enabled, and
// deprecated or discontinued gates are rejected.
// An empty value returns an empty list.
func ParseKubeVirtFeatureGates(value string) ([]string, error) {
	gates := []string{}
	seen := map[string]struct{}{}
	for _, raw := range strings.Split(value, ",") {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("feature gate %q is listed more than once", name)
		}
		seen[name] = struct{}{}

		if _, ok := KubeVirtFeatureGatesDenyList[name]; ok {
			return nil, fmt.Errorf("feature gate %q is not supported by Harvester", name)
		}

		// KubeVirt marks PersistentReservation as GA, but the pr-helper is only
		// deployed when spec.configuration.persistentReservationConfiguration is
		// enabled (or the gate name is listed literally), so it is not on by
		// default. It is configured through its own field, not as a feature gate.
		if name == kvfeaturegate.PersistentReservation {
			return nil, fmt.Errorf("feature gate %q is not managed by this setting, it is enabled through spec.configuration.persistentReservationConfiguration of the KubeVirt CR", name)
		}

		info := kvfeaturegate.FeatureGateInfo(name)
		if info == nil {
			return nil, fmt.Errorf("feature gate %q is not a known KubeVirt feature gate", name)
		}
		switch info.State {
		case kvfeaturegate.Alpha, kvfeaturegate.Beta:
		case kvfeaturegate.GA:
			return nil, fmt.Errorf("feature gate %q is generally available in KubeVirt and is always enabled, please remove it", name)
		default:
			return nil, fmt.Errorf("feature gate %q is %s in KubeVirt and cannot be enabled, please remove it", name, strings.ToLower(string(info.State)))
		}
		gates = append(gates, name)
	}
	return gates, nil
}
