package util

import (
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
			expectedErr: false, // 254 usable addresses
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
		wantMin   int
		wantErr   bool
	}{
		{
			name:      "v4 only range",
			v4Range:   "192.168.2.0/24",
			v6Range:   "",
			v4Exclude: []string{},
			v6Exclude: []string{},
			wantMin:   16,
			wantErr:   false,
		},
		{
			name:      "v6 only range",
			v4Range:   "",
			v6Range:   "2001:db8::/120",
			v4Exclude: []string{},
			v6Exclude: []string{},
			wantMin:   16,
			wantErr:   false,
		},
		{
			name:      "dual range sum",
			v4Range:   "192.168.2.0/24",
			v6Range:   "2001:db8::/120",
			v4Exclude: []string{},
			v6Exclude: []string{},
			wantMin:   32,
			wantErr:   false,
		},
		{
			name:      "invalid v4 range returns error",
			v4Range:   "not-a-cidr",
			v6Range:   "",
			v4Exclude: []string{},
			v6Exclude: []string{},
			wantMin:   0,
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
				assert.GreaterOrEqual(t, count, tt.wantMin)
			}
		})
	}
}
