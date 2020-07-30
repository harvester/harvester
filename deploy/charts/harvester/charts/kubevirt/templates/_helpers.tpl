{{/* vim: set filetype=mustache: */}}

{{/*
Expand the name of the chart, which is no longer than 63 chars.
We truncate at 63 chars as some Kubernetes naming policies are limited to this.
*/}}
{{- define "kubevirt.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{/*
Create a default fully qualified app name, which is no longer than 63 chars.
We truncate at 63 chars as some Kubernetes naming policies are limited to this.
*/}}
{{- define "kubevirt.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "kubevirt.chartref" -}}
{{- replace "+" "_" .Chart.Version | printf "%s-%s" .Chart.Name -}}
{{- end }}

{{/*
Generate immutable labels.
We use the immutable labels to select low-level components(like the Deployment selects the Pods).
*/}}
{{- define "kubevirt.immutableLabels" -}}
helm.sh/release: {{ .Release.Name }}
app.kubernetes.io/part-of: {{ template "kubevirt.name" . }}
{{- end }}

{{/*
Generate basic labels.
*/}}
{{- define "kubevirt.labels" -}}
app.kubernetes.io/managed-by: {{ default "helm" $.Release.Service | quote }}
helm.sh/chart: {{ template "kubevirt.chartref" . }}
app.kubernetes.io/version: {{ $.Chart.AppVersion | quote }}
{{ include "kubevirt.immutableLabels" . }}
{{- end }}
