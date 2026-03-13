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
Default image tag derived from chart appVersion.
*/}}
{{- define "astradns.defaultImageTag" -}}
{{- printf "v%s" .Chart.AppVersion -}}
{{- end }}

{{/*
Operator image
*/}}
{{- define "astradns.operator.image" -}}
{{- printf "ghcr.io/astradns/astradns-operator:%s" (include "astradns.defaultImageTag" .) -}}
{{- end }}

{{/*
Agent engine type with validation.
*/}}
{{- define "astradns.agent.engineType" -}}
{{- $engine := default "unbound" .Values.agent.engineType -}}
{{- if not (has $engine (list "unbound" "coredns" "powerdns" "bind")) -}}
{{- fail "agent.engineType must be one of: unbound, coredns, powerdns, bind" -}}
{{- end -}}
{{- $engine -}}
{{- end }}

{{/*
Agent image pinned to published engine variant.
*/}}
{{- define "astradns.agent.image" -}}
{{- $engine := include "astradns.agent.engineType" . -}}
{{- printf "ghcr.io/astradns/astradns-agent:%s-%s" (include "astradns.defaultImageTag" .) $engine -}}
{{- end }}

{{/*
Agent DNS listen address
*/}}
{{- define "astradns.agent.listenAddr" -}}
{{- $mode := default "hostPort" .Values.agent.network.mode -}}
{{- if eq $mode "linkLocal" -}}
{{- printf "%s:5353" .Values.agent.network.linkLocalIP -}}
{{- else if eq $mode "hostPort" -}}
0.0.0.0:5353
{{- else -}}
{{- fail "agent.network.mode must be one of: hostPort, linkLocal" -}}
{{- end -}}
{{- end }}

{{/*
Agent data-path mode helpers
*/}}
{{- define "astradns.agent.network.mode" -}}
{{- default "hostPort" .Values.agent.network.mode -}}
{{- end }}

{{/*
ConfigMap name for agent configuration
*/}}
{{- define "astradns.agent.configmap" -}}
{{- printf "%s-agent-config" (include "astradns.fullname" .) }}
{{- end }}

{{/*
Agent topology profile with validation and guardrails.
Returns "node-local" or "central".
*/}}
{{- define "astradns.agent.topology.profile" -}}
{{- $profile := "node-local" -}}
{{- with .Values.agent.topology -}}
{{- $profile = default "node-local" .profile -}}
{{- end -}}
{{- if not (has $profile (list "node-local" "central")) -}}
{{- fail "agent.topology.profile must be one of: node-local, central" -}}
{{- end -}}
{{- if and (eq $profile "central") (eq (include "astradns.agent.network.mode" $) "linkLocal") -}}
{{- fail "agent.topology.profile=central is incompatible with agent.network.mode=linkLocal — central mode uses a ClusterIP Service, not link-local addressing" -}}
{{- end -}}
{{- $profile -}}
{{- end }}

{{/*
DNS Service name for central mode
*/}}
{{- define "astradns.agent.dnsServiceName" -}}
{{- printf "%s-agent-dns" (include "astradns.fullname" .) -}}
{{- end }}

{{/*
DNS Service FQDN for central mode CoreDNS forwarding
*/}}
{{- define "astradns.agent.dnsServiceFQDN" -}}
{{- printf "%s.%s.svc.cluster.local" (include "astradns.agent.dnsServiceName" .) (include "astradns.namespace" .) -}}
{{- end }}

{{/*
Join host and port while preserving IPv6 bracket format.
*/}}
{{- define "astradns.joinHostPort" -}}
{{- $host := .host -}}
{{- $port := toString .port -}}
{{- if contains ":" $host -}}
{{- printf "[%s]:%s" $host $port -}}
{{- else -}}
{{- printf "%s:%s" $host $port -}}
{{- end -}}
{{- end }}

{{/*
Optional fixed ClusterIP for central DNS service.
*/}}
{{- define "astradns.agent.dnsServiceClusterIP" -}}
{{- $clusterIP := trim (default "" .Values.agent.dnsService.clusterIP) -}}
{{- if ne $clusterIP "" -}}
{{- $ipv4Pattern := "^((25[0-5]|2[0-4][0-9]|[01]?[0-9]?[0-9])\\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9]?[0-9])$" -}}
{{- $ipv6Pattern := "^[0-9a-fA-F:]+$" -}}
{{- if not (or (regexMatch $ipv4Pattern $clusterIP) (regexMatch $ipv6Pattern $clusterIP)) -}}
{{- fail "agent.dnsService.clusterIP must be a valid IPv4 or IPv6 address when set" -}}
{{- end -}}
{{- end -}}
{{- $clusterIP -}}
{{- end }}

