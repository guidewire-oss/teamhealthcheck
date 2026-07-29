// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const meterName = "teams360"

// Business metrics for the application
var (
	// --- Authentication Metrics ---
	authLoginTotal        metric.Int64Counter
	authLoginDuration     metric.Float64Histogram
	authLogoutTotal       metric.Int64Counter
	authTokenRefresh      metric.Int64Counter
	authFailuresTotal     metric.Int64Counter
	passwordResetRequest  metric.Int64Counter
	passwordResetComplete metric.Int64Counter

	// --- Session & Engagement Metrics (Product Analytics) ---
	activeSessionsGauge metric.Int64UpDownCounter // Currently logged-in users
	sessionDuration     metric.Float64Histogram   // How long users stay logged in
	pageViews           metric.Int64Counter       // Page/endpoint views
	featureUsage        metric.Int64Counter       // Feature engagement tracking
	userReturnVisits    metric.Int64Counter       // Returning users (vs new)
	dailyActiveUsers    metric.Int64Gauge         // DAU tracking
	weeklyActiveUsers   metric.Int64Gauge         // WAU tracking
	monthlyActiveUsers  metric.Int64Gauge         // MAU tracking

	// --- Health Check / Survey Metrics ---
	surveySubmittedTotal  metric.Int64Counter
	surveySubmitDuration  metric.Float64Histogram
	surveyResponsesTotal  metric.Int64Counter
	surveyDimensionScores metric.Float64Histogram
	surveyCompletionRate  metric.Float64Gauge
	activeSurveySessions  metric.Int64UpDownCounter

	// --- Survey Funnel Metrics (Drop-off Analysis) ---
	surveyStartedTotal   metric.Int64Counter     // Surveys started (page loaded)
	surveyAbandonedTotal metric.Int64Counter     // Surveys abandoned (started but not completed)
	surveyTimeToComplete metric.Float64Histogram // Full time from start to submit
	surveyCommentsTotal  metric.Int64Counter     // Responses with comments
	surveyCommentRate    metric.Float64Gauge     // % of responses with comments

	// --- Team Health Business Metrics ---
	teamHealthQueriesTotal  metric.Int64Counter
	teamHealthQueryDuration metric.Float64Histogram
	teamsActiveTotal        metric.Int64Gauge
	teamsAtRiskTotal        metric.Int64Gauge       // Teams with health < 2.0
	teamHealthScoreAvg      metric.Float64Gauge     // Org-wide average health
	teamHealthByDimension   metric.Float64Histogram // Health distribution by dimension
	teamsImprovingTotal     metric.Int64Counter     // Teams showing improvement
	teamsDecliningTotal     metric.Int64Counter     // Teams showing decline

	// --- Manager/Dashboard Engagement ---
	managerDashboardViews  metric.Int64Counter // Manager dashboard access
	teamLeadDashboardViews metric.Int64Counter // Team lead dashboard access
	trendReportViews       metric.Int64Counter // Trend analysis views
	exportReportTotal      metric.Int64Counter // Report exports (future feature)

	// --- User Metrics ---
	userRegistrations metric.Int64Counter
	activeUsersGauge  metric.Int64Gauge

	// --- Database Metrics ---
	dbQueryTotal        metric.Int64Counter
	dbQueryDuration     metric.Float64Histogram
	dbErrorsTotal       metric.Int64Counter
	dbConnectionsActive metric.Int64UpDownCounter

	// --- Rate Limiting Metrics ---
	rateLimitExceeded metric.Int64Counter

	// --- General HTTP Metrics (supplemental to otelgin) ---
	httpRequestsInflight metric.Int64UpDownCounter

	// --- API Performance Metrics ---
	apiLatencyByEndpoint metric.Float64Histogram // Latency breakdown by endpoint
	apiErrorsByEndpoint  metric.Int64Counter     // Errors by endpoint and status code
)

