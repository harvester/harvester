package bench

import (
	"text/template"
)

var tmpl = `
{{if gt (len .ErrMsg) 0}}Error message:         {{range $errmsg, $num := .ErrMsg}} {{$errmsg}}:{{$num}} {{end}} [errmsg:count]{{end}}
Status Codes:           {{range $code, $num := .StatusCodes}} {{$num}}:{{$code}} {{end}} [count:code]
Concurrency Level:      {{.Concurrency}}
Time taken for tests:   {{.Duration}}
Complete requests:      {{.CompleteRequest}}
Failed requests:        {{.Failed}}
Total Read Data:        {{.TotalRead}} bytes
Total Read body         {{.TotalBody}} bytes
Total Write Body        {{.TotalWriteBody}} bytes
Requests per second:    {{.Tps}} [#/sec] (mean)
Time per request:       {{.Mean}} [ms] (mean)
Time per request:       {{.AllMean}} [ms] (mean, across all concurrent requests)
Transfer rate:          {{.Kbs}} [Kbytes/sec] received
Percentage of the requests served within a certain time (ms)
  50%    {{.Percentage55}}
  66%    {{.Percentage66}}
  75%    {{.Percentage75}}
  80%    {{.Percentage80}}
  90%    {{.Percentage90}}
  95%    {{.Percentage95}}
  98%    {{.Percentage98}}
  99%    {{.Percentage99}}
 100%    {{.Percentage100}}
`

// 后面要加新的显示格式，只要加新的模版就行
func newTemplate() *template.Template {
	return template.Must(template.New("text").Parse(tmpl))
}
