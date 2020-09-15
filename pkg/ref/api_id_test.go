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
				ref: "default/test",
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

func TestConstruct(t *testing.T) {
	type input struct {
		namespace string
		name      string
	}
	type output struct {
		ref string
	}

	var testCases = []struct {
		name     string
		given    input
		expected output
	}{
		{
			name: "cluster scope ref",
			given: input{
				namespace: "",
				name:      "test",
			},
			expected: output{
				ref: "test",
			},
		},
		{
			name: "namespace scope ref",
			given: input{
				namespace: "default",
				name:      "test",
			},
			expected: output{
				ref: "default/test",
			},
		},
	}

	for _, tc := range testCases {
		var actual output
		actual.ref = Construct(tc.given.namespace, tc.given.name)
		assert.Equal(t, tc.expected, actual, "case %q", tc.name)
	}

}
