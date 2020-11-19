package export

import (
	"text/template"
)

var shortTmpl = `
curl{{if gt (len .Method) 0}} -X {{.Method}}{{end}}{{range $_, $header := .Header}} -H {{$header}}{{end}}{{if gt (len .Data) 0}} -d {{.Data}}{{end}}{{range $_, $formData := .FormData}} -F {{$formData}}{{end}} {{.URL}}
`

var longTmpl = `
curl{{if gt (len .Method) 0}} --request {{.Method}}{{end}}{{range $_, $header := .Header}} --header {{$header}}{{end}}{{if gt (len .Data) 0}} --data {{.Data}}{{end}}{{range $_, $formData := .FormData}} --form {{$formData}}{{end}} --url {{.URL}}
`

func newTemplate(long bool) *template.Template {
	tmpl := shortTmpl
	if long {
		tmpl = longTmpl
	}

	return template.Must(template.New("generate-curl").Parse(tmpl))
}
