{{/*
Generate DockerConfigJson for image pull secrets
*/}}
{{- define "imagePullSecret" }}
{{- printf "{\"auths\":{\"%s\":{\"auth\":\"%s\"}}}" .Values.imagePullSecret.registry (printf "%s:%s" .Values.imagePullSecret.username .Values.imagePullSecret.password | b64enc) | b64enc }}
{{- end }}

{{/*
Create secret to access NGC Api
*/}}
{{- define "ngcApiSecret" }}
{{- printf "%s" .Values.ngcApiSecret.password | b64enc }}
{{- end }}

{{- define "generateGPUDPResource" }}
{{- if .migProfile -}}
nvidia.com/mig-{{ .migProfile }}: {{ .count | default 1 }}
{{- else -}}
nvidia.com/gpu: {{ .count | default 1 }}
{{- end -}}
{{- end }}

{{/*
Merge GPU resources with existing resources, avoiding duplication
*/}}
{{- define "mergeResources" }}
{{- $existingResources := .existingResources }}
{{- $gpuConfig := .gpuConfig }}
{{- $draEnabled := .draEnabled }}
{{- $mergedResources := dict }}
{{- if $existingResources }}
{{- $mergedResources = deepCopy $existingResources }}
{{- end }}
{{- if and $gpuConfig (not $draEnabled) }}
{{- $gpuResourceKey := "" }}
{{- if $gpuConfig.migProfile }}
{{- $gpuResourceKey = printf "nvidia.com/mig-%s" $gpuConfig.migProfile }}
{{- else }}
{{- $gpuResourceKey = "nvidia.com/gpu" }}
{{- end }}
{{- if not (hasKey $mergedResources $gpuResourceKey) }}
{{- $_ := set $mergedResources $gpuResourceKey ($gpuConfig.count | default 1) }}
{{- end }}
{{- end }}
{{- toYaml $mergedResources }}
{{- end }}

{{- define "generateGPUDRAResources" -}}
{{- $count := .count | default 1 -}}
{{- range $i := until ($count | int) }}
{{- if $.migProfile -}}
- resourceClaimTemplateName: mig-claim-template-{{ $.migProfile }}
{{- else -}}
- resourceClaimTemplateName: gpu-claim-template
{{- end -}}
{{- if lt (add $i 1) ($count | int) }}
{{ end }}
{{- end -}}
{{- end }}