// initBusinessMetrics initializes all business metrics
func initBusinessMetrics() error {
	meter := otel.Meter(meterName)
	var err error

	// --- Authentication Metrics ---
	authLoginTotal, err = meter.Int64Counter("auth.login.total",
		metric.WithDescription("Total number of login attempts"),
		metric.WithUnit("{attempts}"),
	)
	if err != nil {
		return err
	}

	authLoginDuration, err = meter.Float64Histogram("auth.login.duration",
		metric.WithDescription("Login operation duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0),
	)
	if err != nil {
		return err
	}

	authLogoutTotal, err = meter.Int64Counter("auth.logout.total",
		metric.WithDescription("Total number of logout operations"),
		metric.WithUnit("{operations}"),
	)
	if err != nil {
		return err
	}

	authTokenRefresh, err = meter.Int64Counter("auth.token.refresh.total",
		metric.WithDescription("Total number of token refresh operations"),
		metric.WithUnit("{operations}"),
	)
	if err != nil {
		return err
	}

	authFailuresTotal, err = meter.Int64Counter("auth.failures.total",
		metric.WithDescription("Total number of authentication failures by reason"),
		metric.WithUnit("{failures}"),
	)
	if err != nil {
		return err
	}

	passwordResetRequest, err = meter.Int64Counter("auth.password_reset.request.total",
		metric.WithDescription("Total number of password reset requests"),
		metric.WithUnit("{requests}"),
	)
	if err != nil {
		return err
	}

	passwordResetComplete, err = meter.Int64Counter("auth.password_reset.complete.total",
		metric.WithDescription("Total number of completed password resets"),
		metric.WithUnit("{completions}"),
	)
	if err != nil {
		return err
	}

	// --- Health Check / Survey Metrics ---
	surveySubmittedTotal, err = meter.Int64Counter("survey.submitted.total",
		metric.WithDescription("Total number of health check surveys submitted"),
		metric.WithUnit("{surveys}"),
	)
	if err != nil {
		return err
	}

	surveySubmitDuration, err = meter.Float64Histogram("survey.submit.duration",
		metric.WithDescription("Survey submission duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0),
	)
	if err != nil {
		return err
	}

	surveyResponsesTotal, err = meter.Int64Counter("survey.responses.total",
		metric.WithDescription("Total number of individual dimension responses"),
		metric.WithUnit("{responses}"),
	)
	if err != nil {
		return err
	}

	surveyDimensionScores, err = meter.Float64Histogram("survey.dimension.score",
		metric.WithDescription("Distribution of dimension scores (1=red, 2=yellow, 3=green)"),
		metric.WithUnit("{score}"),
		metric.WithExplicitBucketBoundaries(1.0, 1.5, 2.0, 2.5, 3.0),
	)
	if err != nil {
		return err
	}

	surveyCompletionRate, err = meter.Float64Gauge("survey.completion.rate",
		metric.WithDescription("Survey completion rate per team"),
		metric.WithUnit("{ratio}"),
	)
	if err != nil {
		return err
	}

	activeSurveySessions, err = meter.Int64UpDownCounter("survey.sessions.active",
		metric.WithDescription("Number of active survey sessions"),
		metric.WithUnit("{sessions}"),
	)
	if err != nil {
		return err
	}

	// --- Team Metrics ---
	teamHealthQueriesTotal, err = meter.Int64Counter("team.health.queries.total",
		metric.WithDescription("Total number of team health queries"),
		metric.WithUnit("{queries}"),
	)
	if err != nil {
		return err
	}

	teamHealthQueryDuration, err = meter.Float64Histogram("team.health.query.duration",
		metric.WithDescription("Team health query duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5),
	)
	if err != nil {
		return err
	}

	teamsActiveTotal, err = meter.Int64Gauge("team.active.total",
		metric.WithDescription("Total number of active teams"),
		metric.WithUnit("{teams}"),
	)
	if err != nil {
		return err
	}

	// --- User Metrics ---
	userRegistrations, err = meter.Int64Counter("user.registrations.total",
		metric.WithDescription("Total number of user registrations"),
		metric.WithUnit("{registrations}"),
	)
	if err != nil {
		return err
	}

	activeUsersGauge, err = meter.Int64Gauge("user.active.total",
		metric.WithDescription("Total number of active users"),
		metric.WithUnit("{users}"),
	)
	if err != nil {
		return err
	}

	// --- Database Metrics ---
	dbQueryTotal, err = meter.Int64Counter("db.query.total",
		metric.WithDescription("Total number of database queries"),
		metric.WithUnit("{queries}"),
	)
	if err != nil {
		return err
	}

	dbQueryDuration, err = meter.Float64Histogram("db.query.duration",
		metric.WithDescription("Database query duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0),
	)
	if err != nil {
		return err
	}

	dbErrorsTotal, err = meter.Int64Counter("db.errors.total",
		metric.WithDescription("Total number of database errors"),
		metric.WithUnit("{errors}"),
	)
	if err != nil {
		return err
	}

	dbConnectionsActive, err = meter.Int64UpDownCounter("db.connections.active",
		metric.WithDescription("Number of active database connections"),
		metric.WithUnit("{connections}"),
	)
	if err != nil {
		return err
	}

	// --- Rate Limiting Metrics ---
	rateLimitExceeded, err = meter.Int64Counter("ratelimit.exceeded.total",
		metric.WithDescription("Total number of rate limit exceeded events"),
		metric.WithUnit("{events}"),
	)
	if err != nil {
		return err
	}

	// --- HTTP Metrics ---
	httpRequestsInflight, err = meter.Int64UpDownCounter("http.requests.inflight",
		metric.WithDescription("Number of HTTP requests currently being processed"),
		metric.WithUnit("{requests}"),
	)
	if err != nil {
		return err
	}

	// --- Session & Engagement Metrics (Product Analytics) ---
	activeSessionsGauge, err = meter.Int64UpDownCounter("session.active.total",
		metric.WithDescription("Number of currently active user sessions"),
		metric.WithUnit("{sessions}"),
	)
	if err != nil {
		return err
	}

	sessionDuration, err = meter.Float64Histogram("session.duration",
		metric.WithDescription("User session duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(60, 300, 600, 1800, 3600, 7200, 14400, 28800), // 1m to 8h
	)
	if err != nil {
		return err
	}

	pageViews, err = meter.Int64Counter("engagement.page_views.total",
		metric.WithDescription("Total page/endpoint views"),
		metric.WithUnit("{views}"),
	)
	if err != nil {
		return err
	}

	featureUsage, err = meter.Int64Counter("engagement.feature_usage.total",
		metric.WithDescription("Feature usage count"),
		metric.WithUnit("{uses}"),
	)
	if err != nil {
		return err
	}

	userReturnVisits, err = meter.Int64Counter("engagement.return_visits.total",
		metric.WithDescription("Returning user visits"),
		metric.WithUnit("{visits}"),
	)
	if err != nil {
		return err
	}

	dailyActiveUsers, err = meter.Int64Gauge("engagement.dau",
		metric.WithDescription("Daily active users"),
		metric.WithUnit("{users}"),
	)
	if err != nil {
		return err
	}

	weeklyActiveUsers, err = meter.Int64Gauge("engagement.wau",
		metric.WithDescription("Weekly active users"),
		metric.WithUnit("{users}"),
	)
	if err != nil {
		return err
	}

	monthlyActiveUsers, err = meter.Int64Gauge("engagement.mau",
		metric.WithDescription("Monthly active users"),
		metric.WithUnit("{users}"),
	)
	if err != nil {
		return err
	}

	// --- Survey Funnel Metrics ---
	surveyStartedTotal, err = meter.Int64Counter("survey.started.total",
		metric.WithDescription("Surveys started (survey page loaded)"),
		metric.WithUnit("{surveys}"),
	)
	if err != nil {
		return err
	}

	surveyAbandonedTotal, err = meter.Int64Counter("survey.abandoned.total",
		metric.WithDescription("Surveys abandoned without completion"),
		metric.WithUnit("{surveys}"),
	)
	if err != nil {
		return err
	}

	surveyTimeToComplete, err = meter.Float64Histogram("survey.time_to_complete",
		metric.WithDescription("Total time from survey start to submission in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(30, 60, 120, 180, 300, 600, 900, 1800), // 30s to 30m
	)
	if err != nil {
		return err
	}

	surveyCommentsTotal, err = meter.Int64Counter("survey.comments.total",
		metric.WithDescription("Responses with comments provided"),
		metric.WithUnit("{comments}"),
	)
	if err != nil {
		return err
	}

	surveyCommentRate, err = meter.Float64Gauge("survey.comment_rate",
		metric.WithDescription("Percentage of responses with comments"),
		metric.WithUnit("{ratio}"),
	)
	if err != nil {
		return err
	}

	// --- Team Health Business Metrics ---
	teamsAtRiskTotal, err = meter.Int64Gauge("team.at_risk.total",
		metric.WithDescription("Number of teams with health score below 2.0"),
		metric.WithUnit("{teams}"),
	)
	if err != nil {
		return err
	}

	teamHealthScoreAvg, err = meter.Float64Gauge("team.health.average",
		metric.WithDescription("Organization-wide average health score"),
		metric.WithUnit("{score}"),
	)
	if err != nil {
		return err
	}

	teamHealthByDimension, err = meter.Float64Histogram("team.health.by_dimension",
		metric.WithDescription("Health score distribution by dimension"),
		metric.WithUnit("{score}"),
		metric.WithExplicitBucketBoundaries(1.0, 1.5, 2.0, 2.5, 3.0),
	)
	if err != nil {
		return err
	}

	teamsImprovingTotal, err = meter.Int64Counter("team.improving.total",
		metric.WithDescription("Teams showing improvement"),
		metric.WithUnit("{teams}"),
	)
	if err != nil {
		return err
	}

	teamsDecliningTotal, err = meter.Int64Counter("team.declining.total",
		metric.WithDescription("Teams showing decline"),
		metric.WithUnit("{teams}"),
	)
	if err != nil {
		return err
	}

	// --- Manager/Dashboard Engagement ---
	managerDashboardViews, err = meter.Int64Counter("dashboard.manager.views.total",
		metric.WithDescription("Manager dashboard views"),
		metric.WithUnit("{views}"),
	)
	if err != nil {
		return err
	}

	teamLeadDashboardViews, err = meter.Int64Counter("dashboard.teamlead.views.total",
		metric.WithDescription("Team lead dashboard views"),
		metric.WithUnit("{views}"),
	)
	if err != nil {
		return err
	}

	trendReportViews, err = meter.Int64Counter("dashboard.trends.views.total",
		metric.WithDescription("Trend report views"),
		metric.WithUnit("{views}"),
	)
	if err != nil {
		return err
	}

	exportReportTotal, err = meter.Int64Counter("dashboard.exports.total",
		metric.WithDescription("Report exports"),
		metric.WithUnit("{exports}"),
	)
	if err != nil {
		return err
	}

	// --- API Performance ---
	apiLatencyByEndpoint, err = meter.Float64Histogram("api.latency.by_endpoint",
		metric.WithDescription("API latency by endpoint in milliseconds"),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000),
	)
	if err != nil {
		return err
	}

	apiErrorsByEndpoint, err = meter.Int64Counter("api.errors.by_endpoint",
		metric.WithDescription("API errors by endpoint and status"),
		metric.WithUnit("{errors}"),
	)
	if err != nil {
		return err
	}

	return nil
}

// --- Authentication Metric Recording ---

// RecordLogin records a login attempt
func RecordLogin(ctx context.Context, success bool, duration time.Duration, reason string) {
	// Skip if metrics not initialized (e.g., in tests)
	if authLoginTotal == nil {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.Bool("success", success),
	}
	if !success && reason != "" {
		attrs = append(attrs, attribute.String("reason", reason))
	}

	authLoginTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	authLoginDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))

	if !success {
		authFailuresTotal.Add(ctx, 1, metric.WithAttributes(
			attribute.String("type", "login"),
			attribute.String("reason", reason),
		))
	}
}

// RecordLogout records a logout operation
func RecordLogout(ctx context.Context) {
	// Skip if metrics not initialized (e.g., in tests)
	if authLogoutTotal == nil {
		return
	}
	authLogoutTotal.Add(ctx, 1)
}

// RecordTokenRefresh records a token refresh operation
func RecordTokenRefresh(ctx context.Context, success bool, reason string) {
	// Skip if metrics not initialized (e.g., in tests)
	if authTokenRefresh == nil {
		return
	}

	attrs := []attribute.KeyValue{attribute.Bool("success", success)}
	if !success && reason != "" {
		attrs = append(attrs, attribute.String("reason", reason))
	}
	authTokenRefresh.Add(ctx, 1, metric.WithAttributes(attrs...))

	if !success && authFailuresTotal != nil {
		authFailuresTotal.Add(ctx, 1, metric.WithAttributes(
			attribute.String("type", "token_refresh"),
			attribute.String("reason", reason),
		))
	}
}

// RecordPasswordResetRequest records a password reset request
func RecordPasswordResetRequest(ctx context.Context, userFound bool) {
	// Skip if metrics not initialized (e.g., in tests)
	if passwordResetRequest == nil {
		return
	}
	passwordResetRequest.Add(ctx, 1, metric.WithAttributes(
		attribute.Bool("user_found", userFound),
	))
}

// RecordPasswordResetComplete records a completed password reset
func RecordPasswordResetComplete(ctx context.Context, success bool, reason string) {
	// Skip if metrics not initialized (e.g., in tests)
	if passwordResetComplete == nil {
		return
	}
	attrs := []attribute.KeyValue{attribute.Bool("success", success)}
	if !success && reason != "" {
		attrs = append(attrs, attribute.String("reason", reason))
	}
	passwordResetComplete.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// --- Survey Metric Recording ---

// RecordSurveySubmission records a survey submission
func RecordSurveySubmission(ctx context.Context, teamID, assessmentPeriod string, dimensionCount int, duration time.Duration) {
	// Skip if metrics not initialized (e.g., in tests)
	if surveySubmittedTotal == nil {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String("team_id", teamID),
		attribute.String("assessment_period", assessmentPeriod),
	}

	surveySubmittedTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	surveySubmitDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
	surveyResponsesTotal.Add(ctx, int64(dimensionCount), metric.WithAttributes(attrs...))
}

// RecordDimensionScore records an individual dimension score
func RecordDimensionScore(ctx context.Context, dimensionID string, score float64, trend string) {
	// Skip if metrics not initialized (e.g., in tests)
	if surveyDimensionScores == nil {
		return
	}
	surveyDimensionScores.Record(ctx, score, metric.WithAttributes(
		attribute.String("dimension_id", dimensionID),
		attribute.String("trend", trend),
	))
}

// RecordSurveyCompletionRate records the completion rate for a team
func RecordSurveyCompletionRate(ctx context.Context, teamID string, rate float64) {
	// Skip if metrics not initialized (e.g., in tests)
	if surveyCompletionRate == nil {
		return
	}
	surveyCompletionRate.Record(ctx, rate, metric.WithAttributes(
		attribute.String("team_id", teamID),
	))
}

// IncrementActiveSurveySessions increments the active survey sessions counter
func IncrementActiveSurveySessions(ctx context.Context) {
	// Skip if metrics not initialized (e.g., in tests)
	if activeSurveySessions == nil {
		return
	}
	activeSurveySessions.Add(ctx, 1)
}

// DecrementActiveSurveySessions decrements the active survey sessions counter
func DecrementActiveSurveySessions(ctx context.Context) {
	// Skip if metrics not initialized (e.g., in tests)
	if activeSurveySessions == nil {
		return
	}
	activeSurveySessions.Add(ctx, -1)
}

// --- Team Metric Recording ---

// RecordTeamHealthQuery records a team health query
func RecordTeamHealthQuery(ctx context.Context, teamID string, duration time.Duration) {
	// Skip if metrics not initialized (e.g., in tests)
	if teamHealthQueriesTotal == nil {
		return
	}
	teamHealthQueriesTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("team_id", teamID),
	))
	teamHealthQueryDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(
		attribute.String("team_id", teamID),
	))
}

// RecordActiveTeams records the total number of active teams
func RecordActiveTeams(ctx context.Context, count int64) {
	// Skip if metrics not initialized (e.g., in tests)
	if teamsActiveTotal == nil {
		return
	}
	teamsActiveTotal.Record(ctx, count)
}

// --- User Metric Recording ---

// RecordUserRegistration records a user registration
func RecordUserRegistration(ctx context.Context, hierarchyLevel string) {
	// Skip if metrics not initialized (e.g., in tests)
	if userRegistrations == nil {
		return
	}
	userRegistrations.Add(ctx, 1, metric.WithAttributes(
		attribute.String("hierarchy_level", hierarchyLevel),
	))
}

// RecordActiveUsers records the total number of active users
func RecordActiveUsers(ctx context.Context, count int64) {
	// Skip if metrics not initialized (e.g., in tests)
	if activeUsersGauge == nil {
		return
	}
	activeUsersGauge.Record(ctx, count)
}

// --- Database Metric Recording ---

// RecordDBQuery records a database query
func RecordDBQuery(ctx context.Context, operation, table string, duration time.Duration, err error) {
	// Skip if metrics not initialized (e.g., in tests)
	if dbQueryTotal == nil {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String("operation", operation),
		attribute.String("table", table),
	}

	dbQueryTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	dbQueryDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))

	if err != nil && dbErrorsTotal != nil {
		dbErrorsTotal.Add(ctx, 1, metric.WithAttributes(
			attribute.String("operation", operation),
			attribute.String("table", table),
		))
	}
}

