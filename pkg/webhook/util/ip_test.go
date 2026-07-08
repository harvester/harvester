package util

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	networkutil "github.com/harvester/harvester/pkg/util/network"
)

func Test_ipAddressRange(t *testing.T) {
	tests := []struct {
		name          string
		config        *networkutil.Config
		expectedErr   bool
		expectedCount int
	}{
		{
			name: "IPv4 /24 exclude /30 subset",
			config: &networkutil.Config{
				Range:   "192.168.2.0/24",
				Exclude: []string{"192.168.2.1/30"},
			},
			expectedCount: 250,
		},
		{
			name: "IPv4 /24 exclude /28 subset",
			config: &networkutil.Config{
				Range:   "192.168.2.0/24",
				Exclude: []string{"192.168.2.1/28"},
			},
			expectedCount: 238,
		},
		{
			name: "IPv4 /27 no excludes",
			config: &networkutil.Config{
				Range:   "192.168.2.0/27",
				Exclude: []string{},
			},
			expectedCount: 30,
		},
		{
			name: "IPv4 /24 exclude entire subnet returns 0",
			config: &networkutil.Config{
				Range:   "192.168.2.0/24",
				Exclude: []string{"192.168.2.0/24"},
			},
			expectedCount: 0,
		},
		{
			name: "IPv4 /30 exclude same-width range returns 0",
			config: &networkutil.Config{
				Range:   "192.168.2.0/30",
				Exclude: []string{"192.168.2.1/30"},
			},
			expectedCount: 0,
		},
		{
			name: "IPv4 /32 single host has no usable addresses",
			config: &networkutil.Config{
				Range:   "192.168.2.0/32",
				Exclude: []string{},
			},
			expectedCount: 0,
		},
		{
			name: "IPv4 /31 has no usable addresses",
			config: &networkutil.Config{
				Range:   "192.168.2.0/31",
				Exclude: []string{},
			},
			expectedCount: 0,
		},
		{
			name: "invalid include range returns parse error",
			config: &networkutil.Config{
				Range:   "invalid/31",
				Exclude: []string{},
			},
			expectedErr: true,
		},
		{
			name: "invalid exclude range returns parse error",
			config: &networkutil.Config{
				Range:   "2001:db8::/120",
				Exclude: []string{"invalid/120"},
			},
			expectedErr: true,
		},
		{
			name: "IPv6 /120 has 255 usable addresses",
			config: &networkutil.Config{
				Range:   "2001:db8::/120",
				Exclude: []string{},
			},
			expectedCount: 255, // 256 total, minus network address only (no broadcast in IPv6)
		},
		{
			name: "IPv6 /128 single host has no usable addresses",
			config: &networkutil.Config{
				Range:   "2001:db8::1/128",
				Exclude: []string{},
			},
			expectedCount: 0,
		},
		{
			name: "IPv6 /120 exclude entire subnet returns 0",
			config: &networkutil.Config{
				Range:   "2001:db8::/120",
				Exclude: []string{"2001:db8::/120"},
			},
			expectedCount: 0,
		},
		{
			name: "IPv4 /0 full address space",
			config: &networkutil.Config{
				Range:   "0.0.0.0/0",
				Exclude: []string{},
			},
			expectedCount: (1 << 32) - 2,
		},
		{
			name: "IPv6 /0 full address space",
			config: &networkutil.Config{
				Range:   "::/0",
				Exclude: []string{},
			},
			expectedCount: math.MaxInt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, err := GetUsableIPAddressesCount(tt.config.Range, tt.config.Exclude)
			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectedCount, count)
		})
	}
}
