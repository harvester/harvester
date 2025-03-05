{{/* Hook annotations */}}
{{- define "cdi.hook.annotations" -}}
  annotations:
    "helm.sh/hook": {{ .hookType }}
    "helm.sh/hook-delete-policy": before-hook-creation,hook-succeeded
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

{{/* Custom resource uninstalling hook annotations */}}
{{- define "cdi.crUninstallHook.annotations" -}}
{{ template "cdi.hook.annotations" merge (dict "hookType" "pre-delete") . }}
{{- end -}}

{{/* CRD uninstalling hook annotations */}}
{{- define "cdi.crdUninstallHook.annotations" -}}
{{ template "cdi.hook.annotations" merge (dict "hookType" "post-delete") . }}
{{- end -}}

{{/* Namespace modifying hook name */}}
{{- define "cdi.namespaceHook.name" -}}
{{ include "cdi.fullname" . }}-namespace-modify
{{- end }}

{{/* CRD upgrading hook name */}}
{{- define "cdi.crdUpgradeHook.name" -}}
{{ include "cdi.fullname" . }}-crd-upgrade
{{- end }}

{{/* Custom resource uninstalling hook name */}}
{{- define "cdi.crUninstallHook.name" -}}
{{ include "cdi.fullname" . }}-uninstall
{{- end }}

{{/* CRD uninstalling hook name */}}
{{- define "cdi.crdUninstallHook.name" -}}
{{ include "cdi.fullname" . }}-crd-uninstall
{{- end }}
