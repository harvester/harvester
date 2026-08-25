package storagenetwork

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPoolNameFromCIDR(t *testing.T) {
	tests := []struct {
		name    string
		cidr    string
		want    string
		wantErr bool
	}{
		{
			name: "IPv4 /24",
			cidr: "10.0.0.0/24",
			want: "10.0.0.0-24",
		},
		{
			name: "IPv4 /16",
			cidr: "192.168.0.0/16",
			want: "192.168.0.0-16",
		},
		{
			name: "IPv4 host bits masked out",
			cidr: "10.0.0.5/24",
			want: "10.0.0.0-24",
		},
		{
			name: "IPv6 /64 with double colon",
			cidr: "fd00::/64",
			want: "fd00---64",
		},
		{
			name: "IPv6 /112",
			cidr: "2001:db8::/112",
			want: "2001-db8---112",
		},
		{
			name: "IPv6 host bits masked out",
			cidr: "fd00::1/64",
			want: "fd00---64",
		},
		{
			name:    "invalid CIDR returns error",
			cidr:    "not-a-cidr",
			wantErr: true,
		},
		{
			name:    "empty string returns error",
			cidr:    "",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := poolNameFromCIDR(tc.cidr)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
