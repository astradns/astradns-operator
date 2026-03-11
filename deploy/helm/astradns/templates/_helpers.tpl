{{/*
Expand the name of the chart.
*/}}
{{- define "astradns.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
*/}}
{{- define "astradns.fullname" -}}
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
{{- define "astradns.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "astradns.labels" -}}
helm.sh/chart: {{ include "astradns.chart" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: astradns
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}

{{/*
Operator labels
*/}}
{{- define "astradns.operator.labels" -}}
{{ include "astradns.labels" . }}
{{ include "astradns.operator.selectorLabels" . }}
{{- end }}

{{/*
Operator selector labels
*/}}
{{- define "astradns.operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "astradns.name" . }}-operator
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: operator
{{- end }}

{{/*
Agent labels
*/}}
{{- define "astradns.agent.labels" -}}
{{ include "astradns.labels" . }}
{{ include "astradns.agent.selectorLabels" . }}
{{- end }}

{{/*
Agent selector labels
*/}}
{{- define "astradns.agent.selectorLabels" -}}
app.kubernetes.io/name: {{ include "astradns.name" . }}-agent
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: agent
{{- end }}

{{/*
Namespace helper
*/}}
{{- define "astradns.namespace" -}}
{{- default .Release.Namespace .Values.namespace }}
{{- end }}

{{/*
Operator image
*/}}
{{- define "astradns.operator.image" -}}
{{- $tag := default .Chart.AppVersion .Values.operator.image.tag }}
{{- printf "%s:%s" .Values.operator.image.repository $tag }}
{{- end }}

{{/*
Agent image
*/}}
{{- define "astradns.agent.image" -}}
{{- $tag := default .Chart.AppVersion .Values.agent.image.tag }}
{{- printf "%s:%s" .Values.agent.image.repository $tag }}
{{- end }}

{{/*
ConfigMap name for agent configuration
*/}}
{{- define "astradns.agent.configmap" -}}
{{- printf "%s-agent-config" (include "astradns.fullname" .) }}
{{- end }}
