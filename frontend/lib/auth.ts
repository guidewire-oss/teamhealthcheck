// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

import Cookies from 'js-cookie';
import type { User as DomainUser } from '@/lib/types';
import { API_BASE_URL } from '@/lib/api/client';

/**
 * User type returned from API login
 * Note: Authentication is now handled via backend API (POST /api/v1/auth/login)
 * This module handles JWT-based session management with cookie storage
 */
export interface APIUser {
  id: string;
  username: string;
  email: string;
  fullName: string;
  hierarchyLevel: string;
  teamIds: string[];
  canTakeSurvey?: boolean;
}

/**
 * Login response from the API includes JWT tokens
 */
export interface LoginResponse {
  user: APIUser;
  accessToken: string;
  refreshToken: string;
  expiresIn: number;
}

/**
 * AuthUser type used for authenticated user in frontend components.
 *
 * Based on the canonical User type but omits password (not sent to frontend)
 * and adds API-aligned field aliases:
 * - fullName: User's display name (matches API, alias for name)
 * - hierarchyLevel: User's level ID (matches API, alias for hierarchyLevelId)
 *
 * For backwards compatibility with components using old field names:
 * - name: Alias for fullName
 * - hierarchyLevelId: Alias for hierarchyLevel
 */
export interface AuthUser extends Omit<DomainUser, 'password'> {
  // API-aligned field names (aliases for backwards compatibility)
  fullName: string;
  hierarchyLevel: string;
  canTakeSurvey?: boolean;
}

// For backwards compatibility - components can still import "User" from auth.ts
// but it will actually be the AuthUser type (without password)
export type User = AuthUser;

// Token storage keys
const ACCESS_TOKEN_KEY = 'accessToken';
const REFRESH_TOKEN_KEY = 'refreshToken';
const USER_KEY = 'user';

/**
 * Stores authentication tokens and user data
 */
export const setAuthData = (loginResponse: LoginResponse) => {
  // Store tokens in localStorage (more secure than cookies for JWT)
  localStorage.setItem(ACCESS_TOKEN_KEY, loginResponse.accessToken);
  localStorage.setItem(REFRESH_TOKEN_KEY, loginResponse.refreshToken);

  // Also store user in cookie for middleware access (URL encoded)
  const userJson = JSON.stringify(loginResponse.user);
  // Cookie must outlive the refresh token (7 days = 604800s)
  document.cookie = `${USER_KEY}=${encodeURIComponent(userJson)}; path=/; max-age=604800`;
};

/**
 * Gets the access token
 */
export const getAccessToken = (): string | null => {
  if (typeof window === 'undefined') return null;
  return localStorage.getItem(ACCESS_TOKEN_KEY);
};

/**
 * Gets the refresh token
 */
export const getRefreshToken = (): string | null => {
  if (typeof window === 'undefined') return null;
  return localStorage.getItem(REFRESH_TOKEN_KEY);
};

/**
 * Refreshes the access token using the refresh token
 */
export const refreshAccessToken = async (): Promise<string | null> => {
  const refreshToken = getRefreshToken();
  if (!refreshToken) return null;

  try {
    const response = await fetch(`${API_BASE_URL}/api/v1/auth/refresh`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ refreshToken }),
    });

    if (!response.ok) {
      // Refresh failed - clear auth data
      clearAuthData();
      return null;
    }

    const data = await response.json();
    localStorage.setItem(ACCESS_TOKEN_KEY, data.accessToken);
    return data.accessToken;
  } catch {
    clearAuthData();
    return null;
  }
};

/**
 * Makes an authenticated API request with automatic token refresh
 */
export const authenticatedFetch = async (
  url: string,
  options: RequestInit = {}
): Promise<Response> => {
  let accessToken = getAccessToken();

  // First attempt with current token
  const headers = new Headers(options.headers);
  if (accessToken) {
    headers.set('Authorization', `Bearer ${accessToken}`);
  }

  let response = await fetch(url, { ...options, headers });

  // If 401, try to refresh token and retry
  if (response.status === 401 && getRefreshToken()) {
    accessToken = await refreshAccessToken();
    if (accessToken) {
      headers.set('Authorization', `Bearer ${accessToken}`);
      response = await fetch(url, { ...options, headers });
    } else {
      // Refresh failed — session is expired; redirect to login
      if (typeof window !== 'undefined') {
        window.location.href = '/login?expired=true';
      }
      return response;
    }
  }

  return response;
};

/**
 * Clears all authentication data
 */
export const clearAuthData = () => {
  if (typeof window === 'undefined') return;
  localStorage.removeItem(ACCESS_TOKEN_KEY);
  localStorage.removeItem(REFRESH_TOKEN_KEY);
  // Remove cookie using same method as setting (document.cookie)
  // Set expiry to past date to delete
  document.cookie = `${USER_KEY}=; path=/; max-age=0; expires=Thu, 01 Jan 1970 00:00:00 GMT`;
};

/**
 * Gets the current user from the cookie.
 * Returns user with both API-aligned and legacy field names for compatibility.
 */
export const getCurrentUser = (): AuthUser | null => {
  const userCookie = Cookies.get(USER_KEY);
  if (userCookie) {
    try {
      // Decode URL-encoded cookie value before parsing JSON
      const decodedCookie = decodeURIComponent(userCookie);
      const apiUser = JSON.parse(decodedCookie) as APIUser;
      const fullName = apiUser.fullName || '';
      const hierarchyLevel = apiUser.hierarchyLevel || '';

      return {
        id: apiUser.id,
        username: apiUser.username,
        email: apiUser.email,
        teamIds: apiUser.teamIds || [],
        isAdmin: hierarchyLevel === 'admin' || hierarchyLevel === 'level-admin',
        // API-aligned names
        fullName,
        hierarchyLevel,
        canTakeSurvey: apiUser.canTakeSurvey || false,
        // Backwards-compatible aliases
        name: fullName,
        hierarchyLevelId: hierarchyLevel,
      };
    } catch {
      return null;
    }
  }
  return null;
};

/**
 * Logs out the current user by clearing all auth data and calling logout API
 */
export const logout = async () => {
  try {
    await fetch(`${API_BASE_URL}/api/v1/auth/logout`, { method: 'POST' });
  } catch {
    // Ignore errors - clear local data regardless
  }
  clearAuthData();
};

/**
 * Checks if a user is currently authenticated
 */
export const isAuthenticated = (): boolean => {
  return !!getCurrentUser() && !!getAccessToken();
};
