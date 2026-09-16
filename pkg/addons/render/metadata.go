// Package render implements the addon-repository-consolidation generator:
// it discovers categorized addon directories under addons/, assembles built-in addon
// fragments into the rancherd bootstrap template, renders disabled addon
// manifests for the upgrade bundle, and validates the packaging/maturity
// contract declared in each addon's metadata.yaml.
package render

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"sigs.k8s.io/yaml"
)

// Stage is an addon's maturity/support-commitment level. It is orthogonal to
// whether the addon is built into the ISO; that classification is derived from
// the addon's category directory by Discover.
type Stage string

const (
	StageExperimental Stage = "experimental"
	StagePreview      Stage = "preview"
	StageGA           Stage = "ga"

	metadataFileName    = "metadata.yaml"
	builtInTemplateFile = "addon-template.yaml"
	standaloneAddonFile = "addon.yaml"
	builtInCategory     = "built-in"
	standaloneCategory  = "standalone"
	packagingDirectory  = "packaging"
)

type addonCategory struct {
	name    string
	builtIn bool
}

var addonCategories = []addonCategory{
	{name: builtInCategory, builtIn: true},
	{name: standaloneCategory, builtIn: false},
}

// Metadata is the packaging/maturity contract declared by
// addons/{built-in,standalone}/<name>/metadata.yaml.
type Metadata struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Stage     Stage  `json:"stage"`
	// BuiltIn is derived from the addon's category directory by Discover. It
	// is not part of metadata.yaml.
	BuiltIn    bool `json:"-"`
	Deprecated bool `json:"deprecated,omitempty"`

	// Dir is the addon's path relative to the addons/ root, populated by
	// Discover (for example, built-in/descheduler).
	// It is not part of the on-disk metadata.yaml.
	Dir string `json:"-"`
}

func (s Stage) valid() bool {
	switch s {
	case StageExperimental, StagePreview, StageGA:
		return true
	default:
		return false
	}
}

// LoadMetadata reads and parses an addon's metadata.yaml.
func LoadMetadata(addonDir string) (*Metadata, error) {
	path := filepath.Join(addonDir, metadataFileName)
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading %s: %w", path, err)
	}

	meta := &Metadata{}
	if err := yaml.Unmarshal(contents, meta); err != nil {
		return nil, fmt.Errorf("error parsing %s: %w", path, err)
	}
	var declaration map[string]interface{}
	if err := yaml.Unmarshal(contents, &declaration); err != nil {
		return nil, fmt.Errorf("error parsing %s: %w", path, err)
	}
	meta.Dir = filepath.Base(addonDir)

	if meta.Name == "" {
		return nil, fmt.Errorf("%s: name is required", path)
	}
	if meta.Namespace == "" {
		return nil, fmt.Errorf("%s: namespace is required", path)
	}
	if !meta.Stage.valid() {
		return nil, fmt.Errorf("%s: stage %q is not one of experimental|preview|ga", path, meta.Stage)
	}
	if _, exists := declaration["builtIn"]; exists {
		return nil, fmt.Errorf("%s: builtIn is no longer supported; use the built-in or standalone directory", path)
	}

	return meta, nil
}

// Discover reads direct addon children from the built-in and standalone
// category directories and returns their parsed metadata, sorted
// alphabetically by addon name. The category layout is intentionally explicit:
// packaging inputs and arbitrary nested metadata.yaml files are not addons.
func Discover(addonsDir string) ([]*Metadata, error) {
	entries, err := os.ReadDir(addonsDir)
	if err != nil {
		return nil, fmt.Errorf("error reading addons dir %s: %w", addonsDir, err)
	}

	knownCategories := map[string]bool{
		builtInCategory:    true,
		standaloneCategory: true,
	}
	for _, entry := range entries {
		if entry.Name() == packagingDirectory {
			if !entry.IsDir() {
				return nil, fmt.Errorf("%s must be a directory", filepath.Join(addonsDir, entry.Name()))
			}
			continue
		}
		if _, ok := knownCategories[entry.Name()]; ok {
			if !entry.IsDir() {
				return nil, fmt.Errorf("%s must be a directory", filepath.Join(addonsDir, entry.Name()))
			}
			continue
		}
		if entry.IsDir() {
			return nil, fmt.Errorf("unknown addon category directory %s", filepath.Join(addonsDir, entry.Name()))
		}
	}

	var result []*Metadata
	names := map[string]string{}
	for _, category := range addonCategories {
		categoryDir := filepath.Join(addonsDir, category.name)
		categoryEntries, err := os.ReadDir(categoryDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("error reading addon category %s: %w", categoryDir, err)
		}

		for _, entry := range categoryEntries {
			addonDir := filepath.Join(categoryDir, entry.Name())
			if !entry.IsDir() {
				return nil, fmt.Errorf("%s: category entries must be addon directories", addonDir)
			}

			meta, err := LoadMetadata(addonDir)
			if err != nil {
				return nil, err
			}
			meta.BuiltIn = category.builtIn

			meta.Dir = filepath.ToSlash(filepath.Join(category.name, entry.Name()))
			if previousDir, exists := names[meta.Name]; exists {
				return nil, fmt.Errorf("duplicate addon name %q in %s and %s", meta.Name, previousDir, meta.Dir)
			}
			names[meta.Name] = meta.Dir
			result = append(result, meta)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].Dir < result[j].Dir
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}
