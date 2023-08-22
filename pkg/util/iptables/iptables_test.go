package iptables

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_generatePortString(t *testing.T) {
	assert := require.New(t)

	tests := []struct {
		name     string
		ports    []uint32
		expected string
	}{
		{
			name:     "single port",
			ports:    []uint32{1},
			expected: "1",
		},
		{
			name:     "multiple ports",
			ports:    []uint32{1, 3, 5},
			expected: "1,3,5",
		},
	}

	for _, v := range tests {
		stringPort := generatePortString(v.ports)
		assert.Equal(v.expected, stringPort, "expected generated stringPort to meet expectation", v.name)
	}

}
