{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "harvester-journal.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "harvester-journal.fullname" -}}
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
{{- define "harvester-journal.namespace" -}}
{{- if .Values.namespaceOverride -}}
{{ .Values.namespaceOverride -}}
{{- else -}}
{{ .Release.Namespace }}
{{- end -}}
{{- end -}}


{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "harvester-journal.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "harvester-journal.labels" -}}
app.kubernetes.io/name: {{ include "harvester-journal.name" . }}
helm.sh/chart: {{ include "harvester-journal.chart" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
A shared list of custom parsers for the vairous fluentbit pods rancher creates
*/}}
{{- define "harvester-journal.parsers" -}}
[PARSER]
    Name              klog
    Format            regex
    Regex             ^(?<level>[IWEF])(?<timestamp>\d{4} \d{2}:\d{2}:\d{2}).\d{6} +?(?<thread_id>\d+) (?<filename>.+):(?<linenumber>\d+)] (?<message>.+)
    Time_Key          timestamp
    Time_Format       %m%d %T

[PARSER]
    Name              rancher
    Format            regex
    Regex             ^time="(?<timestamp>.+)" level=(?<level>.+) msg="(?<msg>.+)"$
    Time_Key          timestamp
    Time_Format       %FT%H:%M:%S

[PARSER]
    Name              etcd
    Format            json
    Time_Key          timestamp
    Time_Format       %FT%H:%M:%S.%L
{{- end -}}

{{- define "system_default_registry" -}}
{{- if .Values.global.cattle.systemDefaultRegistry -}}
{{- printf "%s/" .Values.global.cattle.systemDefaultRegistry -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end -}}

{{/*Fluent Bit Image Repository */}}
{{- define "harvester-journal.fluentbitImageRepository" -}}
{{- if .Values.debug -}}
{{ template "system_default_registry" . }}{{ .Values.images.fluentbit_debug.repository }}
{{- else -}}
{{ template "system_default_registry" . }}{{ .Values.images.fluentbit.repository }}
{{- end -}}
{{- end -}}

{{/*Fluent Bit Image Tag */}}
{{- define "harvester-journal.fluentbitImageTag" -}}
{{- if .Values.debug -}}
{{ .Values.images.fluentbit_debug.tag }}
{{- else -}}
{{ .Values.images.fluentbit.tag }}
{{- end -}}
{{- end -}}

{{/*Fluent Bit Image */}}
{{- define "harvester-journal.fluentbitImage" -}}
{{ template "harvester-journal.fluentbitImageRepository" . }}:{{ template "harvester-journal.fluentbitImageTag" . }}
{{- end -}}
