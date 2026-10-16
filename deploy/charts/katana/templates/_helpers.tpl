{{/*
Expand the name of the chart.
*/}}
{{- define "katana.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "katana.fullname" -}}
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

{{- define "katana.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "katana.labels" -}}
helm.sh/chart: {{ include "katana.chart" . }}
{{ include "katana.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "katana.selectorLabels" -}}
app.kubernetes.io/name: {{ include "katana.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app: katana
{{- end }}

{{- define "katana.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "katana.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "katana.secretName" -}}
{{- if .Values.secrets.existingSecret }}
{{- .Values.secrets.existingSecret }}
{{- else }}
{{- printf "%s-secrets" (include "katana.fullname" .) }}
{{- end }}
{{- end }}

{{- define "katana.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion }}
{{- printf "%s:%s" .Values.image.repository $tag }}
{{- end }}

{{- define "katana.caBundle" -}}
{{- if .Values.webhook.caBundle }}
{{- .Values.webhook.caBundle }}
{{- else if and .Values.webhook.createSecret .Values.webhook.tls.cert }}
{{- .Values.webhook.tls.cert }}
{{- end }}
{{- end }}
