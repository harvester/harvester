package network

import (
	"encoding/json"
	"net/netip"
	"strings"
	"testing"

	whereaboutsv1alpha1 "github.com/k8snetworkplumbingwg/whereabouts/pkg/api/whereabouts.cni.cncf.io/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssignHostIPs(t *testing.T) {
	tests := []struct {
		name           string
		hostIPRange    string
		current        map[string]string
		nodes          []string
		wantAssigned   map[string]string
		wantUnassigned []string
		wantErr        bool
	}{
		{
			name:        "assigns lowest addresses in node name order",
			hostIPRange: "172.16.0.240/29",
			nodes:       []string{"node-c", "node-a", "node-b"},
			wantAssigned: map[string]string{
				"node-a": "172.16.0.240",
				"node-b": "172.16.0.241",
				"node-c": "172.16.0.242",
			},
		},
		{
			name:        "keeps existing bindings and fills the lowest gap",
			hostIPRange: "172.16.0.240/29",
			current: map[string]string{
				"node-b": "172.16.0.240",
				"node-c": "172.16.0.242",
			},
			nodes: []string{"node-a", "node-b", "node-c"},
			wantAssigned: map[string]string{
				"node-a": "172.16.0.241",
				"node-b": "172.16.0.240",
				"node-c": "172.16.0.242",
			},
		},
		{
			name:        "releases addresses of removed nodes for reuse",
			hostIPRange: "172.16.0.240/31",
			current: map[string]string{
				"node-a": "172.16.0.240",
				"node-b": "172.16.0.241",
			},
			nodes: []string{"node-b", "node-c"},
			wantAssigned: map[string]string{
				"node-b": "172.16.0.241",
				"node-c": "172.16.0.240",
			},
		},
		{
			name:        "rebinds addresses that fall outside the range",
			hostIPRange: "172.16.0.240/30",
			current: map[string]string{
				"node-a": "172.16.0.10",
				"node-b": "172.16.0.241",
			},
			nodes: []string{"node-a", "node-b"},
			wantAssigned: map[string]string{
				"node-a": "172.16.0.240",
				"node-b": "172.16.0.241",
			},
		},
		{
			name:        "rebinds a duplicated address for all but one node",
			hostIPRange: "172.16.0.240/30",
			current: map[string]string{
				"node-a": "172.16.0.241",
				"node-b": "172.16.0.241",
			},
			nodes: []string{"node-a", "node-b"},
			wantAssigned: map[string]string{
				"node-a": "172.16.0.241",
				"node-b": "172.16.0.240",
			},
		},
		{
			name:        "reports nodes left without an address when the range is exhausted",
			hostIPRange: "172.16.0.240/31",
			current: map[string]string{
				"node-c": "172.16.0.241",
			},
			nodes: []string{"node-a", "node-b", "node-c", "node-d"},
			wantAssigned: map[string]string{
				"node-a": "172.16.0.240",
				"node-c": "172.16.0.241",
			},
			wantUnassigned: []string{"node-b", "node-d"},
		},
		{
			name:         "no nodes",
			hostIPRange:  "172.16.0.240/29",
			wantAssigned: map[string]string{},
		},
		{
			name:        "invalid range",
			hostIPRange: "172.16.0.240",
			nodes:       []string{"node-a"},
			wantErr:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assigned, unassigned, err := AssignHostIPs(tc.hostIPRange, tc.current, tc.nodes)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantAssigned, assigned)
			assert.Equal(t, tc.wantUnassigned, unassigned)
		})
	}
}

