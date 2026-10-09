/**
 * Health Check API Client
 *
 * Provides methods to interact with the backend API for health check surveys.
 * Handles authentication, error handling, and data transformation.
 */

import { API_BASE_URL, APIError, APIRequestError, apiRequest, handleResponse } from './client';
import type { HealthCheckResponse, HealthCheckSession, HealthDimension } from '@/lib/types';

// Re-export domain types from the canonical source for backwards compatibility
export type { HealthCheckResponse, HealthCheckSession, HealthDimension };

// API-specific request/response wrapper types
export interface SubmitHealthCheckRequest {
  id?: string;
  teamId: string;
  userId: string;
  date: string; // RFC3339 format (e.g., 2024-01-15T10:30:00Z)
  assessmentPeriod?: string;
  surveyType?: 'individual' | 'post_workshop';
  responses: HealthCheckResponse[];
  completed: boolean;
}

export interface TeamSubmissionStatus {
  teamId: string;
  assessmentPeriod: string;
  totalMembers: number;
  submittedMembers: number;
  allSubmitted: boolean;
  postWorkshopExists: boolean;
}

export interface HealthDimensionsResponse {
  dimensions: HealthDimension[];
}

export interface HealthCheckSessionsResponse {
  sessions: HealthCheckSession[];
  total: number;
}

/**
 * Payload for autosaving an in-progress survey draft. Saves always overwrite (last write wins by
 * arrival order at the server) -- only one user is ever editing their own draft, so there is
 * nothing to reconcile a conflict against. `clientUpdatedAt` is optional display-only metadata
 * (epoch-millis, e.g. "saved 5s ago") and never affects ordering.
 */
export interface SaveDraftPayload {
  teamId: string;
  userId: string;
  surveyType?: 'individual' | 'post_workshop';
  assessmentPeriod: string;
  currentDimension: number;
  responses: HealthCheckResponse[];
  clientUpdatedAt?: number;
}

export interface DraftRecord {
  id: string;
  teamId: string;
  userId: string;
  surveyType: 'individual' | 'post_workshop';
  assessmentPeriod: string;
  currentDimension: number;
  responses: HealthCheckResponse[];
  clientUpdatedAt?: number;
  updatedAt?: string;
}

// Re-export APIError and APIRequestError for backwards compatibility
export type { APIError };
export { APIRequestError as HealthCheckAPIError };

/**
 * Submits a health check survey
 *
 * @param data Survey submission data
 * @returns The created health check session
 */
export async function submitHealthCheck(
  data: SubmitHealthCheckRequest
): Promise<HealthCheckSession> {
  const response = await apiRequest(`${API_BASE_URL}/api/v1/health-checks`, {
    method: 'POST',
    body: JSON.stringify(data),
  });

  return handleResponse<HealthCheckSession>(response);
}

/**
 * Fetches all active health dimensions
 *
 * @returns Array of health dimensions
 */
export async function getHealthDimensions(): Promise<HealthDimension[]> {
  const response = await apiRequest(`${API_BASE_URL}/api/v1/health-dimensions`);

  const data = await handleResponse<HealthDimensionsResponse>(response);
  return data.dimensions;
}

/**
 * Fetches a specific health check session by ID
 *
 * @param id Session ID
 * @returns Health check session
 */
export async function getHealthCheckById(id: string): Promise<HealthCheckSession> {
  const response = await apiRequest(`${API_BASE_URL}/api/v1/health-checks/${id}`);

  return handleResponse<HealthCheckSession>(response);
}

/**
 * Fetches all health check sessions for a team
 *
 * @param teamId Team ID
 * @param assessmentPeriod Optional assessment period filter
 * @returns Array of health check sessions
 */
