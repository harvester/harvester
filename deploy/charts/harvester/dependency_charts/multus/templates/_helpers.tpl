{{/*
Expand the name of the chart.
*/}}
{{- define "multus.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "multus.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "multus.labels" -}}
helm.sh/chart: {{ include "multus.chart" . }}
{{ include "multus.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "multus.selectorLabels" -}}
app.kubernetes.io/name: {{ include "multus.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
