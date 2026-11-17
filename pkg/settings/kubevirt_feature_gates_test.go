package settings

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseKubeVirtFeatureGates(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		expected  []string
		expectErr string
	}{
		{name: "empty value", value: "", expected: []string{}},
		{name: "only separators and spaces", value: " , ,", expected: []string{}},
		{name: "alpha gates", value: "IncrementalBackup,VSOCK", expected: []string{"IncrementalBackup", "VSOCK"}},
		{name: "beta gates with spaces", value: " Snapshot , ImageVolume ", expected: []string{"Snapshot", "ImageVolume"}},
		{name: "duplicate gate", value: "VSOCK,VSOCK", expectErr: "listed more than once"},
		{name: "unknown gate", value: "NotAGate", expectErr: "not a known KubeVirt feature gate"},
		{name: "GA gate", value: "ExpandDisks", expectErr: "generally available"},
		{name: "deprecated gate", value: "HotplugVolumes", expectErr: "deprecated"},
		{name: "discontinued gate", value: "Macvtap", expectErr: "discontinued"},
		{name: "deny-listed gate", value: "ExternalNetResourceInjection", expectErr: "not supported by Harvester"},
		{name: "persistent reservation", value: "PersistentReservation", expectErr: "persistentReservationConfiguration"},
		{name: "valid gate followed by invalid gate", value: "VSOCK,Root", expectErr: "not supported by Harvester"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gates, err := ParseKubeVirtFeatureGates(tc.value)
			if tc.expectErr != "" {
				assert.ErrorContains(t, err, tc.expectErr)
				assert.Nil(t, gates)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, gates)
		})
	}
}
