// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

/**
 * SSO utilities: PKCE generation and OAuth Authorization Code flow helpers.
 * Uses the Web Crypto API (built into all modern browsers and the Next.js runtime).
 */

import { API_BASE_URL } from '@/lib/api/client';

export interface OAuthConfig {
  clientId: string;
  authorizeUrl: string;
  redirectUri: string;
  scopes: string;
}

/**
 * Fetches SSO config from the backend's runtime config endpoint.
 * Returns null when SSO is not configured (OAUTH_CLIENT_ID not set on backend).
 */
export async function fetchSSOConfig(): Promise<OAuthConfig | null> {
  try {
    const res = await fetch(`${API_BASE_URL}/api/v1/config`);
    if (!res.ok) return null;
    const data = await res.json();
    if (!data.sso) return null;
    return data.sso as OAuthConfig;
  } catch {
    return null;
  }
}

/**
 * Starts the OAuth Authorization Code + PKCE flow.
 * Generates a code verifier, stores it in sessionStorage, then redirects
 * the browser to the provider's authorization endpoint.
 */
export async function startSSOFlow(config: OAuthConfig): Promise<void> {
  const verifier = generateCodeVerifier();
  const challenge = await generateCodeChallenge(verifier);
  const state = generateState();

  sessionStorage.setItem('pkce_verifier', verifier);
  sessionStorage.setItem('oauth_state', state);

  const params = new URLSearchParams({
    response_type: 'code',
    client_id: config.clientId,
    redirect_uri: config.redirectUri,
    scope: config.scopes,
    code_challenge: challenge,
    code_challenge_method: 'S256',
    state,
  });

  window.location.href = `${config.authorizeUrl}?${params.toString()}`;
}

// ── PKCE + state helpers ───────────────────────────────────────────────────────

function generateState(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return base64urlEncode(bytes);
}

function generateCodeVerifier(): string {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  return base64urlEncode(bytes);
}

async function generateCodeChallenge(verifier: string): Promise<string> {
  const encoded = new TextEncoder().encode(verifier);
  const digest = await crypto.subtle.digest('SHA-256', encoded);
  return base64urlEncode(new Uint8Array(digest));
}

function base64urlEncode(bytes: Uint8Array): string {
  return btoa(String.fromCharCode(...bytes))
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=/g, '');
}
