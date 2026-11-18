package settings

import (
	"testing"

	"github.com/stretchr/testify/assert"
	kvfeaturegate "kubevirt.io/kubevirt/pkg/virt-config/featuregate"
)

func TestValidateKubeVirtFeatureGates(t *testing.T) {
	tests := []struct {
		name      string
		oldValue  string
		newValue  string
		expectErr string
	}{
		{name: "empty value", newValue: ""},
		{name: "only separators and spaces", newValue: " , ,"},
		{name: "alpha gates", newValue: "IncrementalBackup,VSOCK"},
		{name: "beta gates with spaces", newValue: " Snapshot , ImageVolume "},
		{name: "duplicate gate", newValue: "VSOCK,VSOCK", expectErr: "listed more than once"},
		{name: "unknown gate", newValue: "NotAGate", expectErr: "not a known KubeVirt feature gate"},
		{name: "GA gate", newValue: "ExpandDisks", expectErr: "generally available"},
		{name: "deprecated gate", newValue: "HotplugVolumes", expectErr: "deprecated"},
		{name: "discontinued gate", newValue: "Macvtap", expectErr: "discontinued"},
		{name: "deny-listed gate", newValue: "ExternalNetResourceInjection", expectErr: "not supported by Harvester"},
		{name: "persistent reservation", newValue: "PersistentReservation", expectErr: "persistentReservationConfiguration"},
		{name: "valid gate followed by invalid gate", newValue: "VSOCK,Root", expectErr: "not supported by Harvester"},
		{name: "carried-over GA gate is allowed", oldValue: "ExpandDisks", newValue: "ExpandDisks,VSOCK"},
		{name: "carried-over unknown gate is allowed", oldValue: "RemovedGate", newValue: "RemovedGate,VSOCK"},
		{name: "carried-over discontinued gate is allowed", oldValue: "Macvtap", newValue: "VSOCK,Macvtap"},
		{name: "carried-over gate is still checked for duplicates", oldValue: "ExpandDisks", newValue: "ExpandDisks,ExpandDisks", expectErr: "listed more than once"},
		{name: "carried-over deny-listed gate is rejected", oldValue: "Root", newValue: "Root", expectErr: "not supported by Harvester"},
		{name: "carried-over persistent reservation is rejected", oldValue: "PersistentReservation", newValue: "PersistentReservation", expectErr: "persistentReservationConfiguration"},
		{name: "a GA gate added next to carried-over gates is rejected", oldValue: "VSOCK", newValue: "VSOCK,ExpandDisks", expectErr: "generally available"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateKubeVirtFeatureGates(tc.oldValue, tc.newValue)
			if tc.expectErr != "" {
				assert.ErrorContains(t, err, tc.expectErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestKubeVirtFeatureGatesToApply(t *testing.T) {
	tests := []struct {
		name          string
		value         string
		expected      []string
		expectIgnored []string
	}{
		{name: "empty value", value: "", expected: []string{}},
		{name: "alpha and beta gates keep their order", value: " Snapshot, VSOCK ", expected: []string{"Snapshot", "VSOCK"}},
		{name: "duplicates are removed", value: "VSOCK,VSOCK", expected: []string{"VSOCK"}},
		{name: "GA gate is ignored", value: "ExpandDisks,VSOCK", expected: []string{"VSOCK"}, expectIgnored: []string{"ExpandDisks"}},
		{name: "deprecated gate is still applied", value: "HotplugVolumes", expected: []string{"HotplugVolumes"}},
		{name: "discontinued gate is ignored", value: "Macvtap", expected: []string{}, expectIgnored: []string{"Macvtap"}},
		{name: "unknown gate is ignored", value: "RemovedGate,Snapshot", expected: []string{"Snapshot"}, expectIgnored: []string{"RemovedGate"}},
		{name: "deny-listed gate is ignored", value: "Root", expected: []string{}, expectIgnored: []string{"Root"}},
		{name: "persistent reservation is ignored", value: "PersistentReservation", expected: []string{}, expectIgnored: []string{"PersistentReservation"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gates, ignored := KubeVirtFeatureGatesToApply(tc.value)
			assert.Equal(t, tc.expected, gates)
			ignoredNames := []string{}
			for name := range ignored {
				ignoredNames = append(ignoredNames, name)
			}
			assert.ElementsMatch(t, tc.expectIgnored, ignoredNames)
		})
	}
}

// TestKubeVirtFeatureGateGraduatesAfterUpgrade simulates a gate that is Beta
// when the setting is written and changes state in the KubeVirt version of a
// later Harvester release.
func TestKubeVirtFeatureGateGraduatesAfterUpgrade(t *testing.T) {
	const gate = "HarvesterTestGraduatingGate"
	t.Cleanup(func() { kvfeaturegate.UnregisterFeatureGate(gate) })

	// Before the upgrade the gate is Beta: it can be added and is applied.
	kvfeaturegate.RegisterFeatureGate(kvfeaturegate.FeatureGate{Name: gate, State: kvfeaturegate.Beta})
	stored := gate + ",VSOCK"
	assert.NoError(t, ValidateKubeVirtFeatureGates("", stored))
	gates, ignored := KubeVirtFeatureGatesToApply(stored)
	assert.Equal(t, []string{gate, "VSOCK"}, gates)
	assert.Empty(t, ignored)

	for _, state := range []kvfeaturegate.State{kvfeaturegate.GA, kvfeaturegate.Discontinued} {
		t.Run(string(state), func(t *testing.T) {
			kvfeaturegate.RegisterFeatureGate(kvfeaturegate.FeatureGate{Name: gate, State: state})

			// The stored value still syncs: the gate is skipped, VSOCK is applied.
			gates, ignored := KubeVirtFeatureGatesToApply(stored)
			assert.Equal(t, []string{"VSOCK"}, gates)
			assert.Contains(t, ignored, gate)

			// Editing the setting is not blocked by the carried-over gate.
			assert.NoError(t, ValidateKubeVirtFeatureGates(stored, stored+",Snapshot"))
			assert.NoError(t, ValidateKubeVirtFeatureGates(stored, "VSOCK"))

			// Adding the gate again from an empty value is rejected.
			assert.Error(t, ValidateKubeVirtFeatureGates("", gate))
		})
	}

	t.Run("Deprecated", func(t *testing.T) {
		kvfeaturegate.RegisterFeatureGate(kvfeaturegate.FeatureGate{Name: gate, State: kvfeaturegate.Deprecated})

		// A deprecated gate still works, so it stays applied.
		gates, ignored := KubeVirtFeatureGatesToApply(stored)
		assert.Equal(t, []string{gate, "VSOCK"}, gates)
		assert.Empty(t, ignored)
		assert.NoError(t, ValidateKubeVirtFeatureGates(stored, stored+",Snapshot"))
		assert.ErrorContains(t, ValidateKubeVirtFeatureGates("", gate), "deprecated")
	})
}
