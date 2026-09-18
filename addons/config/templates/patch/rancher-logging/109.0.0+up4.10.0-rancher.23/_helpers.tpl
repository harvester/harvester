{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "logging-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "logging-operator.fullname" -}}
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
Expand the name of the release.
*/}}
{{- define "logging-operator.releasename" -}}
{{- default .Release.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Provides the namespace the chart will be installed in using the builtin .Release.Namespace,
or, if provided, a manually overwritten namespace value.
*/}}
{{- define "logging-operator.namespace" -}}
{{- if .Values.namespaceOverride -}}
{{ .Values.namespaceOverride -}}
{{- else -}}
{{ .Release.Namespace }}
{{- end -}}
{{- end -}}


{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "logging-operator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "logging-operator.labels" -}}
app.kubernetes.io/name: {{ include "logging-operator.name" . }}
helm.sh/chart: {{ include "logging-operator.chart" . }}
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

{{- define "windowsEnabled" }}
{{- if not (kindIs "invalid" .Values.global.cattle.windows) }}
{{- if not (kindIs "invalid" .Values.global.cattle.windows.enabled) }}
{{- if .Values.global.cattle.windows.enabled }}
true
{{- end }}
{{- end }}
{{- end }}
{{- end }}

{{- define "windowsPathPrefix" -}}
{{- trimSuffix "/" (default "c:\\" .Values.global.rkeWindowsPathPrefix | replace "\\" "/" | replace "//" "/" | replace "c:" "C:") -}}
{{- end -}}

{{- define "windowsKubernetesFilter" -}}
{{- printf "kubernetes.%s" ((include "windowsPathPrefix" .) | replace ":" "" | replace "/" ".") -}}
{{- end -}}

{{- define "windowsInputTailMount" -}}
{{- (include "windowsPathPrefix" .) | replace "C:" "" -}}
{{- end -}}

{{/*
Set tolerations based on Kubernetes distribution and merge with values.yaml
*/}}
{{- define "customTolerations" -}}
{{- $isRKE := .Values.additionalLoggingSources.rke.enabled -}}
{{- $defaultTolerations := list -}}
{{- if $isRKE }}
  {{- $defaultTolerations = append $defaultTolerations (dict "key" "node-role.kubernetes.io/controlplane" "operator" "Exists" "effect" "NoSchedule") -}}
{{- else }}
  {{- $defaultTolerations = append $defaultTolerations (dict "key" "node-role.kubernetes.io/control-plane" "operator" "Exists" "effect" "NoSchedule") -}}
{{- end }}
{{- $defaultTolerations = append $defaultTolerations (dict "key" "node-role.kubernetes.io/etcd" "operator" "Exists" "effect" "NoExecute") -}}
{{- $userTolerations := .Values.tolerations | default list -}}
{{- $fluentbitTolerations := .Values.fluentbit.tolerations | default list -}}
{{- $mergedTolerations := concat $defaultTolerations $userTolerations $fluentbitTolerations -}}
{{- toYaml $mergedTolerations }}
{{- end -}}

{{/*
Set the controlplane selector based on kubernetes distribution
# harvester-patch to strip node-role.kubernetes.io/master and stick to node-role.kubernetes.io/control-plane
*/}}
{{- define "controlplaneSelector" -}}
{{- $isControlPlane := or .Values.additionalLoggingSources.rke2.enabled .Values.additionalLoggingSources.k3s.enabled -}}
{{- $defaultSelector := $isControlPlane | ternary (dict "node-role.kubernetes.io/control-plane" "true") (dict) -}}
{{ default $defaultSelector .Values.additionalLoggingSources.kubeAudit.nodeSelector | toYaml }}
{{- end -}}

{{/*
Set kube-audit file path prefix based on distribution
*/}}
{{- define "kubeAuditPathPrefix" -}}
{{- if .Values.additionalLoggingSources.rke.enabled -}}
{{ default "/var/log/kube-audit" .Values.additionalLoggingSources.kubeAudit.pathPrefix }}
{{- else if .Values.additionalLoggingSources.rke2.enabled -}}
{{ default "/var/lib/rancher/rke2/server/logs" .Values.additionalLoggingSources.kubeAudit.pathPrefix }}
{{- else -}}
{{ required "Directory PathPrefix of the kube-audit location is required" .Values.additionalLoggingSources.kubeAudit.pathPrefix }}
{{- end -}}
{{- end -}}

{{/*
Set kube-audit file name based on distribution
*/}}
{{- define "kubeAuditFilename" -}}
{{- if .Values.additionalLoggingSources.rke.enabled -}}
{{ default "audit-log.json" .Values.additionalLoggingSources.kubeAudit.auditFilename }}
{{- else if .Values.additionalLoggingSources.rke2.enabled -}}
{{ default "audit.log" .Values.additionalLoggingSources.kubeAudit.auditFilename }}
{{- else -}}
{{ required "Filename of the kube-audit log is required" .Values.additionalLoggingSources.kubeAudit.auditFilename }}
{{- end -}}
{{- end -}}

{{/*
A shared list of custom parsers for the vairous fluentbit pods rancher creates
*/}}
{{- define "logging-operator.parsers" -}}
[PARSER]
    Name              klog
    Format            regex
    Regex             ^(?<level>[IWEF])(?<timestamp>\d{4} \d{2}:\d{2}:\d{2}\.\d{6}) +?(?<thread_id>\d+) (?<filename>.+?):(?<linenumber>\d+)] (?<message>.+)
    Time_Key          timestamp
    Time_Format       %m%d %H:%M:%S.%L
    Time_System_timezone  On
