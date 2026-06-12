package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEscapeMustaches(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single mustache pattern",
			input:    "{{ foo }}",
			expected: "{{ `{{ foo }}` }}",
		},
		{
			name:     "multiple mustache patterns",
			input:    "{{ foo }} and {{ bar }}",
			expected: "{{ `{{ foo }}` }} and {{ `{{ bar }}` }}",
		},
		{
			name:     "no mustache pattern",
			input:    "plain text",
			expected: "plain text",
		},
		{
			name:     "mustache pattern with variable",
			input:    "Value: {{ .Name }}",
			expected: "Value: {{ `{{ .Name }}` }}",
		},
		{
			name:     "nested braces",
			input:    "{{ range .Items }}{{ .Value }}{{ end }}",
			expected: "{{ `{{ range .Items }}` }}{{ `{{ .Value }}` }}{{ `{{ end }}` }}",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "mustache at start",
			input:    "{{ start }} continues",
			expected: "{{ `{{ start }}` }} continues",
		},
		{
			name:     "mustache at end",
			input:    "starts {{ end }}",
			expected: "starts {{ `{{ end }}` }}",
		},
		{
			name:     "spaces inside mustache",
			input:    "{{   spaces   }}",
			expected: "{{ `{{   spaces   }}` }}",
		},
		{
			name:     "complex template",
			input:    "Config: {{ .Config }}, Status: {{ .Status }}",
			expected: "Config: {{ `{{ .Config }}` }}, Status: {{ `{{ .Status }}` }}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeMustaches(tt.input)
			if result != tt.expected {
				t.Errorf("EscapeMustaches(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestEscapeMustachesInTpl(t *testing.T) {
	tplBytes, err := templFS.ReadFile(filepath.Join(templateFolder, "rancherd-12-monitoring-dashboard.yaml"))
	assert.NoError(t, err)

	escapedTpl := escapeMustaches(string(tplBytes))
	assert.NotRegexp(t, "[^`]\\{\\{name\\}\\}[^`]", escapedTpl)
	assert.NotRegexp(t, "[^`]\\{\\{name\\}\\}[^`]: [^`]\\{\\{drive\\}\\}[^`]", escapedTpl)
	assert.NotRegexp(t, "[^`]\\{\\{drive\\}\\}[^`]-read", escapedTpl)
	assert.NotRegexp(t, "[^`]\\{\\{drive\\}\\}[^`]-write", escapedTpl)
	assert.Contains(t, escapedTpl, "{{ `{{name}}` }}")
	assert.Contains(t, escapedTpl, "{{ `{{name}}` }}: {{ `{{drive}}` }}")
	assert.Contains(t, escapedTpl, "{{ `{{drive}}` }}-read")
	assert.Contains(t, escapedTpl, "{{ `{{drive}}` }}-write")
}
