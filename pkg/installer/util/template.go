package util

import (
	"bytes"
	"strings"
	"text/template"
)

// RenderTemplate renders a template that has no template reference in it
func RenderTemplate(templ string, context interface{}) (string, error) {
	result := bytes.NewBufferString("")
	funcMap := template.FuncMap{
		"replace": func(old, new, s string) string {
			return strings.ReplaceAll(s, old, new)
		},
	}
	tmpl, err := template.New("").Funcs(funcMap).Parse(templ)
	if err != nil {
		return "", err
	}
	err = tmpl.Execute(result, context)
	if err != nil {
		return "", err
	}
	return result.String(), nil
}
