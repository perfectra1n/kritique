{{- $roles := splitList "," (include "kritique.enabledRoles" .) -}}
{{- if not (include "kritique.enabledRoles" .) -}}
{{- fail "no role is enabled: set roles.all.enabled, or roles.ingest.enabled and roles.worker.enabled" -}}
{{- end -}}
{{- if and .Values.roles.all.enabled (or .Values.roles.ingest.enabled .Values.roles.worker.enabled) -}}
{{- fail "roles.all is the single-process topology; do not enable it together with roles.ingest or roles.worker" -}}
{{- end -}}
{{- if not .Values.database.app.existingSecret -}}
{{- fail "database.app.existingSecret is required: the Secret holding the application role's connection URI" -}}
{{- end -}}
{{- if and (include "kritique.hasWorker" .) (not .Values.database.owner.existingSecret) -}}
{{- fail "database.owner.existingSecret is required for roles.all / roles.worker: the leader runs migrations with it" -}}
{{- end -}}
{{- if and (include "kritique.hasWorker" .) (not .Values.database.runner.existingSecret) -}}
{{- fail "database.runner.existingSecret is required for roles.all / roles.worker: runner Jobs connect with it" -}}
{{- end -}}
{{- if and (include "kritique.embeddingEnabled" .) (not (include "kritique.embeddingSecretName" .)) -}}
{{- fail "embedding.model is set but no key is given: set embedding.apiKey or embedding.existingSecret" -}}
{{- end -}}
{{- if and .Values.roles.web.enabled (not .Values.web.url) -}}
{{- fail "web.url is required when roles.web.enabled is true" -}}
{{- end -}}
{{- if and .Values.web.url (not (regexMatch "^https?://" .Values.web.url)) -}}
{{- fail "web.url must be an http(s) URL" -}}
{{- end -}}
{{- range $role := $roles }}
{{- $spec := index $.Values.roles $role }}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "kritique.roleName" (dict "root" $ "role" $role) }}
  namespace: {{ $.Release.Namespace }}
  labels:
    {{- include "kritique.labels" $ | nindent 4 }}
    app.kubernetes.io/component: {{ $role }}
  {{- with $.Values.deploymentAnnotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  replicas: {{ $spec.replicas }}
  selector:
    matchLabels:
      {{- include "kritique.selectorLabels" $ | nindent 6 }}
      app.kubernetes.io/component: {{ $role }}
  template:
    metadata:
      labels:
        {{- include "kritique.labels" $ | nindent 8 }}
        app.kubernetes.io/component: {{ $role }}
        {{- if and (ne $role "worker") (ne $role "web") }}
        # Selected by the webhook Service.
        kritique.perfectra1n.github.io/hooks: "true"
        {{- end }}
        {{- if and (ne $role "ingest") (ne $role "web") $.Values.gateway.enabled }}
        # Selected by the gateway Service and the runner network policy.
        kritique.perfectra1n.github.io/gateway: "true"
        {{- end }}
        {{- if or (eq $role "web") (and (eq $role "all") $.Values.web.url) }}
        # Selected by the dashboard Service and the web-ingress network policy rule.
        kritique.perfectra1n.github.io/web: "true"
        {{- end }}
        {{- with $.Values.podLabels }}
        {{- tpl (toYaml .) $ | nindent 8 }}
        {{- end }}
      {{- with $.Values.podAnnotations }}
      annotations:
        {{- tpl (toYaml .) $ | nindent 8 }}
      {{- end }}
    spec:
      # The Service is named after the release; the kubelet's legacy link
      # variables would otherwise inject KRITIQUE_PORT and collide with config.
      enableServiceLinks: false
      {{- with $.Values.imagePullSecrets }}
      imagePullSecrets:
        {{- tpl (toYaml .) $ | nindent 8 }}
      {{- end }}
      serviceAccountName: {{ include "kritique.serviceAccountName" $ | quote }}
      automountServiceAccountToken: {{ $.Values.serviceAccount.automount }}
      {{- with $.Values.priorityClassName }}
      priorityClassName: {{ tpl . $ | quote }}
      {{- end }}
      terminationGracePeriodSeconds: {{ $.Values.terminationGracePeriodSeconds }}
      securityContext:
        {{- tpl (toYaml $.Values.podSecurityContext) $ | nindent 8 }}
      containers:
        - name: kritique
          image: {{ include "kritique.image" $ | quote }}
          imagePullPolicy: {{ $.Values.image.pullPolicy }}
          args:
            - --role
            - {{ $role }}
          securityContext:
            {{- tpl (toYaml $.Values.securityContext) $ | nindent 12 }}
          env:
            - name: KRITIQUE_CONFIG_FILE
              value: /etc/kritique/config.yaml
            - name: KRITIQUE_CONFIG_RELOAD_INTERVAL
              value: {{ tpl (toString $.Values.config.reloadInterval) $ | quote }}
            - name: KRITIQUE_LOG_LEVEL
              value: {{ tpl $.Values.config.logLevel $ | quote }}
            - name: KRITIQUE_LOG_FORMAT
              value: {{ tpl $.Values.config.logFormat $ | quote }}
            - name: KRITIQUE_ADDR
              value: {{ printf ":%d" (int $.Values.service.port) | quote }}
            - name: KRITIQUE_METRICS_ADDR
              value: {{ printf ":%d" (int $.Values.service.metricsPort) | quote }}
            - name: KRITIQUE_DATABASE_APP_ROLE
              value: {{ $.Values.database.app.role | quote }}
            - name: KRITIQUE_DATABASE_RUNNER_ROLE
              value: {{ $.Values.database.runner.role | quote }}
            - name: KRITIQUE_DATABASE_URL
              valueFrom:
                secretKeyRef:
                  name: {{ tpl $.Values.database.app.existingSecret $ | quote }}
                  key: {{ $.Values.database.app.key | quote }}
            {{- with $.Values.dashboard.keySecret.name }}
            - name: KRITIQUE_DASHBOARD_KEY
              valueFrom:
                secretKeyRef:
                  name: {{ tpl . $ | quote }}
                  key: {{ $.Values.dashboard.keySecret.key | quote }}
            {{- end }}
            {{- with $.Values.dashboard.oldKeysSecret.name }}
            - name: KRITIQUE_DASHBOARD_OLD_KEYS
              valueFrom:
                secretKeyRef:
                  name: {{ tpl . $ | quote }}
                  key: {{ $.Values.dashboard.oldKeysSecret.key | quote }}
            {{- end }}
            {{- if and (ne $role "ingest") (ne $role "web") }}
            - name: KRITIQUE_DATABASE_OWNER_URL
              valueFrom:
                secretKeyRef:
                  name: {{ tpl $.Values.database.owner.existingSecret $ | quote }}
                  key: {{ $.Values.database.owner.key | quote }}
            - name: KRITIQUE_EXECUTOR
              value: kubernetes
            - name: KRITIQUE_RUNNER_IMAGE
              value: {{ include "kritique.runnerImage" $ | quote }}
            - name: KRITIQUE_RUNNER_SERVICE_ACCOUNT
              value: {{ include "kritique.runnerServiceAccountName" $ | quote }}
            - name: KRITIQUE_RUNNER_DATABASE_SECRET
              value: {{ tpl $.Values.database.runner.existingSecret $ | quote }}
            - name: KRITIQUE_RUNNER_DATABASE_SECRET_KEY
              value: {{ $.Values.database.runner.key | quote }}
            - name: KRITIQUE_RUNNER_TTL
              value: {{ tpl (toString $.Values.runner.ttl) $ | quote }}
            {{- with $.Values.runner.runtimeClassName }}
            - name: KRITIQUE_RUNNER_RUNTIME_CLASS
              value: {{ tpl . $ | quote }}
            {{- end }}
            - name: KRITIQUE_GATEWAY_ADDR
              value: {{ printf ":%d" (int $.Values.gateway.port) | quote }}
            {{- with include "kritique.gatewayURL" $ }}
            - name: KRITIQUE_GATEWAY_URL
              value: {{ . | quote }}
            {{- end }}
            - name: KRITIQUE_REVIEW_WORKERS
              value: {{ $.Values.config.reviewWorkers | quote }}
            - name: KRITIQUE_INDEX_WORKERS
              value: {{ $.Values.config.indexWorkers | quote }}
            {{- if include "kritique.embeddingEnabled" $ }}
            - name: KRITIQUE_EMBED_BASE_URL
              value: {{ tpl $.Values.embedding.baseUrl $ | quote }}
            - name: KRITIQUE_EMBED_MODEL
              value: {{ tpl $.Values.embedding.model $ | quote }}
            - name: KRITIQUE_EMBED_DIMS
              value: {{ $.Values.embedding.dims | quote }}
            - name: KRITIQUE_EMBED_API_KEY
              valueFrom:
                secretKeyRef:
                  name: {{ include "kritique.embeddingSecretName" $ | quote }}
                  key: {{ include "kritique.embeddingSecretKey" $ | quote }}
            - name: KRITIQUE_EMBED_MAX_BATCH
              value: {{ $.Values.embedding.maxBatch | quote }}
            - name: KRITIQUE_EMBED_MAX_BATCH_CHARS
              value: {{ $.Values.embedding.maxBatchChars | quote }}
            - name: KRITIQUE_EMBED_MAX_ITEM_CHARS
              value: {{ $.Values.embedding.maxItemChars | quote }}
            {{- if $.Values.embedding.reindexOnModelChange }}
            - name: KRITIQUE_REINDEX_ON_MODEL_CHANGE
              value: "true"
            {{- end }}
            {{- end }}
            {{- end }}
            {{- if or (eq $role "web") (and (eq $role "all") $.Values.web.url) }}
            - name: KRITIQUE_WEB_URL
              value: {{ tpl $.Values.web.url $ | quote }}
            - name: KRITIQUE_WEB_ADDR
              value: {{ printf ":%d" (int $.Values.web.port) | quote }}
            {{- end }}
            {{- with $.Values.config.extraEnv }}
            {{- tpl (toYaml .) $ | nindent 12 }}
            {{- end }}
          ports:
            {{- if and (ne $role "worker") (ne $role "web") }}
            - name: http
              containerPort: {{ $.Values.service.port }}
              protocol: TCP
            {{- end }}
            - name: metrics
              containerPort: {{ $.Values.service.metricsPort }}
              protocol: TCP
            {{- if and (ne $role "ingest") (ne $role "web") $.Values.gateway.enabled }}
            - name: gateway
              containerPort: {{ $.Values.gateway.port }}
              protocol: TCP
            {{- end }}
            {{- if or (eq $role "web") (and (eq $role "all") $.Values.web.url) }}
            - name: web
              containerPort: {{ $.Values.web.port }}
              protocol: TCP
            {{- end }}
          livenessProbe:
            {{- tpl (toYaml $.Values.livenessProbe) $ | nindent 12 }}
          readinessProbe:
            {{- tpl (toYaml $.Values.readinessProbe) $ | nindent 12 }}
          {{- with $.Values.startupProbe }}
          startupProbe:
            {{- tpl (toYaml .) $ | nindent 12 }}
          {{- end }}
          {{- with (default $.Values.resources $spec.resources) }}
          resources:
            {{- tpl (toYaml .) $ | nindent 12 }}
          {{- end }}
          volumeMounts:
            - name: config
              mountPath: /etc/kritique
              readOnly: true
            {{- range $.Values.secretMounts }}
            - name: {{ .name }}
              mountPath: {{ tpl .mountPath $ }}
              readOnly: true
            {{- end }}
            {{- with $.Values.volumeMounts }}
            {{- tpl (toYaml .) $ | nindent 12 }}
            {{- end }}
      volumes:
        - name: config
          configMap:
            name: {{ include "kritique.configMapName" $ | quote }}
        {{- range $.Values.secretMounts }}
        - name: {{ .name }}
          secret:
            secretName: {{ tpl .secretName $ | quote }}
        {{- end }}
        {{- with $.Values.volumes }}
        {{- tpl (toYaml .) $ | nindent 8 }}
        {{- end }}
      {{- with $.Values.nodeSelector }}
      nodeSelector:
        {{- tpl (toYaml .) $ | nindent 8 }}
      {{- end }}
      {{- with $.Values.affinity }}
      affinity:
        {{- tpl (toYaml .) $ | nindent 8 }}
      {{- end }}
      {{- with $.Values.tolerations }}
      tolerations:
        {{- tpl (toYaml .) $ | nindent 8 }}
      {{- end }}
{{- end }}