// IncrementDBConnections increments active DB connections
func IncrementDBConnections(ctx context.Context) {
	// Skip if metrics not initialized (e.g., in tests)
	if dbConnectionsActive == nil {
		return
	}
	dbConnectionsActive.Add(ctx, 1)
}

// DecrementDBConnections decrements active DB connections
func DecrementDBConnections(ctx context.Context) {
	// Skip if metrics not initialized (e.g., in tests)
	if dbConnectionsActive == nil {
		return
	}
	dbConnectionsActive.Add(ctx, -1)
}

// --- Rate Limiting Metric Recording ---

// RecordRateLimitExceeded records a rate limit exceeded event
func RecordRateLimitExceeded(ctx context.Context, clientIP, endpoint string) {
	// Skip if metrics not initialized (e.g., in tests)
	if rateLimitExceeded == nil {
		return
	}
	rateLimitExceeded.Add(ctx, 1, metric.WithAttributes(
		attribute.String("endpoint", endpoint),
		// Note: Not recording IP for privacy
	))
}

// --- HTTP Metric Recording ---

// IncrementInflightRequests increments inflight HTTP requests
func IncrementInflightRequests(ctx context.Context) {
	// Skip if metrics not initialized (e.g., in tests)
	if httpRequestsInflight == nil {
		return
	}
	httpRequestsInflight.Add(ctx, 1)
}

