package render

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AssembleTemplate reads every built-in addon's addon-template.yaml fragment
// (in the order given by metas, expected to be alphabetical by addon name —
// see Discover), injects its derived labels, and concatenates the fragments
// into a single rancherd bootstrap template text equivalent to the old
// monolithic pkg/templates/rancherd-22-addons.yaml.
//
// The returned text still contains unresolved "<< >>" version placeholders
// and literal "{{ }}" runtime toggles; see RenderTemplate.
func AssembleTemplate(addonsDir string, metas []*Metadata) (string, error) {
	var sb strings.Builder
	sb.WriteString("resources:\n")

	for _, meta := range metas {
		if !meta.BuiltIn {
			continue
		}

		fragPath := filepath.Join(addonsDir, meta.Dir, builtInTemplateFile)
		contents, err := os.ReadFile(fragPath)
		if err != nil {
			return "", fmt.Errorf("error reading %s: %w", fragPath, err)
		}

		fragment, err := injectLabels(strings.TrimSuffix(string(contents), "\n"), DeriveLabels(meta))
		if err != nil {
			return "", fmt.Errorf("error injecting labels for %s: %w", meta.Name, err)
		}

		if err := writeListItem(&sb, fragment); err != nil {
			return "", fmt.Errorf("error assembling fragment for %s: %w", meta.Name, err)
		}
	}

	return sb.String(), nil
}

// writeListItem re-indents a de-indented, column-0 fragment (as stored in
// addon-template.yaml/addon.yaml) back into a "resources:" list item: the
// first line is prefixed with "  - " and every subsequent line with 4 spaces.
func writeListItem(sb *strings.Builder, fragment string) error {
	lines := strings.Split(fragment, "\n")
	if len(lines) == 0 || lines[0] == "" {
		return fmt.Errorf("empty fragment")
	}

	sb.WriteString("  - ")
	sb.WriteString(lines[0])
	sb.WriteString("\n")
	for _, l := range lines[1:] {
		if l == "" {
			sb.WriteString("\n")
			continue
		}
		sb.WriteString("    ")
		sb.WriteString(l)
		sb.WriteString("\n")
	}
	return nil
}
