package console

import (
	"net"
	"testing"

	"github.com/vishvananda/netlink"
)

func Test_isDefaultRoute(t *testing.T) {
	tests := []struct {
		name     string
		route    netlink.Route
		expected bool
	}{
		{
			name: "nil destination returns true",
			route: netlink.Route{
				Dst: nil,
			},
			expected: true,
		},
		{
			name: "destination with zero mask bits returns true",
			route: netlink.Route{
				Dst: &net.IPNet{
					IP:   net.ParseIP("0.0.0.0"),
					Mask: net.CIDRMask(0, 32),
				},
			},
			expected: true,
		},
		{
			name: "destination with non-zero mask bits returns false",
			route: netlink.Route{
				Dst: &net.IPNet{
					IP:   net.ParseIP("192.168.1.0"),
					Mask: net.CIDRMask(24, 32),
				},
			},
			expected: false,
		},
		{
			name: "destination with /8 mask returns false",
			route: netlink.Route{
				Dst: &net.IPNet{
					IP:   net.ParseIP("10.0.0.0"),
					Mask: net.CIDRMask(8, 32),
				},
			},
			expected: false,
		},
		{
			name: "destination with /16 mask returns false",
			route: netlink.Route{
				Dst: &net.IPNet{
					IP:   net.ParseIP("172.16.0.0"),
					Mask: net.CIDRMask(16, 32),
				},
			},
			expected: false,
		},
		{
			name: "destination with nil mask returns false",
			route: netlink.Route{
				Dst: &net.IPNet{
					IP:   net.ParseIP("192.168.1.0"),
					Mask: nil,
				},
			},
			expected: false,
		},
		{
			name: "IPv6 destination with zero mask bits returns true",
			route: netlink.Route{
				Dst: &net.IPNet{
					IP:   net.ParseIP("::"),
					Mask: net.CIDRMask(0, 128),
				},
			},
			expected: true,
		},
		{
			name: "IPv6 destination with non-zero mask bits returns false",
			route: netlink.Route{
				Dst: &net.IPNet{
					IP:   net.ParseIP("2001:db8::"),
					Mask: net.CIDRMask(64, 128),
				},
			},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isDefaultRoute(tc.route)
			if result != tc.expected {
				t.Errorf("isDefaultRoute() = %v, expected %v", result, tc.expected)
			}
		})
	}
}
