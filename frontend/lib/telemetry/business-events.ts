// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

/**
 * Business Event Tracing for Team360 Frontend
 *
 * Custom spans for business-level events:
 * - Authentication (login, logout, token refresh)
 * - Survey operations (start, submit, save draft)
 * - Dashboard interactions (view team health, filter data)
 * - Navigation events
 *
 * License: Apache 2.0
 */

import { SpanStatusCode } from '@opentelemetry/api';
import { getTracer, startAsyncSpan } from './tracer';
import { telemetryConfig } from './config';

// ============================================================================
// Authentication Events
// ============================================================================

/**
 * Record a login attempt
 */
export async function traceLogin<T>(
  username: string,
  loginFn: () => Promise<T>
): Promise<T> {
  if (!telemetryConfig.enabled) {
    return loginFn();
  }

  return startAsyncSpan('auth.login', async (span) => {
    span.setAttribute('auth.username_masked', maskUsername(username));
    span.setAttribute('auth.type', 'password');

    try {
      const result = await loginFn();
      span.setAttribute('auth.success', true);
      return result;
    } catch (error) {
      span.setAttribute('auth.success', false);
      span.setAttribute('auth.error', String(error));
      span.setStatus({ code: SpanStatusCode.ERROR, message: String(error) });
      throw error;
    }
  });
}

/**
 * Record a logout event
 */
export function traceLogout(): void {
  if (!telemetryConfig.enabled) return;

  const tracer = getTracer();
  const span = tracer.startSpan('auth.logout');
  span.setAttribute('auth.type', 'logout');
  span.end();
}

/**
 * Record a token refresh attempt
 */
export async function traceTokenRefresh<T>(refreshFn: () => Promise<T>): Promise<T> {
  if (!telemetryConfig.enabled) {
    return refreshFn();
  }

  return startAsyncSpan('auth.token_refresh', async (span) => {
    try {
      const result = await refreshFn();
      span.setAttribute('auth.refresh_success', true);
      return result;
    } catch (error) {
      span.setAttribute('auth.refresh_success', false);
      span.setStatus({ code: SpanStatusCode.ERROR, message: String(error) });
      throw error;
    }
  });
}

// ============================================================================
// Survey Events
// ============================================================================

/**
 * Record starting a health check survey
 */
export function traceSurveyStart(teamId: string, assessmentPeriod: string): void {
  if (!telemetryConfig.enabled) return;

  const tracer = getTracer();
  const span = tracer.startSpan('survey.start');
  span.setAttribute('survey.team_id', teamId);
  span.setAttribute('survey.assessment_period', assessmentPeriod);
  span.setAttribute('survey.timestamp', new Date().toISOString());
  span.end();
}

/**
 * Record submitting a health check survey
 */
export async function traceSurveySubmit<T>(
  teamId: string,
  assessmentPeriod: string,
  dimensionCount: number,
  submitFn: () => Promise<T>
): Promise<T> {
  if (!telemetryConfig.enabled) {
    return submitFn();
  }

  return startAsyncSpan('survey.submit', async (span) => {
    span.setAttribute('survey.team_id', teamId);
    span.setAttribute('survey.assessment_period', assessmentPeriod);
    span.setAttribute('survey.dimension_count', dimensionCount);

    const startTime = performance.now();

    try {
      const result = await submitFn();
      const duration = performance.now() - startTime;

      span.setAttribute('survey.success', true);
      span.setAttribute('survey.duration_ms', duration);
      return result;
    } catch (error) {
      span.setAttribute('survey.success', false);
      span.setAttribute('survey.error', String(error));
      span.setStatus({ code: SpanStatusCode.ERROR, message: String(error) });
      throw error;
    }
  });
}

/**
 * Record viewing survey history
 */
export function traceSurveyHistoryView(userId: string): void {
  if (!telemetryConfig.enabled) return;

  const tracer = getTracer();
  const span = tracer.startSpan('survey.history_view');
  span.setAttribute('survey.user_id', userId);
  span.end();
}

// ============================================================================
// Dashboard Events
// ============================================================================

/**
 * Record loading a team dashboard
 */
export async function traceTeamDashboardLoad<T>(
  teamId: string,
  loadFn: () => Promise<T>
): Promise<T> {
  if (!telemetryConfig.enabled) {
    return loadFn();
  }

  return startAsyncSpan('dashboard.team_load', async (span) => {
    span.setAttribute('dashboard.team_id', teamId);
    span.setAttribute('dashboard.type', 'team_lead');

    const startTime = performance.now();

    try {
      const result = await loadFn();
      const duration = performance.now() - startTime;

      span.setAttribute('dashboard.load_success', true);
      span.setAttribute('dashboard.load_duration_ms', duration);
      return result;
    } catch (error) {
      span.setAttribute('dashboard.load_success', false);
      span.setStatus({ code: SpanStatusCode.ERROR, message: String(error) });
      throw error;
    }
  });
}

/**
 * Record loading a manager dashboard
 */
export async function traceManagerDashboardLoad<T>(
  managerId: string,
  loadFn: () => Promise<T>
): Promise<T> {
  if (!telemetryConfig.enabled) {
    return loadFn();
  }

  return startAsyncSpan('dashboard.manager_load', async (span) => {
    span.setAttribute('dashboard.manager_id', managerId);
    span.setAttribute('dashboard.type', 'manager');

    const startTime = performance.now();

    try {
      const result = await loadFn();
      const duration = performance.now() - startTime;

      span.setAttribute('dashboard.load_success', true);
      span.setAttribute('dashboard.load_duration_ms', duration);
      return result;
    } catch (error) {
      span.setAttribute('dashboard.load_success', false);
      span.setStatus({ code: SpanStatusCode.ERROR, message: String(error) });
      throw error;
    }
  });
}

/**
 * Record filtering dashboard data
 */
export function traceDashboardFilter(filterType: string, filterValue: string): void {
  if (!telemetryConfig.enabled) return;

  const tracer = getTracer();
  const span = tracer.startSpan('dashboard.filter');
  span.setAttribute('dashboard.filter_type', filterType);
  span.setAttribute('dashboard.filter_value', filterValue);
  span.end();
}

// ============================================================================
// Navigation Events
// ============================================================================

/**
 * Record page navigation
 */
export function tracePageNavigation(fromPath: string, toPath: string): void {
  if (!telemetryConfig.enabled) return;

  const tracer = getTracer();
  const span = tracer.startSpan('navigation.page_change');
  span.setAttribute('navigation.from_path', fromPath);
  span.setAttribute('navigation.to_path', toPath);
  span.setAttribute('navigation.timestamp', new Date().toISOString());
  span.end();
}

// ============================================================================
// Error Events
// ============================================================================

/**
 * Record a client-side error
 */
export function traceError(error: Error, context?: Record<string, string>): void {
  if (!telemetryConfig.enabled) return;

  const tracer = getTracer();
  const span = tracer.startSpan('error.client');
  span.setAttribute('error.message', error.message);
  span.setAttribute('error.name', error.name);
  span.setAttribute('error.stack', error.stack || '');

  if (context) {
    Object.entries(context).forEach(([key, value]) => {
      span.setAttribute(`error.context.${key}`, value);
    });
  }

  span.setStatus({ code: SpanStatusCode.ERROR, message: error.message });
  span.end();
}

// ============================================================================
// Helpers
// ============================================================================

/**
 * Mask username for privacy (show first 2 chars only)
 */
function maskUsername(username: string): string {
  if (username.length <= 2) {
    return '**';
  }
  return username.substring(0, 2) + '*'.repeat(username.length - 2);
}
