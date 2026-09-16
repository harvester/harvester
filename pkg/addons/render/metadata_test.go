package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeMinimalMetadataAddon(t *testing.T, root, category, directory, name string) string {
	t.Helper()

	addonDir := filepath.Join(root, category, directory)
	if err := os.MkdirAll(addonDir, 0755); err != nil {
		t.Fatal(err)
	}
	metadata := "name: " + name + "\n" +
		"namespace: default\n" +
		"stage: experimental\n"
	if err := os.WriteFile(filepath.Join(addonDir, metadataFileName), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}
	return addonDir
}

func TestDiscoverAcrossCategoriesAndSortsByName(t *testing.T) {
	root := t.TempDir()
	writeMinimalMetadataAddon(t, root, builtInCategory, "zeta-directory", "zeta")
	writeMinimalMetadataAddon(t, root, standaloneCategory, "alpha-directory", "alpha")

	metas, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 2 {
		t.Fatalf("got %d addons, want 2", len(metas))
	}

	if metas[0].Name != "alpha" || metas[0].Dir != "standalone/alpha-directory" {
		t.Errorf("first addon = (%s, %s), want (alpha, standalone/alpha-directory)", metas[0].Name, metas[0].Dir)
	}
	if metas[0].BuiltIn {
		t.Error("standalone addon was classified as built-in")
	}
	if metas[1].Name != "zeta" || metas[1].Dir != "built-in/zeta-directory" {
		t.Errorf("second addon = (%s, %s), want (zeta, built-in/zeta-directory)", metas[1].Name, metas[1].Dir)
	}
	if !metas[1].BuiltIn {
		t.Error("built-in addon was not classified as built-in")
	}
}

func TestDiscoverRejectsLegacyBuiltInDeclaration(t *testing.T) {
	root := t.TempDir()
	addonDir := filepath.Join(root, builtInCategory, "foo")
	if err := os.MkdirAll(addonDir, 0755); err != nil {
		t.Fatal(err)
	}
	metadata := "name: foo\nnamespace: default\nstage: experimental\nbuiltIn: true\n"
	if err := os.WriteFile(filepath.Join(addonDir, metadataFileName), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Discover(root)
	if err == nil || !strings.Contains(err.Error(), "builtIn is no longer supported") {
		t.Fatalf("expected legacy builtIn error, got %v", err)
	}
}

func TestDiscoverRejectsMissingMetadata(t *testing.T) {
	root := t.TempDir()
	addonDir := filepath.Join(root, builtInCategory, "incomplete")
	if err := os.MkdirAll(addonDir, 0755); err != nil {
		t.Fatal(err)
	}

	_, err := Discover(root)
	if err == nil || !strings.Contains(err.Error(), metadataFileName) {
		t.Fatalf("expected missing metadata error, got %v", err)
	}
}

func TestDiscoverDerivesStandaloneClassification(t *testing.T) {
	root := t.TempDir()
	addonDir := filepath.Join(root, standaloneCategory, "missing-built-in")
	if err := os.MkdirAll(addonDir, 0755); err != nil {
		t.Fatal(err)
	}
	metadata := "name: missing-built-in\nnamespace: default\nstage: experimental\n"
	if err := os.WriteFile(filepath.Join(addonDir, metadataFileName), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	metas, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 || metas[0].BuiltIn {
		t.Fatalf("got %#v, want one standalone addon classified as non-built-in", metas)
	}
}

func TestDiscoverRejectsDuplicateNamesAcrossCategories(t *testing.T) {
	root := t.TempDir()
	writeMinimalMetadataAddon(t, root, builtInCategory, "built-in-copy", "same-name")
	writeMinimalMetadataAddon(t, root, standaloneCategory, "standalone-copy", "same-name")

	_, err := Discover(root)
	if err == nil || !strings.Contains(err.Error(), `duplicate addon name "same-name"`) {
		t.Fatalf("expected duplicate addon name error, got %v", err)
	}
}

func TestDiscoverRejectsUnknownCategoryDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "unknown"), 0755); err != nil {
		t.Fatal(err)
	}

	_, err := Discover(root)
	if err == nil || !strings.Contains(err.Error(), "unknown addon category directory") {
		t.Fatalf("expected unknown category error, got %v", err)
	}
}
