package util

import (
	"math"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"

	networkutil "github.com/harvester/harvester/pkg/util/network"
)

func Test_ipAddressRange(t *testing.T) {

	tests := []struct {
		name        string
		config      *networkutil.Config
		expectedErr bool
	}{
		{
			name: "exclude a subset of include range,returns > MinallocatableIPAddrs addresses",
			config: &networkutil.Config{
				Range:   "192.168.2.0/24",
				Exclude: []string{"192.168.2.1/30"},
			},
			expectedErr: false,
		},
		{
			name: "exclude a subset of include range,returns > MinallocatableIPAddrs addresses",
			config: &networkutil.Config{
				Range:   "192.168.2.0/24",
				Exclude: []string{"192.168.2.1/28"},
			},
			expectedErr: false,
		},
		{
			name: "valid include range,returns > MinallocatableIPAddrs addresses",
			config: &networkutil.Config{
				Range:   "192.168.2.0/27",
				Exclude: []string{},
			},
			expectedErr: false,
		},
		{
			name: "exclude all from include subnet,returns no allocatable addresses",
			config: &networkutil.Config{
				Range:   "192.168.2.0/24",
				Exclude: []string{"192.168.2.0/24"},
			},
			expectedErr: true,
		},
		{
			name: "exclude all from include subnet,returns no allocatable addresses",
			config: &networkutil.Config{
				Range:   "192.168.2.0/30",
				Exclude: []string{"192.168.2.1/30"},
			},
			expectedErr: true,
		},
		{
			name: "no allocatable ip addresses in include range",
			config: &networkutil.Config{
				Range:   "192.168.2.0/32",
				Exclude: []string{},
			},
			expectedErr: true,
		},
		{
			name: "no allocatable ip addresses in include range",
			config: &networkutil.Config{
				Range:   "192.168.2.0/31",
				Exclude: []string{},
			},
			expectedErr: true,
		},
		{
			name: "IPv6 /120 subnet returns > 16 usable addresses",
			config: &networkutil.Config{
				Range:   "2001:db8::/120",
				Exclude: []string{},
			},
			expectedErr: false, // 255 usable addresses (256 total, minus network address; no broadcast exclusion for IPv6)
		},
		{
			name: "IPv6 /128 subnet returns 0 usable addresses",
			config: &networkutil.Config{
				Range:   "2001:db8::1/128",
				Exclude: []string{},
			},
			expectedErr: true,
		},
		{
			name: "IPv6 /120 with full exclusion returns 0 usable addresses",
			config: &networkutil.Config{
				Range:   "2001:db8::/120",
				Exclude: []string{"2001:db8::/120"},
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, _ := GetUsableIPAddressesCount(tt.config.Range, tt.config.Exclude)
			assert.Equal(t, tt.expectedErr, count < 16)
		})
	}
}

func Test_GetUsableIPAddressesCountDualStack(t *testing.T) {
	tests := []struct {
		name      string
		v4Range   string
		v6Range   string
		v4Exclude []string
		v6Exclude []string
		wantCount int // exact expected count; -1 means math.MaxInt
		wantMin   int // fallback minimum check when wantCount == -1
		wantErr   bool
	}{
		{
			name:      "v4 only range",
			v4Range:   "192.168.2.0/24",
			v6Range:   "",
			v4Exclude: []string{},
			v6Exclude: []string{},
			wantCount: 254, // 256 − network address − broadcast
			wantErr:   false,
		},
		{
			name:      "v6 only /120 range",
			v4Range:   "",
			v6Range:   "2001:db8::/120",
			v4Exclude: []string{},
			v6Exclude: []string{},
			wantCount: 255, // 256 total − 1 network address
			wantErr:   false,
		},
		{
			name:      "dual range sum",
			v4Range:   "192.168.2.0/24",
			v6Range:   "2001:db8::/120",
			v4Exclude: []string{},
			v6Exclude: []string{},
			wantCount: 509, // 254 + 255
			wantErr:   false,
		},
		{
			name:      "v6 large /64 range returns MaxInt",
			v4Range:   "",
			v6Range:   "2001:db8::/64",
			v4Exclude: []string{},
			v6Exclude: []string{},
			wantCount: -1, // math.MaxInt
			wantMin:   math.MaxInt,
			wantErr:   false,
		},
		{
			name:      "v6 /120 with exclude",
			v4Range:   "",
			v6Range:   "2001:db8::/120",
			v4Exclude: []string{},
			v6Exclude: []string{"2001:db8::/121"},
			wantCount: 128, // 255 − 127 (the /121's 128 addresses minus the already-excluded network addr)
			wantErr:   false,
		},
		{
			name:      "invalid v4 range returns error",
			v4Range:   "not-a-cidr",
			v6Range:   "",
			v4Exclude: []string{},
			v6Exclude: []string{},
			wantCount: 0,
			wantErr:   true,
		},
		{
			name:      "invalid v6 range returns error",
			v4Range:   "",
			v6Range:   "not-a-cidr",
			v4Exclude: []string{},
			v6Exclude: []string{},
			wantCount: 0,
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, err := GetUsableIPAddressesCountDualStack(tt.v4Range, tt.v6Range, tt.v4Exclude, tt.v6Exclude)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.wantCount == -1 {
					assert.Equal(t, math.MaxInt, count)
				} else {
					assert.Equal(t, tt.wantCount, count)
				}
			}
		})
	}
}

func Test_IsCoveredByPrefixes(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		excludes []string
		want     bool
	}{
		{
			name:     "two /25s fully cover a /24",
			target:   "10.0.4.0/24",
			excludes: []string{"10.0.4.0/25", "10.0.4.128/25"},
			want:     true,
		},
		{
			name:     "single exclude equal to target",
			target:   "192.168.1.0/24",
			excludes: []string{"192.168.1.0/24"},
			want:     true,
		},
		{
			name:     "exclude more general than target covers it",
			target:   "192.168.1.128/25",
			excludes: []string{"192.168.1.0/24"},
			want:     true,
		},
		{
			name:     "one /25 does not cover the full /24",
			target:   "10.0.4.0/24",
			excludes: []string{"10.0.4.0/25"},
			want:     false,
		},
		{
			name:     "no excludes leaves target uncovered",
			target:   "192.168.1.0/24",
			excludes: []string{},
			want:     false,
		},
		{
			name:     "IPv6: two /121s fully cover a /120",
			target:   "2001:db8::/120",
			excludes: []string{"2001:db8::/121", "2001:db8::80/121"},
			want:     true,
		},
		{
			name:     "IPv6: only one /121 does not cover the /120",
			target:   "2001:db8::/120",
			excludes: []string{"2001:db8::/121"},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := netip.ParsePrefix(tt.target)
			assert.NoError(t, err)
			got := IsCoveredByPrefixes(target.Masked(), tt.excludes)
			assert.Equal(t, tt.want, got)
		})
	}
}
