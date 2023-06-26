{{/*
Set the namespace of the chart.
Default set to kube-system
*/}}
{{- define "snapshot-validation-webhook.namespace" -}}
{{- if .Values.namespaceOverride -}}
{{- .Values.namespaceOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- .Release.Namespace | trunc 63 | trimSuffix "-" -}}
{{- end }}
{{- end }}

{{/*
Expand the name of the chart.
*/}}
{{- define "snapshot-validation-webhook.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "snapshot-validation-webhook.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "snapshot-validation-webhook.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Generate immutable labels.
We use the immutable labels to select low-level components(like the Deployment selects the Pods).
*/}}
{{- define "snapshot-validation-webhook.immutableLabels" -}}
helm.sh/release: {{ .Release.Name }}
app.kubernetes.io/part-of: {{ template "snapshot-validation-webhook.name" . }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "snapshot-validation-webhook.selectorLabels" -}}
app.kubernetes.io/name: {{ include "snapshot-validation-webhook.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Generate basic labels.
*/}}
{{- define "snapshot-validation-webhook.labels" -}}
app.kubernetes.io/managed-by: {{ default "helm" $.Release.Service | quote }}
helm.sh/chart: {{ template "snapshot-validation-webhook.chart" . }}
app.kubernetes.io/version: {{ $.Chart.AppVersion | quote }}
{{ include "snapshot-validation-webhook.immutableLabels" . }}
{{ include "snapshot-validation-webhook.selectorLabels" . }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "snapshot-validation-webhook.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "snapshot-validation-webhook.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
