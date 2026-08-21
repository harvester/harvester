package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateCIDR(t *testing.T) {
	tests := []struct {
		name        string
		cidr        string
		wantErr     bool
		errContains string
	}{
		{
			name:    "empty string is allowed",
			cidr:    "",
			wantErr: false,
		},
		{
			name:    "whitespace-only is allowed",
			cidr:    "  ",
			wantErr: false,
		},
		{
			name:    "valid single IPv4 CIDR",
			cidr:    "10.52.0.0/16",
			wantErr: false,
		},
		{
			name:    "valid dual-stack IPv4-first",
			cidr:    "10.52.0.0/16,fd52::/56",
			wantErr: false,
		},
		{
			name:    "valid dual-stack with spaces around comma",
			cidr:    "10.52.0.0/16 , fd52::/56",
			wantErr: false,
		},
		{
			name:    "single IPv6 CIDR is rejected",
			cidr:    "fd52::/56",
			wantErr: true,
		},
		{
			name:    "dual-stack IPv6-first order is rejected",
			cidr:    "fd52::/56,10.52.0.0/16",
			wantErr: true,
		},
		{
			name:    "dual-stack two IPv4 CIDRs is rejected",
			cidr:    "10.52.0.0/16,10.53.0.0/16",
			wantErr: true,
		},
		{
			name:    "three CIDRs are rejected",
			cidr:    "10.52.0.0/16,fd52::/56,10.53.0.0/16",
			wantErr: true,
		},
		{
			name:    "invalid CIDR string is rejected",
			cidr:    "not-a-cidr",
			wantErr: true,
		},
		{
			name:    "bare IP without prefix length is rejected",
			cidr:    "10.52.0.1",
			wantErr: true,
		},
		// Large-pool cases: validate no integer overflow in prefix parsing.
		{
			name:    "IPv4 /1 covers half the address space",
			cidr:    "0.0.0.0/1",
			wantErr: false,
		},
		{
			name:    "IPv4 /0 covers the entire address space",
			cidr:    "0.0.0.0/0",
			wantErr: false,
		},
		{
			name:    "IPv6 /1 as single CIDR is rejected (must be IPv4)",
			cidr:    "::/1",
			wantErr: true,
		},
		{
			name:    "IPv6 /0 as single CIDR is rejected (must be IPv4)",
			cidr:    "::/0",
			wantErr: true,
		},
		{
			name:    "dual-stack with IPv4 /1 and IPv6 /1",
			cidr:    "0.0.0.0/1,::/1",
			wantErr: false,
		},
		{
			name:    "dual-stack with IPv4 /0 and IPv6 /0",
			cidr:    "0.0.0.0/0,::/0",
			wantErr: false,
		},
		{
			name:    "dual-stack with IPv4 /2 and IPv6 /2",
			cidr:    "0.0.0.0/2,::/2",
			wantErr: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCIDR(tc.cidr)
			if tc.wantErr {
				assert.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestValidateCIDRConsistency(t *testing.T) {
	tests := []struct {
		name        string
		podCIDR     string
		svcCIDR     string
		wantErr     bool
		errContains string
	}{
		{
			name:    "both empty is allowed",
			wantErr: false,
		},
		{
			name:    "empty pod CIDR is allowed",
			svcCIDR: "10.53.0.0/16",
			wantErr: false,
		},
		{
			name:    "empty service CIDR is allowed",
			podCIDR: "10.52.0.0/16",
			wantErr: false,
		},
		{
			name:    "matching single-stack CIDRs",
			podCIDR: "10.52.0.0/16",
			svcCIDR: "10.53.0.0/16",
			wantErr: false,
		},
		{
			name:    "matching dual-stack CIDRs",
			podCIDR: "10.52.0.0/16,fd52::/56",
			svcCIDR: "10.53.0.0/16,fd53::/112",
			wantErr: false,
		},
		{
			name:        "pod dual-stack, service single-stack is rejected",
			podCIDR:     "10.52.0.0/16,fd52::/56",
			svcCIDR:     "10.53.0.0/16",
			wantErr:     true,
			errContains: "pod CIDR is dual-stack but service CIDR is single-stack",
		},
		{
			name:        "pod single-stack, service dual-stack is rejected",
			podCIDR:     "10.52.0.0/16",
			svcCIDR:     "10.53.0.0/16,fd53::/112",
			wantErr:     true,
			errContains: "service CIDR is dual-stack but pod CIDR is single-stack",
		},
		{
			name:        "overlapping IPv4 CIDRs are rejected",
			podCIDR:     "10.52.0.0/16",
			svcCIDR:     "10.52.0.0/24",
			wantErr:     true,
			errContains: "must not overlap",
		},
		{
			name:        "overlapping IPv6 CIDRs in dual-stack are rejected",
			podCIDR:     "10.52.0.0/16,fd52::/56",
			svcCIDR:     "10.53.0.0/16,fd52::/64",
			wantErr:     true,
			errContains: "must not overlap",
		},
		// Large-pool cases: validate no integer overflow in overlap detection.
		{
			name:    "non-overlapping IPv4 /1 halves of the address space",
			podCIDR: "0.0.0.0/1",
			svcCIDR: "128.0.0.0/1",
			wantErr: false,
		},
		{
			name:        "overlapping huge IPv4 /1 and /2 are rejected",
			podCIDR:     "0.0.0.0/1",
			svcCIDR:     "0.0.0.0/2",
			wantErr:     true,
			errContains: "must not overlap",
		},
		{
			name:    "non-overlapping dual-stack /1 halves of both address spaces",
			podCIDR: "0.0.0.0/1,::/1",
			svcCIDR: "128.0.0.0/1,8000::/1",
			wantErr: false,
		},
		{
			name:        "overlapping huge dual-stack /1 and /2 IPv6 are rejected",
			podCIDR:     "0.0.0.0/1,::/1",
			svcCIDR:     "128.0.0.0/1,::/2",
			wantErr:     true,
			errContains: "must not overlap",
		},
		{
			name:        "entire IPv4 /0 and entire IPv6 /0 dual-stack do not overlap each other",
			podCIDR:     "0.0.0.0/0,::/0",
			svcCIDR:     "0.0.0.0/0,::/0",
			wantErr:     true,
			errContains: "must not overlap",
		}, {
			name:    "invalid pod CIDR part returns parse error",
			podCIDR: "not-a-cidr",
			svcCIDR: "10.53.0.0/16",
			wantErr: true,
		},
		{
			name:    "invalid service CIDR part returns parse error",
			podCIDR: "10.52.0.0/16",
			svcCIDR: "not-a-cidr",
			wantErr: true,
		}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCIDRConsistency(tc.podCIDR, tc.svcCIDR)
			if tc.wantErr {
				assert.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestValidateDNSIP(t *testing.T) {
	tests := []struct {
		name        string
		ip          string
		serviceCIDR string
		wantErr     bool
		errContains string
	}{
		{
			name:    "both empty is allowed",
			wantErr: false,
		},
		{
			name:        "empty DNS with valid service CIDR is allowed (defaults used)",
			serviceCIDR: "10.53.0.0/16",
			wantErr:     false,
		},
		{
			name:        "valid IPv4 DNS within service CIDR",
			ip:          "10.53.0.10",
			serviceCIDR: "10.53.0.0/16",
			wantErr:     false,
		},
		{
			name:        "valid dual-stack DNS within dual-stack service CIDR",
			ip:          "10.53.0.10,fd53::a",
			serviceCIDR: "10.53.0.0/16,fd53::/112",
			wantErr:     false,
		},
		{
			name:        "single IPv6 DNS is rejected",
			ip:          "fd53::a",
			serviceCIDR: "fd53::/112",
			wantErr:     true,
			errContains: "a single DNS IP must be IPv4",
		},
		{
			name:        "DNS not in service CIDR is rejected",
			ip:          "10.54.0.10",
			serviceCIDR: "10.53.0.0/16",
			wantErr:     true,
			errContains: "is not in the service CIDR",
		},
		{
			name:        "DNS is the network address of service CIDR",
			ip:          "10.53.0.0",
			serviceCIDR: "10.53.0.0/16",
			wantErr:     true,
			errContains: "is the network address",
		},
		{
			name:        "three DNS IPs are rejected",
			ip:          "10.53.0.10,fd53::a,10.53.0.11",
			serviceCIDR: "10.53.0.0/16",
			wantErr:     true,
			errContains: "at most two DNS IPs",
		},
		{
			name:        "dual-stack DNS in IPv6-first order is rejected",
			ip:          "fd53::a,10.53.0.10",
			serviceCIDR: "10.53.0.0/16,fd53::/112",
			wantErr:     true,
			errContains: "IPv4-first order",
		},
		{
			name:        "IPv4 DNS with no matching service CIDR family",
			ip:          "10.53.0.10",
			serviceCIDR: "fd53::/112",
			wantErr:     true,
			errContains: "no matching service CIDR",
		},
		{
			name:        "invalid DNS IP string is rejected",
			ip:          "not-an-ip",
			serviceCIDR: "10.53.0.0/16",
			wantErr:     true,
		},
		// Large-pool cases: validate no integer overflow in membership check.
		{
			name:        "IPv4 DNS within a /1 service pool",
			ip:          "1.2.3.4",
			serviceCIDR: "0.0.0.0/1",
			wantErr:     false,
		},
		{
			name:        "IPv4 DNS outside a /1 service pool",
			ip:          "200.0.0.1",
			serviceCIDR: "0.0.0.0/1",
			wantErr:     true,
			errContains: "is not in the service CIDR",
		},
		{
			name:        "network address of a /1 pool is rejected",
			ip:          "0.0.0.0",
			serviceCIDR: "0.0.0.0/1",
			wantErr:     true,
			errContains: "is the network address",
		},
		{
			name:        "dual-stack DNS within /1 pools for both families",
			ip:          "1.2.3.4,::1",
			serviceCIDR: "0.0.0.0/1,::/1",
			wantErr:     false,
		},
		{
			name:        "IPv6 network address of /1 pool is rejected",
			ip:          "1.2.3.4,::",
			serviceCIDR: "0.0.0.0/1,::/1",
			wantErr:     true,
			errContains: "is the network address",
		},
		{
			name:        "IPv4 DNS within a /0 entire-space service pool",
			ip:          "192.168.1.1",
			serviceCIDR: "0.0.0.0/0",
			wantErr:     false,
		}, {
			name:        "service CIDR with trailing comma skips empty part",
			ip:          "10.53.0.10",
			serviceCIDR: "10.53.0.0/16,",
			wantErr:     false,
		},
		{
			name:        "invalid service CIDR returns parse error",
			ip:          "10.53.0.10",
			serviceCIDR: "not-a-cidr",
			wantErr:     true,
			errContains: "the service CIDR must be valid",
		}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateDNSIP(tc.ip, tc.serviceCIDR)
			if tc.wantErr {
				assert.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestValidateCIDRMatchesIPFamilies(t *testing.T) {
	tests := []struct {
		name        string
		cidr        string
		ipv6Enabled bool
		wantErr     bool
		errContains string
	}{
		{
			name:    "empty CIDR is always allowed",
			cidr:    "",
			wantErr: false,
		},
		{
			name:        "IPv4-only mode accepts single IPv4 CIDR",
			cidr:        "10.52.0.0/16",
			ipv6Enabled: false,
			wantErr:     false,
		},
		{
			name:        "dual-stack mode accepts dual-stack CIDR",
			cidr:        "10.52.0.0/16,fd52::/56",
			ipv6Enabled: true,
			wantErr:     false,
		},
		{
			name:        "IPv4-only mode rejects dual-stack CIDR",
			cidr:        "10.52.0.0/16,fd52::/56",
			ipv6Enabled: false,
			wantErr:     true,
			errContains: "IPv4-only mode requires a single IPv4 CIDR",
		},
		{
			name:        "dual-stack mode rejects single CIDR",
			cidr:        "10.52.0.0/16",
			ipv6Enabled: true,
			wantErr:     true,
			errContains: "dual-stack mode (IPv4,IPv6) requires both an IPv4 and an IPv6 CIDR",
		},
		// Large-pool cases: family-check must not depend on prefix size.
		{
			name:        "IPv4-only mode accepts /1",
			cidr:        "0.0.0.0/1",
			ipv6Enabled: false,
			wantErr:     false,
		},
		{
			name:        "IPv4-only mode accepts /0",
			cidr:        "0.0.0.0/0",
			ipv6Enabled: false,
			wantErr:     false,
		},
		{
			name:        "dual-stack mode accepts /1 IPv4 and /1 IPv6",
			cidr:        "0.0.0.0/1,::/1",
			ipv6Enabled: true,
			wantErr:     false,
		},
		{
			name:        "dual-stack mode accepts /0 IPv4 and /0 IPv6",
			cidr:        "0.0.0.0/0,::/0",
			ipv6Enabled: true,
			wantErr:     false,
		},
		{
			name:        "dual-stack mode accepts /2 IPv4 and /2 IPv6",
			cidr:        "0.0.0.0/2,::/2",
			ipv6Enabled: true,
			wantErr:     false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCIDRMatchesIPFamilies(tc.cidr, tc.ipv6Enabled)
			if tc.wantErr {
				assert.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestValidateDNSMatchesIPFamilies(t *testing.T) {
	tests := []struct {
		name        string
		dns         string
		ipv6Enabled bool
		wantErr     bool
		errContains string
	}{
		{
			name:    "empty DNS is always allowed",
			dns:     "",
			wantErr: false,
		},
		{
			name:        "IPv4-only mode accepts single IPv4 DNS",
			dns:         "10.53.0.10",
			ipv6Enabled: false,
			wantErr:     false,
		},
		{
			name:        "dual-stack mode accepts single IPv4 DNS",
			dns:         "10.53.0.10",
			ipv6Enabled: true,
			wantErr:     false,
		},
		{
			name:        "dual-stack mode accepts IPv4+IPv6 DNS pair",
			dns:         "10.53.0.10,fd53::a",
			ipv6Enabled: true,
			wantErr:     false,
		},
		{
			name:        "IPv4-only mode rejects dual DNS pair",
			dns:         "10.53.0.10,fd53::a",
			ipv6Enabled: false,
			wantErr:     true,
			errContains: "IPv4-only mode requires a single IPv4 DNS address",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateDNSMatchesIPFamilies(tc.dns, tc.ipv6Enabled)
			if tc.wantErr {
				assert.Error(t, err)
				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}
				return
			}
			assert.NoError(t, err)
		})
	}
}
