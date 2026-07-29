// SPDX-License-Identifier: Apache-2.0

// Package logger provides structured logging using zerolog.
// It is designed for security-conscious, cost-effective logging
// with a focus on debugging value.
package logger

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	// RequestIDKey is the context key for request ID
	RequestIDKey contextKey = "request_id"
	// UserIDKey is the context key for user ID
	UserIDKey contextKey = "user_id"
)

// Logger wraps zerolog.Logger with additional context methods
type Logger struct {
	zl zerolog.Logger
}

var (
	// globalLogger is the default logger instance
	globalLogger *Logger
)

// Config holds logger configuration
type Config struct {
	// Level sets the minimum log level (debug, info, warn, error)
	Level string
	// Pretty enables human-readable output (for development)
	Pretty bool
	// Output sets the output writer (defaults to os.Stdout)
	Output io.Writer
}

// Init initializes the global logger with the given configuration
func Init(cfg Config) {
	level := parseLevel(cfg.Level)
	zerolog.SetGlobalLevel(level)

	// Set timestamp format for consistency
	zerolog.TimeFieldFormat = time.RFC3339

	output := cfg.Output
	if output == nil {
		output = os.Stdout
	}

	var zl zerolog.Logger
	if cfg.Pretty {
		// Human-readable output for development
		zl = zerolog.New(zerolog.ConsoleWriter{
			Out:        output,
			TimeFormat: "15:04:05",
		}).With().Timestamp().Logger()
	} else {
		// JSON output for production
		zl = zerolog.New(output).With().Timestamp().Logger()
	}

	globalLogger = &Logger{zl: zl}
}

// parseLevel converts string level to zerolog.Level
func parseLevel(level string) zerolog.Level {
	switch level {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	default:
		return zerolog.InfoLevel
	}
}

// Get returns the global logger instance
func Get() *Logger {
	if globalLogger == nil {
		// Initialize with defaults if not configured
		Init(Config{Level: "info", Pretty: false})
	}
	return globalLogger
}

// WithContext returns a logger with context values (request_id, user_id, trace_id, span_id)
func (l *Logger) WithContext(ctx context.Context) *Logger {
	zl := l.zl

	if requestID, ok := ctx.Value(RequestIDKey).(string); ok && requestID != "" {
		zl = zl.With().Str("request_id", requestID).Logger()
	}

	if userID, ok := ctx.Value(UserIDKey).(string); ok && userID != "" {
		zl = zl.With().Str("user_id", userID).Logger()
	}

	// Add trace context from OpenTelemetry
	span := trace.SpanFromContext(ctx)
	if span != nil {
		sc := span.SpanContext()
		if sc.HasTraceID() {
			zl = zl.With().Str("trace_id", sc.TraceID().String()).Logger()
		}
		if sc.HasSpanID() {
			zl = zl.With().Str("span_id", sc.SpanID().String()).Logger()
		}
	}

	return &Logger{zl: zl}
}

// TraceID extracts trace ID from context as a string
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

// SpanID extracts span ID from context as a string
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

// WithField returns a logger with an additional field
func (l *Logger) WithField(key string, value interface{}) *Logger {
	return &Logger{zl: l.zl.With().Interface(key, value).Logger()}
}

// WithFields returns a logger with multiple additional fields
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	ctx := l.zl.With()
	for k, v := range fields {
		ctx = ctx.Interface(k, v)
	}
	return &Logger{zl: ctx.Logger()}
}

// WithError returns a logger with an error field
func (l *Logger) WithError(err error) *Logger {
	return &Logger{zl: l.zl.With().Err(err).Logger()}
}

// Debug logs a debug message (for development troubleshooting)
func (l *Logger) Debug(msg string) {
	l.zl.Debug().Msg(msg)
}

// Info logs an info message (normal operations)
func (l *Logger) Info(msg string) {
	l.zl.Info().Msg(msg)
}

// Warn logs a warning message (recoverable issues)
func (l *Logger) Warn(msg string) {
	l.zl.Warn().Msg(msg)
}

// Error logs an error message (failures that need attention)
func (l *Logger) Error(msg string) {
	l.zl.Error().Msg(msg)
}

// Fatal logs a fatal message and exits (unrecoverable errors)
func (l *Logger) Fatal(msg string) {
	l.zl.Fatal().Msg(msg)
}

// ============================================================================
// Convenience functions for common logging patterns
// ============================================================================

// Auth logs authentication-related events
func (l *Logger) Auth(action string) *AuthEvent {
	return &AuthEvent{
		logger: l,
		action: action,
	}
}

// AuthEvent represents an authentication event for structured logging
type AuthEvent struct {
	logger    *Logger
	action    string
	username  string
	userID    string
	ip        string
	reason    string
	endpoint  string
	requestID string
	details   string
}