[PARSER]
    Name              rancher
    Format            regex
    Regex             ^time="(?<timestamp>.+)" level=(?<level>.+) msg="(?<msg>.+)"$
    Time_Key          timestamp
    Time_Format       %FT%H:%M:%S%z
[PARSER]
    Name              etcd
    Format            json
    Time_Key          ts
    Time_Format       %FT%H:%M:%S.%L
{{- end -}}

{{/*
Set kubernetes log options if they are configured
*/}}
{{- define "requireFilterKubernetes" -}}
{{- if or .Values.fluentbit.filterKubernetes.Merge_Log .Values.fluentbit.filterKubernetes.Merge_Log_Key .Values.fluentbit.filterKubernetes.Merge_Log_Trim .Values.fluentbit.filterKubernetes.Merge_Parser -}}
true
{{- end -}}
{{- end -}}

{{/*Fluent Bit Image Repository */}}
{{- define "logging-operator.fluentbitImageRepository" -}}
{{- if .Values.debug -}}
{{ template "system_default_registry" . }}{{ .Values.images.fluentbit_debug.repository }}
{{- else -}}
{{ template "system_default_registry" . }}{{ .Values.images.fluentbit.repository }}
{{- end -}}
{{- end -}}

{{/*Fluent Bit Image Tag */}}
{{- define "logging-operator.fluentbitImageTag" -}}
{{- if .Values.debug -}}
{{ .Values.images.fluentbit_debug.tag }}
{{- else -}}
{{ .Values.images.fluentbit.tag }}
{{- end -}}
{{- end -}}

{{/*Fluent Bit Image */}}
{{- define "logging-operator.fluentbitImage" -}}
{{ template "logging-operator.fluentbitImageRepository" . }}:{{ template "logging-operator.fluentbitImageTag" . }}
{{- end -}}

{{/*
Formats the cluster domain as a suffix, e.g.:
.Values.clusterDomain == "", returns ""
.Values.clusterDomain == "cluster.local.", returns ".cluster.local."
*/}}
{{- define "logging-operator.clusterDomainAsSuffix" -}}
{{- if .Values.clusterDomain -}}
{{- printf ".%s" .Values.clusterDomain -}}
{{- end -}}
{{- end -}}

{{/* Implements logic to add the loggingRef field to custom loggings based on the cluster type */}}
{{- define "logging-operator.individualLoggingRef" -}}
{{- with .loggingRef -}}
loggingRef: {{ . }}
{{- end -}}
{{- end -}}

{{/* Implements logic to add fluentd spec fields to custom loggings based on the cluster type */}}
{{- define "logging-operator.individualFluentd" -}}
{{- if .fluentd -}}
{{- if .fluentd.scaling -}}
scaling:
    replicas: {{ .fluentd.scaling.replicas }}
{{- end }}
{{- with .fluentd.resources }}
resources: {{ toYaml . | nindent 2 }}
{{- end -}}
{{- end -}}
{{- end -}}

{{/* Implements logic to add fluentbit loggingRef field to custom loggings based on the cluster type */}}
{{- define "logging-operator.individualFluentbitLoggingRef" -}}
{{- with .loggingRef -}}
loggingRef: {{ . }}
{{- end -}}
{{- end -}}

{{/* Implements logic to add fluentbit spec fields to custom fluentBitAgents based on the cluster type */}}
{{- define "logging-operator.individualFluentbit" -}}
{{- with .resources }}
resources: {{ toYaml . | nindent 2 }}
{{- end -}}
{{- end -}}