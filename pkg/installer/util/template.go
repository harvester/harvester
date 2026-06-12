package util

import (
	"bytes"
	"text/template"
)

// RenderTemplate renders a template that has no template reference in it
func RenderTemplate(templ string, context interface{}) (string, error) {
	result := bytes.NewBufferString("")
	tmpl, err := template.New("").Parse(templ)
	if err != nil {
		return "", err
	}
	err = tmpl.Execute(result, context)
	if err != nil {
		return "", err
	}
	return result.String(), nil
}
