package network

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateBridgeConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config

		wantBridge   string
		wantVlan     int
		wantRange    string
		wantExclude  []string
		wantIPRanges []RangeConfiguration

		jsonContains    []string
		jsonNotContains []string
	}{
		{
			name: "single-stack uses legacy flat range/exclude path",
			cfg: Config{
				ClusterNetwork: "mgmt",
				Range:          "10.0.0.0/24",
				Exclude:        []string{"10.0.0.1/32"},
			},
			wantBridge:      "mgmt" + BridgeSuffix,
			wantVlan:        DefaultPVID,
			wantRange:       "10.0.0.0/24",
			wantExclude:     []string{"10.0.0.1/32"},
			jsonContains:    []string{`"range":"10.0.0.0/24"`},
			jsonNotContains: []string{`"ipRanges"`},
		},
		{
			name: "single-stack without excludes omits exclude field",
			cfg: Config{
				ClusterNetwork: "mgmt",
				Range:          "192.168.0.0/16",
			},
			wantBridge:      "mgmt" + BridgeSuffix,
			wantVlan:        DefaultPVID,
			wantRange:       "192.168.0.0/16",
			jsonNotContains: []string{`"exclude"`, `"ipRanges"`},
		},
		{
			name: "dual-stack uses ipRanges path and omits flat range/exclude",
			cfg: Config{
				ClusterNetwork: "mgmt",
				Range:          "10.0.0.0/24",
				Exclude:        []string{"10.0.0.1/32"},
				RangeV6:        "fd00::/120",
				ExcludeV6:      []string{"fd00::1/128"},
			},
			wantBridge: "mgmt" + BridgeSuffix,
			wantVlan:   DefaultPVID,
			wantIPRanges: []RangeConfiguration{
				{Range: "10.0.0.0/24", Exclude: []string{"10.0.0.1/32"}},
				{Range: "fd00::/120", Exclude: []string{"fd00::1/128"}},
			},
			jsonContains:    []string{`"ipRanges"`},
			jsonNotContains: []string{`"ipam":{"type":"whereabouts","range"`},
		},
		{
			name: "dual-stack without excludes emits ipRanges with no exclude fields",
			cfg: Config{
				ClusterNetwork: "mgmt",
				Range:          "10.1.0.0/24",
				RangeV6:        "fd01::/112",
			},
			wantBridge: "mgmt" + BridgeSuffix,
			wantVlan:   DefaultPVID,
			wantIPRanges: []RangeConfiguration{
				{Range: "10.1.0.0/24"},
				{Range: "fd01::/112"},
			},
			jsonNotContains: []string{`"exclude"`},
		},
		{
			name:       "bridge name is derived from ClusterNetwork",
			cfg:        Config{ClusterNetwork: "storage", Range: "10.2.0.0/24"},
			wantBridge: "storage" + BridgeSuffix,
			wantVlan:   DefaultPVID,
			wantRange:  "10.2.0.0/24",
		},
		{
			name:      "vlan defaults to DefaultPVID when zero",
			cfg:       Config{Range: "10.3.0.0/24"},
			wantVlan:  DefaultPVID,
			wantRange: "10.3.0.0/24",
		},
		{
			name:      "explicit vlan is preserved",
			cfg:       Config{Range: "10.4.0.0/24", Vlan: 100},
			wantVlan:  100,
			wantRange: "10.4.0.0/24",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bc := CreateBridgeConfig(tc.cfg)

			if tc.wantBridge != "" {
				assert.Equal(t, tc.wantBridge, bc.Bridge)
			}
			assert.Equal(t, tc.wantVlan, bc.Vlan)
			assert.Equal(t, tc.wantRange, bc.IPAM.Range)
			if len(tc.wantExclude) > 0 {
				assert.Equal(t, tc.wantExclude, bc.IPAM.Exclude)
			} else {
				assert.Empty(t, bc.IPAM.Exclude)
			}
			if len(tc.wantIPRanges) > 0 {
				assert.Equal(t, tc.wantIPRanges, bc.IPAM.IPRanges)
			} else {
				assert.Empty(t, bc.IPAM.IPRanges)
			}

			data, err := json.Marshal(bc)
			assert.NoError(t, err)
			for _, s := range tc.jsonContains {
				assert.Contains(t, string(data), s)
			}
			for _, s := range tc.jsonNotContains {
				assert.NotContains(t, string(data), s)
			}
		})
	}
}
