{{- if not .Values.config.existingConfigMap }}
{{- if not .Values.config.file }}
{{- fail "config.file is required unless config.existingConfigMap names a ConfigMap with a config.yaml key" }}
{{- end }}
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "kritique.fullname" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "kritique.labels" . | nindent 4 }}
data:
  # NOT tpl'd: the file is kritique's own schema, validated at load with unknown
  # keys rejected; Helm passes it through as given.
  config.yaml: |
    {{- toYaml .Values.config.file | nindent 4 }}
{{- end }}
