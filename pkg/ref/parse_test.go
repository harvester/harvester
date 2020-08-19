package ref

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	type input struct {
		ref string
	}
	type output struct {
		namespace string
		name      string
	}

	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "cluster scope ref",
			given: input{
				ref: "test",
			},
			expected: output{
				namespace: "",
				name:      "test",
			},
		},
		{
			name: "namespace scope ref",
			given: input{
				ref: "default:test",
			},
			expected: output{
				namespace: "default",
				name:      "test",
			},
		},
	}

	for _, tc := range testCases {
		var actual output
		actual.namespace, actual.name = Parse(tc.given.ref)
		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}

}