// DecrementInflightRequests decrements inflight HTTP requests
func DecrementInflightRequests(ctx context.Context) {
	// Skip if metrics not initialized (e.g., in tests)
	if httpRequestsInflight == nil {
		return
	}
	httpRequestsInflight.Add(ctx, -1)
}

// ==========================================
// SESSION & ENGAGEMENT METRICS (Product Analytics)
// ==========================================

// IncrementActiveSessions increments active sessions (call on login)
func IncrementActiveSessions(ctx context.Context) {
	if activeSessionsGauge == nil {
		return
	}
	activeSessionsGauge.Add(ctx, 1)
}

// DecrementActiveSessions decrements active sessions (call on logout/expiry)
func DecrementActiveSessions(ctx context.Context) {
	if activeSessionsGauge == nil {
		return
	}
	activeSessionsGauge.Add(ctx, -1)
}

// RecordSessionDuration records how long a user session lasted
func RecordSessionDuration(ctx context.Context, durationSeconds float64, userID string) {
	if sessionDuration == nil {
		return
	}
	sessionDuration.Record(ctx, durationSeconds, metric.WithAttributes(
		attribute.String("user_id", userID),
	))
}

// RecordPageView records a page/endpoint view
func RecordPageView(ctx context.Context, page, userRole, userID string) {
	if pageViews == nil {
		return
	}
	pageViews.Add(ctx, 1, metric.WithAttributes(
		attribute.String("page", page),
		attribute.String("role", userRole),
	))
}

