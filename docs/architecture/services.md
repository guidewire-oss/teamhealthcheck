# Background & External Services

Beyond the HTTP request path, the backend integrates with an email provider and an
OpenTelemetry pipeline.

## Email

`backend/infrastructure/email/` defines a small `Sender` interface with two
implementations, selected once at startup in `main.go` with a strict precedence:

```
AWS SES  →  SMTP  →  disabled
```

1. `email.LoadSESConfig()` returns non-nil when both `AWS_SES_REGION` and
   `SES_FROM_ADDRESS` are set. `AWS_SES_ACCESS_KEY_ID` / `AWS_SES_SECRET_ACCESS_KEY` are
   optional — when omitted, the AWS SDK falls back to ambient credentials (instance
   role, shared config, etc.). A construction failure here is **fatal**.
2. Otherwise `email.LoadConfig()` returns non-nil when `SMTP_HOST` is set, using
   `SMTP_PORT` (default `587`), `SMTP_FROM` (default `noreply@teams360.example.com`),
   `SMTP_USERNAME`, and `SMTP_PASSWORD`.
3. Otherwise the sender is `nil` and the log records
   `No email service configured, email notifications disabled`.

| File | Role |
|------|------|
| `email/email.go` | `Sender` interface and shared types |
| `email/ses_service.go` | AWS SES implementation, `LoadSESConfig` |
| `email/smtp_service.go` | SMTP implementation, `LoadConfig`, `SendPasswordResetEmail` |
| `email/templates.go` | Message bodies |

### Notification service

`application/services/notification_service.go` wraps the sender with the team and user
repositories. It is invoked from exactly one place: `HealthCheckHandler.SubmitHealthCheck`
fires it in a goroutine after a successful submission, so email latency never blocks the
API response and email failures never fail the request.

| Survey type | Method called |
|-------------|---------------|
| `individual` (or empty) | `SendIndividualSurveyEmail` |
| `post_workshop` | `SendPostWorkshopEmails` |

Team-level recipients come from `teams.distribution_list_email`, which administrators set
per team.

The admin API exposes `emailEnabled`, `slackEnabled`, and `notifyOnSubmission` toggles
stored in `app_settings`, and reports `smtpConfigured` derived from
`os.Getenv("SMTP_HOST") != ""`.

!!! warning "The Slack toggle is not connected to anything"
    `app_settings.slack_notifications` is persisted and surfaced in the admin UI as
    "Notify managers when team health declines", but there is no Slack client, webhook
    URL, or send path anywhere in the backend. The stored boolean is never read by a
    sender. The same applies to `weekly_digest` — no digest job exists.

!!! warning "Retention and anonymization are configuration without an implementation"
    `GET/PUT /api/v1/admin/settings/retention` stores `retention_months` and reports a
    derived `anonymizeAfterDays` (`retention_months * 30`), but no scheduled job, cron,
    or cleanup routine reads these values. Nothing is ever deleted or anonymized.
    `archiveEnabled` is hardcoded `false` and is not persisted at all.

## Telemetry and observability

Instrumentation is set up in `backend/pkg/telemetry/` and `backend/pkg/metrics/`, with
deployment manifests for the collector stack in `backend/deploy/{otel,prometheus,grafana}`.

Three integration points sit in the request path:

- `otelsql.Open` wraps the database driver, so every SQL statement produces a child span
  tagged with `db.system=postgresql` and `db.name=teams360`.
- `otelgin.Middleware("teams360-api")` creates the server span for each HTTP request.
- `pkg/logger` emits structured zerolog records that carry the request ID.

Configuration is entirely environment-driven — `OTEL_ENABLED` (default off),
`OTEL_EXPORTER_OTLP_ENDPOINT` (default `localhost:4317`), and `ENVIRONMENT` (default
`development`) on the backend; the `NEXT_PUBLIC_OTEL_*` family on the frontend. A
telemetry initialization failure is non-fatal: the server logs a warning and runs without
tracing.

Local tooling is available through `make otel-start`, `otel-stop`, `otel-status`,
`otel-logs`, and `run-with-otel`, which drive
`backend/deploy/docker-compose.observability.yaml` (Jaeger on 16686, Prometheus on 9090,
Grafana on 3001).

See [Observability](../observability.md) for the full treatment — span attributes,
dashboards, alerting rules, and the collector configuration — and
[Configuration](../operations/configuration.md) for the variable reference.

## Health endpoint

`GET /health` is registered directly in `main.go` and returns a static
`200 {"status": "healthy"}`. It is consumed by the `Dockerfile` `HEALTHCHECK`, the
`docker-compose.yml` service healthcheck, and both Kubernetes probes in
`kubevela/teams360-kubevela.yaml`.

!!! warning "`/health` does not check dependencies"
    The handler is a literal JSON response. It does not ping Postgres or verify any
    downstream service, so orchestrators will report the container healthy while the
    database is unreachable. There is no separate readiness endpoint that does check.
    Note also that `request_logger.go` excludes both `/health` and `/api/health` from
    logging, but `/api/health` is never registered.
