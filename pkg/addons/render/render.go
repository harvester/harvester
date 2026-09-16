package render

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"
)

// TemplateFileName is the name of the assembled rancherd bootstrap template
// consumed by the installer, matching the pre-consolidation file name.
const TemplateFileName = "rancherd-22-addons.yaml"

// GenerateTemplates assembles all built-in addon-template.yaml fragments
// under addonsDir, renders their "<< >>" version placeholders from
// versionFilePath, and writes the result to destPath/TemplateFileName. The
// literal "{{ }}" runtime toggles are left unresolved for the installer to
// render at bootstrap time.
func GenerateTemplates(addonsDir, destPath, versionFilePath string) error {
	metas, err := Discover(addonsDir)
	if err != nil {
		return err
	}

	assembled, err := AssembleTemplate(addonsDir, metas)
	if err != nil {
		return err
	}

	rendered, err := renderVersionPlaceholders(assembled, versionFilePath)
	if err != nil {
		return fmt.Errorf("error rendering template: %w", err)
	}

	if err := os.MkdirAll(destPath, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(destPath, TemplateFileName), rendered, 0644)
}

// GenerateAddons assembles built-in addon fragments, fully renders them
// (both "<< >>" version placeholders and "{{ }}" runtime toggles, the latter
// falling back to their disabled default since no rancherd bootstrap data is
// supplied), and writes one disabled Addon manifest per built-in addon to
// destPath/<name>.yaml. This mirrors the manifests bundled with the upgrade
// path.
func GenerateAddons(addonsDir, destPath, versionFilePath string) error {
	metas, err := Discover(addonsDir)
	if err != nil {
		return err
	}

	assembled, err := AssembleTemplate(addonsDir, metas)
	if err != nil {
		return err
	}

	versionRendered, err := renderVersionPlaceholders(assembled, versionFilePath)
	if err != nil {
		return fmt.Errorf("error rendering template: %w", err)
	}

	tmpl, err := template.New("").Parse(string(versionRendered))
	if err != nil {
		return fmt.Errorf("error parsing assembled template: %w", err)
	}

	fullyRendered, err := renderWithVersionMap(tmpl, versionFilePath)
	if err != nil {
		return fmt.Errorf("error rendering runtime toggles: %w", err)
	}

	resources := &addonResources{}
	if err := yaml.Unmarshal(fullyRendered, resources); err != nil {
		return fmt.Errorf("error unmarshalling resources: %w", err)
	}

	if err := os.MkdirAll(destPath, 0755); err != nil {
		return err
	}

	for _, res := range resources.Resources {
		metadata, ok := res["metadata"].(map[string]interface{})
		if !ok {
			logrus.Errorf("skipping resource since metadata is missing or malformed: %v", res)
			continue
		}
		name, ok := metadata["name"].(string)
		if !ok {
			logrus.Errorf("skipping resource since name is missing in metadata: %v", res)
			continue
		}

		contents, err := yaml.Marshal(res)
		if err != nil {
			return fmt.Errorf("error marshalling addon %s: %w", name, err)
		}
		fileName := fmt.Sprintf("%s.yaml", name)
		if err := os.WriteFile(filepath.Join(destPath, fileName), contents, 0644); err != nil {
			return fmt.Errorf("error writing %s: %w", fileName, err)
		}
	}
	return nil
}

type addonResources struct {
	Resources []map[string]interface{} `json:"resources,omitempty"`
}

// renderVersionPlaceholders renders only the "<< >>" delimited version
// placeholders in templateText, leaving "{{ }}" runtime toggles as literal
// text. It fails if any placeholder is left unresolved.
func renderVersionPlaceholders(templateText, versionFilePath string) ([]byte, error) {
	tmpl, err := template.New("").Delims("<<", ">>").Parse(templateText)
	if err != nil {
		return nil, err
	}

	rendered, err := renderWithVersionMap(tmpl, versionFilePath)
	if err != nil {
		return nil, err
	}

	if strings.Contains(string(rendered), "<no value>") {
		return nil, fmt.Errorf("some templates are not effectively rendered, search keyword: <no value>:\n%s", rendered)
	}
	return rendered, nil
}

func renderWithVersionMap(tmpl *template.Template, versionFilePath string) ([]byte, error) {
	envMap, err := loadVersionInfo(versionFilePath)
	if err != nil {
		return nil, err
	}

	result := bytes.NewBufferString("")
	if err := tmpl.Execute(result, envMap); err != nil {
		return nil, err
	}
	return result.Bytes(), nil
}

// loadVersionInfo reads a bash-style "KEY=\"value\"" file (addons/version_info)
// into a string map for template rendering.
func loadVersionInfo(versionFilePath string) (map[string]string, error) {
	f, err := os.Open(versionFilePath)
	if err != nil {
		return nil, fmt.Errorf("error opening version file: %w", err)
	}
	defer f.Close()

	result := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			// ignore lines without "=", e.g. the "#!/bin/bash" shebang.
			continue
		}
		result[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), "\"")
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading version file: %w", err)
	}
	return result, nil
}
