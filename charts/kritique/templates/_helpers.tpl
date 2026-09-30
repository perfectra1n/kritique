{{/*
Expand the name of the chart.
*/}}
{{- define "kritique.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name (truncated to the 63-char DNS limit).
*/}}
{{- define "kritique.fullname" -}}
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
Chart name and version as used by the chart label.
*/}}
{{- define "kritique.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "kritique.labels" -}}
helm.sh/chart: {{ include "kritique.chart" . }}
{{ include "kritique.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels shared by every role's pods.
*/}}
{{- define "kritique.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kritique.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Name of a role's Deployment: the release name for `all`, suffixed otherwise.
*/}}
{{- define "kritique.roleName" -}}
{{- if eq .role "all" -}}
{{- include "kritique.fullname" .root -}}
{{- else -}}
{{- printf "%s-%s" (include "kritique.fullname" .root) .role | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end }}

{{/*
Service account name to use.
*/}}
{{- define "kritique.serviceAccountName" -}}
{{- $name := tpl (.Values.serviceAccount.name | default "") $ -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "kritique.fullname" .) $name }}
{{- else }}
{{- default "default" $name }}
{{- end }}
{{- end }}

{{/*
Runner service account name.
*/}}
{{- define "kritique.runnerServiceAccountName" -}}
{{- $name := tpl (.Values.runner.serviceAccount.name | default "") $ -}}
{{- if .Values.runner.serviceAccount.create }}
{{- default (printf "%s-runner" (include "kritique.fullname" .)) $name }}
{{- else }}
{{- default "default" $name }}
{{- end }}
{{- end }}

{{/*
Container image reference. A digest pins immutably and wins when set;
otherwise it's repository:tag, with tag defaulting to the chart appVersion.
*/}}
{{- define "kritique.image" -}}
{{- if .Values.image.digest -}}
{{- printf "%s@%s" .Values.image.repository .Values.image.digest -}}
{{- else -}}
{{- printf "%s:%s" .Values.image.repository (.Values.image.tag | default .Chart.AppVersion) -}}
{{- end -}}
{{- end }}

{{/*
Runner image: the runner block's override, else the chart image.
*/}}
{{- define "kritique.runnerImage" -}}
{{- .Values.runner.image | default (include "kritique.image" .) -}}
{{- end }}

{{/*
ConfigMap the file is read from.
*/}}
{{- define "kritique.configMapName" -}}
{{- if .Values.config.existingConfigMap -}}
{{- tpl .Values.config.existingConfigMap $ -}}
{{- else -}}
{{- include "kritique.fullname" . -}}
{{- end -}}
{{- end }}

{{/*
Whether indexing is configured.
*/}}
{{- define "kritique.embeddingEnabled" -}}
{{- if .Values.embedding.model -}}true{{- end -}}
{{- end }}

{{/*
Secret holding the embedding API key: the existing one, or the chart's.
*/}}
{{- define "kritique.embeddingSecretName" -}}
{{- if .Values.embedding.existingSecret -}}
{{- tpl .Values.embedding.existingSecret $ -}}
{{- else if .Values.embedding.apiKey -}}
{{- printf "%s-embedding" (include "kritique.fullname" .) -}}
{{- end -}}
{{- end }}

{{- define "kritique.embeddingSecretKey" -}}
{{- if .Values.embedding.existingSecret -}}
{{- .Values.embedding.existingSecretKey -}}
{{- else -}}
api-key
{{- end -}}
{{- end }}

{{/*
Roles that are enabled, in a fixed order, so templates that range over them
render deterministically.
*/}}
{{- define "kritique.enabledRoles" -}}
{{- $out := list -}}
{{- range $r := list "all" "ingest" "worker" "web" -}}
{{- if (index $.Values.roles $r).enabled -}}
{{- $out = append $out $r -}}
{{- end -}}
{{- end -}}
{{- join "," $out -}}
{{- end }}

{{/*
Whether any enabled role works jobs (and so needs the worker RBAC, the
runner account and the owner/runner database secrets).
*/}}
{{/*
In-cluster URL runner Jobs are handed as HTTPS_PROXY, empty when the gateway
is off.
*/}}
{{- define "kritique.gatewayURL" -}}
{{- if .Values.gateway.enabled -}}
{{- printf "http://%s-gateway.%s.svc.cluster.local:%d" (include "kritique.fullname" .) .Release.Namespace (int .Values.gateway.port) -}}
{{- end -}}
{{- end }}

{{- define "kritique.hasWorker" -}}
{{- if or .Values.roles.all.enabled .Values.roles.worker.enabled -}}true{{- end -}}
{{- end }}

{{/*
Whether any enabled role serves webhooks.
*/}}
{{- define "kritique.hasIngest" -}}
{{- if or .Values.roles.all.enabled .Values.roles.ingest.enabled -}}true{{- end -}}
{{- end }}

{{/*
Whether any enabled role serves the dashboard.
*/}}
{{- define "kritique.hasWeb" -}}
{{- if or .Values.roles.web.enabled (and .Values.roles.all.enabled .Values.web.url) -}}true{{- end -}}
{{- end }}
