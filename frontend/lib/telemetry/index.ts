// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

/**
 * OpenTelemetry Telemetry Module for Team360 Frontend
 *
 * Provides:
 * - Distributed tracing for API calls and page loads
 * - Web Vitals metrics (LCP, FID, CLS, FCP, TTFB, INP)
 * - Business event tracing (login, survey, dashboard)
 *
 * All dependencies are Apache 2.0 licensed.
 *
 * Usage:
 *   import { initTelemetry, traceLogin, traceSurveySubmit } from '@/lib/telemetry';
 *
 *   // Initialize once at app startup
 *   initTelemetry();
 *
 *   // Use business event tracers
 *   await traceLogin(username, () => api.login(username, password));
 */

// Configuration
export { telemetryConfig, getTelemetryConfig, type TelemetryConfig } from './config';

// Tracer initialization and utilities
export {
  initTracer,
  getTracer,
  startSpan,
  startAsyncSpan,
  shutdownTracer,
} from './tracer';

// Web Vitals
export { initWebVitals } from './web-vitals';

// Business event tracers
export {
  // Auth
  traceLogin,
  traceLogout,
  traceTokenRefresh,
  // Survey
  traceSurveyStart,
  traceSurveySubmit,
  traceSurveyHistoryView,
  // Dashboard
  traceTeamDashboardLoad,
  traceManagerDashboardLoad,
  traceDashboardFilter,
  // Navigation
  tracePageNavigation,
  // Errors
  traceError,
} from './business-events';

/**
 * Initialize all telemetry (tracer + web vitals)
 * Call this once at app startup
 */
export function initTelemetry(): void {
  // Only run in browser
  if (typeof window === 'undefined') {
    return;
  }

  // Import dynamically to avoid SSR issues
  import('./tracer').then(({ initTracer }) => {
    initTracer();
  });

  import('./web-vitals').then(({ initWebVitals }) => {
    initWebVitals();
  });
}
