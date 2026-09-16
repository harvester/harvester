package render

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"sigs.k8s.io/yaml"
)

// derivable is the set of label keys that DeriveLabels may ever produce.
// Used to detect hand-written drift: an addon manifest is not allowed to
// carry one of these keys with a value that metadata.yaml doesn't call for.
var derivableLabelKeys = []string{
	labelPrefix + "experimental",
	labelPrefix + "preview",
	labelPrefix + "ga",
	labelPrefix + "deprecated",
}

// Validate enforces the addons/ packaging contract for every categorized addon
// under addonsDir and returns one error per violation found (nil if none).
func Validate(addonsDir string) []error {
	metas, err := Discover(addonsDir)
	if err != nil {
		return []error{err}
	}

	var errs []error
	for _, meta := range metas {
		errs = append(errs, validateAddon(addonsDir, meta)...)
	}
	return errs
}

func validateAddon(addonsDir string, meta *Metadata) []error {
	var errs []error
	dir := filepath.Join(addonsDir, meta.Dir)

	if meta.Deprecated && meta.BuiltIn {
		errs = append(errs, fmt.Errorf("%s: deprecated addons must be under the standalone directory", meta.Dir))
	}

	if meta.BuiltIn {
		errs = append(errs, validateBuiltIn(addonsDir, dir, meta)...)
	} else {
		errs = append(errs, validateStandalone(dir, meta)...)
	}

	return errs
}

// validateBuiltIn validates the packaging contract for a built-in addon.
// The addon must provide addon-template.yaml, all << >> version placeholders
// in that template must resolve from version_info, and any hand-written
// derived labels must agree with metadata.yaml. Derived labels may be omitted
// from the template because generation injects them automatically.
//
// For example, an addon with stage: experimental may omit
// addon.harvesterhci.io/experimental from addon-template.yaml, but an
// explicitly set value of "false" is invalid.
func validateBuiltIn(addonsDir, dir string, meta *Metadata) []error {
	var errs []error

	fragPath := filepath.Join(dir, builtInTemplateFile)
	contents, err := os.ReadFile(fragPath)
	if err != nil {
		return []error{fmt.Errorf("%s: built-in addon requires %s: %w", meta.Dir, builtInTemplateFile, err)}
	}
	fragment := strings.TrimSuffix(string(contents), "\n")

	// every "<< >>" placeholder must resolve against version_info; assembling
	// a single-addon template and rendering it exercises exactly that.
	var sb strings.Builder
	sb.WriteString("resources:\n")
	if err := writeListItem(&sb, fragment); err != nil {
		return []error{fmt.Errorf("%s: %w", meta.Dir, err)}
	}
	versionFile := filepath.Join(addonsDir, "version_info")
	if _, err := renderVersionPlaceholders(sb.String(), versionFile); err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", meta.Dir, err))
	}

	errs = append(errs, checkLabelDrift(meta.Dir, extractRawLabels(fragment), DeriveLabels(meta), false)...)
	return errs
}

// validateStandalone validates the packaging contract for a standalone addon.
// The addon must provide a valid addon.yaml containing at least one Addon
// resource, and its derived labels must already match metadata.yaml because
// standalone manifests are not modified by the generator.
//
// For example, an addon with stage: ga must include
// addon.harvesterhci.io/ga: "true" in the Addon resource's metadata.labels.
func validateStandalone(dir string, meta *Metadata) []error {
	var errs []error

	addonPath := filepath.Join(dir, standaloneAddonFile)
	contents, err := os.ReadFile(addonPath)
	if err != nil {
		return []error{fmt.Errorf("%s: standalone addon requires %s: %w", meta.Dir, standaloneAddonFile, err)}
	}

	docs, err := splitYAMLDocuments(string(contents))
	if err != nil {
		return []error{fmt.Errorf("%s: %s is not valid YAML: %w", meta.Dir, standaloneAddonFile, err)}
	}

	var found bool
	for _, doc := range docs {
		var res map[string]interface{}
		if err := yaml.Unmarshal([]byte(doc), &res); err != nil {
			errs = append(errs, fmt.Errorf("%s: %s is not valid YAML: %w", meta.Dir, standaloneAddonFile, err))
			continue
		}
		if res["kind"] != "Addon" {
			continue
		}
		found = true
		errs = append(errs, checkLabelDrift(meta.Dir, extractResourceLabels(res), DeriveLabels(meta), true)...)
	}
	if !found {
		errs = append(errs, fmt.Errorf("%s: %s does not contain an Addon resource", meta.Dir, standaloneAddonFile))
	}

	return errs
}

