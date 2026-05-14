package widgets

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatContent(t *testing.T) {
	testCases := []struct {
		name       string
		content    string
		maxColumns int
		maxRows    int
		wrap       bool
		truncate   bool
		expected   string
	}{
		{
			name:       "Do not wrap or truncate the content",
			content:    "abcdefghijklmn",
			maxColumns: 1000,
			maxRows:    1,
			wrap:       false,
			truncate:   false,
			expected:   "abcdefghijklmn",
		},
		{
			name:       "No truncation of the content needed",
			content:    "abcdef\nghijkl\nmnop",
			maxColumns: 6,
			maxRows:    3,
			wrap:       false,
			truncate:   true,
			expected:   "abcdef\nghijkl\nmnop",
		},
		{
			name:       "Truncate the content [1]",
			content:    "abcdef\nghijkl\nmnijkl\nopqr",
			maxColumns: 6,
			maxRows:    3,
			wrap:       false,
			truncate:   true,
			expected:   "abcdef\nghijkl\nmn ...",
		},
		{
			name:       "Truncate the content [2]",
			content:    "abcdef\nghijkl\nmnijkl\nopqr",
			maxColumns: 0,
			maxRows:    3,
			wrap:       false,
			truncate:   true,
			expected:   "abcdef\nghijkl\nmnijkl ...",
		},
		{
			name:       "Won't wrap since no blanks",
			content:    "abcdefghijklmn",
			maxColumns: 5,
			maxRows:    1,
			wrap:       true,
			truncate:   false,
			expected:   "abcdefghijklmn",
		},
		{
			name:       "Wrap content [1]",
			content:    "abcde fghijklmn",
			maxColumns: 8,
			maxRows:    -1,
			wrap:       true,
			truncate:   false,
			expected:   "abcde\nfghijklmn",
		},
		{
			name:       "Wrap content [2]",
			content:    "Lorem ipsum dolor sit amet, consetetur sadipscing elitr",
			maxColumns: 10,
			maxRows:    -1,
			wrap:       true,
			truncate:   false,
			expected:   "Lorem\nipsum\ndolor sit\namet,\nconsetetur sadipscing elitr",
		},
		{
			name:       "Wrap and truncate the content [1]",
			content:    "Lorem ipsum dolor sit amet, consetetur sadipscing elitr",
			maxColumns: 10,
			maxRows:    4,
			wrap:       true,
			truncate:   true,
			expected:   "Lorem\nipsum\ndolor sit\namet, ...",
		},
		{
			name:       "Wrap and truncate the content [2]",
			content:    "Lorem ipsum dolor sit amet, ",
			maxColumns: 10,
			maxRows:    4,
			wrap:       true,
			truncate:   true,
			expected:   "Lorem\nipsum\ndolor sit\namet, ",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := formatContent(tc.content, tc.maxColumns, tc.maxRows, tc.wrap, tc.truncate)
			assert.Equal(t, tc.expected, result)
		})
	}
}
