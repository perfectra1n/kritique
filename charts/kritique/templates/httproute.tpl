{{- if and .Values.httpRoute.enabled (include "kritique.hasIngest" .) -}}
{{- $route := .Values.httpRoute -}}
apiVersion: {{ $route.apiVersion | default "gateway.networking.k8s.io/v1" }}
kind: HTTPRoute
metadata:
  name: {{ include "kritique.fullname" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "kritique.labels" . | nindent 4 }}
    {{- with $route.labels }}
    {{- tpl (toYaml .) $ | nindent 4 }}
    {{- end }}
  {{- with $route.annotations }}
  annotations:
    {{- tpl (toYaml .) $ | nindent 4 }}
  {{- end }}
spec:
  {{- with $route.parentRefs }}
  parentRefs:
    {{- tpl (toYaml .) $ | nindent 4 }}
  {{- end }}
  {{- with $route.hostnames }}
  hostnames:
    {{- tpl (toYaml .) $ | nindent 4 }}
  {{- end }}
  rules:
    - backendRefs:
        - name: {{ include "kritique.fullname" . }}
          port: {{ .Values.service.port }}
      {{- with $route.matches }}
      matches:
        {{- tpl (toYaml .) $ | nindent 8 }}
      {{- end }}
{{- end }}
{{- if and .Values.httpRoute.web.enabled (include "kritique.hasWeb" .) }}
---
{{- $route := .Values.httpRoute.web -}}
apiVersion: {{ $route.apiVersion | default "gateway.networking.k8s.io/v1" }}
kind: HTTPRoute
metadata:
  name: {{ include "kritique.fullname" . }}-web
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "kritique.labels" . | nindent 4 }}
    {{- with $route.labels }}
    {{- tpl (toYaml .) $ | nindent 4 }}
    {{- end }}
  {{- with $route.annotations }}
  annotations:
    {{- tpl (toYaml .) $ | nindent 4 }}
  {{- end }}
spec:
  {{- with $route.parentRefs }}
  parentRefs:
    {{- tpl (toYaml .) $ | nindent 4 }}
  {{- end }}
  {{- with $route.hostnames }}
  hostnames:
    {{- tpl (toYaml .) $ | nindent 4 }}
  {{- end }}
  rules:
    - backendRefs:
        - name: {{ include "kritique.fullname" . }}-web
          port: {{ .Values.service.webPort }}
      {{- with $route.matches }}
      matches:
        {{- tpl (toYaml .) $ | nindent 8 }}
      {{- end }}
{{- end }}
