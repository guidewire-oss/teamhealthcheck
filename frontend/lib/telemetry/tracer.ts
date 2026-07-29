// SPDX-License-Identifier: Apache-2.0

/**
 * OpenTelemetry Browser Tracer for Team360 Frontend
 *
 * Provides distributed tracing for:
 * - Page loads and navigation
 * - Fetch/XHR API calls
 * - Custom business events (login, survey submission, etc.)
 *
 * License: Apache 2.0
 */

import { WebTracerProvider } from '@opentelemetry/sdk-trace-web';
import { BatchSpanProcessor, ConsoleSpanExporter } from '@opentelemetry/sdk-trace-base';
import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-http';
import { ZoneContextManager } from '@opentelemetry/context-zone';
import { registerInstrumentations } from '@opentelemetry/instrumentation';
import { FetchInstrumentation } from '@opentelemetry/instrumentation-fetch';
import { DocumentLoadInstrumentation } from '@opentelemetry/instrumentation-document-load';
import { Resource } from '@opentelemetry/resources';
import {
  SEMRESATTRS_SERVICE_NAME,
  SEMRESATTRS_SERVICE_VERSION,
  SEMRESATTRS_DEPLOYMENT_ENVIRONMENT,
} from '@opentelemetry/semantic-conventions';
import { trace, context, SpanStatusCode, Span, propagation } from '@opentelemetry/api';
import { W3CTraceContextPropagator } from '@opentelemetry/core';
import { telemetryConfig } from './config';

let initialized = false;
let provider: WebTracerProvider | null = null;

/**
 * Initialize OpenTelemetry tracing for the browser
 * Should be called once at app startup
 */
export function initTracer(): void {
  // Only run in browser
  if (typeof window === 'undefined') {
    return;
  }

  // Skip if disabled or already initialized
  if (!telemetryConfig.enabled || initialized) {
    if (telemetryConfig.debug) {
      console.log('[OTel] Tracing disabled or already initialized');
    }
    return;
  }

  try {
    // Create resource describing this service
    const resource = new Resource({
      [SEMRESATTRS_SERVICE_NAME]: telemetryConfig.serviceName,
      [SEMRESATTRS_SERVICE_VERSION]: telemetryConfig.serviceVersion,
      [SEMRESATTRS_DEPLOYMENT_ENVIRONMENT]: telemetryConfig.environment,
    });

    // Create trace provider
    provider = new WebTracerProvider({
      resource,
    });

    // Add OTLP exporter (sends traces to collector)
    const otlpExporter = new OTLPTraceExporter({
      url: `${telemetryConfig.otlpEndpoint}/v1/traces`,
    });
    provider.addSpanProcessor(new BatchSpanProcessor(otlpExporter));

    // Optionally add console exporter for debugging
    if (telemetryConfig.debug) {
      provider.addSpanProcessor(new BatchSpanProcessor(new ConsoleSpanExporter()));
    }

    // Set up W3C Trace Context propagator for distributed tracing
    // This injects traceparent/tracestate headers into outgoing requests
    propagation.setGlobalPropagator(new W3CTraceContextPropagator());

    // Register the provider globally
    provider.register({
      contextManager: new ZoneContextManager(),
    });

    // Register auto-instrumentations
    registerInstrumentations({
      instrumentations: [
        // Auto-instrument fetch() calls
        new FetchInstrumentation({
          // Propagate trace context to backend
          propagateTraceHeaderCorsUrls: [
            /localhost/,
            /127\.0\.0\.1/,
            // Add production domains here
          ],
          // Don't trace requests to the OTel collector itself
          ignoreUrls: [/\/v1\/traces/, /\/v1\/metrics/],
          // Add custom attributes to fetch spans
          applyCustomAttributesOnSpan: (span, request, response) => {
            if (request instanceof Request) {
              span.setAttribute('http.request.url', request.url);
            }
          },
        }),
        // Auto-instrument page loads
        new DocumentLoadInstrumentation(),
      ],
    });

    initialized = true;

    if (telemetryConfig.debug) {
      console.log('[OTel] Browser tracing initialized', {
        serviceName: telemetryConfig.serviceName,
        endpoint: telemetryConfig.otlpEndpoint,
      });
    }
  } catch (error) {
    console.warn('[OTel] Failed to initialize browser tracing:', error);
  }
}

/**
 * Get a tracer instance for creating custom spans
 */
export function getTracer(name: string = 'teams360-frontend') {
  return trace.getTracer(name, telemetryConfig.serviceVersion);
}

/**
 * Create a span for a custom operation
 */
export function startSpan(name: string, fn: (span: Span) => void): void {
  const tracer = getTracer();
  const span = tracer.startSpan(name);

  try {
    fn(span);
  } catch (error) {
    span.setStatus({ code: SpanStatusCode.ERROR, message: String(error) });
    throw error;
  } finally {
    span.end();
  }
}

/**
 * Create an async span for a custom operation
 */
export async function startAsyncSpan<T>(
  name: string,
  fn: (span: Span) => Promise<T>
): Promise<T> {
  const tracer = getTracer();
  const span = tracer.startSpan(name);

  try {
    const result = await context.with(trace.setSpan(context.active(), span), () => fn(span));
    return result;
  } catch (error) {
    span.setStatus({ code: SpanStatusCode.ERROR, message: String(error) });
    throw error;
  } finally {
    span.end();
  }
}

/**
 * Shutdown the tracer (call on app unmount)
 */
export async function shutdownTracer(): Promise<void> {
  if (provider) {
    await provider.shutdown();
    initialized = false;
    provider = null;
  }
}
