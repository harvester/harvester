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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, _ := GetUsableIPAddressesCount(tt.config.Range, tt.config.Exclude)
			assert.Equal(t, tt.expectedErr, count < 16)
		})
	}
}
