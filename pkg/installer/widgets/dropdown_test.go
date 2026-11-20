package widgets

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDropDownInputOffset(t *testing.T) {
	testCases := []struct {
		name         string
		content      string
		inputPadding int
		want         int
	}{
		{
			name:         "long label keeps the requested gap before the selector",
			content:      "Longhorn default disk",
			inputPadding: 4,
			want:         len("Longhorn default disk") + 1 + 4,
		},
		{
			name:    "short label retains the standard selector alignment",
			content: "Role",
			want:    20,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			dropdown := &DropDown{
				Panel:        &Panel{Content: testCase.content},
				InputPadding: testCase.inputPadding,
			}

			assert.Equal(t, testCase.want, dropdown.inputOffset())
		})
	}
}
