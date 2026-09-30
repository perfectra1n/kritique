{{- if .Values.networkPolicy.enabled }}
{{- $np := .Values.networkPolicy }}
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: {{ include "kritique.fullname" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "kritique.labels" . | nindent 4 }}
spec:
  podSelector:
    matchLabels:
      {{- include "kritique.selectorLabels" . | nindent 6 }}
  policyTypes:
    - Ingress
    - Egress
  ingress:
    # No `from`: the webhook and metrics ports are reachable by any peer;
    # lock down per cluster with your own policy if needed.
    - ports:
        - port: {{ .Values.service.port }}
          protocol: TCP
        - port: {{ .Values.service.metricsPort }}
          protocol: TCP
        {{- if include "kritique.hasWeb" . }}
        - port: {{ .Values.web.port }}
          protocol: TCP
        {{- end }}
    {{- if .Values.gateway.enabled }}
    # The gateway is for runner pods alone.
    - from:
        - podSelector:
            matchLabels:
              kritique.perfectra1n.github.io/role: runner
      ports:
        - port: {{ .Values.gateway.port }}
          protocol: TCP
    {{- end }}
  egress:
    {{- if $np.allowDNS }}
    - ports:
        - port: 53
          protocol: UDP
        - port: 53
          protocol: TCP
    {{- end }}
    - ports:
        {{- range $np.egressPorts }}
        - port: {{ . }}
          protocol: TCP
        {{- end }}
        - port: {{ $np.postgresPort }}
          protocol: TCP
    {{- if include "kritique.hasWorker" . }}
    # The worker talks to the API server to create and watch runner Jobs.
    - ports:
        - port: 443
          protocol: TCP
        - port: 6443
          protocol: TCP
    {{- end }}
{{- if include "kritique.hasWorker" . }}
---
# Runner pods: no ingress at all; egress to DNS, Postgres and, with the
# gateway, the gateway port on worker-capable pods, through which the git
# remote, the model endpoint and every allowed host are reached. Without
# the gateway, the egressPorts to anywhere, as before.
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: {{ include "kritique.fullname" . }}-runner
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "kritique.labels" . | nindent 4 }}
    app.kubernetes.io/component: runner
spec:
  podSelector:
    matchLabels:
      kritique.perfectra1n.github.io/role: runner
  policyTypes:
    - Ingress
    - Egress
  egress:
    {{- if $np.allowDNS }}
    - ports:
        - port: 53
          protocol: UDP
        - port: 53
          protocol: TCP
    {{- end }}
    - ports:
        {{- if not .Values.gateway.enabled }}
        {{- range $np.egressPorts }}
        - port: {{ . }}
          protocol: TCP
        {{- end }}
        {{- end }}
        - port: {{ $np.postgresPort }}
          protocol: TCP
    {{- if .Values.gateway.enabled }}
    - to:
        - podSelector:
            matchLabels:
              kritique.perfectra1n.github.io/gateway: "true"
      ports:
        - port: {{ .Values.gateway.port }}
          protocol: TCP
    {{- end }}
{{- end }}
{{- end }}