export async function getTeamHealthChecks(
  teamId: string,
  assessmentPeriod?: string
): Promise<HealthCheckSession[]> {
  const params = new URLSearchParams();
  if (assessmentPeriod) {
    params.append('assessmentPeriod', assessmentPeriod);
  }

  const url = `${API_BASE_URL}/api/v1/health-checks/team/${teamId}${
    params.toString() ? `?${params.toString()}` : ''
  }`;

  const response = await apiRequest(url);

  const data = await handleResponse<HealthCheckSessionsResponse>(response);
  return data.sessions;
}

/**
 * Saves (upserts) the current user's in-progress survey draft to the server,
 * so it can be restored on another browser or device. Callers should treat a
 * rejected promise as non-fatal and keep relying on the localStorage fallback
 * (e.g. the API being temporarily unreachable).
 *
 * @param payload Draft contents plus a client-generated `clientUpdatedAt` timestamp
 * @returns The persisted draft record
 */
export async function saveDraft(payload: SaveDraftPayload): Promise<DraftRecord> {
  const response = await apiRequest(`${API_BASE_URL}/api/v1/health-checks/draft`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  });

  return handleResponse<DraftRecord>(response);
}

/**
 * Fetches the current user's in-progress survey draft for a team/survey type.
 *
 * Returns null when no draft exists (404) or when the request could not reach the server at all
 * (a network-level failure, e.g. offline/unreachable) — both are treated as "no server draft
 * available right now" so callers can fall back to localStorage. Any other HTTP failure (401,
 * 403, 500, ...) is a real error and is rethrown rather than silently swallowed, so a caller
 * doesn't mistake an auth or server failure for "no draft exists".
 */
export async function getDraft(
  teamId: string,
  userId: string,
  surveyType: 'individual' | 'post_workshop' = 'individual'
): Promise<DraftRecord | null> {
  const params = new URLSearchParams({ teamId, userId, surveyType });

  let response: Response;
  try {
    response = await apiRequest(`${API_BASE_URL}/api/v1/health-checks/draft?${params.toString()}`);
  } catch {
    // apiRequest/fetch itself threw — a network-level failure, not an HTTP response.
    return null;
  }

  if (response.status === 404) {
    return null;
  }

  return handleResponse<DraftRecord>(response);
}

/**
 * Helper function to format date as RFC3339 (without milliseconds)
 */
export function formatDateForAPI(date: Date = new Date()): string {
  return date.toISOString().replace(/\.\d{3}Z$/, 'Z');
}

/**
 * Fetches team submission status for post-workshop survey enablement
 *
 * @param teamId Team ID
 * @param assessmentPeriod Assessment period string
 * @returns Team submission status
 */
export async function getTeamSubmissionStatus(
  teamId: string,
  assessmentPeriod: string
): Promise<TeamSubmissionStatus | null> {
  const params = new URLSearchParams({ assessmentPeriod });
  const response = await apiRequest(
    `${API_BASE_URL}/api/v1/teams/${teamId}/submission-status?${params.toString()}`
  );

  // Return null when the endpoint is unavailable (e.g. backend not yet upgraded)
  // so the dashboard degrades gracefully rather than throwing a console error.
  if (response.status === 404 || response.status === 503) {
    return null;
  }

  return handleResponse<TeamSubmissionStatus>(response);
}

/**
 * Fetches all distinct assessment periods from the database
 *
 * @returns Array of assessment period strings (e.g., ["2024 - 2nd Half", "2024 - 1st Half"])
 */
export async function getAssessmentPeriods(): Promise<string[]> {
  const response = await apiRequest(`${API_BASE_URL}/api/v1/assessment-periods`);

  const data = await handleResponse<{ periods: string[] }>(response);
  return data.periods;
}

/**
 * Checks if the API is reachable
 *
 * @returns true if API is healthy
 */
export async function checkAPIHealth(): Promise<boolean> {
  try {
    const response = await fetch(`${API_BASE_URL}/health`, {
      method: 'GET',
    });
    return response.ok;
  } catch {
    return false;
  }
}