// RecordFeatureUsage records when a feature is used
func RecordFeatureUsage(ctx context.Context, feature, userRole string) {
	if featureUsage == nil {
		return
	}
	featureUsage.Add(ctx, 1, metric.WithAttributes(
		attribute.String("feature", feature),
		attribute.String("role", userRole),
	))
}

// RecordReturnVisit records a returning user
func RecordReturnVisit(ctx context.Context, userID string) {
	if userReturnVisits == nil {
		return
	}
	userReturnVisits.Add(ctx, 1, metric.WithAttributes(
		attribute.String("user_id", userID),
	))
}

// RecordDAU records daily active users count
func RecordDAU(ctx context.Context, count int64) {
	if dailyActiveUsers == nil {
		return
	}
	dailyActiveUsers.Record(ctx, count)
}

// RecordWAU records weekly active users count
func RecordWAU(ctx context.Context, count int64) {
	if weeklyActiveUsers == nil {
		return
	}
	weeklyActiveUsers.Record(ctx, count)
}

// RecordMAU records monthly active users count
func RecordMAU(ctx context.Context, count int64) {
	if monthlyActiveUsers == nil {
		return
	}
	monthlyActiveUsers.Record(ctx, count)
}

// ==========================================
// SURVEY FUNNEL METRICS
// ==========================================

