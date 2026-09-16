package render

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"text/template"

	"sigs.k8s.io/yaml"
)

// repoAddonsDir points at the real addons/ directory two levels up from this
// package (pkg/addons/render -> repo root -> addons).
func repoAddonsDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("../../../addons")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("addons dir not found at %s: %v", dir, err)
	}
	return dir
}

// TestGenerateAddonsMatchesGolden is the migration-equivalence test: it
// compares the in-tree generator's -generateAddons output for every built-in
// addon against a golden snapshot captured from the pre-migration standalone
// harvester/addons repo (same pkg/templates/rancherd-22-addons.yaml +
// version_info). The Addon resources must be deeply equal modulo derived
// stage/deprecated labels (see stripDerivedLabels): those are an intentional,
// separately-tracked change (the "ga"/"preview" labels didn't exist pre-
// migration), not something the directory split itself should affect.
func TestGenerateAddonsMatchesGolden(t *testing.T) {
	addonsDir := repoAddonsDir(t)
	versionFile := filepath.Join(addonsDir, "version_info")

	destDir := t.TempDir()
	if err := GenerateAddons(addonsDir, destDir, versionFile); err != nil {
		t.Fatalf("GenerateAddons failed: %v", err)
	}

	goldenDir := "testdata/golden/addons-manifests"
	goldenFiles, err := os.ReadDir(goldenDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(goldenFiles) == 0 {
		t.Fatal("no golden fixtures found")
	}

	for _, gf := range goldenFiles {
		t.Run(gf.Name(), func(t *testing.T) {
			golden, err := os.ReadFile(filepath.Join(goldenDir, gf.Name()))
			if err != nil {
				t.Fatal(err)
			}
			generated, err := os.ReadFile(filepath.Join(destDir, gf.Name()))
			if err != nil {
				t.Fatalf("generator did not produce %s: %v", gf.Name(), err)
			}

			var wantRes, gotRes map[string]interface{}
			if err := yaml.Unmarshal(golden, &wantRes); err != nil {
				t.Fatalf("golden fixture is not valid YAML: %v", err)
			}
			if err := yaml.Unmarshal(generated, &gotRes); err != nil {
				t.Fatalf("generated output is not valid YAML: %v", err)
			}
			stripDerivedLabels(wantRes)
			stripDerivedLabels(gotRes)

			if !reflect.DeepEqual(wantRes, gotRes) {
				t.Errorf("rendered addon differs from pre-migration golden output.\ngolden:\n%s\ngenerated:\n%s", golden, generated)
			}
		})
	}

	// every golden addon must have been generated, and vice versa: no
	// addon silently dropped or added by the migration.
	generatedFiles, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(generatedFiles) != len(goldenFiles) {
		t.Errorf("generated %d addon manifests, golden has %d", len(generatedFiles), len(goldenFiles))
	}
}

// stripDerivedLabels removes every label DeriveLabels might produce from a
// resource's metadata.labels in place (deleting the labels key entirely if
// it becomes empty), so golden comparisons focus on content the directory
// split itself should never change.
func stripDerivedLabels(res map[string]interface{}) {
	metadata, ok := res["metadata"].(map[string]interface{})
	if !ok {
		return
	}
	labels, ok := metadata["labels"].(map[string]interface{})
	if !ok {
		return
	}
	for _, key := range derivableLabelKeys {
		delete(labels, key)
	}
	if len(labels) == 0 {
		delete(metadata, "labels")
	}
}

// TestGenerateAddonsAppliesGALabel confirms the one intentional divergence
// from the pre-migration golden output: GA-stage built-in addons now carry
// an explicit "ga" label (they carried none before).
func TestGenerateAddonsAppliesGALabel(t *testing.T) {
	addonsDir := repoAddonsDir(t)
	versionFile := filepath.Join(addonsDir, "version_info")
	destDir := t.TempDir()

	if err := GenerateAddons(addonsDir, destDir, versionFile); err != nil {
		t.Fatalf("GenerateAddons failed: %v", err)
	}

	contents, err := os.ReadFile(filepath.Join(destDir, "vm-import-controller.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var res map[string]interface{}
	if err := yaml.Unmarshal(contents, &res); err != nil {
		t.Fatal(err)
	}
	labels := extractResourceLabels(res)
	if labels["addon.harvesterhci.io/ga"] != "true" {
		t.Errorf("expected vm-import-controller to carry the ga label, got labels: %v", labels)
	}
}

func TestValidateRealAddonsDir(t *testing.T) {
	addonsDir := repoAddonsDir(t)
	if errs := Validate(addonsDir); len(errs) > 0 {
		for _, err := range errs {
			t.Error(err)
		}
	}
}

func TestGenerateTemplatesQuotesVersions(t *testing.T) {
	addonsDir := repoAddonsDir(t)
	versionFile := filepath.Join(addonsDir, "version_info")
	destDir := t.TempDir()

	if err := GenerateTemplates(addonsDir, destDir, versionFile); err != nil {
		t.Fatalf("GenerateTemplates failed: %v", err)
	}

	contents, err := os.ReadFile(filepath.Join(destDir, TemplateFileName))
	if err != nil {
		t.Fatal(err)
	}

	metas, err := Discover(addonsDir)
	if err != nil {
		t.Fatal(err)
	}
	builtInCount := 0
	for _, meta := range metas {
		if meta.BuiltIn {
			builtInCount++
		}
	}

	versionCount := 0
	for _, line := range strings.Split(string(contents), "\n") {
		if !strings.HasPrefix(line, "      version: ") {
			continue
		}
		versionCount++
		if !strings.HasPrefix(line, `      version: "`) || !strings.HasSuffix(line, `"`) {
			t.Errorf("addon version is not explicitly quoted: %s", line)
		}
	}
	if versionCount != builtInCount {
		t.Errorf("generated template contains %d addon versions, want %d", versionCount, builtInCount)
	}
}

func TestGenerateTemplatesMatchesGolden(t *testing.T) {
	addonsDir := repoAddonsDir(t)
	versionFile := filepath.Join(addonsDir, "version_info")
	destDir := t.TempDir()

	if err := GenerateTemplates(addonsDir, destDir, versionFile); err != nil {
		t.Fatalf("GenerateTemplates failed: %v", err)
	}

	contents, err := os.ReadFile(filepath.Join(destDir, TemplateFileName))
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) == 0 {
		t.Fatal("generated template is empty")
	}

	golden, err := os.ReadFile("testdata/golden/rancherd-22-addons.yaml")
	if err != nil {
		t.Fatal(err)
	}

	// Render both templates with representative bootstrap data before
	// comparing their resource sets. This exercises runtime-only branches
	// such as .Vip and addon enablement while allowing the directory split to
	// reorder resources and add metadata-derived stage labels.
	bootstrapData := map[string]interface{}{
		"Vip": "192.0.2.1",
		"Addons": map[string]interface{}{
			"descheduler":                     map[string]interface{}{"Enabled": true},
			"harvester_pcidevices_controller": map[string]interface{}{"Enabled": true},
			"harvester_seeder":                map[string]interface{}{"Enabled": true},
			"harvester_vm_import_controller":  map[string]interface{}{"Enabled": true},
			"kubeovn_operator":                map[string]interface{}{"Enabled": true},
			"nvidia_driver_toolkit":           map[string]interface{}{"Enabled": true},
			"rancher_logging":                 map[string]interface{}{"Enabled": true},
			"rancher_monitoring":              map[string]interface{}{"Enabled": true},
		},
	}

	want := renderTemplateResources(t, golden, bootstrapData)
	got := renderTemplateResources(t, contents, bootstrapData)
	if !reflect.DeepEqual(want, got) {
		t.Errorf("generated rancherd addon resources differ from pre-migration golden output.\ngolden: %#v\ngenerated: %#v", want, got)
	}
}

func renderTemplateResources(t *testing.T, contents []byte, data map[string]interface{}) map[string]map[string]interface{} {
	t.Helper()

	tmpl, err := template.New(TemplateFileName).Parse(string(contents))
	if err != nil {
		t.Fatalf("template is not valid: %v", err)
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, data); err != nil {
		t.Fatalf("template cannot be rendered: %v", err)
	}

	resources := &addonResources{}
	if err := yaml.Unmarshal(rendered.Bytes(), resources); err != nil {
		t.Fatalf("rendered template is not valid YAML: %v", err)
	}

	result := make(map[string]map[string]interface{}, len(resources.Resources))
	for _, resource := range resources.Resources {
		metadata, ok := resource["metadata"].(map[string]interface{})
		if !ok {
			t.Fatalf("resource has malformed metadata: %v", resource)
		}
		name, ok := metadata["name"].(string)
		if !ok || name == "" {
			t.Fatalf("resource has no name: %v", resource)
		}
		if _, exists := result[name]; exists {
			t.Fatalf("template contains duplicate addon %q", name)
		}
		stripDerivedLabels(resource)
		result[name] = resource
	}
	return result
}
