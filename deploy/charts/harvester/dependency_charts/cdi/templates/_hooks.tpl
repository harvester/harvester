{{/* Hook annotations */}}
{{- define "cdi.hook.annotations" -}}
  annotations:
    "helm.sh/hook": {{ .hookType }}
    "helm.sh/hook-weight": {{ .hookWeight | quote }}
{{- end -}}

{{/* Namespace modifying hook annotations */}}
{{- define "cdi.namespaceHook.annotations" -}}
{{ template "cdi.hook.annotations" merge (dict "hookType" "pre-install") . }}
{{- end -}}

{{/* CRD upgrading hook annotations */}}
{{- define "cdi.crdUpgradeHook.annotations" -}}
{{ template "cdi.hook.annotations" merge (dict "hookType" "pre-upgrade") . }}
{{- end -}}

{{/* Namespace modifying hook name */}}
{{- define "cdi.namespaceHook.name" -}}
{{ include "cdi.fullname" . }}-namespace-modify
{{- end }}

{{/* CRD upgrading hook name */}}
{{- define "cdi.crdUpgradeHook.name" -}}
{{ include "cdi.fullname" . }}-crd-upgrade
{{- end }}