{{/*
Central-mode forward target when fixed service ClusterIP is provided.
Returns empty string when dynamic service discovery should be used.
*/}}
{{- define "astradns.agent.centralForwardTarget" -}}
{{- $clusterIP := include "astradns.agent.dnsServiceClusterIP" . | trim -}}
{{- if ne $clusterIP "" -}}
{{- include "astradns.joinHostPort" (dict "host" $clusterIP "port" (default 53 .Values.agent.dnsService.port)) -}}
{{- end -}}
{{- end }}

{{/*
CoreDNS forward target — auto-computed based on topology profile.
Central mode prefers fixed DNS Service ClusterIP and falls back to Service FQDN.
Node-local uses the configured static target.
*/}}
{{- define "astradns.agent.corednsForwardTarget" -}}
{{- $profile := include "astradns.agent.topology.profile" . -}}
{{- if eq $profile "central" -}}
{{- $centralTarget := include "astradns.agent.centralForwardTarget" . | trim -}}
{{- if ne $centralTarget "" -}}
{{- $centralTarget -}}
{{- else -}}
{{- printf "%s:%s" (include "astradns.agent.dnsServiceFQDN" .) (toString (default 53 .Values.agent.dnsService.port)) -}}
{{- end -}}
{{- else -}}
{{- .Values.clusterDNS.forwardExternalToAstraDNS.forwardTarget -}}
{{- end -}}
{{- end }}

{{/*
Agent pod spec — shared between DaemonSet (node-local) and Deployment (central).
Outputs the full pod spec starting at indentation level 0.
Caller must render hostNetwork/dnsPolicy before including this template,
then apply nindent to position within the workload resource.
*/}}
{{- define "astradns.agent.podSpec" -}}
{{- $profile := include "astradns.agent.topology.profile" . -}}
{{- $networkMode := include "astradns.agent.network.mode" . }}
serviceAccountName: {{ include "astradns.fullname" . }}-agent
automountServiceAccountToken: {{ .Values.agent.serviceAccount.automountServiceAccountToken }}
{{- if .Values.agent.priorityClassName }}
priorityClassName: {{ .Values.agent.priorityClassName | quote }}
{{- end }}
{{- with .Values.imagePullSecrets }}
imagePullSecrets:
  {{- toYaml . | nindent 2 }}
{{- end }}
securityContext:
  runAsNonRoot: true
  seccompProfile:
    type: RuntimeDefault
