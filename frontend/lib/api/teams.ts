// SPDX-License-Identifier: Apache-2.0

/**
 * Teams API Client
 *
 * Provides methods to interact with the backend API for team information.
 */

import { API_BASE_URL, APIError, APIRequestError, apiRequest, handleResponse } from './client';

// Types matching backend DTOs
export interface TeamMember {
  id: string;
  username: string;
  fullName: string;
}

export interface TeamInfo {
  id: string;
  name: string;
  cadence: string;
  members: TeamMember[];
  teamLeadId?: string;
  teamLeadName?: string;
}

// Re-export APIError and APIRequestError for backwards compatibility
export type { APIError };
export { APIRequestError as TeamsAPIError };

/**
 * Fetches team info by team ID
 *
 * @param teamId Team ID
 * @returns Team information including name, cadence, and members
 */
export async function getTeamInfo(teamId: string): Promise<TeamInfo> {
  const response = await apiRequest(`${API_BASE_URL}/api/v1/teams/${teamId}/info`);

  return handleResponse<TeamInfo>(response);
}

/**
 * Cache for team info to avoid repeated API calls
 */
const teamCache = new Map<string, { data: TeamInfo; timestamp: number }>();
const CACHE_TTL = 5 * 60 * 1000; // 5 minutes

/**
 * Fetches team info with caching
 *
 * @param teamId Team ID
 * @returns Team information (cached)
 */
export async function getTeamInfoCached(teamId: string): Promise<TeamInfo> {
  const cached = teamCache.get(teamId);
  const now = Date.now();

  if (cached && (now - cached.timestamp) < CACHE_TTL) {
    return cached.data;
  }

  const data = await getTeamInfo(teamId);
  teamCache.set(teamId, { data, timestamp: now });
  return data;
}

/**
 * Clears the team cache
 */
export function clearTeamCache(): void {
  teamCache.clear();
}

/**
 * Team summary for list view
 */
export interface TeamSummary {
  id: string;
  name: string;
  cadence: string;
  memberCount: number;
  teamLeadId?: string;
  teamLeadName?: string;
  nextCheckDate?: string;
}

export interface TeamsListResponse {
  teams: TeamSummary[];
  total: number;
}

/**
 * Fetches list of all teams
 *
 * @returns List of all teams
 */
export async function listTeams(): Promise<TeamsListResponse> {
  const response = await apiRequest(`${API_BASE_URL}/api/v1/teams`);

  return handleResponse<TeamsListResponse>(response);
}