// RecordSurveyStarted records when a user starts a survey (loads the survey page)
func RecordSurveyStarted(ctx context.Context, teamID, userID string) {
	if surveyStartedTotal == nil {
		return
	}
	surveyStartedTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("team_id", teamID),
	))
}

// RecordSurveyAbandoned records when a survey is abandoned
func RecordSurveyAbandoned(ctx context.Context, teamID, abandonedAtDimension string, timeSpentSeconds float64) {
	if surveyAbandonedTotal == nil {
		return
	}
	surveyAbandonedTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("team_id", teamID),
		attribute.String("abandoned_at", abandonedAtDimension),
	))
}

// RecordSurveyTimeToComplete records full time from start to submit
func RecordSurveyTimeToComplete(ctx context.Context, teamID string, durationSeconds float64) {
	if surveyTimeToComplete == nil {
		return
	}
	surveyTimeToComplete.Record(ctx, durationSeconds, metric.WithAttributes(
		attribute.String("team_id", teamID),
	))
}

// RecordSurveyWithComments records a response with comments
func RecordSurveyWithComments(ctx context.Context, teamID, dimensionID string) {
	if surveyCommentsTotal == nil {
		return
	}
	surveyCommentsTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("team_id", teamID),
		attribute.String("dimension_id", dimensionID),
	))
}

