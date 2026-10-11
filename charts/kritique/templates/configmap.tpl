{{- if and .Values.configFile (not .Values.existingConfigMap) }}
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
    {{- toYaml .Values.configFile | nindent 4 }}
{{- end }}
