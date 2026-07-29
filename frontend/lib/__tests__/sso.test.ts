// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fetchSSOConfig } from '../sso';

// Mock the API client module so API_BASE_URL is always ''
vi.mock('@/lib/api/client', () => ({ API_BASE_URL: '' }));

beforeEach(() => {
  vi.restoreAllMocks();
});

// ── 1. SSO disabled — backend returns { sso: null } ─────────────────────────

describe('fetchSSOConfig — SSO disabled', () => {
  it('returns null when the backend returns { sso: null }', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ sso: null }),
    });

    const config = await fetchSSOConfig();

    expect(config).toBeNull();
    expect(fetch).toHaveBeenCalledWith('/api/v1/config');
  });
});

// ── 2. SSO enabled — backend returns config ──────────────────────────────────

describe('fetchSSOConfig — SSO enabled', () => {
  const ssoPayload = {
    clientId: 'test-client-id',
    authorizeUrl: 'https://auth.example.com/authorize',
    redirectUri: 'http://localhost:3000/auth/callback',
    scopes: 'openid email profile',
  };

  it('returns the SSO config from the backend', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ sso: ssoPayload }),
    });

    const config = await fetchSSOConfig();

    expect(config).not.toBeNull();
    expect(config?.clientId).toBe('test-client-id');
    expect(config?.authorizeUrl).toBe('https://auth.example.com/authorize');
    expect(config?.redirectUri).toBe('http://localhost:3000/auth/callback');
    expect(config?.scopes).toBe('openid email profile');
  });
});

// ── 3. Network error — returns null gracefully ───────────────────────────────

describe('fetchSSOConfig — error handling', () => {
  it('returns null on network error', async () => {
    global.fetch = vi.fn().mockRejectedValue(new Error('Network error'));

    const config = await fetchSSOConfig();

    expect(config).toBeNull();
  });

  it('returns null when backend responds with non-OK status', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
    });

    const config = await fetchSSOConfig();

    expect(config).toBeNull();
  });
});

// ── 4. Login page contract ───────────────────────────────────────────────────

describe('fetchSSOConfig — login page contract', () => {
  it('returns null (hide SSO button) when SSO is not configured', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ sso: null }),
    });

    expect(await fetchSSOConfig()).toBeNull();
  });

  it('returns config (show SSO button) when SSO is configured', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        sso: {
          clientId: 'my-app',
          authorizeUrl: 'https://idp.example.com/auth',
          redirectUri: 'http://localhost:3000/auth/callback',
          scopes: 'openid email profile',
        },
      }),
    });

    expect(await fetchSSOConfig()).not.toBeNull();
  });
});
