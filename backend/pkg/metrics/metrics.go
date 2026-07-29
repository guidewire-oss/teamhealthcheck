// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

// Package metrics provides OpenTelemetry metrics for Team360 product analytics and business metrics
package metrics

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	// MeterName is the name of the meter for Team360 metrics
	MeterName = "teams360"
)

var (
	once     sync.Once
	instance *Metrics
)

// Metrics holds all OpenTelemetry metric instruments for Team360
type Metrics struct {
	meter metric.Meter

	// ==========================================
	// USER ENGAGEMENT & PRODUCT USAGE METRICS
	// ==========================================

	// Login metrics
	LoginAttempts  metric.Int64Counter       // Total login attempts
	LoginSuccesses metric.Int64Counter       // Successful logins
	LoginFailures  metric.Int64Counter       // Failed logins (wrong password, user not found)
	ActiveSessions metric.Int64UpDownCounter // Currently active sessions

	// Session metrics
	SessionDuration metric.Float64Histogram // How long users stay logged in (seconds)

	// Page/Feature engagement
	PageViews      metric.Int64Counter     // Page views by page name
	DashboardViews metric.Int64Counter     // Dashboard views by role (manager, team lead)
	FeatureUsage   metric.Int64Counter     // Feature usage (export, filter, etc.)
	TimeOnPage     metric.Float64Histogram // Time spent on each page (seconds)

	// ==========================================
	// SURVEY FUNNEL METRICS
	// ==========================================

	// Survey lifecycle
	SurveysStarted   metric.Int64Counter     // Surveys initiated
	SurveysCompleted metric.Int64Counter     // Surveys fully submitted
	SurveysAbandoned metric.Int64Counter     // Surveys started but not completed
	SurveyDuration   metric.Float64Histogram // Time to complete survey (seconds)

	// Survey response details
	ResponsesSubmitted metric.Int64Counter // Individual dimension responses
	CommentsProvided   metric.Int64Counter // Responses that include comments

	// ==========================================
	// TEAM HEALTH BUSINESS METRICS
	// ==========================================

	// Health scores (recorded when surveys are submitted)
	HealthScoreSubmitted metric.Float64Histogram // Distribution of submitted health scores (1-3)
	DimensionScores      metric.Float64Histogram // Scores by dimension

	// Team health gauges (updated periodically or on query)
	TeamsAtRisk      metric.Int64UpDownCounter // Teams with avg score < 2.0
	ActiveTeams      metric.Int64UpDownCounter // Teams with recent submissions
	OverallHealthAvg metric.Float64Gauge       // Organization-wide health average

	// Trend indicators
	TeamsImproving metric.Int64Counter // Teams showing improvement
	TeamsDeclining metric.Int64Counter // Teams showing decline
	TeamsStable    metric.Int64Counter // Teams with stable scores

	// ==========================================
	// API PERFORMANCE METRICS
	// ==========================================

	// Request metrics (complements otelgin middleware)
	APIRequestDuration metric.Float64Histogram // Request latency by endpoint
	APIErrors          metric.Int64Counter     // API errors by endpoint and status code
	DBQueryDuration    metric.Float64Histogram // Database query latency

	// ==========================================
	// MANAGER/LEADERSHIP METRICS
	// ==========================================

	ManagerDashboardAccess metric.Int64Counter // Manager dashboard access count
	TeamComparisonViews    metric.Int64Counter // Cross-team comparison views
	TrendReportViews       metric.Int64Counter // Trend report views
}

// Get returns the singleton Metrics instance, initializing it if necessary
func Get() *Metrics {
	once.Do(func() {
		instance = &Metrics{}
		instance.init()
	})
	return instance
}