func TestSetManagedExcludes(t *testing.T) {
	singleStack := `{"cniVersion":"0.3.1","type":"bridge","bridge":"cn-br","promiscMode":true,"vlan":2017,"mtu":9000,` +
		`"ipam":{"type":"whereabouts","range":"172.16.0.0/24","exclude":["172.16.0.1/32"]}}`
	dualStack := `{"cniVersion":"0.3.1","type":"bridge","bridge":"cn-br","vlan":2017,` +
		`"ipam":{"type":"whereabouts","ipRanges":[` +
		`{"range":"fd00::/64","range_start":"fd00::10","range_end":"fd00::20"},` +
		`{"range":"172.16.0.0/24","exclude":["172.16.0.1/32"]}]}}`

	tests := []struct {
		name        string
		config      string
		previous    []string
		desired     []string
		wantChanged bool
		wantExclude []string
		wantErr     bool
		check       func(t *testing.T, conf map[string]any)
	}{
		{
			name:        "appends managed entries after user entries",
			config:      singleStack,
			desired:     []string{"172.16.0.240/28", "172.16.0.192/27"},
			wantChanged: true,
			wantExclude: []string{"172.16.0.1/32", "172.16.0.240/28", "172.16.0.192/27"},
		},
		{
			name: "replaces previously managed entries",
			config: `{"type":"bridge","ipam":{"type":"whereabouts","range":"172.16.0.0/24",` +
				`"exclude":["172.16.0.1/32","172.16.0.240/28","172.16.0.192/27"]}}`,
			previous:    []string{"172.16.0.240/28", "172.16.0.192/27"},
			desired:     []string{"172.16.0.224/28", "172.16.0.192/27"},
			wantChanged: true,
			wantExclude: []string{"172.16.0.1/32", "172.16.0.192/27", "172.16.0.224/28"},
		},
		{
			name: "removes managed entries and keeps user entries",
			config: `{"type":"bridge","ipam":{"type":"whereabouts","range":"172.16.0.0/24",` +
				`"exclude":["172.16.0.1/32","172.16.0.240/28"]}}`,
			previous:    []string{"172.16.0.240/28"},
			wantChanged: true,
			wantExclude: []string{"172.16.0.1/32"},
		},
		{
			name: "drops the exclude field when nothing is left",
			config: `{"type":"bridge","ipam":{"type":"whereabouts","range":"172.16.0.0/24",` +
				`"exclude":["172.16.0.240/28"]}}`,
			previous:    []string{"172.16.0.240/28"},
			wantChanged: true,
			check: func(t *testing.T, conf map[string]any) {
				assert.NotContains(t, conf["ipam"], "exclude")
			},
		},
		{
			name:        "adds the exclude field when missing",
			config:      `{"type":"bridge","ipam":{"type":"whereabouts","range":"172.16.0.0/24"}}`,
			desired:     []string{"172.16.0.240/28"},
			wantChanged: true,
			wantExclude: []string{"172.16.0.240/28"},
		},
		{
			name: "reports no change when entries are already in place",
			config: `{"type":"bridge","ipam":{"type":"whereabouts","range":"172.16.0.0/24",` +
				`"exclude":["172.16.0.1/32","172.16.0.240/28"]}}`,
			previous:    []string{"172.16.0.240/28"},
			desired:     []string{"172.16.0.240/28"},
			wantChanged: false,
			wantExclude: []string{"172.16.0.1/32", "172.16.0.240/28"},
		},
		{
			name:        "preserves fields unknown to Harvester",
			config:      singleStack,
			desired:     []string{"172.16.0.240/28"},
			wantChanged: true,
			check: func(t *testing.T, conf map[string]any) {
				assert.Equal(t, json.Number("9000"), conf["mtu"])
				assert.Equal(t, json.Number("2017"), conf["vlan"])
				assert.Equal(t, true, conf["promiscMode"])
			},
		},
		{
			name:        "writes into the IPv4 entry of a dual-stack config",
			config:      dualStack,
			desired:     []string{"172.16.0.240/28"},
			wantChanged: true,
			check: func(t *testing.T, conf map[string]any) {
				ranges := conf["ipam"].(map[string]any)["ipRanges"].([]any)
				assert.NotContains(t, ranges[0], "exclude")
				assert.Equal(t, []any{"172.16.0.1/32", "172.16.0.240/28"}, ranges[1].(map[string]any)["exclude"])
			},
		},
		{
			name:    "rejects a config without ipam",
			config:  `{"type":"bridge"}`,
			desired: []string{"172.16.0.240/28"},
			wantErr: true,
		},
		{
			name:    "rejects a dual-stack config without an IPv4 range",
			config:  `{"type":"bridge","ipam":{"type":"whereabouts","ipRanges":[{"range":"fd00::/64"}]}}`,
			desired: []string{"172.16.0.240/28"},
			wantErr: true,
		},
		{
			name:    "rejects invalid JSON",
			config:  `{`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, changed, err := SetManagedExcludes(tc.config, tc.previous, tc.desired)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantChanged, changed)
			if !changed {
				assert.Equal(t, tc.config, out)
			}

			var conf map[string]any
			dec := json.NewDecoder(strings.NewReader(out))
			dec.UseNumber()
			require.NoError(t, dec.Decode(&conf))
			if tc.wantExclude != nil {
				got := []string{}
				for _, e := range conf["ipam"].(map[string]any)["exclude"].([]any) {
					got = append(got, e.(string))
				}
				assert.ElementsMatch(t, tc.wantExclude, got)
			}
			if tc.check != nil {
				tc.check(t, conf)
			}
		})
	}
}

func TestIPPoolAllocatedAddrs(t *testing.T) {
	pool := &whereaboutsv1alpha1.IPPool{
		Spec: whereaboutsv1alpha1.IPPoolSpec{
			Range: "172.16.0.0/24",
			Allocations: map[string]whereaboutsv1alpha1.IPAllocation{
				"1":   {PodRef: "longhorn-system/csi-a"},
				"254": {PodRef: "longhorn-system/csi-b"},
				// Whereabouts ignores keys that are not offsets.
				"x": {PodRef: "longhorn-system/csi-c"},
			},
		},
	}

	addrs, err := IPPoolAllocatedAddrs(pool)
	require.NoError(t, err)
	assert.ElementsMatch(t, []netip.Addr{
		netip.MustParseAddr("172.16.0.1"),
		netip.MustParseAddr("172.16.0.254"),
	}, addrs)
}
