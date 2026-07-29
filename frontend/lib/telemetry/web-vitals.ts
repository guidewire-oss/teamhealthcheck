// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

/**
 * Web Vitals Metrics Collection for Team360 Frontend
 *
 * Collects Core Web Vitals and sends them to the OTel collector:
 * - LCP (Largest Contentful Paint) - loading performance
 * - FID (First Input Delay) - interactivity
 * - CLS (Cumulative Layout Shift) - visual stability
 * - FCP (First Contentful Paint) - perceived load speed
 * - TTFB (Time to First Byte) - server response time
 * - INP (Interaction to Next Paint) - responsiveness
 *
 * License: Apache 2.0
 */

import { onCLS, onFCP, onLCP, onTTFB, onINP, type Metric } from 'web-vitals';
import { getTracer } from './tracer';
import { telemetryConfig } from './config';

/**
 * Send a Web Vital metric as a span
 */
function reportWebVital(metric: Metric): void {
  if (!telemetryConfig.enabled) {
    return;
  }

  const tracer = getTracer('web-vitals');
  const span = tracer.startSpan(`web-vital.${metric.name.toLowerCase()}`);

  // Add metric attributes
  span.setAttribute('web_vital.name', metric.name);
  span.setAttribute('web_vital.value', metric.value);
  span.setAttribute('web_vital.rating', metric.rating); // 'good', 'needs-improvement', 'poor'
  span.setAttribute('web_vital.delta', metric.delta);
  span.setAttribute('web_vital.id', metric.id);
  span.setAttribute('web_vital.navigation_type', metric.navigationType);

  // Add rating thresholds for context
  switch (metric.name) {
    case 'LCP':
      span.setAttribute('web_vital.threshold_good', 2500);
      span.setAttribute('web_vital.threshold_poor', 4000);
      span.setAttribute('web_vital.unit', 'milliseconds');
      break;
    case 'FID':
      span.setAttribute('web_vital.threshold_good', 100);
      span.setAttribute('web_vital.threshold_poor', 300);
      span.setAttribute('web_vital.unit', 'milliseconds');
      break;
    case 'CLS':
      span.setAttribute('web_vital.threshold_good', 0.1);
      span.setAttribute('web_vital.threshold_poor', 0.25);
      span.setAttribute('web_vital.unit', 'score');
      break;
    case 'FCP':
      span.setAttribute('web_vital.threshold_good', 1800);
      span.setAttribute('web_vital.threshold_poor', 3000);
      span.setAttribute('web_vital.unit', 'milliseconds');
      break;
    case 'TTFB':
      span.setAttribute('web_vital.threshold_good', 800);
      span.setAttribute('web_vital.threshold_poor', 1800);
      span.setAttribute('web_vital.unit', 'milliseconds');
      break;
    case 'INP':
      span.setAttribute('web_vital.threshold_good', 200);
      span.setAttribute('web_vital.threshold_poor', 500);
      span.setAttribute('web_vital.unit', 'milliseconds');
      break;
  }

  // Add page context
  if (typeof window !== 'undefined') {
    span.setAttribute('page.url', window.location.href);
    span.setAttribute('page.path', window.location.pathname);
  }

  span.end();

  if (telemetryConfig.debug) {
    console.log(`[WebVitals] ${metric.name}:`, {
      value: metric.value,
      rating: metric.rating,
    });
  }
}

/**
 * Initialize Web Vitals collection
 * Should be called once at app startup
 */
export function initWebVitals(): void {
  // Only run in browser
  if (typeof window === 'undefined') {
    return;
  }

  if (!telemetryConfig.enabled) {
    if (telemetryConfig.debug) {
      console.log('[WebVitals] Collection disabled');
    }
    return;
  }

  try {
    // Register all Web Vitals metrics
    onCLS(reportWebVital);
    onFCP(reportWebVital);
    onLCP(reportWebVital);
    onTTFB(reportWebVital);
    onINP(reportWebVital);

    if (telemetryConfig.debug) {
      console.log('[WebVitals] Collection initialized');
    }
  } catch (error) {
    console.warn('[WebVitals] Failed to initialize:', error);
  }
}