// Username sets the username (masked for security)
func (e *AuthEvent) Username(username string) *AuthEvent {
	e.username = maskUsername(username)
	return e
}

// UserID sets the user ID
func (e *AuthEvent) UserID(userID string) *AuthEvent {
	e.userID = userID
	return e
}

// IP sets the client IP
func (e *AuthEvent) IP(ip string) *AuthEvent {
	e.ip = ip
	return e
}

// Reason sets the failure reason
func (e *AuthEvent) Reason(reason string) *AuthEvent {
	e.reason = reason
	return e
}

// Endpoint sets the API endpoint being accessed
func (e *AuthEvent) Endpoint(endpoint string) *AuthEvent {
	e.endpoint = endpoint
	return e
}

// RequestID sets the request ID for correlation
func (e *AuthEvent) RequestID(requestID string) *AuthEvent {
	e.requestID = requestID
	return e
}

// Details sets additional context about what happened
func (e *AuthEvent) Details(details string) *AuthEvent {
	e.details = details
	return e
}

// Success logs a successful auth event
func (e *AuthEvent) Success() {
	event := e.logger.zl.Info().
		Str("component", "auth").
		Str("action", e.action)

	if e.username != "" {
		event = event.Str("username", e.username)
	}
	if e.userID != "" {
		event = event.Str("user_id", e.userID)
	}
	if e.ip != "" {
		event = event.Str("client_ip", e.ip)
	}
	if e.endpoint != "" {
		event = event.Str("endpoint", e.endpoint)
	}
	if e.requestID != "" {
		event = event.Str("request_id", e.requestID)
	}
	if e.details != "" {
		event = event.Str("details", e.details)
	}

	event.Msg("auth_success")
}

// Failure logs a failed auth event
func (e *AuthEvent) Failure() {
	event := e.logger.zl.Warn().
		Str("component", "auth").
		Str("action", e.action)

	if e.username != "" {
		event = event.Str("username", e.username)
	}
	if e.userID != "" {
		event = event.Str("user_id", e.userID)
	}
	if e.ip != "" {
		event = event.Str("client_ip", e.ip)
	}
	if e.endpoint != "" {
		event = event.Str("endpoint", e.endpoint)
	}
	if e.requestID != "" {
		event = event.Str("request_id", e.requestID)
	}
	if e.reason != "" {
		event = event.Str("reason", e.reason)
	}
	if e.details != "" {
		event = event.Str("details", e.details)
	}

	event.Msg("auth_failure")
}

// DB logs database-related events
func (l *Logger) DB(operation string) *DBEvent {
	return &DBEvent{
		logger:    l,
		operation: operation,
	}
}

// DBEvent represents a database event for structured logging
type DBEvent struct {
	logger    *Logger
	operation string
	table     string
	duration  time.Duration
	err       error
	context   string // what was being attempted (e.g., "creating new user", "updating password")
	recordID  string // ID of the record being operated on (if applicable)
}

// Table sets the table name
func (e *DBEvent) Table(table string) *DBEvent {
	e.table = table
	return e
}

// Duration sets the operation duration
func (e *DBEvent) Duration(d time.Duration) *DBEvent {
	e.duration = d
	return e
}

// Error sets the error
func (e *DBEvent) Error(err error) *DBEvent {
	e.err = err
	return e
}

// Context sets what operation was being attempted (e.g., "creating new user")
func (e *DBEvent) Context(ctx string) *DBEvent {
	e.context = ctx
	return e
}

// RecordID sets the ID of the record being operated on
func (e *DBEvent) RecordID(id string) *DBEvent {
	e.recordID = id
	return e
}

// Success logs a successful DB operation (debug level)
func (e *DBEvent) Success() {
	event := e.logger.zl.Debug().
		Str("component", "db").
		Str("operation", e.operation)

	if e.table != "" {
		event = event.Str("table", e.table)
	}
	if e.duration > 0 {
		event = event.Dur("duration_ms", e.duration)
	}
	if e.context != "" {
		event = event.Str("context", e.context)
	}
	if e.recordID != "" {
		event = event.Str("record_id", e.recordID)
	}

	event.Msg("db_success")
}

// Failure logs a failed DB operation
func (e *DBEvent) Failure() {
	event := e.logger.zl.Error().
		Str("component", "db").
		Str("operation", e.operation)

	if e.table != "" {
		event = event.Str("table", e.table)
	}
	if e.duration > 0 {
		event = event.Dur("duration_ms", e.duration)
	}
	if e.context != "" {
		event = event.Str("context", e.context)
	}
	if e.recordID != "" {
		event = event.Str("record_id", e.recordID)
	}
	if e.err != nil {
		event = event.Err(e.err)
	}

	event.Msg("db_failure")
}

// HTTP logs HTTP request/response events
func (l *Logger) HTTP() *HTTPEvent {
	return &HTTPEvent{
		logger: l,
	}
}