// checkLabelDrift ensures that, for every label key DeriveLabels might ever
// produce, the manifest's actual value (if any) matches what metadata.yaml
// currently calls for.
//
// requirePresence controls whether a wanted label that isn't present yet is
// itself an error. Built-in addon-template.yaml fragments don't need to
// hand-carry derived labels ahead of time: GenerateTemplates/GenerateAddons
// inject them at generation time, so absence there is expected, not drift.
// Standalone addon.yaml files are static, fully-rendered manifests with no
// injection step, so a wanted label that's actually missing is drift.
//
// For example, if expected contains addon.harvesterhci.io/ga: "true":
//   - actual contains addon.harvesterhci.io/experimental: "true": stale label
//   - actual contains addon.harvesterhci.io/ga: "false": mismatched value
//   - actual omits the label: an error when requirePresence is true, but valid
//     when requirePresence is false
//
// Labels outside derivableLabelKeys are ignored, so hand-authored labels such
// as addon.harvesterhci.io/displayName are not treated as drift.
func checkLabelDrift(addonDir string, actual, expected map[string]string, requirePresence bool) []error {
	var errs []error
	for _, key := range derivableLabelKeys {
		actualVal, hasActual := actual[key]
		expectedVal, wantsLabel := expected[key]
		switch {
		case hasActual && !wantsLabel:
			errs = append(errs, fmt.Errorf("%s: label %s=%q is stale; metadata.yaml no longer calls for it", addonDir, key, actualVal))
		case hasActual && wantsLabel && actualVal != expectedVal:
			errs = append(errs, fmt.Errorf("%s: label %s=%q does not match metadata-derived value %q", addonDir, key, actualVal, expectedVal))
		case !hasActual && wantsLabel && requirePresence:
			errs = append(errs, fmt.Errorf("%s: label %s=%q is required by metadata.yaml but missing from the manifest", addonDir, key, expectedVal))
		}
	}
	return errs
}

// extractRawLabels scans a de-indented, not-yet-rendered fragment's
// metadata.labels block textually (it may still contain template
// placeholders elsewhere, so it can't be YAML-unmarshalled directly).
func extractRawLabels(fragment string) map[string]string {
	labels := map[string]string{}
	lines := strings.Split(fragment, "\n")

	metaStart := -1
	for i, l := range lines {
		if l == "metadata:" {
			metaStart = i
			break
		}
	}
	if metaStart == -1 {
		return labels
	}
	metaEnd := len(lines)
	for i := metaStart + 1; i < len(lines); i++ {
		if lines[i] != "" && !strings.HasPrefix(lines[i], " ") {
			metaEnd = i
			break
		}
	}
	for i := metaStart + 1; i < metaEnd; i++ {
		if lines[i] != "  labels:" {
			continue
		}
		for j := i + 1; j < metaEnd && strings.HasPrefix(lines[j], "    "); j++ {
			key, value, ok := strings.Cut(strings.TrimSpace(lines[j]), ":")
			if !ok {
				continue
			}
			if unquoted, err := strconv.Unquote(strings.TrimSpace(value)); err == nil {
				value = unquoted
			} else {
				value = strings.TrimSpace(value)
			}
			labels[key] = value
		}
		break
	}
	return labels
}

// extractResourceLabels reads metadata.labels off an already-unmarshalled
// resource (used for fully-rendered, standalone addon.yaml files).
func extractResourceLabels(res map[string]interface{}) map[string]string {
	labels := map[string]string{}
	metadata, ok := res["metadata"].(map[string]interface{})
	if !ok {
		return labels
	}
	rawLabels, ok := metadata["labels"].(map[string]interface{})
	if !ok {
		return labels
	}
	for k, v := range rawLabels {
		if s, ok := v.(string); ok {
			labels[k] = s
		}
	}
	return labels
}

// splitYAMLDocuments splits a "---"-separated multi-document YAML file into
// its individual documents.
func splitYAMLDocuments(contents string) ([]string, error) {
	var docs []string
	for _, doc := range strings.Split(contents, "\n---") {
		trimmed := strings.TrimSpace(doc)
		if trimmed == "" {
			continue
		}
		docs = append(docs, trimmed)
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("no YAML documents found")
	}
	return docs, nil
}
