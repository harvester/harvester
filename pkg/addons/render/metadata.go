// Package render implements the addon-repository-consolidation generator:
// it walks per-addon directories under addons/, assembles built-in addon
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
// whether the addon is built into the ISO (see Metadata.BuiltIn).
type Stage string

const (
	StageExperimental Stage = "experimental"
	StagePreview      Stage = "preview"
	StageGA           Stage = "ga"

	metadataFileName    = "metadata.yaml"
	builtInTemplateFile = "addon-template.yaml"
	nonBuiltInAddonFile = "addon.yaml"
	readmeFileName      = "README.md"
)

// Metadata is the packaging/maturity contract declared by addons/<name>/metadata.yaml.
type Metadata struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Stage      Stage  `json:"stage"`
	BuiltIn    bool   `json:"builtIn"`
	Deprecated bool   `json:"deprecated,omitempty"`

	// Dir is the addon's directory name (basename), populated by Discover.
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

// LoadMetadata reads and parses addons/<name>/metadata.yaml.
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

	return meta, nil
}

// Discover walks addonsDir/*/metadata.yaml and returns the parsed metadata,
// sorted alphabetically by addon name.
func Discover(addonsDir string) ([]*Metadata, error) {
	entries, err := os.ReadDir(addonsDir)
	if err != nil {
		return nil, fmt.Errorf("error reading addons dir %s: %w", addonsDir, err)
	}

	var result []*Metadata
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		addonDir := filepath.Join(addonsDir, entry.Name())
		metadataPath := filepath.Join(addonDir, metadataFileName)
		if _, err := os.Stat(metadataPath); err != nil {
			// not an addon directory (e.g. hack/, config/)
			continue
		}
		meta, err := LoadMetadata(addonDir)
		if err != nil {
			return nil, err
		}
		result = append(result, meta)
	}

	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}