// RecordSurveyCommentRate records the comment rate for a team
func RecordSurveyCommentRate(ctx context.Context, teamID string, rate float64) {
	if surveyCommentRate == nil {
		return
	}
	surveyCommentRate.Record(ctx, rate, metric.WithAttributes(
		attribute.String("team_id", teamID),
	))
}

// ==========================================
// TEAM HEALTH BUSINESS METRICS
// ==========================================

// RecordTeamsAtRisk records number of teams with health < 2.0
func RecordTeamsAtRisk(ctx context.Context, count int64) {
	if teamsAtRiskTotal == nil {
		return
	}
	teamsAtRiskTotal.Record(ctx, count)
}

// RecordOrgHealthAverage records organization-wide average health
func RecordOrgHealthAverage(ctx context.Context, avgScore float64) {
	if teamHealthScoreAvg == nil {
		return
	}
	teamHealthScoreAvg.Record(ctx, avgScore)
}

// RecordHealthByDimension records health score distribution by dimension
func RecordHealthByDimension(ctx context.Context, dimensionID string, score float64) {
	if teamHealthByDimension == nil {
		return
	}
	teamHealthByDimension.Record(ctx, score, metric.WithAttributes(
		attribute.String("dimension_id", dimensionID),
	))
}

// RecordTeamImproving records a team showing improvement
func RecordTeamImproving(ctx context.Context, teamID string) {
	if teamsImprovingTotal == nil {
		return
	}
	teamsImprovingTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("team_id", teamID),
	))
}

