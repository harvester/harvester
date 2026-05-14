package config

import (
	"embed"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/harvester/harvester/pkg/installer/util"
)

const (
	templateFolder = "templates"
)

var (
	//go:embed templates/*
	templFS embed.FS
)

// render renders a template in the package `templates` folder. The template
// files are embedded in build-time.
func render(template string, context interface{}) (string, error) {
	templBytes, err := templFS.ReadFile(filepath.Join(templateFolder, template))
	if err != nil {
		return "", err
	}
	return util.RenderTemplate(string(templBytes), context)
}

func escapeMustaches(input string) string {
	re := regexp.MustCompile(`\{\{.*?\}\}`)
	return re.ReplaceAllStringFunc(input, func(match string) string {
		return fmt.Sprintf("{{ `%s` }}", match)
	})
}
