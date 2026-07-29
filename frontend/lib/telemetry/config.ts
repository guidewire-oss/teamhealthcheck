// SPDX-License-Identifier: Apache-2.0

/**
 * OpenTelemetry Configuration for Team360 Frontend
 *
 * All libraries used are Apache 2.0 licensed:
 * - @opentelemetry/* packages: Apache 2.0 (CNCF project)
 * - web-vitals: Apache 2.0 (Google)
 */

export interface TelemetryConfig {
  enabled: boolean;
  serviceName: string;
  serviceVersion: string;
  environment: string;
  otlpEndpoint: string;
  // Sampling rate for traces (0.0 to 1.0)
  sampleRate: number;
  // Enable console logging of telemetry data (for debugging)
  debug: boolean;
}

/**
 * Get telemetry configuration from environment variables
 * Defaults to disabled unless NEXT_PUBLIC_OTEL_ENABLED=true
 */
export function getTelemetryConfig(): TelemetryConfig {
  const enabled = process.env.NEXT_PUBLIC_OTEL_ENABLED === 'true';

  return {
    enabled,
    serviceName: process.env.NEXT_PUBLIC_OTEL_SERVICE_NAME || 'teams360-frontend',
    serviceVersion: process.env.NEXT_PUBLIC_OTEL_SERVICE_VERSION || '1.0.0',
    environment: process.env.NEXT_PUBLIC_ENVIRONMENT || 'development',
    // Default to OTel collector HTTP endpoint for browser traces
    otlpEndpoint: process.env.NEXT_PUBLIC_OTEL_EXPORTER_OTLP_ENDPOINT || 'http://localhost:4318',
    sampleRate: parseFloat(process.env.NEXT_PUBLIC_OTEL_SAMPLE_RATE || '1.0'),
    debug: process.env.NEXT_PUBLIC_OTEL_DEBUG === 'true',
  };
}

export const telemetryConfig = getTelemetryConfig();
