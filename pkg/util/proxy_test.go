package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_AddBuiltInNoProxy(t *testing.T) {
	var testCases = []struct {
		name   string
		input  string
		output string
	}{
		{
			name:   "single item",
			input:  "192.168.1.0/24",
			output: "192.168.1.0/24,localhost,127.0.0.1,0.0.0.0,10.0.0.0/8,longhorn-system,cattle-system,cattle-system.svc,harvester-system,.svc,.cluster.local",
		},
		{
			name:   "multiple items",
			input:  "192.168.1.0/24,example.com",
			output: "192.168.1.0/24,example.com,localhost,127.0.0.1,0.0.0.0,10.0.0.0/8,longhorn-system,cattle-system,cattle-system.svc,harvester-system,.svc,.cluster.local",
		},
		{
			name:   "overlapped items",
			input:  "10.0.0.0/8,127.0.0.1",
			output: "10.0.0.0/8,127.0.0.1,localhost,0.0.0.0,longhorn-system,cattle-system,cattle-system.svc,harvester-system,.svc,.cluster.local",
		},
	}

	for _, testCase := range testCases {
		result := AddBuiltInNoProxy(testCase.input)
		assert.Equal(t, testCase.output, result)
	}
}