// RecordTeamDeclining records a team showing decline
func RecordTeamDeclining(ctx context.Context, teamID string) {
	if teamsDecliningTotal == nil {
		return
	}
	teamsDecliningTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("team_id", teamID),
	))
}

// ==========================================
// DASHBOARD ENGAGEMENT METRICS
// ==========================================

// RecordManagerDashboardView records a manager dashboard view
func RecordManagerDashboardView(ctx context.Context, managerID, viewType string) {
	if managerDashboardViews == nil {
		return
	}
	managerDashboardViews.Add(ctx, 1, metric.WithAttributes(
		attribute.String("manager_id", managerID),
		attribute.String("view_type", viewType),
	))
}

// RecordTeamLeadDashboardView records a team lead dashboard view
func RecordTeamLeadDashboardView(ctx context.Context, teamLeadID, viewType string) {
	if teamLeadDashboardViews == nil {
		return
	}
	teamLeadDashboardViews.Add(ctx, 1, metric.WithAttributes(
		attribute.String("team_lead_id", teamLeadID),
		attribute.String("view_type", viewType),
	))
}

// RecordTrendReportView records a trend report view
func RecordTrendReportView(ctx context.Context, userID, reportType string) {
	if trendReportViews == nil {
		return
	}
	trendReportViews.Add(ctx, 1, metric.WithAttributes(
		attribute.String("user_id", userID),
		attribute.String("report_type", reportType),
	))
}

// RecordExport records a report export
func RecordExport(ctx context.Context, exportType, format string) {
	if exportReportTotal == nil {
		return
	}
	exportReportTotal.Add(ctx, 1, metric.WithAttributes(
		attribute.String("export_type", exportType),
		attribute.String("format", format),
	))
}

// ==========================================
// API PERFORMANCE METRICS
// ==========================================

// RecordAPILatency records API latency for an endpoint
func RecordAPILatency(ctx context.Context, endpoint, method string, statusCode int, durationMs float64) {
	if apiLatencyByEndpoint == nil {
		return
	}
	apiLatencyByEndpoint.Record(ctx, durationMs, metric.WithAttributes(
		attribute.String("endpoint", endpoint),
		attribute.String("method", method),
		attribute.Int("status_code", statusCode),
	))
}

// RecordAPIError records an API error
func RecordAPIError(ctx context.Context, endpoint, method string, statusCode int, errorType string) {
	if apiErrorsByEndpoint == nil {
		return
	}
	apiErrorsByEndpoint.Add(ctx, 1, metric.WithAttributes(
		attribute.String("endpoint", endpoint),
		attribute.String("method", method),
		attribute.Int("status_code", statusCode),
		attribute.String("error_type", errorType),
	))
}
