package render

import (
	"bytes"
	"os"
	"path/filepath"
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

// TestGenerateAddonsProducesBuiltInManifests is a smoke test for
// GenerateAddons: its only other caller is cmd/addon-generator, so this is
// the sole guard against the upgrade-manifest generator silently regressing.
// It checks generation succeeds and produces one flat manifest per built-in
// addon; per-addon namespace and derived-label correctness are already
// covered by TestValidateRealAddonsDir and the label tests, so they aren't
// re-asserted here.
func TestGenerateAddonsProducesBuiltInManifests(t *testing.T) {
	addonsDir := repoAddonsDir(t)
	versionFile := filepath.Join(addonsDir, "version_info")

	metas, err := Discover(addonsDir)
	if err != nil {
		t.Fatal(err)
	}
	var builtInCount int
	for _, meta := range metas {
		if meta.BuiltIn {
			builtInCount++
		}
	}
	if builtInCount == 0 {
		t.Fatal("no built-in addons discovered")
	}

	destDir := t.TempDir()
	if err := GenerateAddons(addonsDir, destDir, versionFile); err != nil {
		t.Fatalf("GenerateAddons failed: %v", err)
	}

	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != builtInCount {
		t.Errorf("generated %d addon manifests, want %d built-in addons", len(entries), builtInCount)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			t.Errorf("generated output contains category directory %q", entry.Name())
		}
		if filepath.Base(entry.Name()) != entry.Name() {
			t.Errorf("generated output filename %q is not flat", entry.Name())
		}
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

// TestGenerateTemplatesRendersWithBootstrapData is a smoke test for the
// rancherd bootstrap template: it is the only test that actually parses and
// executes the assembled Go template, exercising the runtime "{{ }}"
// enablement branches and .Vip substitution that
// TestGenerateTemplatesQuotesVersions (a plain string scan) does not reach.
// It does not compare the rendered resource set against Discover or assert
// per-resource enabled state; addon membership and versions are already
// covered by TestGenerateTemplatesQuotesVersions and the validate/label
// tests.
func TestGenerateTemplatesRendersWithBootstrapData(t *testing.T) {
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

	// Enabling every built-in addon here exercises the runtime enablement
	// branches and .Vip substitution. Keys are part of the rancherd
	// configuration contract and don't always match addon names (e.g.
	// harvester_vm_import_controller vs. vm-import-controller).
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

	renderTemplateResources(t, contents, bootstrapData)
}

// renderTemplateResources parses contents as a Go template, executes it with
// data, unmarshals the result as addonResources, and fails the test if the
// template doesn't parse/execute, the rendered output isn't valid YAML, a
// resource is missing a name, or two resources share a name.
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
		result[name] = resource
	}
	return result
}
