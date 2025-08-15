{{/*
Expand the name of the chart.
*/}}
{{- define "k8s-nim-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "k8s-nim-operator.fullname" -}}
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
{{- define "k8s-nim-operator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "k8s-nim-operator.labels" -}}
helm.sh/chart: {{ include "k8s-nim-operator.chart" . }}
{{ include "k8s-nim-operator.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "k8s-nim-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "k8s-nim-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Full image name with tag
*/}}
{{- define "k8s-nim-operator.fullimage" -}}
{{- .Values.operator.image.repository -}}:{{- .Values.operator.image.tag | default (printf "v%s" .Chart.AppVersion) -}}
{{- end }}

{{/*
Return merged operator args with zap logging args
*/}}
{{- define "k8s-nim-operator.fullArgs" -}}
{{- $op := .Values.operator | default dict -}}
{{- $log := $op.log | default dict -}}

{{- $zapDevel   := $log.development     | default false -}}
{{- $zapLevel   := $log.level           | default "info" -}}
{{- $zapStack   := $log.stacktraceLevel | default "error" -}}
{{- $zapEncoder := $log.encoder         | default "json" -}}

{{- $args := list -}}
{{- range ($op.args | default (list)) }}
  {{- $args = append $args . -}}
{{- end -}}

{{- $args = append $args (printf "--zap-devel=%v" $zapDevel) -}}
{{- $args = append $args (printf "--zap-log-level=%s" $zapLevel) -}}
{{- $args = append $args (printf "--zap-stacktrace-level=%s" $zapStack) -}}
{{- $args = append $args (printf "--zap-encoder=%s" $zapEncoder) -}}

{{- toYaml $args -}}
{{- end }}
