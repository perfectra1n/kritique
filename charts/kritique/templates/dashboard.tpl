{{- if .Values.monitoring.dashboards.enabled }}
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "kritique.fullname" . }}-dashboard
  namespace: {{ .Values.monitoring.dashboards.namespace | default .Release.Namespace }}
  {{- with .Values.monitoring.dashboards.annotations }}
  annotations:
    {{- tpl (toYaml .) $ | nindent 4 }}
  {{- end }}
  labels:
    {{- include "kritique.labels" . | nindent 4 }}
    {{- if not .Values.monitoring.dashboards.grafanaOperator.enabled }}
    grafana_dashboard: "1"
    {{- end }}
    {{- with .Values.monitoring.dashboards.labels }}
    {{- tpl (toYaml .) $ | nindent 4 }}
    {{- end }}
data:
  kritique.json: |
{{ .Files.Get "dashboards/kritique.json" | nindent 4 }}
{{- end }}
