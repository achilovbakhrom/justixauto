{{- define "justixauto.name" -}}{{ .Release.Name }}{{- end }}

{{- define "justixauto.labels" -}}
app.kubernetes.io/name: justixauto
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Values.image.tag | default .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
justixauto/environment: {{ .Values.environment }}
{{- end }}

{{- define "justixauto.selector" -}}
app.kubernetes.io/name: justixauto
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: api
{{- end }}

{{- define "justixauto.image" -}}
{{- if .Values.image.digest -}}{{ .Values.image.repository }}@{{ .Values.image.digest }}
{{- else -}}{{ .Values.image.repository }}:{{ required "image.tag or image.digest is required" .Values.image.tag }}{{- end -}}
{{- end }}

{{- define "justixauto.serviceAccount" -}}
{{- if .Values.serviceAccount.create }}{{ include "justixauto.name" . }}{{ else }}default{{ end }}
{{- end }}

{{/* Guards: production must pin the image by digest and keep MFA on; replicas share S3. */}}
{{- define "justixauto.guards" -}}
{{- if eq .Values.environment "prod" }}
{{- if .Values.config.mfaDisabled }}{{ fail "config.mfaDisabled must be false in prod" }}{{ end }}
{{- if not .Values.image.digest }}{{ fail "image.digest is required in prod (immutable image reference)" }}{{ end }}
{{- if not .Values.config.cookieSecure }}{{ fail "config.cookieSecure must be true in prod" }}{{ end }}
{{- end }}
{{- if and (gt (int .Values.replicaCount) 1) (ne .Values.config.fileStorage "s3") }}{{ fail "several replicas need config.fileStorage=s3" }}{{ end }}
{{- end }}

{{/* Environment shared by the API and the migration job. */}}
{{- define "justixauto.env" -}}
- name: DATABASE_URL
  valueFrom: { secretKeyRef: { name: {{ .Values.existingSecret }}, key: DATABASE_URL } }
- name: MFA_KEY
  valueFrom: { secretKeyRef: { name: {{ .Values.existingSecret }}, key: MFA_KEY } }
{{- end }}