// HTTPEvent represents an HTTP event for structured logging
type HTTPEvent struct {
	logger    *Logger
	method    string
	path      string
	status    int
	duration  time.Duration
	ip        string
	userAgent string
	requestID string
	err       error
}

// Method sets the HTTP method
func (e *HTTPEvent) Method(method string) *HTTPEvent {
	e.method = method
	return e
}

// Path sets the request path
func (e *HTTPEvent) Path(path string) *HTTPEvent {
	e.path = path
	return e
}

// Status sets the response status code
func (e *HTTPEvent) Status(status int) *HTTPEvent {
	e.status = status
	return e
}

// Duration sets the request duration
func (e *HTTPEvent) Duration(d time.Duration) *HTTPEvent {
	e.duration = d
	return e
}

// IP sets the client IP
func (e *HTTPEvent) IP(ip string) *HTTPEvent {
	e.ip = ip
	return e
}

// UserAgent sets the user agent
func (e *HTTPEvent) UserAgent(ua string) *HTTPEvent {
	e.userAgent = ua
	return e
}

// RequestID sets the request ID
func (e *HTTPEvent) RequestID(id string) *HTTPEvent {
	e.requestID = id
	return e
}

// Error sets the error
func (e *HTTPEvent) Error(err error) *HTTPEvent {
	e.err = err
	return e
}

// Log logs the HTTP event
func (e *HTTPEvent) Log() {
	// Determine log level based on status code
	var event *zerolog.Event
	switch {
	case e.status >= 500:
		event = e.logger.zl.Error()
	case e.status >= 400:
		event = e.logger.zl.Warn()
	default:
		event = e.logger.zl.Info()
	}

	event = event.Str("component", "http")

	if e.method != "" {
		event = event.Str("method", e.method)
	}
	if e.path != "" {
		event = event.Str("path", e.path)
	}
	if e.status > 0 {
		event = event.Int("status", e.status)
	}
	if e.duration > 0 {
		event = event.Dur("duration_ms", e.duration)
	}
	if e.ip != "" {
		event = event.Str("ip", e.ip)
	}
	if e.requestID != "" {
		event = event.Str("request_id", e.requestID)
	}
	if e.err != nil {
		event = event.Err(e.err)
	}

	event.Msg("http_request")
}

// Security logs security-related events (rate limits, suspicious activity)
func (l *Logger) Security(eventType string) *SecurityEvent {
	return &SecurityEvent{
		logger:    l,
		eventType: eventType,
	}
}

// SecurityEvent represents a security event for structured logging
type SecurityEvent struct {
	logger    *Logger
	eventType string
	ip        string
	userID    string
	details   string
	endpoint  string
	requestID string
}

// IP sets the client IP
func (e *SecurityEvent) IP(ip string) *SecurityEvent {
	e.ip = ip
	return e
}

// UserID sets the user ID
func (e *SecurityEvent) UserID(userID string) *SecurityEvent {
	e.userID = userID
	return e
}

// Details sets additional details
func (e *SecurityEvent) Details(details string) *SecurityEvent {
	e.details = details
	return e
}

// Endpoint sets the API endpoint
func (e *SecurityEvent) Endpoint(endpoint string) *SecurityEvent {
	e.endpoint = endpoint
	return e
}

// RequestID sets the request ID for correlation
func (e *SecurityEvent) RequestID(requestID string) *SecurityEvent {
	e.requestID = requestID
	return e
}

// Log logs the security event (always WARN level)
func (e *SecurityEvent) Log() {
	event := e.logger.zl.Warn().
		Str("component", "security").
		Str("event_type", e.eventType)

	if e.ip != "" {
		event = event.Str("client_ip", e.ip)
	}
	if e.userID != "" {
		event = event.Str("user_id", e.userID)
	}
	if e.endpoint != "" {
		event = event.Str("endpoint", e.endpoint)
	}
	if e.requestID != "" {
		event = event.Str("request_id", e.requestID)
	}
	if e.details != "" {
		event = event.Str("details", e.details)
	}

	event.Msg("security_event")
}

// ============================================================================
// Helper functions for data masking
// ============================================================================

// maskUsername masks username for privacy (shows first 2 and last char)
// Example: "johndoe" -> "jo***e"
func maskUsername(username string) string {
	if len(username) <= 3 {
		return "***"
	}
	return username[:2] + "***" + string(username[len(username)-1])
}

// MaskEmail masks email for privacy (shows first 2 chars of local part)
// Example: "john@example.com" -> "jo***@example.com"
func MaskEmail(email string) string {
	for i, c := range email {
		if c == '@' {
			if i <= 2 {
				return "***" + email[i:]
			}
			return email[:2] + "***" + email[i:]
		}
	}
	return "***"
}
