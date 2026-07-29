// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

'use client';

/**
 * TelemetryProvider - Initializes OpenTelemetry for the frontend
 *
 * Add this to your root layout to enable telemetry.
 * Telemetry is disabled by default - set NEXT_PUBLIC_OTEL_ENABLED=true to enable.
 */

import { useEffect } from 'react';
import { initTelemetry } from '@/lib/telemetry';

interface TelemetryProviderProps {
  children: React.ReactNode;
}

export function TelemetryProvider({ children }: TelemetryProviderProps) {
  useEffect(() => {
    // Initialize telemetry on client-side mount
    initTelemetry();
  }, []);

  return <>{children}</>;
}
