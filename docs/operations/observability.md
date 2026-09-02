
# Team Health Check Observability Guide

Team Health Check provides comprehensive observability through **metrics**, **distributed tracing**, and **structured logging**. The system is designed to be backend-agnostic, allowing you to use your preferred observability platform.

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Quick Start](#quick-start)
3. [Metrics](#metrics)
   - [User Engagement Metrics](#user-engagement-metrics)
   - [Survey/Health Check Metrics](#surveyhealth-check-metrics)
   - [Team Health Metrics](#team-health-metrics)
   - [Dashboard Engagement Metrics](#dashboard-engagement-metrics)
   - [API Performance Metrics](#api-performance-metrics)
   - [Database Metrics](#database-metrics)
4. [Distributed Tracing](#distributed-tracing)
5. [Structured Logging](#structured-logging)
6. [Grafana Dashboard](#grafana-dashboard)
7. [Alternative Backends](#alternative-backends)
   - [Datadog Configuration](#datadog-configuration)
   - [Honeycomb Configuration](#honeycomb-configuration)
8. [Production Considerations](#production-considerations)

---

## Architecture Overview

Team Health Check uses **OpenTelemetry (OTel)** as the telemetry standard, providing vendor-agnostic instrumentation:

```
┌─────────────────┐      ┌──────────────────┐      ┌─────────────────────┐
│  Go Backend     │─────▶│  OTel Collector  │─────▶│  Observability      │
│  (Gin + OTel)   │      │  (traces/metrics)│      │  Backend            │
└─────────────────┘      └──────────────────┘      │  - Prometheus       │
                                │                   │  - Jaeger           │
┌─────────────────┐             │                   │  - Datadog          │
│  Next.js        │─────────────┘                   │  - Honeycomb        │
│  Frontend       │                                 │  - Grafana Cloud    │
└─────────────────┘                                 └─────────────────────┘
```

**Key Components:**

| Component | Purpose | Port |
|-----------|---------|------|
| OTel Collector | Receives, processes, and exports telemetry | 4317 (gRPC), 4318 (HTTP) |
| Prometheus | Time-series metrics storage | 9090 |
| Jaeger | Distributed trace visualization | 16686 |
| Grafana | Dashboards and visualization | 3001 |

---

## Quick Start

### 1. Start the Observability Stack

```bash
# Start all observability services
cd backend/deploy
docker-compose -f docker-compose.observability.yaml up -d

# Verify services are running
docker-compose -f docker-compose.observability.yaml ps
```

### 2. Start the Backend with Telemetry

```bash
# From repository root
export OTEL_ENABLED=true
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
make run-backend

# Or use the combined command
make run-with-otel
```

### 3. Access the Tools

| Tool | URL | Purpose |
|------|-----|---------|
| Grafana | http://localhost:3001 | Dashboards (admin/admin) |
| Prometheus | http://localhost:9090 | Metrics queries |
| Jaeger | http://localhost:16686 | Trace visualization |

---

## Metrics

Team Health Check exposes 40+ metrics organized into six categories. All metrics use the `teams360_` prefix when exported through Prometheus.

### User Engagement Metrics

These metrics track user activity and session behavior, helping you understand **platform adoption**, **user retention**, and **authentication health**.

| Metric Name | Type | Description | Labels |
|-------------|------|-------------|--------|
| `session.active.total` | UpDownCounter | Currently active user sessions | - |
| `session.duration` | Histogram | User session duration in seconds | `user_id` |
| `auth.login.total` | Counter | Total login attempts | `success`, `reason` |
| `auth.login.duration` | Histogram | Login operation latency | `success` |
| `auth.logout.total` | Counter | Total logout operations | - |
| `auth.token.refresh.total` | Counter | Token refresh operations | `success`, `reason` |
| `auth.failures.total` | Counter | Authentication failures | `type`, `reason` |
| `auth.password_reset.request.total` | Counter | Password reset requests | `user_found` |
| `auth.password_reset.complete.total` | Counter | Completed password resets | `success`, `reason` |
| `engagement.page_views.total` | Counter | Page/endpoint views | `page`, `role` |
| `engagement.feature_usage.total` | Counter | Feature usage tracking | `feature`, `role` |
| `engagement.return_visits.total` | Counter | Returning user visits | `user_id` |
| `engagement.dau` | Gauge | Daily active users | - |
| `engagement.wau` | Gauge | Weekly active users | - |
| `engagement.mau` | Gauge | Monthly active users | - |

**Histogram Buckets (Login Duration):** 10ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s

**Histogram Buckets (Session Duration):** 1m, 5m, 10m, 30m, 1h, 2h, 4h, 8h

#### Business Insights & Interpretation

| Metric | What It Tells You | Good | Warning | Critical |
|--------|-------------------|------|---------|----------|
| **Active Sessions** | Real-time platform usage; correlate with business hours | Steady during work hours | Unexpected drops during peak | Zero during expected active periods |
| **Login Success Rate** | Authentication health; user friction | >99% | 95-99% | <95% (investigate immediately) |
| **Login Latency (p50)** | User experience on entry | <100ms | 100-500ms | >500ms (frustrating UX) |
| **Login Latency (p99)** | Worst-case user experience | <500ms | 500ms-2s | >2s (users may abandon) |
| **Auth Failures** | Security signals; UX issues | <1% of attempts | 1-5% | >5% (credential issues or attack) |
| **Session Duration** | User engagement depth | 15-60 min avg | <5 min (not engaging) | >4h (forgot to logout?) |
| **DAU/MAU Ratio** | User stickiness | >20% (healthy) | 10-20% | <10% (low engagement) |
| **Password Reset Rate** | Credential friction | <2% of MAU/month | 2-5% | >5% (password policy issues) |

#### Business Questions These Metrics Answer

1. **"Is our platform being adopted?"**
   - Track DAU/WAU/MAU trends over time
   - Compare active sessions during deployment windows vs normal
   - Monitor login counts by role to see if managers are using dashboards

2. **"Are users having trouble logging in?"**
   - High `auth.failures.total` with reason `invalid_credentials` → password education needed
   - High `auth.failures.total` with reason `user_not_found` → provisioning/SSO issues
   - High `password_reset.request.total` → password policy may be too complex

3. **"Is the authentication system healthy?"**
   - p99 login latency >2s → database or auth service issues
   - Token refresh failures → JWT configuration or clock skew issues

4. **"Are users engaged or just checking boxes?"**
   - Session duration <5 minutes → users may be completing surveys perfunctorily
   - Low `engagement.return_visits` → one-time usage, not habitual
   - DAU/MAU <10% → tool not embedded in workflows

#### Alerts to Configure

```yaml
# Example Prometheus alerting rules
groups:
  - name: user_engagement
    rules:
      - alert: HighLoginFailureRate
        expr: sum(rate(teams360_auth_login_total{success="false"}[5m])) / sum(rate(teams360_auth_login_total[5m])) > 0.05
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Login failure rate above 5%"

      - alert: LoginLatencyHigh
        expr: histogram_quantile(0.99, sum(rate(teams360_auth_login_duration_seconds_bucket[5m])) by (le)) > 2
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "p99 login latency above 2 seconds"

      - alert: NoActiveSessions
        expr: teams360_session_active_total == 0
        for: 30m
        labels:
          severity: warning
        annotations:
          summary: "No active sessions for 30 minutes during business hours"
```

### Survey/Health Check Metrics

These are the **core product metrics** for Team Health Check, tracking health check survey participation and quality. These metrics directly measure whether the product is delivering value.

| Metric Name | Type | Description | Labels |
|-------------|------|-------------|--------|
| `survey.submitted.total` | Counter | Surveys submitted | `team_id`, `assessment_period` |
| `survey.submit.duration` | Histogram | Survey submission latency | `team_id`, `assessment_period` |
| `survey.responses.total` | Counter | Individual dimension responses | `team_id`, `assessment_period` |
| `survey.dimension.score` | Histogram | Score distribution (1-3) | `dimension_id`, `trend` |
| `survey.completion.rate` | Gauge | Completion rate per team | `team_id` |
| `survey.sessions.active` | UpDownCounter | Active survey sessions | - |
| `survey.started.total` | Counter | Surveys started (page loaded) | `team_id` |
| `survey.abandoned.total` | Counter | Abandoned surveys | `team_id`, `abandoned_at` |
| `survey.time_to_complete` | Histogram | Time from start to submit | `team_id` |
| `survey.comments.total` | Counter | Responses with comments | `team_id`, `dimension_id` |
| `survey.comment_rate` | Gauge | % of responses with comments | `team_id` |

**Histogram Buckets (Dimension Score):** 1.0, 1.5, 2.0, 2.5, 3.0

**Histogram Buckets (Time to Complete):** 30s, 1m, 2m, 3m, 5m, 10m, 15m, 30m

#### Business Insights & Interpretation

| Metric | What It Tells You | Good | Warning | Critical |
|--------|-------------------|------|---------|----------|
| **Completion Rate** | Team engagement with health checks | >80% | 50-80% | <50% (team not engaged) |
| **Abandonment Rate** | Survey UX friction | <10% | 10-25% | >25% (survey too long/confusing) |
| **Time to Complete** | Thoughtfulness vs. speed | 2-5 min (thoughtful) | <1 min (rushing) | >15 min (struggling) |
| **Avg Health Score** | Overall organizational health | 2.5-3.0 (green zone) | 2.0-2.5 (yellow zone) | <2.0 (red zone) |
| **Comment Rate** | Qualitative feedback engagement | >30% | 10-30% | <10% (not sharing context) |
| **Score Distribution** | Response diversity | Normal distribution | All same (gaming?) | Bimodal (polarized) |

#### Understanding the Health Score Scale

Team Health Check uses a 3-point scale based on Spotify's model:

| Score | Color | Meaning | Action |
|-------|-------|---------|--------|
| **3** | 🟢 Green | "We're doing great!" | Celebrate and maintain |
| **2** | 🟡 Yellow | "Some issues, but manageable" | Monitor and plan improvements |
| **1** | 🔴 Red | "This needs attention" | Prioritize and address |

**Important**: A red score is NOT a bad team—it's a team that needs support. The health check is a support tool, not a performance evaluation.

#### Business Questions These Metrics Answer

1. **"Are teams actually participating in health checks?"**
   - `survey.completion.rate` by team shows participation
   - `survey.abandoned.total` indicates friction points
   - Compare `survey.started.total` vs `survey.submitted.total` for funnel drop-off

2. **"Is the organization healthy?"**
   - `survey.dimension.score` histogram shows score distribution
   - Average score trending toward red (<2.0) = systemic issues
   - Scores clustering at extremes (all 1s or all 3s) = potential gaming

3. **"Which dimensions need attention?"**
   - Break down `team.health.by_dimension` to find weak spots
   - Low scores on "Fun" or "Learning" = burnout risk
   - Low scores on "Delivering Value" or "Speed" = delivery concerns

4. **"Are teams giving thoughtful responses?"**
   - `survey.time_to_complete` <1 minute = likely not thoughtful
   - `survey.comment_rate` <10% = missing qualitative insights
   - All scores identical across dimensions = pattern response

5. **"Is the survey experience good?"**
   - High `survey.abandoned_at` for specific dimensions = confusing question
   - `survey.submit.duration` >1s = API performance issue
   - Abandonment spikes after product changes = UX regression

#### Funnel Analysis

Track the survey funnel to identify drop-off points:

```
Survey Funnel:
┌──────────────────────────────────────────────────────────────┐
│ survey.started.total     │████████████████████████│ 100%     │
├──────────────────────────────────────────────────────────────┤
│ (dimension 1 answered)   │██████████████████████  │ 95%      │
├──────────────────────────────────────────────────────────────┤
│ (dimension 5 answered)   │████████████████████    │ 85%      │
├──────────────────────────────────────────────────────────────┤
│ (dimension 11 answered)  │██████████████████      │ 78%      │
├──────────────────────────────────────────────────────────────┤
│ survey.submitted.total   │█████████████████       │ 75%      │
└──────────────────────────────────────────────────────────────┘

Drop-off points indicate UX issues or confusing dimensions
```

#### Alerts to Configure

```yaml
groups:
  - name: survey_health
    rules:
      - alert: LowSurveyCompletionRate
        expr: teams360_survey_completion_rate < 0.5
        for: 24h
        labels:
          severity: warning
        annotations:
          summary: "Team {{ $labels.team_id }} has <50% survey completion"

      - alert: HighSurveyAbandonmentRate
        expr: sum(rate(teams360_survey_abandoned_total[1h])) / sum(rate(teams360_survey_started_total[1h])) > 0.25
        for: 1h
        labels:
          severity: warning
        annotations:
          summary: "Survey abandonment rate above 25%"

      - alert: OrgHealthCritical
        expr: (sum(teams360_survey_dimension_score_sum) / sum(teams360_survey_dimension_score_count)) < 2.0
        for: 24h
        labels:
          severity: critical
        annotations:
          summary: "Organization-wide health score below 2.0 (red zone)"

      - alert: SurveyRushing
        expr: histogram_quantile(0.5, sum(rate(teams360_survey_time_to_complete_bucket[1h])) by (le)) < 60
        for: 1h
        labels:
          severity: info
        annotations:
          summary: "Median survey completion time under 1 minute (users may be rushing)"
```

### Team Health Metrics

These metrics provide **executive-level visibility** into organizational health, helping leadership identify teams that need support and track improvement over time.

| Metric Name | Type | Description | Labels |
|-------------|------|-------------|--------|
| `team.health.queries.total` | Counter | Health data queries | `team_id` |
| `team.health.query.duration` | Histogram | Query latency | `team_id` |
| `team.active.total` | Gauge | Active teams count | - |
| `team.at_risk.total` | Gauge | Teams with health < 2.0 | - |
| `team.health.average` | Gauge | Org-wide average health | - |
| `team.health.by_dimension` | Histogram | Health by dimension | `dimension_id` |
| `team.improving.total` | Counter | Teams showing improvement | `team_id` |
| `team.declining.total` | Counter | Teams showing decline | `team_id` |

#### Business Insights & Interpretation

| Metric | What It Tells You | Good | Warning | Critical |
|--------|-------------------|------|---------|----------|
| **Teams At Risk** | Teams needing immediate support | 0-10% of teams | 10-25% | >25% (systemic issues) |
| **Org Health Average** | Overall organizational wellbeing | 2.5-3.0 | 2.0-2.5 | <2.0 (widespread issues) |
| **Improving Teams** | Positive momentum | Increasing trend | Flat | Decreasing |
| **Declining Teams** | Negative momentum | <5% of teams | 5-15% | >15% (morale problem) |
| **Health by Dimension** | Systemic weak spots | All dimensions >2.5 | 1-2 dimensions <2.0 | Multiple dimensions <2.0 |

#### Understanding At-Risk Teams

A team is flagged as "at-risk" when their average health score falls below 2.0. This is NOT a punishment—it's a signal for support:

```
At-Risk Assessment:
┌─────────────────────────────────────────────────────────────────────┐
│ Score Range │ Status      │ Meaning                                 │
├─────────────────────────────────────────────────────────────────────┤
│ 2.5 - 3.0   │ Healthy     │ Team thriving, maintain current support │
│ 2.0 - 2.5   │ Watch       │ Some concerns, check in with team lead  │
│ 1.5 - 2.0   │ At Risk     │ Multiple issues, active intervention    │
│ 1.0 - 1.5   │ Critical    │ Urgent support needed                   │
└─────────────────────────────────────────────────────────────────────┘
```

#### Business Questions These Metrics Answer

1. **"How healthy is our organization overall?"**
   - `team.health.average` gives the org-wide pulse
   - Compare against historical baseline (is it trending up or down?)
   - Healthy orgs typically average 2.3-2.7

2. **"Which teams need support?"**
   - `team.at_risk.total` counts teams below 2.0
   - Use this to prioritize 1:1s and resource allocation
   - Don't wait for critical—intervene at "watch" level

3. **"Are our improvement efforts working?"**
   - Compare `team.improving.total` vs `team.declining.total`
   - Healthy ratio: improving > declining
   - Warning: If declining consistently > improving, check systemic issues

4. **"What are our organizational blind spots?"**
   - `team.health.by_dimension` reveals patterns
   - Common patterns:
     - Low "Fun" + Low "Learning" = burnout culture
     - Low "Speed" + Low "Easy to Release" = technical debt
     - Low "Pawns or Players" = micromanagement concerns
     - Low "Support" = siloed organization

5. **"Is health improving period-over-period?"**
   - Compare assessment periods (e.g., "2024 - 1st Half" vs "2024 - 2nd Half")
   - Expect gradual improvement (0.1-0.2 per period is good)
   - Sudden large swings may indicate gaming or external events

#### Example: Reading the Health Trend

```
Organization Health Trend:
2023 H2: 2.1 ████████████████████░░░░░░░░░░
2024 H1: 2.3 ██████████████████████░░░░░░░░  (+0.2 improvement)
2024 H2: 2.4 ████████████████████████░░░░░░  (+0.1 improvement)

This shows healthy, gradual improvement. Sudden jumps (e.g., 2.1 → 2.8)
should be investigated—may indicate response bias changes.
```

#### Alerts to Configure

```yaml
groups:
  - name: team_health
    rules:
      - alert: HighAtRiskTeams
        expr: teams360_team_at_risk_total / teams360_team_active_total > 0.25
        for: 24h
        labels:
          severity: critical
        annotations:
          summary: "More than 25% of teams are at-risk (health < 2.0)"

      - alert: OrgHealthDeclining
        expr: delta(teams360_team_health_average[7d]) < -0.2
        for: 1h
        labels:
          severity: warning
        annotations:
          summary: "Organization health dropped by >0.2 in the past week"

      - alert: TeamDecliningConsistently
        expr: increase(teams360_team_declining_total[30d]) > increase(teams360_team_improving_total[30d]) * 2
        for: 1h
        labels:
          severity: warning
        annotations:
          summary: "Declining teams outnumber improving teams 2:1 over past month"
```

### Dashboard Engagement Metrics

Track how users interact with dashboards:

| Metric Name | Type | Description | Labels |
|-------------|------|-------------|--------|
| `dashboard.manager.views.total` | Counter | Manager dashboard views | `manager_id`, `view_type` |
| `dashboard.teamlead.views.total` | Counter | Team lead dashboard views | `team_lead_id`, `view_type` |
| `dashboard.trends.views.total` | Counter | Trend report views | `user_id`, `report_type` |
| `dashboard.exports.total` | Counter | Report exports | `export_type`, `format` |

**View Types:** `teams_health`, `radar`, `trends`, `health_summary`, `response_distribution`, `individual_responses`

#### Business Insights & Interpretation

Dashboard engagement metrics reveal whether leadership is actively using health check data to make decisions:

| Metric | Good | Warning | Critical | Business Meaning |
|--------|------|---------|----------|------------------|
| **Manager Views / Week** | ≥3 per manager | 1-2 per manager | 0 views | Managers reviewing health data regularly vs. ignoring it |
| **Team Lead Views / Week** | ≥5 per lead | 2-4 per lead | 0-1 views | Team leads staying informed about their team's pulse |
| **Trend Report Views** | ≥1/week after check-in | Sporadic | Never viewed | Leaders analyzing patterns vs. one-time glances |
| **Report Exports** | Regular quarterly | Rare | Never | Data being shared in leadership meetings |

**Feature Adoption Analysis:**

Track which dashboard features are actually used to guide product investment:

```
View Type Distribution (target):
├── health_summary:           30% (entry point)
├── radar (aggregated):       25% (org-wide view)
├── trends:                   20% (longitudinal analysis)
├── response_distribution:    15% (drill-down)
└── individual_responses:     10% (detailed investigation)
```

**Business Questions These Metrics Answer:**

1. **"Are managers using health check insights?"**
   - Low view counts suggest health checks are "check the box" exercises
   - High views + low action = analysis paralysis (need clearer recommendations)

2. **"Is there leadership engagement across all levels?"**
   - Compare manager vs. team lead engagement ratios
   - VPs viewing should trigger cascade of manager views

3. **"Which features drive value?"**
   - High `trends` views = longitudinal thinking (mature usage)
   - Only `health_summary` views = surface-level engagement

4. **"Is health data informing decisions?"**
   - Exports before quarterly reviews = data-driven planning
   - No exports = insights staying in the tool

**Example Alerts:**

```yaml
groups:
  - name: dashboard_engagement
    rules:
      - alert: ManagerDashboardAbandonment
        expr: increase(teams360_dashboard_manager_views_total[7d]) == 0
        for: 7d
        labels:
          severity: warning
        annotations:
          summary: "No manager dashboard views in the past week"
          action: "Send engagement reminder or schedule training"

      - alert: LowTrendAnalysis
        expr: |
          sum(rate(teams360_dashboard_trends_views_total[30d])) /
          sum(rate(teams360_dashboard_manager_views_total[30d])) < 0.1
        for: 24h
        labels:
          severity: info
        annotations:
          summary: "Less than 10% of dashboard views include trend analysis"
          action: "Highlight trend features in next training session"
```

### API Performance Metrics

Monitor HTTP and API performance:

| Metric Name | Type | Description | Labels |
|-------------|------|-------------|--------|
| `api.latency.by_endpoint` | Histogram | Endpoint latency (ms) | `endpoint`, `method`, `status_code` |
| `api.errors.by_endpoint` | Counter | API errors | `endpoint`, `method`, `status_code`, `error_type` |
| `http.requests.inflight` | UpDownCounter | Current inflight requests | - |
| `ratelimit.exceeded.total` | Counter | Rate limit violations | `endpoint` |

**Histogram Buckets (Latency):** 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s

#### Business Insights & Interpretation

API metrics ensure a responsive user experience and help identify technical debt:

| Metric | Good | Warning | Critical | User Impact |
|--------|------|---------|----------|-------------|
| **p50 Latency** | <50ms | 50-200ms | >200ms | Majority of users experience snappy UI |
| **p95 Latency** | <200ms | 200-500ms | >500ms | Even slower requests feel acceptable |
| **p99 Latency** | <500ms | 500ms-1s | >1s | Worst case still usable (affects power users) |
| **Error Rate** | <0.1% | 0.1-1% | >1% | User trust and data integrity |
| **Inflight Requests** | <100 | 100-500 | >500 | System under load, potential queuing |

**Endpoint Performance Targets:**

Different endpoints have different acceptable latencies based on user expectations:

| Endpoint Category | Target p95 | Rationale |
|-------------------|------------|-----------|
| `GET /health` | <10ms | Health checks should be instant |
| `POST /auth/login` | <300ms | Users expect login to take a moment |
| `GET /dashboard/*` | <200ms | Dashboard loads should feel fast |
| `POST /health-checks` | <500ms | Submitting a survey can take longer |
| `GET /trends` | <1s | Aggregation queries are expected to be slower |

**Business Questions These Metrics Answer:**

1. **"Is the application performant enough for user adoption?"**
   - Latency >500ms correlates with abandonment
   - Slow dashboards = managers stop checking health data

2. **"Where should engineering invest in optimization?"**
   - Sort endpoints by p99 latency × request volume
   - High-traffic slow endpoints = highest ROI fixes

3. **"Are we meeting SLAs?"**
   - Define SLOs: "99% of requests under 500ms"
   - Track error budgets for informed risk-taking

4. **"Is there abuse or unexpected load?"**
   - Rate limit violations indicate potential abuse
   - High inflight requests suggest capacity planning needs

**SLO Calculation Example:**

```promql
# SLO: 99.9% of requests complete successfully under 500ms
# Error budget: 0.1% (43 minutes/month)

# Current SLO attainment:
(
  sum(rate(teams360_api_latency_bucket{le="500"}[30d])) /
  sum(rate(teams360_api_latency_count[30d]))
) * 100

# Error budget remaining this month:
(0.001 - (1 - <slo_attainment>)) / 0.001 * 100
```

**Example Alerts:**

```yaml
groups:
  - name: api_performance
    rules:
      - alert: HighAPILatency
        expr: histogram_quantile(0.95, rate(teams360_api_latency_bucket[5m])) > 500
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "API p95 latency exceeds 500ms"
          impact: "Users experiencing slow page loads"

      - alert: HighAPIErrorRate
        expr: |
          sum(rate(teams360_api_errors_total[5m])) /
          sum(rate(teams360_api_latency_count[5m])) > 0.01
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "API error rate exceeds 1%"
          impact: "Users encountering failures, potential data loss"

      - alert: SLOBudgetBurnRate
        expr: |
          (
            1 - (
              sum(rate(teams360_api_latency_bucket{le="500"}[1h])) /
              sum(rate(teams360_api_latency_count[1h]))
            )
          ) > 0.001 * 24  # Burning 24 hours of budget per hour
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "SLO error budget burn rate is too high"
          impact: "Will exhaust monthly error budget within days"
```

### Database Metrics

Monitor database performance:

| Metric Name | Type | Description | Labels |
|-------------|------|-------------|--------|
| `db.query.total` | Counter | Database queries | `operation`, `table` |
| `db.query.duration` | Histogram | Query latency | `operation`, `table` |
| `db.errors.total` | Counter | Database errors | `operation`, `table` |
| `db.connections.active` | UpDownCounter | Active connections | - |

**Histogram Buckets (Query Duration):** 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s

#### Business Insights & Interpretation

Database metrics are the foundation of application reliability. Database issues cascade to all users:

| Metric | Good | Warning | Critical | System Impact |
|--------|------|---------|----------|---------------|
| **Query p50** | <10ms | 10-50ms | >50ms | Baseline database responsiveness |
| **Query p95** | <50ms | 50-100ms | >100ms | Complex queries still fast |
| **Query p99** | <100ms | 100-250ms | >250ms | Worst case acceptable |
| **Error Rate** | <0.01% | 0.01-0.1% | >0.1% | Data integrity risk |
| **Active Connections** | <50% pool | 50-80% pool | >80% pool | Connection exhaustion risk |

**Query Performance by Table:**

Different tables have different performance characteristics based on data volume:

| Table | Expected p95 | Notes |
|-------|-------------|-------|
| `users` | <5ms | Small table, indexed lookups |
| `teams` | <5ms | Small table, indexed lookups |
| `health_check_sessions` | <25ms | Growing table, date-range queries |
| `health_check_responses` | <50ms | High volume, aggregation queries |
| `dimensions` | <5ms | Static reference data |

**Business Questions These Metrics Answer:**

1. **"Is the database a bottleneck?"**
   - If DB p95 > API p95, database is the limiting factor
   - High connection usage = need connection pooling tuning or scale-up

2. **"Which queries need optimization?"**
   - Sort by: `p99_latency × query_count` to find highest-impact queries
   - `SELECT` on `health_check_responses` likely needs indexing attention

3. **"Are we at risk of data loss?"**
   - Database errors should be extremely rare
   - Any increase in error rate requires immediate investigation

4. **"Do we need to scale?"**
   - Connection pool saturation = vertical scaling needed
   - Query latency increasing over time = data growth requires optimization

**Connection Pool Health:**

```
Connection Pool Utilization:
├── 0-50%:   ✅ Healthy headroom
├── 50-80%:  ⚠️ Monitor during peak hours
├── 80-95%:  🔶 Scale soon, connection queuing likely
└── 95-100%: 🔴 Connection exhaustion imminent
```

**Slow Query Analysis:**

Track slow queries to identify optimization candidates:

```promql
# Top 5 slowest operations by p99
topk(5,
  histogram_quantile(0.99,
    sum(rate(teams360_db_query_duration_bucket[1h])) by (operation, table, le)
  )
)

# Operations with highest total time (latency × count)
topk(5,
  sum(rate(teams360_db_query_duration_sum[1h])) by (operation, table)
)
```

**Example Alerts:**

```yaml
groups:
  - name: database_health
    rules:
      - alert: SlowDatabaseQueries
        expr: histogram_quantile(0.95, rate(teams360_db_query_duration_bucket[5m])) > 100
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Database query p95 latency exceeds 100ms"
          impact: "Slow page loads for all users"
          action: "Check for missing indexes or lock contention"

      - alert: DatabaseErrors
        expr: increase(teams360_db_errors_total[5m]) > 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Database errors detected"
          impact: "Potential data loss or corruption"
          action: "Check database logs immediately"

      - alert: ConnectionPoolExhaustion
        expr: teams360_db_connections_active > 40  # Assuming 50 max connections
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Database connection pool near exhaustion (>80%)"
          impact: "New requests may fail to get database connection"
          action: "Scale database or optimize connection usage"

      - alert: QueryVolumeSpike
        expr: |
          rate(teams360_db_query_total[5m]) >
          rate(teams360_db_query_total[1h] offset 1d) * 2
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Database query volume 2x higher than same time yesterday"
          impact: "Potential performance degradation"
          action: "Investigate traffic source and scale if needed"
```

---

## Distributed Tracing

Team Health Check implements distributed tracing using OpenTelemetry with named tracers for different domains:

### Tracer Names

| Tracer | Purpose | Example Spans |
|--------|---------|---------------|
| `teams360.auth` | Authentication flows | `login`, `logout`, `token_refresh` |
| `teams360.healthcheck` | Survey operations | `submit`, `get_by_id`, `get_team_sessions`, `get_dimensions` |
| `teams360.team` | Team operations | `get_health`, `get_members` |
| `teams360.user` | User management | `get_profile`, `update_profile` |
| `teams360.database` | Database operations | `query`, `insert`, `update` |

### Span Attributes

Each span includes relevant business context:

```go
// Health Check Span Attributes
span.SetAttributes(
    attribute.String("healthcheck.id", id),
    attribute.String("team.id", teamID),
    attribute.String("healthcheck.assessment_period", period),
    attribute.Int("healthcheck.dimension_count", count),
    attribute.Bool("survey.complete", completed),
)
```

### Trace Context Propagation

Team Health Check uses W3C TraceContext for distributed tracing across services:

```go
// Propagators configured in telemetry initialization
otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
    propagation.TraceContext{},
    propagation.Baggage{},
))
```

### PII Protection

Sensitive data is masked in traces:

```go
// Username masking: "johndoe" -> "jo***e"
func maskUsername(username string) string {
    if len(username) <= 3 {
        return "***"
    }
    return username[:2] + "***" + string(username[len(username)-1])
}
```

---

## Structured Logging

Team Health Check uses **zerolog** for high-performance structured logging with automatic trace correlation.

### Log Levels

| Level | Usage |
|-------|-------|
| `debug` | Development troubleshooting, successful DB operations |
| `info` | Normal operations, successful requests |
| `warn` | Recoverable issues, auth failures, security events |
| `error` | Failures requiring attention, 5xx responses |
| `fatal` | Unrecoverable errors causing shutdown |

### Automatic Context Enrichment

Logs automatically include trace context for correlation:

```json
{
  "level": "info",
  "time": "2024-01-15T10:30:00Z",
  "request_id": "req-abc123",
  "user_id": "user-456",
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
  "span_id": "00f067aa0ba902b7",
  "message": "health check submitted successfully"
}
```

### Specialized Log Events

**Authentication Events:**
```go
log.Auth("login").
    Username("johndoe").  // Automatically masked
    IP("192.168.1.1").
    Success()

// Output: {"component":"auth","action":"login","username":"jo***e","client_ip":"192.168.1.1","msg":"auth_success"}
```

**Database Events:**
```go
log.DB("query").
    Table("health_check_sessions").
    Duration(15 * time.Millisecond).
    Context("fetching team health data").
    Success()
```

**HTTP Events:**
```go
log.HTTP().
    Method("POST").
    Path("/api/v1/health-checks").
    Status(201).
    Duration(150 * time.Millisecond).
    RequestID("req-abc123").
    Log()
```

**Security Events:**
```go
log.Security("rate_limit_exceeded").
    IP("192.168.1.1").
    Endpoint("/api/v1/auth/login").
    Details("exceeded 100 requests per minute").
    Log()
```

### Configuration

```go
logger.Init(logger.Config{
    Level:  "info",   // debug, info, warn, error
    Pretty: true,     // Human-readable for dev, JSON for prod
    Output: os.Stdout,
})
```

---

## Grafana Dashboard

Team Health Check ships with a pre-configured Grafana dashboard (`Team Health Check Product Analytics`) containing 18 panels organized into three sections.

### User Engagement Overview

| Panel | Type | Metric | Purpose |
|-------|------|--------|---------|
| Active Sessions | Stat | `teams360_session_active_total` | Current logged-in users |
| Total Successful Logins | Stat | `sum(teams360_auth_login_total{success="true"})` | Login count |
| Total Logouts | Stat | `sum(teams360_auth_logout_total)` | Logout count |
| Login Latency (p50) | Stat | `histogram_quantile(0.50, ...)` | Median login time |
| Login Activity | Time Series | `increase(teams360_auth_login_total[5m])` | Login trends over time |
| Login Latency Distribution | Time Series | p50, p95, p99 percentiles | Latency distribution |

### Survey Analytics

| Panel | Type | Metric | Purpose |
|-------|------|--------|---------|
| Total Surveys Submitted | Stat | `sum(teams360_survey_submitted_total)` | Survey count |
| Total Dimension Responses | Stat | `sum(teams360_survey_responses_total)` | Response count |
| Survey Submit Time (p50) | Stat | `histogram_quantile(0.50, ...)` | Submission latency |
| Avg Health Score | Stat | `sum/count of dimension scores` | Organization health |
| Submissions Over Time | Time Series | By team_id | Participation trends |
| Health by Dimension | Bar Chart | Avg score by dimension | Dimension analysis |

### API Performance

| Panel | Type | Metric | Purpose |
|-------|------|--------|---------|
| HTTP Request Latency | Time Series | p50, p95 by route | Endpoint performance |
| Requests per Second | Time Series | Rate by route | Traffic patterns |
| Database Query Latency | Time Series | p50, p95, p99 | DB performance |
| HTTP Response Status | Time Series | Count by status code | Error monitoring |

---

## Alternative Backends

### Datadog Configuration

To send telemetry to Datadog instead of the local stack:

**1. Update OTel Collector Configuration**

Create `collector-datadog.yaml`:

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: "0.0.0.0:4317"
      http:
        endpoint: "0.0.0.0:4318"

processors:
  batch:
    timeout: 10s
    send_batch_size: 1000

  resource:
    attributes:
      - key: deployment.environment
        value: production
        action: upsert

exporters:
  datadog:
    api:
      key: ${DD_API_KEY}
      site: datadoghq.com  # or datadoghq.eu for EU
    traces:
      span_name_as_resource_name: true
    metrics:
      resource_attributes_as_tags: true
      histograms:
        mode: distributions
        send_aggregation_metrics: true

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch, resource]
      exporters: [datadog]
    metrics:
      receivers: [otlp]
      processors: [batch, resource]
      exporters: [datadog]
```

**2. Set Environment Variables**

```bash
export DD_API_KEY=your-datadog-api-key
export DD_SITE=datadoghq.com
export OTEL_ENABLED=true
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
```

**3. Run the Collector**

```bash
docker run -d \
  -e DD_API_KEY \
  -v $(pwd)/collector-datadog.yaml:/etc/otel/config.yaml \
  -p 4317:4317 -p 4318:4318 \
  otel/opentelemetry-collector-contrib:latest \
  --config /etc/otel/config.yaml
```

**Datadog-Specific Features:**
- Automatic service mapping
- APM trace correlation
- Unified dashboards with infrastructure metrics
- Anomaly detection
- SLO tracking

### Honeycomb Configuration

To send telemetry to Honeycomb:

**1. Update OTel Collector Configuration**

Create `collector-honeycomb.yaml`:

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: "0.0.0.0:4317"
      http:
        endpoint: "0.0.0.0:4318"

processors:
  batch:
    timeout: 5s
    send_batch_size: 500

exporters:
  otlp/honeycomb-traces:
    endpoint: "api.honeycomb.io:443"
    headers:
      "x-honeycomb-team": ${HONEYCOMB_API_KEY}
      "x-honeycomb-dataset": teams360-traces

  otlp/honeycomb-metrics:
    endpoint: "api.honeycomb.io:443"
    headers:
      "x-honeycomb-team": ${HONEYCOMB_API_KEY}
      "x-honeycomb-dataset": teams360-metrics

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [otlp/honeycomb-traces]
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [otlp/honeycomb-metrics]
```

**2. Set Environment Variables**

```bash
export HONEYCOMB_API_KEY=your-honeycomb-api-key
export OTEL_ENABLED=true
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
```

**3. Run the Collector**

```bash
docker run -d \
  -e HONEYCOMB_API_KEY \
  -v $(pwd)/collector-honeycomb.yaml:/etc/otel/config.yaml \
  -p 4317:4317 -p 4318:4318 \
  otel/opentelemetry-collector-contrib:latest \
  --config /etc/otel/config.yaml
```

**Honeycomb-Specific Features:**
- BubbleUp for automatic analysis
- High-cardinality support (user_id, team_id as first-class dimensions)
- SLOs with burn alerts
- Query-driven exploration

### Other Supported Backends

The OTel Collector supports 50+ backends. Common alternatives:

| Backend | Exporter | Notes |
|---------|----------|-------|
| Grafana Cloud | `otlp` | Use Grafana's OTLP endpoint |
| New Relic | `otlp` | Native OTLP support |
| Splunk | `splunk_hec` | Splunk HEC exporter |
| AWS X-Ray | `awsxray` | AWS native tracing |
| Google Cloud | `googlecloud` | GCP operations suite |
| Elastic APM | `elasticsearch` | Elastic observability |

---

## Production Considerations

### Sample Rate Configuration

Adjust sampling in production to reduce costs:

```go
// In telemetry.go
Config{
    SampleRate: 0.1,  // 10% sampling for high-traffic
}
```

Or use adaptive sampling:
```go
sampler := trace.ParentBased(
    trace.TraceIDRatioBased(0.1),  // 10% baseline
    trace.WithRemoteParentSampled(trace.AlwaysSample()),  // Honor upstream decisions
)
```

### Metric Cardinality

High-cardinality labels can cause storage issues. Team Health Check limits cardinality by:
- Not including `user_id` on high-volume metrics
- Using `team_id` selectively (11 teams max by default)
- Aggregating paths/endpoints rather than full URLs

### Resource Limits

Configure collector memory limits:
```yaml
processors:
  memory_limiter:
    check_interval: 1s
    limit_mib: 1024      # Production limit
    spike_limit_mib: 256
```

### Security

For production:
1. Enable TLS for OTLP endpoints
2. Use authentication tokens
3. Restrict collector access via network policies
4. Mask PII in spans (implemented in tracer.go)

```yaml
# Production collector with TLS
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: "0.0.0.0:4317"
        tls:
          cert_file: /etc/otel/server.crt
          key_file: /etc/otel/server.key
```

### High Availability

For production, deploy multiple collectors behind a load balancer:

```yaml
# kubernetes deployment example
apiVersion: apps/v1
kind: Deployment
metadata:
  name: otel-collector
spec:
  replicas: 3
  selector:
    matchLabels:
      app: otel-collector
```

---

## Environment Variables Reference

| Variable | Default | Description |
|----------|---------|-------------|
| `OTEL_ENABLED` | `false` | Enable telemetry export |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `localhost:4317` | OTel Collector endpoint |
| `ENVIRONMENT` | `development` | Environment name (added to all telemetry) |
| `LOG_LEVEL` | `info` | Logging level |
| `LOG_PRETTY` | `false` | Human-readable logs (dev only) |

---

## Troubleshooting

### Metrics not appearing in Prometheus

1. Check collector health: `curl http://localhost:13133/`
2. Verify metrics endpoint: `curl http://localhost:8889/metrics | grep teams360`
3. Check Prometheus targets: http://localhost:9090/targets

### Traces not appearing in Jaeger

1. Verify collector is receiving traces: Check collector logs
2. Ensure `OTEL_ENABLED=true` is set
3. Check Jaeger UI search with service name `teams360-api`

### High memory usage

1. Reduce batch sizes in collector config
2. Lower metric cardinality
3. Increase `memory_limiter` check frequency

### Log correlation not working

Ensure trace context is propagated:
```go
log := logger.Get().WithContext(ctx)
log.Info("message")  // Will include trace_id, span_id
```
