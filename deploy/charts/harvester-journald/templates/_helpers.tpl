{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "harvester-journald.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "harvester-journald.fullname" -}}
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
Provides the namespace the chart will be installed in using the builtin .Release.Namespace,
or, if provided, a manually overwritten namespace value.
*/}}
{{- define "harvester-journald.namespace" -}}
{{- if .Values.namespaceOverride -}}
{{ .Values.namespaceOverride -}}
{{- else -}}
{{ .Release.Namespace }}
{{- end -}}
{{- end -}}


{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "harvester-journald.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "harvester-journald.labels" -}}
app.kubernetes.io/name: {{ include "harvester-journald.name" . }}
helm.sh/chart: {{ include "harvester-journald.chart" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "system_default_registry" -}}
{{- if .Values.global.cattle.systemDefaultRegistry -}}
{{- printf "%s/" .Values.global.cattle.systemDefaultRegistry -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end -}}

{{/*Fluent Bit Image Repository */}}
{{- define "harvester-journald.fluentbitImageRepository" -}}
{{- if .Values.debug -}}
{{ template "system_default_registry" . }}{{ .Values.images.fluentbit_debug.repository }}
{{- else -}}
{{ template "system_default_registry" . }}{{ .Values.images.fluentbit.repository }}
{{- end -}}
{{- end -}}

{{/*Fluent Bit Image Tag */}}
{{- define "harvester-journald.fluentbitImageTag" -}}
{{- if .Values.debug -}}
{{ .Values.images.fluentbit_debug.tag }}
{{- else -}}
{{ .Values.images.fluentbit.tag }}
{{- end -}}
{{- end -}}

{{/*Fluent Bit Image */}}
{{- define "harvester-journald.fluentbitImage" -}}
{{ template "harvester-journald.fluentbitImageRepository" . }}:{{ template "harvester-journald.fluentbitImageTag" . }}
{{- end -}}
