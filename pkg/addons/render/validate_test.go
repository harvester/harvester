package render

import (
	"os"
	"path/filepath"
	"testing"
)

// writeAddon writes a minimal built-in or non-built-in addon directory and
// returns its path, for validate tests that need to poke at specific drift
// scenarios rather than the real addons/ tree.
func writeBuiltInAddon(t *testing.T, root, name, stage string, fragmentLabels string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	metadata := "name: " + name + "\nnamespace: default\nstage: " + stage + "\nbuiltIn: true\n"
	if err := os.WriteFile(filepath.Join(dir, metadataFileName), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}
	fragment := "apiVersion: harvesterhci.io/v1beta1\n" +
		"kind: Addon\n" +
		"metadata:\n" +
		"  name: " + name + "\n" +
		"  namespace: default\n" +
		fragmentLabels +
		"spec:\n" +
		"  chart: " + name + "\n" +
		"  version: v1\n"
	if err := os.WriteFile(filepath.Join(dir, builtInTemplateFile), []byte(fragment), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, readmeFileName), []byte("# "+name+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestValidateBuiltInDriftStaleLabel(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "version_info"), []byte("#!/bin/bash\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// ga addon hand-carrying a stale "experimental" label is drift.
	writeBuiltInAddon(t, root, "foo", "ga",
		"  labels:\n    addon.harvesterhci.io/experimental: \"true\"\n")

	errs := Validate(root)
	if len(errs) == 0 {
		t.Fatal("expected drift error for stale experimental label, got none")
	}
}

func TestValidateBuiltInNoLabelYetIsNotDrift(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "version_info"), []byte("#!/bin/bash\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// experimental addon that doesn't hand-carry the label yet: fine, it's
	// injected at generation time.
	writeBuiltInAddon(t, root, "foo", "experimental", "")

	if errs := Validate(root); len(errs) > 0 {
		t.Fatalf("expected no drift errors, got: %v", errs)
	}
}

func TestValidateBuiltInMismatchedValueIsDrift(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "version_info"), []byte("#!/bin/bash\n"), 0644); err != nil {
		t.Fatal(err)
	}

	writeBuiltInAddon(t, root, "foo", "experimental",
		"  labels:\n    addon.harvesterhci.io/experimental: \"false\"\n")

	errs := Validate(root)
	if len(errs) == 0 {
		t.Fatal("expected drift error for mismatched label value, got none")
	}
}

func TestValidateMissingReadme(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "version_info"), []byte("#!/bin/bash\n"), 0644); err != nil {
		t.Fatal(err)
	}
	dir := writeBuiltInAddon(t, root, "foo", "ga", "")
	if err := os.Remove(filepath.Join(dir, readmeFileName)); err != nil {
		t.Fatal(err)
	}

	errs := Validate(root)
	if len(errs) == 0 {
		t.Fatal("expected error for missing README.md, got none")
	}
}

func TestValidateDeprecatedMustNotBeBuiltIn(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "version_info"), []byte("#!/bin/bash\n"), 0644); err != nil {
		t.Fatal(err)
	}
	dir := writeBuiltInAddon(t, root, "foo", "ga", "")
	metadata := "name: foo\nnamespace: default\nstage: ga\nbuiltIn: true\ndeprecated: true\n"
	if err := os.WriteFile(filepath.Join(dir, metadataFileName), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	errs := Validate(root)
	if len(errs) == 0 {
		t.Fatal("expected error for deprecated+builtIn addon, got none")
	}
}

func TestValidateBuiltInUnresolvedPlaceholder(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "version_info"), []byte("#!/bin/bash\nFOO_VERSION=\"1.0.0\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(root, "foo")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, metadataFileName), []byte("name: foo\nnamespace: default\nstage: ga\nbuiltIn: true\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, readmeFileName), []byte("# foo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// references a version_info var that doesn't exist.
	fragment := "apiVersion: harvesterhci.io/v1beta1\nkind: Addon\nmetadata:\n  name: foo\n  namespace: default\nspec:\n  version: << .MISSING_VERSION >>\n"
	if err := os.WriteFile(filepath.Join(dir, builtInTemplateFile), []byte(fragment), 0644); err != nil {
		t.Fatal(err)
	}

	errs := Validate(root)
	if len(errs) == 0 {
		t.Fatal("expected error for unresolved << >> placeholder, got none")
	}
}
