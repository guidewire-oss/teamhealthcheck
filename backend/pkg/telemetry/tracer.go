// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	// Tracer names for different components
	TracerAuth        = "teams360.auth"
	TracerHealthCheck = "teams360.healthcheck"
	TracerTeam        = "teams360.team"
	TracerUser        = "teams360.user"
	TracerDB          = "teams360.database"
	TracerHTTP        = "teams360.http"
)

// Common attribute keys for business context
const (
	AttrUserID           = "user.id"
	AttrUsername         = "user.username"
	AttrTeamID           = "team.id"
	AttrTeamName         = "team.name"
	AttrHierarchyLevel   = "user.hierarchy_level"
	AttrHealthCheckID    = "healthcheck.id"
	AttrAssessmentPeriod = "healthcheck.assessment_period"
	AttrSurveyComplete   = "survey.complete"
	AttrDimensionCount   = "survey.dimension_count"
	AttrAuthMethod       = "auth.method"
	AttrAuthSuccess      = "auth.success"
	AttrDBOperation      = "db.operation"
	AttrDBTable          = "db.table"
	AttrDBRowsAffected   = "db.rows_affected"
)

// Tracer returns a named tracer for the given component
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

// StartSpan starts a new span with the given name and returns the context and span
func StartSpan(ctx context.Context, tracerName, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return Tracer(tracerName).Start(ctx, spanName, opts...)
}

// SpanFromContext returns the current span from context
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// TraceID returns the trace ID from the current context as a string
func TraceID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span == nil {
		return ""
	}
	sc := span.SpanContext()
	if !sc.HasTraceID() {
		return ""
	}
	return sc.TraceID().String()
}

// SpanID returns the span ID from the current context as a string
func SpanID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span == nil {
		return ""
	}
	sc := span.SpanContext()
	if !sc.HasSpanID() {
		return ""
	}
	return sc.SpanID().String()
}

// SetSpanError records an error on the span and sets status to error
func SetSpanError(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// SetSpanOK sets the span status to OK
func SetSpanOK(span trace.Span) {
	span.SetStatus(codes.Ok, "")
}

// --- Auth tracing helpers ---

// StartAuthSpan starts a span for authentication operations
func StartAuthSpan(ctx context.Context, operation string) (context.Context, trace.Span) {
	return StartSpan(ctx, TracerAuth, "auth."+operation,
		trace.WithSpanKind(trace.SpanKindInternal),
	)
}

// SetAuthAttributes sets common auth attributes on a span
func SetAuthAttributes(span trace.Span, userID, username string, success bool) {
	span.SetAttributes(
		attribute.String(AttrUserID, userID),
		attribute.String(AttrUsername, maskUsername(username)),
		attribute.Bool(AttrAuthSuccess, success),
	)
}

// --- Health Check tracing helpers ---

// StartHealthCheckSpan starts a span for health check operations
func StartHealthCheckSpan(ctx context.Context, operation string) (context.Context, trace.Span) {
	return StartSpan(ctx, TracerHealthCheck, "healthcheck."+operation,
		trace.WithSpanKind(trace.SpanKindInternal),
	)
}

// SetHealthCheckAttributes sets health check attributes on a span
func SetHealthCheckAttributes(span trace.Span, sessionID, teamID, assessmentPeriod string, dimensionCount int) {
	span.SetAttributes(
		attribute.String(AttrHealthCheckID, sessionID),
		attribute.String(AttrTeamID, teamID),
		attribute.String(AttrAssessmentPeriod, assessmentPeriod),
		attribute.Int(AttrDimensionCount, dimensionCount),
	)
}

// --- Team tracing helpers ---

// StartTeamSpan starts a span for team operations
func StartTeamSpan(ctx context.Context, operation string) (context.Context, trace.Span) {
	return StartSpan(ctx, TracerTeam, "team."+operation,
		trace.WithSpanKind(trace.SpanKindInternal),
	)
}

// SetTeamAttributes sets team attributes on a span
func SetTeamAttributes(span trace.Span, teamID, teamName string) {
	span.SetAttributes(
		attribute.String(AttrTeamID, teamID),
		attribute.String(AttrTeamName, teamName),
	)
}

// --- User tracing helpers ---

// StartUserSpan starts a span for user operations
func StartUserSpan(ctx context.Context, operation string) (context.Context, trace.Span) {
	return StartSpan(ctx, TracerUser, "user."+operation,
		trace.WithSpanKind(trace.SpanKindInternal),
	)
}

// SetUserAttributes sets user attributes on a span (with PII protection)
func SetUserAttributes(span trace.Span, userID, hierarchyLevel string) {
	span.SetAttributes(
		attribute.String(AttrUserID, userID),
		attribute.String(AttrHierarchyLevel, hierarchyLevel),
	)
}

// --- Database tracing helpers ---

// StartDBSpan starts a span for database operations
func StartDBSpan(ctx context.Context, operation, table string) (context.Context, trace.Span) {
	ctx, span := StartSpan(ctx, TracerDB, "db."+operation,
		trace.WithSpanKind(trace.SpanKindClient),
	)
	span.SetAttributes(
		attribute.String(AttrDBOperation, operation),
		attribute.String(AttrDBTable, table),
	)
	return ctx, span
}

// SetDBRowsAffected sets the rows affected attribute
func SetDBRowsAffected(span trace.Span, rows int64) {
	span.SetAttributes(attribute.Int64(AttrDBRowsAffected, rows))
}

// --- Helper functions ---

// maskUsername masks username for PII protection in traces
func maskUsername(username string) string {
	if len(username) <= 3 {
		return "***"
	}
	return username[:2] + "***" + string(username[len(username)-1])
}

// AddEvent adds an event to the current span
func AddEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span != nil {
		span.AddEvent(name, trace.WithAttributes(attrs...))
	}
}

// WithAttributes returns span start options with the given attributes
func WithAttributes(attrs ...attribute.KeyValue) trace.SpanStartOption {
	return trace.WithAttributes(attrs...)
}
