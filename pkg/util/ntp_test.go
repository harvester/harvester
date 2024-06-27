package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ReGenerateNTPServers(t *testing.T) {
	var testCases = []struct {
		ntpSettings *NTPSettings
		expected    string
	}{
		{
			ntpSettings: nil,
			expected:    "",
		},
		{
			ntpSettings: &NTPSettings{},
			expected:    "",
		},
		{
			ntpSettings: &NTPSettings{
				nil,
			},
			expected: "",
		},
		{
			ntpSettings: &NTPSettings{
				NTPServers: []string{"0.suse.pool.ntp.org", "1.suse.pool.ntp.org"},
			},
			expected: "0.suse.pool.ntp.org 1.suse.pool.ntp.org",
		},
	}

	for _, testCase := range testCases {
		assert.Equal(t, testCase.expected, ReGenerateNTPServers(testCase.ntpSettings))
	}
}
