package render

import (
	"fmt"
	"sort"
	"strings"
)

const labelPrefix = "addon.harvesterhci.io/"

// DeriveLabels computes the set of labels that must be present on an addon's
// rendered manifest, based solely on metadata.yaml. Labels are additive:
// hand-authored labels that aren't part of this derived set (e.g. a
// human-readable displayName) are left untouched by callers.
//
// NOTE: the "ga" and "preview" stage labels are intentionally not derived
// yet; see stageLabel below.
func DeriveLabels(meta *Metadata) map[string]string {
	labels := map[string]string{}
	if key, ok := stageLabel(meta.Stage); ok {
		labels[key] = "true"
	}
	if meta.Deprecated {
		labels[labelPrefix+"deprecated"] = "true"
	}
	return labels
}

// stageLabel returns the derived label key for a stage, if one is currently
// defined. Only "experimental" is derived today (it mirrors the label addons
// already carried by hand before consolidation); "preview" and "ga" labels
// are added in a follow-up change once the migration-equivalence gate has
// verified the directory split renders identically to the pre-migration
// output.
func stageLabel(stage Stage) (string, bool) {
	switch stage {
	case StageExperimental:
		return labelPrefix + "experimental", true
	default:
		return "", false
	}
}

// injectLabels merges the given labels into a fragment's metadata.labels
// block, preserving any hand-authored labels already present (e.g.
// displayName) and leaving the rest of the fragment untouched. The fragment
// is raw, not-yet-rendered addon-template.yaml/addon.yaml text: it may still
// contain "<< >>" and "{{ }}" template placeholders, so this operates on
// text/lines rather than unmarshalling YAML.
//
// labels is expected to be small (0-3 entries); merge order is deterministic
// (sorted by key) so output is stable across runs.
func injectLabels(fragment string, labels map[string]string) (string, error) {
	if len(labels) == 0 {
		return fragment, nil
	}

	lines := strings.Split(fragment, "\n")
	metaStart := -1
	for i, l := range lines {
		if l == "metadata:" {
			metaStart = i
			break
		}
	}
	if metaStart == -1 {
		return "", fmt.Errorf("fragment has no top-level metadata: block")
	}

	// metadata block ends at the next line with no leading whitespace
	// (e.g. "spec:"), or EOF.
	metaEnd := len(lines)
	for i := metaStart + 1; i < len(lines); i++ {
		if lines[i] != "" && !strings.HasPrefix(lines[i], " ") {
			metaEnd = i
			break
		}
	}

	// find an existing "  labels:" line within the metadata block.
	labelsIdx := -1
	for i := metaStart + 1; i < metaEnd; i++ {
		if lines[i] == "  labels:" {
			labelsIdx = i
			break
		}
	}

	existing := map[string]bool{}
	insertAt := metaEnd
	if labelsIdx != -1 {
		j := labelsIdx + 1
		for j < metaEnd && strings.HasPrefix(lines[j], "    ") {
			// existing entry, e.g. `    addon.harvesterhci.io/displayName: "..."`
			trimmed := strings.TrimSpace(lines[j])
			if key, _, ok := strings.Cut(trimmed, ":"); ok {
				existing[key] = true
			}
			j++
		}
		insertAt = j
	}

	var toAdd []string
	for k := range labels {
		if !existing[k] {
			toAdd = append(toAdd, k)
		}
	}
	if len(toAdd) == 0 {
		return fragment, nil
	}
	sort.Strings(toAdd)

	var newLines []string
	for _, k := range toAdd {
		newLines = append(newLines, fmt.Sprintf("    %s: %q", k, labels[k]))
	}

	if labelsIdx == -1 {
		// no existing labels block: append one at the end of the metadata
		// block (after name/namespace), matching the field order addons
		// carried before consolidation.
		out := append([]string{}, lines[:metaEnd]...)
		out = append(out, "  labels:")
		out = append(out, newLines...)
		out = append(out, lines[metaEnd:]...)
		return strings.Join(out, "\n"), nil
	}

	out := append([]string{}, lines[:insertAt]...)
	out = append(out, newLines...)
	out = append(out, lines[insertAt:]...)
	return strings.Join(out, "\n"), nil
}
