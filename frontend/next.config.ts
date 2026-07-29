// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Static export for Docker builds; omit in dev so rewrites work
  ...(process.env.STATIC_EXPORT === 'true' ? { output: 'export' as const } : {}),
  async rewrites() {
    return [
      {
        source: '/api/:path*',
        destination: 'http://localhost:8080/api/:path*',
      },
    ];
  },
};

export default nextConfig;
