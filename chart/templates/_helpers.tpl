{{/*
Expand the name of the chart.
*/}}
{{- define "splunk-sli-provider.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "splunk-sli-provider.fullname" -}}
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
{{- define "splunk-sli-provider.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "splunk-sli-provider.labels" -}}
helm.sh/chart: {{ include "splunk-sli-provider.chart" . }}
{{ include "splunk-sli-provider.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "splunk-sli-provider.selectorLabels" -}}
app.kubernetes.io/name: {{ include "splunk-sli-provider.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "splunk-sli-provider.serviceAccountName" -}}
keptn-splunk-sli-provider
{{- end }}


{{/*
Helper functions for auto detecting Splunk namespace
*/}}
{{- define "splunk-sli-provider.namespace" -}}
    {{- /* Check if autodetect is set */ -}}
    {{- if and (.Values.splunk.autodetect) (eq .Values.splunk.namespace "") }}
        {{- $detectedSplunkServer := list }}
        
        {{- /* Find splunk-server service */ -}}
        {{- $services := lookup "v1" "Service" "" "" }}
        {{- range $index, $srv := $services.items }}
            {{- if (eq "splunk-server" $srv.metadata.name ) }}
                {{- $detectedSplunkServer = append $detectedSplunkServer $srv }}
            {{- end }}
        {{- end }}

        {{- if eq (len $detectedSplunkServer) 0 }}
            {{- fail (printf "Unable to detect Splunk in the kubernetes cluster!") }}
        {{- else if gt (len $detectedSplunkServer) 1 }}
            {{- fail (printf "Detected more than one Splunk installation: %+v" $detectedSplunkServer) }}
        {{ else }}
            {{- (index $detectedSplunkServer 0).metadata.namespace }}
        {{- end }}
    {{- else }}
        {{- .Values.splunk.namespace }}
    {{- end }}
{{- end }}

{{/*
Helper functions for auto detecting Splunk alertmanager namespace
*/}}
{{- define "splunk-am-service.namespace" -}}
    {{- /* Check if autodetect is set */ -}}
    {{- if and (.Values.splunk.autodetect_am) (eq .Values.splunk.namespace_am "") }}
        {{- $detectedSplunkServer := list }}

        {{- /* Find splunk-alertmanager service */ -}}
        {{- $services := lookup "v1" "Service" "" "" }}
        {{- range $index, $srv := $services.items }}
            {{- if (eq "splunk-alertmanager" $srv.metadata.name ) }}
                {{- $detectedSplunkServer = append $detectedSplunkServer $srv }}
            {{- end }}
        {{- end }}

        {{- if eq (len $detectedSplunkServer) 0 }}
            {{- fail (printf "Unable to detect Splunk Alertmanager in the kubernetes cluster!") }}
        {{- else if gt (len $detectedSplunkServer) 1 }}
            {{- fail (printf "Detected more than one Splunk Alertmanager installation: %+v" $detectedSplunkServer) }}
        {{- else }}
            {{- (index $detectedSplunkServer 0).metadata.namespace }}
        {{- end }}
    {{- else }}
        {{- .Values.splunk.namespace_am }}
    {{- end }}
{{- end }}

{{- define "splunk-sli-provider.endpoint" }}
     {{- if and (.Values.splunk.autodetect) (eq .Values.splunk.endpoint "") }}
        {{- printf "%s.%s.%s" "http://splunk-server" (include  "splunk-sli-provider.namespace" .) "svc.cluster.local:80" }}
     {{- else }}
        {{- .Values.splunk.endpoint }}
     {{- end }}
{{- end }}