containers:
  - name: agent
    image: {{ include "astradns.agent.image" . }}
    imagePullPolicy: {{ .Values.agent.imagePullPolicy }}
    env:
      - name: ASTRADNS_ENGINE_TYPE
        value: {{ include "astradns.agent.engineType" . | quote }}
      - name: ASTRADNS_CONFIG_PATH
        value: /etc/astradns/config
      - name: ASTRADNS_LISTEN_ADDR
        value: {{ include "astradns.agent.listenAddr" . | quote }}
      - name: ASTRADNS_ENGINE_ADDR
        value: "127.0.0.1:5354"
      - name: ASTRADNS_METRICS_ADDR
        value: ":9153"
      - name: ASTRADNS_HEALTH_ADDR
        value: ":8080"
      - name: ASTRADNS_LOG_MODE
        value: {{ .Values.agent.logMode | quote }}
      - name: ASTRADNS_LOG_SAMPLE_RATE
        value: {{ .Values.agent.logSampleRate | quote }}
      - name: ASTRADNS_PROXY_TIMEOUT
        value: {{ .Values.agent.proxyTimeout | quote }}
      - name: ASTRADNS_PROXY_CACHE_MAX_ENTRIES
        value: {{ .Values.agent.proxyCacheMaxEntries | quote }}
      - name: ASTRADNS_PROXY_CACHE_DEFAULT_TTL
        value: {{ .Values.agent.proxyCacheDefaultTTL | quote }}
      - name: ASTRADNS_ENGINE_CONN_POOL_SIZE
        value: {{ .Values.agent.engineConnectionPoolSize | quote }}
      - name: ASTRADNS_CONFIG_WATCH_DEBOUNCE
        value: {{ .Values.agent.configWatchDebounce | quote }}
      - name: ASTRADNS_ENGINE_RECOVERY_INTERVAL
        value: {{ .Values.agent.engineRecoveryInterval | quote }}
      - name: ASTRADNS_COMPONENT_ERROR_BUFFER
        value: {{ .Values.agent.componentErrorBuffer | quote }}
      - name: ASTRADNS_DIAGNOSTICS_ENABLED
        value: {{ .Values.agent.diagnostics.enabled | quote }}
      - name: ASTRADNS_DIAGNOSTICS_TARGETS
        value: {{ .Values.agent.diagnostics.targets | quote }}
      - name: ASTRADNS_DIAGNOSTICS_INTERVAL
        value: {{ .Values.agent.diagnostics.interval | quote }}
      - name: ASTRADNS_DIAGNOSTICS_TIMEOUT
        value: {{ .Values.agent.diagnostics.timeout | quote }}
      - name: ASTRADNS_TRACING_ENABLED
        value: {{ .Values.agent.tracing.enabled | quote }}
      - name: ASTRADNS_TRACING_ENDPOINT
        value: {{ .Values.agent.tracing.endpoint | quote }}
      - name: ASTRADNS_TRACING_INSECURE
        value: {{ .Values.agent.tracing.insecure | quote }}
      - name: ASTRADNS_TRACING_SAMPLE_RATIO
        value: {{ .Values.agent.tracing.sampleRatio | quote }}
      - name: ASTRADNS_TRACING_SERVICE_NAME
        value: {{ .Values.agent.tracing.serviceName | quote }}
      {{- if .Values.agent.metrics.bearerToken }}
      - name: ASTRADNS_METRICS_BEARER_TOKEN
        value: {{ .Values.agent.metrics.bearerToken | quote }}
      {{- end }}
    ports:
      {{- if and (eq $profile "node-local") (eq $networkMode "hostPort") }}
      - name: dns-udp
        containerPort: 5353
        hostPort: 5353
        protocol: UDP
      - name: dns-tcp
        containerPort: 5353
        hostPort: 5353
        protocol: TCP
      {{- else }}
      - name: dns-udp
        containerPort: 5353
        protocol: UDP
      - name: dns-tcp
        containerPort: 5353
        protocol: TCP
      {{- end }}
      - name: metrics
        containerPort: 9153
        protocol: TCP
      - name: health
        containerPort: 8080
        protocol: TCP
    livenessProbe:
      httpGet:
        path: /healthz
        port: health
      initialDelaySeconds: 10
      periodSeconds: 15
      timeoutSeconds: 3
      failureThreshold: 3
    readinessProbe:
      httpGet:
        path: /readyz
        port: health
      initialDelaySeconds: 5
      periodSeconds: 10
      timeoutSeconds: 3
      failureThreshold: 3
    securityContext:
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true
      capabilities:
        drop:
          - ALL
    resources:
      {{- toYaml .Values.agent.resources | nindent 6 }}
    volumeMounts:
      - name: config
        mountPath: /etc/astradns/config
        readOnly: true
      - name: engine-runtime
        mountPath: /var/run/astradns/engine
      - name: tmp
        mountPath: /tmp
terminationGracePeriodSeconds: 30
volumes:
  - name: config
    configMap:
      name: {{ include "astradns.agent.configmap" . }}
  - name: engine-runtime
    emptyDir: {}
  - name: tmp
    emptyDir: {}
{{- with .Values.agent.nodeSelector }}
nodeSelector:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- with .Values.agent.affinity }}
affinity:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- with .Values.agent.tolerations }}
tolerations:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- if eq $profile "central" }}
{{- with .Values.agent.deployment.topologySpreadConstraints }}
topologySpreadConstraints:
  {{- range . }}
  - maxSkew: {{ .maxSkew }}
    topologyKey: {{ .topologyKey }}
    whenUnsatisfiable: {{ .whenUnsatisfiable }}
    labelSelector:
      matchLabels:
        {{- include "astradns.agent.selectorLabels" $ | nindent 8 }}
  {{- end }}
{{- end }}
{{- end }}
{{- end }}
