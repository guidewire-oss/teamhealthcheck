// SPDX-License-Identifier: Apache-2.0

/**
 * Shared API Client Utilities
 *
 * Provides common error handling, response processing, and base types
 * for all API client modules.
 */

// In production (unified container), API is same origin so base URL is empty.
// In local dev, NEXT_PUBLIC_API_URL points to the Go backend (e.g. http://localhost:8080).
export const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || '';

/**
 * Standard API error response format
 */
export interface APIError {
  error: string;
  message: string;
  code?: string;
}

/**
 * Custom error class for API errors
 *
 * Used as the base error class for all API client errors.
 * Domain-specific clients can extend this or use it directly.
 */
export class APIRequestError extends Error {
  constructor(
    message: string,
    public statusCode?: number,
    public apiError?: APIError
  ) {
    super(message);
    this.name = 'APIRequestError';
  }
}

/**
 * Handles API responses and errors
 *
 * Processes fetch responses, extracting JSON data on success
 * or throwing an APIRequestError on failure.
 *
 * @param response - Fetch API Response object
 * @returns Parsed JSON data of type T
 * @throws {APIRequestError} When response is not ok (status >= 400)
 *
 * @example
 * ```typescript
 * const response = await fetch('/api/v1/users');
 * const users = await handleResponse<User[]>(response);
 * ```
 */
export async function handleResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    let errorData: APIError | null = null;

    try {
      errorData = await response.json();
    } catch {
      // If response is not JSON, use status text
    }

    throw new APIRequestError(
      errorData?.message || response.statusText || 'An error occurred',
      response.status,
      errorData || undefined
    );
  }

  return response.json();
}

/**
 * Creates a standard fetch request with common headers
 *
 * @param url - Request URL (relative or absolute)
 * @param options - Fetch options (method, body, etc.)
 * @returns Configured fetch request
 *
 * @example
 * ```typescript
 * const response = await apiRequest('/api/v1/users', {
 *   method: 'POST',
 *   body: JSON.stringify({ name: 'John' })
 * });
 * ```
 */
export async function apiRequest(
  url: string,
  options: RequestInit = {}
): Promise<Response> {
  // Lazy import to avoid circular dependency (auth.ts imports from client.ts)
  const { authenticatedFetch } = await import('@/lib/auth');

  const defaultHeaders: HeadersInit = {
    'Content-Type': 'application/json',
  };

  return authenticatedFetch(url, {
    ...options,
    headers: {
      ...defaultHeaders,
      ...options.headers,
    },
  });
}

/**
 * Creates a typed API client factory function
 *
 * Combines apiRequest and handleResponse for convenience.
 *
 * @param url - Request URL
 * @param options - Fetch options
 * @returns Parsed response data of type T
 *
 * @example
 * ```typescript
 * const users = await createApiClient<User[]>('/api/v1/users');
 * ```
 */
export async function createApiClient<T>(
  url: string,
  options: RequestInit = {}
): Promise<T> {
  const response = await apiRequest(url, options);
  return handleResponse<T>(response);
}
