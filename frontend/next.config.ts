import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Static export for Docker builds; omit in dev so rewrites work
  ...(process.env.STATIC_EXPORT === 'true' ? { output: 'export' as const } : {}),
  // Next's built-in trailing-slash redirect runs independently of
  // middleware and fights the /docs trailing-slash logic in middleware.ts,
  // producing a redirect loop. middleware.ts owns that behavior instead.
  skipTrailingSlashRedirect: true,
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
