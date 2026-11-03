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

export interface SurveyEligibility {
  eligible: boolean;
  // Populated when eligible is false: one of "duplicate", "future_period", "past_period", or
  // "invalid_period" (see backend healthcheck.PeriodEligibilityReason). Callers must branch on
  // this rather than assuming every ineligible result is a duplicate -- submittedPeriod and
  // nextEligiblePeriod are only meaningful when reason is "duplicate".
  reason?: string;
  // Populated only when reason is "duplicate": the user-facing label of the blocking
  // submission's period (e.g. "H1 2026"), and the label the caller becomes eligible for next.
  // The next label is absent when there is no later period to point to. Never a day-level
  // date -- the survey experience is based on period values only.
  submittedPeriod?: string;
  nextEligiblePeriod?: string;
}

/**
 * What to check. Each survey type is scoped to exactly one thing, and the endpoint rejects the
 * request with a 400 without it: an Individual Survey to the user, a Post-Workshop Survey to
 * the team.
 */
export type SurveyEligibilityParams =
  | { surveyType: 'individual'; assessmentPeriod: string; userId: string; teamId?: string }
  | { surveyType: 'post_workshop'; assessmentPeriod: string; teamId: string; userId?: string };

/**
 * Checks whether a survey can be submitted for a given assessment period right now, before
 * opening the survey form. A scope may submit each survey type once per period (a half-year,
 * month or year, depending on the team's cadence), and only for a period that is currently
 * open. Individual surveys are scoped to the user; post-workshop surveys are scoped to the
 * team. The server only answers about the caller's own submissions, or a team they belong to
 * or lead.
 *
 * @param params.surveyType 'individual' or 'post_workshop'
 * @param params.assessmentPeriod Assessment period the survey would be recorded under (e.g. "2026 H1")
 * @param params.teamId Required when surveyType is 'post_workshop'
 * @param params.userId Required when surveyType is 'individual'
 * @returns Eligibility result; when ineligible, says why, and for a duplicate includes the
 *          already-submitted period and the next eligible one
 */
export async function checkSurveyEligibility(params: SurveyEligibilityParams): Promise<SurveyEligibility> {
  // The type already forbids these shapes; the check protects callers that are not type-checked.
  if (params.surveyType === 'individual' && !params.userId) {
    throw new Error('checkSurveyEligibility: userId is required for an individual survey');
  }
  if (params.surveyType === 'post_workshop' && !params.teamId) {
    throw new Error('checkSurveyEligibility: teamId is required for a post-workshop survey');
  }

  const query = new URLSearchParams({
    surveyType: params.surveyType,
    assessmentPeriod: params.assessmentPeriod,
  });
  if (params.teamId) query.set('teamId', params.teamId);
  if (params.userId) query.set('userId', params.userId);

  const response = await apiRequest(
    `${API_BASE_URL}/api/v1/health-checks/eligibility?${query.toString()}`
  );

  return handleResponse<SurveyEligibility>(response);
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