// init initializes all metric instruments
func (m *Metrics) init() {
	m.meter = otel.Meter(MeterName)

	var err error

	// ==========================================
	// USER ENGAGEMENT METRICS
	// ==========================================

	m.LoginAttempts, err = m.meter.Int64Counter(
		"teams360.auth.login.attempts",
		metric.WithDescription("Total number of login attempts"),
		metric.WithUnit("{attempt}"),
	)
	handleError(err)

	m.LoginSuccesses, err = m.meter.Int64Counter(
		"teams360.auth.login.successes",
		metric.WithDescription("Number of successful logins"),
		metric.WithUnit("{login}"),
	)
	handleError(err)

	m.LoginFailures, err = m.meter.Int64Counter(
		"teams360.auth.login.failures",
		metric.WithDescription("Number of failed login attempts"),
		metric.WithUnit("{failure}"),
	)
	handleError(err)

	m.ActiveSessions, err = m.meter.Int64UpDownCounter(
		"teams360.auth.sessions.active",
		metric.WithDescription("Number of currently active user sessions"),
		metric.WithUnit("{session}"),
	)
	handleError(err)

	m.SessionDuration, err = m.meter.Float64Histogram(
		"teams360.auth.session.duration",
		metric.WithDescription("Duration of user sessions in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(60, 300, 600, 1800, 3600, 7200, 14400), // 1m, 5m, 10m, 30m, 1h, 2h, 4h
	)
	handleError(err)

	m.PageViews, err = m.meter.Int64Counter(
		"teams360.engagement.page_views",
		metric.WithDescription("Page views by page name"),
		metric.WithUnit("{view}"),
	)
	handleError(err)

	m.DashboardViews, err = m.meter.Int64Counter(
		"teams360.engagement.dashboard_views",
		metric.WithDescription("Dashboard views by user role"),
		metric.WithUnit("{view}"),
	)
	handleError(err)

	m.FeatureUsage, err = m.meter.Int64Counter(
		"teams360.engagement.feature_usage",
		metric.WithDescription("Feature usage count"),
		metric.WithUnit("{use}"),
	)
	handleError(err)

	m.TimeOnPage, err = m.meter.Float64Histogram(
		"teams360.engagement.time_on_page",
		metric.WithDescription("Time spent on each page in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(5, 10, 30, 60, 120, 300, 600), // 5s, 10s, 30s, 1m, 2m, 5m, 10m
	)
	handleError(err)

	// ==========================================
	// SURVEY FUNNEL METRICS
	// ==========================================

	m.SurveysStarted, err = m.meter.Int64Counter(
		"teams360.survey.started",
		metric.WithDescription("Number of surveys started"),
		metric.WithUnit("{survey}"),
	)
	handleError(err)

	m.SurveysCompleted, err = m.meter.Int64Counter(
		"teams360.survey.completed",
		metric.WithDescription("Number of surveys completed"),
		metric.WithUnit("{survey}"),
	)
	handleError(err)

	m.SurveysAbandoned, err = m.meter.Int64Counter(
		"teams360.survey.abandoned",
		metric.WithDescription("Number of surveys abandoned before completion"),
		metric.WithUnit("{survey}"),
	)
	handleError(err)

	m.SurveyDuration, err = m.meter.Float64Histogram(
		"teams360.survey.duration",
		metric.WithDescription("Time taken to complete a survey in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(30, 60, 120, 180, 300, 600, 900), // 30s, 1m, 2m, 3m, 5m, 10m, 15m
	)
	handleError(err)

	m.ResponsesSubmitted, err = m.meter.Int64Counter(
		"teams360.survey.responses",
		metric.WithDescription("Number of individual dimension responses submitted"),
		metric.WithUnit("{response}"),
	)
	handleError(err)

	m.CommentsProvided, err = m.meter.Int64Counter(
		"teams360.survey.comments",
		metric.WithDescription("Number of responses that include comments"),
		metric.WithUnit("{comment}"),
	)
	handleError(err)

	// ==========================================
	// TEAM HEALTH BUSINESS METRICS
	// ==========================================

	m.HealthScoreSubmitted, err = m.meter.Float64Histogram(
		"teams360.health.score",
		metric.WithDescription("Distribution of submitted health scores"),
		metric.WithUnit("{score}"),
		metric.WithExplicitBucketBoundaries(1.0, 1.5, 2.0, 2.5, 3.0),
	)
	handleError(err)

	m.DimensionScores, err = m.meter.Float64Histogram(
		"teams360.health.dimension_score",
		metric.WithDescription("Health scores by dimension"),
		metric.WithUnit("{score}"),
		metric.WithExplicitBucketBoundaries(1.0, 1.5, 2.0, 2.5, 3.0),
	)
	handleError(err)

	m.TeamsAtRisk, err = m.meter.Int64UpDownCounter(
		"teams360.health.teams_at_risk",
		metric.WithDescription("Number of teams with health score below threshold"),
		metric.WithUnit("{team}"),
	)
	handleError(err)

	m.ActiveTeams, err = m.meter.Int64UpDownCounter(
		"teams360.health.active_teams",
		metric.WithDescription("Number of teams with recent health check submissions"),
		metric.WithUnit("{team}"),
	)
	handleError(err)

	// Note: Float64Gauge requires a callback-based approach in OTel Go
	// We'll use an observable gauge for overall health average
	_, err = m.meter.Float64ObservableGauge(
		"teams360.health.overall_avg",
		metric.WithDescription("Organization-wide average health score"),
		metric.WithUnit("{score}"),
	)
	handleError(err)

	m.TeamsImproving, err = m.meter.Int64Counter(
		"teams360.health.teams_improving",
		metric.WithDescription("Number of teams showing improvement"),
		metric.WithUnit("{team}"),
	)
	handleError(err)

	m.TeamsDeclining, err = m.meter.Int64Counter(
		"teams360.health.teams_declining",
		metric.WithDescription("Number of teams showing decline"),
		metric.WithUnit("{team}"),
	)
	handleError(err)

	m.TeamsStable, err = m.meter.Int64Counter(
		"teams360.health.teams_stable",
		metric.WithDescription("Number of teams with stable scores"),
		metric.WithUnit("{team}"),
	)
	handleError(err)

	// ==========================================
	// API PERFORMANCE METRICS
	// ==========================================

	m.APIRequestDuration, err = m.meter.Float64Histogram(
		"teams360.api.request_duration",
		metric.WithDescription("API request duration in milliseconds"),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000),
	)
	handleError(err)

	m.APIErrors, err = m.meter.Int64Counter(
		"teams360.api.errors",
		metric.WithDescription("API error count by endpoint and status"),
		metric.WithUnit("{error}"),
	)
	handleError(err)

	m.DBQueryDuration, err = m.meter.Float64Histogram(
		"teams360.db.query_duration",
		metric.WithDescription("Database query duration in milliseconds"),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(1, 5, 10, 25, 50, 100, 250, 500, 1000),
	)
	handleError(err)

	// ==========================================
	// MANAGER/LEADERSHIP METRICS
	// ==========================================

	m.ManagerDashboardAccess, err = m.meter.Int64Counter(
		"teams360.manager.dashboard_access",
		metric.WithDescription("Manager dashboard access count"),
		metric.WithUnit("{access}"),
	)
	handleError(err)

	m.TeamComparisonViews, err = m.meter.Int64Counter(
		"teams360.manager.team_comparison_views",
		metric.WithDescription("Cross-team comparison view count"),
		metric.WithUnit("{view}"),
	)
	handleError(err)

	m.TrendReportViews, err = m.meter.Int64Counter(
		"teams360.manager.trend_report_views",
		metric.WithDescription("Trend report view count"),
		metric.WithUnit("{view}"),
	)
	handleError(err)
}

func handleError(err error) {
	if err != nil {
		otel.Handle(err)
	}
}

// ==========================================
// CONVENIENCE METHODS FOR RECORDING METRICS
// ==========================================

// RecordLoginAttempt records a login attempt with outcome
func (m *Metrics) RecordLoginAttempt(ctx context.Context, success bool, reason string) {
	m.LoginAttempts.Add(ctx, 1)
	if success {
		m.LoginSuccesses.Add(ctx, 1)
		m.ActiveSessions.Add(ctx, 1)
	} else {
		m.LoginFailures.Add(ctx, 1, metric.WithAttributes(
			attribute.String("reason", reason),
		))
	}
}

// RecordLogout records a logout and session duration
func (m *Metrics) RecordLogout(ctx context.Context, sessionDurationSeconds float64) {
	m.ActiveSessions.Add(ctx, -1)
	if sessionDurationSeconds > 0 {
		m.SessionDuration.Record(ctx, sessionDurationSeconds)
	}
}

// RecordPageView records a page view
func (m *Metrics) RecordPageView(ctx context.Context, pageName string, userRole string) {
	m.PageViews.Add(ctx, 1, metric.WithAttributes(
		attribute.String("page", pageName),
		attribute.String("role", userRole),
	))
}

// RecordDashboardView records a dashboard view by role
func (m *Metrics) RecordDashboardView(ctx context.Context, dashboardType string, userRole string) {
	m.DashboardViews.Add(ctx, 1, metric.WithAttributes(
		attribute.String("dashboard_type", dashboardType),
		attribute.String("role", userRole),
	))
}

// RecordSurveyStarted records when a user starts a survey
func (m *Metrics) RecordSurveyStarted(ctx context.Context, teamID string) {
	m.SurveysStarted.Add(ctx, 1, metric.WithAttributes(
		attribute.String("team_id", teamID),
	))
}

// RecordSurveyCompleted records a completed survey with duration and response details
func (m *Metrics) RecordSurveyCompleted(ctx context.Context, teamID string, durationSeconds float64, responseCount int, commentCount int) {
	m.SurveysCompleted.Add(ctx, 1, metric.WithAttributes(
		attribute.String("team_id", teamID),
	))

	if durationSeconds > 0 {
		m.SurveyDuration.Record(ctx, durationSeconds, metric.WithAttributes(
			attribute.String("team_id", teamID),
		))
	}

	m.ResponsesSubmitted.Add(ctx, int64(responseCount), metric.WithAttributes(
		attribute.String("team_id", teamID),
	))

	if commentCount > 0 {
		m.CommentsProvided.Add(ctx, int64(commentCount), metric.WithAttributes(
			attribute.String("team_id", teamID),
		))
	}
}

// RecordSurveyAbandoned records when a survey is abandoned
func (m *Metrics) RecordSurveyAbandoned(ctx context.Context, teamID string, atDimension string) {
	m.SurveysAbandoned.Add(ctx, 1, metric.WithAttributes(
		attribute.String("team_id", teamID),
		attribute.String("abandoned_at", atDimension),
	))
}

// RecordHealthScore records a submitted health score
func (m *Metrics) RecordHealthScore(ctx context.Context, teamID string, dimensionID string, score float64) {
	m.HealthScoreSubmitted.Record(ctx, score, metric.WithAttributes(
		attribute.String("team_id", teamID),
	))

	m.DimensionScores.Record(ctx, score, metric.WithAttributes(
		attribute.String("dimension_id", dimensionID),
	))
}

// RecordTeamHealthStatus records the health status category of a team
func (m *Metrics) RecordTeamHealthStatus(ctx context.Context, status string, teamID string) {
	attrs := metric.WithAttributes(attribute.String("team_id", teamID))
	switch status {
	case "improving":
		m.TeamsImproving.Add(ctx, 1, attrs)
	case "declining":
		m.TeamsDeclining.Add(ctx, 1, attrs)
	case "stable":
		m.TeamsStable.Add(ctx, 1, attrs)
	}
}

// RecordAPIRequest records API request metrics
func (m *Metrics) RecordAPIRequest(ctx context.Context, endpoint string, method string, statusCode int, durationMs float64) {
	attrs := []attribute.KeyValue{
		attribute.String("endpoint", endpoint),
		attribute.String("method", method),
		attribute.Int("status_code", statusCode),
	}

	m.APIRequestDuration.Record(ctx, durationMs, metric.WithAttributes(attrs...))

	if statusCode >= 400 {
		m.APIErrors.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
}

// RecordDBQuery records database query duration
func (m *Metrics) RecordDBQuery(ctx context.Context, operation string, table string, durationMs float64) {
	m.DBQueryDuration.Record(ctx, durationMs, metric.WithAttributes(
		attribute.String("operation", operation),
		attribute.String("table", table),
	))
}

// RecordManagerDashboardAccess records manager dashboard access
func (m *Metrics) RecordManagerDashboardAccess(ctx context.Context, managerID string, viewType string) {
	m.ManagerDashboardAccess.Add(ctx, 1, metric.WithAttributes(
		attribute.String("manager_id", managerID),
		attribute.String("view_type", viewType),
	))

	switch viewType {
	case "team_comparison":
		m.TeamComparisonViews.Add(ctx, 1)
	case "trends":
		m.TrendReportViews.Add(ctx, 1)
	}
}

// UpdateTeamsAtRisk updates the count of teams at risk (call periodically or on health check submission)
func (m *Metrics) UpdateTeamsAtRisk(ctx context.Context, delta int64) {
	m.TeamsAtRisk.Add(ctx, delta)
}

// UpdateActiveTeams updates the count of active teams
func (m *Metrics) UpdateActiveTeams(ctx context.Context, delta int64) {
	m.ActiveTeams.Add(ctx, delta)
}
