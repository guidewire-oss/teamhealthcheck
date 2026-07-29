// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

/**
 * Admin API Client
 *
 * Provides methods to interact with the backend admin API for:
 * - Hierarchy levels management
 * - User management
 * - Team management
 * - System settings (dimensions, notifications)
 *
 * All endpoints require admin privileges.
 */

import { API_BASE_URL, APIError, APIRequestError, createApiClient } from './client';

// Re-export shared error types
export type { APIError };
export { APIRequestError as AdminAPIError };

// ============================================================================
// HIERARCHY LEVEL TYPES
// ============================================================================

export interface HierarchyPermissions {
  canViewAllTeams: boolean;
  canEditTeams: boolean;
  canManageUsers: boolean;
  canTakeSurvey: boolean;
  canViewAnalytics: boolean;
}

export interface HierarchyLevel {
  id: string;
  name: string;
  position: number;
  permissions: HierarchyPermissions;
  createdAt: string;
  updatedAt: string;
}

export interface CreateHierarchyLevelRequest {
  name: string;
  position: number;
  permissions: HierarchyPermissions;
}

export interface UpdateHierarchyLevelRequest {
  name?: string;
  permissions?: HierarchyPermissions;
}

export interface UpdateHierarchyPositionRequest {
  position: number;
}

// ============================================================================
// USER TYPES
// ============================================================================

export interface AdminUser {
  id: string;
  username: string;
  email: string;
  fullName: string;
  hierarchyLevel: string;
  reportsTo: string | null;
  teamIds: string[];
  authType: 'local' | 'sso';
  createdAt: string;
  updatedAt: string;
}

export interface CreateUserRequest {
  id: string;  // Required for backend
  username: string;
  email: string;
  fullName: string;
  password?: string;  // Required for local users, omit for SSO
  authType?: 'local' | 'sso';
  hierarchyLevel: string;
  reportsTo?: string | null;
}

export interface UpdateUserRequest {
  username?: string;
  email?: string;
  fullName?: string;
  password?: string;
  authType?: 'local' | 'sso';
  hierarchyLevel?: string;
  reportsTo?: string | null;
}

export interface UsersListResponse {
  users: AdminUser[];
  total: number;
}

// ============================================================================
// TEAM TYPES
// ============================================================================

export interface AdminTeam {
  id: string;
  name: string;
  teamLeadId: string | null;
  teamLeadName: string | null;
  cadence: string;
  distributionListEmail?: string | null;
  memberCount: number;
  createdAt: string;
  updatedAt: string;
  // Future OAuth/groups support
  externalGroupId?: string;
}

export interface CreateTeamRequest {
  name: string;
  teamLeadId?: string | null;
  cadence: string;
  distributionListEmail?: string | null;
  memberIds?: string[];
  // Future OAuth/groups support
  externalGroupId?: string;
}

export interface UpdateTeamRequest {
  name?: string;
  teamLeadId?: string | null;
  cadence?: string;
  distributionListEmail?: string | null;
  memberIds?: string[];
  // Future OAuth/groups support
  externalGroupId?: string;
}

export interface AdminTeamsListResponse {
  teams: AdminTeam[];
  total: number;
}

// ============================================================================
// SUPERVISOR CHAIN TYPES
// ============================================================================

export interface SupervisorLink {
  userId: string;
  userName: string;
  levelId: string;
  levelName: string;
}

export interface SupervisorChainResponse {
  teamId: string;
  supervisors: SupervisorLink[];
}

export interface UpdateSupervisorChainRequest {
  supervisors: { userId: string; levelId: string }[];
}

// ============================================================================
// HEALTH DIMENSION TYPES
// ============================================================================

export interface HealthDimension {
  id: string;
  name: string;
  description: string;
  goodDescription: string;
  badDescription: string;
  position: number;
  isActive: boolean;
  weight: number;
  createdAt: string;
  updatedAt: string;
}

export interface CreateDimensionRequest {
  id: string;
  name: string;
  description?: string;
  goodDescription: string;
  badDescription: string;
  isActive?: boolean;
  weight?: number;
}

export interface UpdateDimensionRequest {
  name?: string;
  description?: string;
  goodDescription?: string;
  badDescription?: string;
  position?: number;
  isActive?: boolean;
  weight?: number;
}

export interface DimensionsListResponse {
  dimensions: HealthDimension[];
  total: number;
}

// ============================================================================
// NOTIFICATION SETTINGS TYPES
// ============================================================================

export interface NotificationSettings {
  emailEnabled: boolean;
  slackEnabled: boolean;
  notifyOnSubmission: boolean;
  notifyManagers: boolean;
  reminderDaysBefore: number;
  reminderRecipients: string[];
  smtpConfigured: boolean;
}

export interface UpdateNotificationSettingsRequest {
  emailEnabled?: boolean;
  slackEnabled?: boolean;
  notifyOnSubmission?: boolean;
}

export interface RetentionPolicy {
  keepSessionsMonths: number;
  archiveEnabled: boolean;
  anonymizeAfterDays: number;
}

export interface UpdateRetentionPolicyRequest {
  keepSessionsMonths: number;
}

// ============================================================================
// HIERARCHY LEVEL API METHODS
// ============================================================================

/**
 * Response wrapper for hierarchy levels list
 */
interface HierarchyLevelsResponse {
  levels: HierarchyLevel[];
}

/**
 * Fetches all hierarchy levels
 *
 * @returns List of all hierarchy levels ordered by position
 */
export async function listHierarchyLevels(): Promise<HierarchyLevel[]> {
  const response = await createApiClient<HierarchyLevelsResponse>(`${API_BASE_URL}/api/v1/admin/hierarchy-levels`);
  return response.levels;
}

/**
 * Creates a new hierarchy level
 *
 * @param request - Hierarchy level data
 * @returns Created hierarchy level
 */
export async function createHierarchyLevel(
  request: CreateHierarchyLevelRequest
): Promise<HierarchyLevel> {
  return createApiClient<HierarchyLevel>(`${API_BASE_URL}/api/v1/admin/hierarchy-levels`, {
    method: 'POST',
    body: JSON.stringify(request),
  });
}

/**
 * Updates an existing hierarchy level
 *
 * @param levelId - Hierarchy level ID
 * @param request - Fields to update
 * @returns Updated hierarchy level
 */
export async function updateHierarchyLevel(
  levelId: string,
  request: UpdateHierarchyLevelRequest
): Promise<HierarchyLevel> {
  return createApiClient<HierarchyLevel>(
    `${API_BASE_URL}/api/v1/admin/hierarchy-levels/${levelId}`,
    {
      method: 'PUT',
      body: JSON.stringify(request),
    }
  );
}

/**
 * Updates hierarchy level position (for reordering)
 *
 * @param levelId - Hierarchy level ID
 * @param request - New position
 * @returns Updated hierarchy level
 */
export async function updateHierarchyPosition(
  levelId: string,
  request: UpdateHierarchyPositionRequest
): Promise<HierarchyLevel> {
  return createApiClient<HierarchyLevel>(
    `${API_BASE_URL}/api/v1/admin/hierarchy-levels/${levelId}/position`,
    {
      method: 'PATCH',
      body: JSON.stringify(request),
    }
  );
}

/**
 * Deletes a hierarchy level
 *
 * @param levelId - Hierarchy level ID
 */
export async function deleteHierarchyLevel(levelId: string): Promise<void> {
  await createApiClient<void>(`${API_BASE_URL}/api/v1/admin/hierarchy-levels/${levelId}`, {
    method: 'DELETE',
  });
}

// ============================================================================
// USER MANAGEMENT API METHODS
// ============================================================================

/**
 * Fetches all users
 *
 * @returns List of all users with pagination info
 */
export async function listUsers(): Promise<UsersListResponse> {
  return createApiClient<UsersListResponse>(`${API_BASE_URL}/api/v1/admin/users`);
}

/**
 * Creates a new user
 *
 * @param request - User data including credentials
 * @returns Created user
 */
export async function createUser(request: CreateUserRequest): Promise<AdminUser> {
  return createApiClient<AdminUser>(`${API_BASE_URL}/api/v1/admin/users`, {
    method: 'POST',
    body: JSON.stringify(request),
  });
}

/**
 * Updates an existing user
 *
 * @param userId - User ID
 * @param request - Fields to update
 * @returns Updated user
 */
export async function updateUser(
  userId: string,
  request: UpdateUserRequest
): Promise<AdminUser> {
  return createApiClient<AdminUser>(`${API_BASE_URL}/api/v1/admin/users/${userId}`, {
    method: 'PUT',
    body: JSON.stringify(request),
  });
}

/**
 * Deletes a user
 *
 * @param userId - User ID
 */
export async function deleteUser(userId: string): Promise<void> {
  await createApiClient<void>(`${API_BASE_URL}/api/v1/admin/users/${userId}`, {
    method: 'DELETE',
  });
}

// ============================================================================
// TEAM MANAGEMENT API METHODS
// ============================================================================

/**
 * Fetches all teams (admin view with detailed info)
 *
 * @returns List of all teams with pagination info
 */
export async function listAdminTeams(): Promise<AdminTeamsListResponse> {
  return createApiClient<AdminTeamsListResponse>(`${API_BASE_URL}/api/v1/admin/teams`);
}

/**
 * Creates a new team
 *
 * @param request - Team data
 * @returns Created team
 */
export async function createTeam(request: CreateTeamRequest): Promise<AdminTeam> {
  return createApiClient<AdminTeam>(`${API_BASE_URL}/api/v1/admin/teams`, {
    method: 'POST',
    body: JSON.stringify(request),
  });
}

/**
 * Updates an existing team
 *
 * @param teamId - Team ID
 * @param request - Fields to update
 * @returns Updated team
 */
export async function updateTeam(
  teamId: string,
  request: UpdateTeamRequest
): Promise<AdminTeam> {
  return createApiClient<AdminTeam>(`${API_BASE_URL}/api/v1/admin/teams/${teamId}`, {
    method: 'PUT',
    body: JSON.stringify(request),
  });
}

/**
 * Deletes a team
 *
 * @param teamId - Team ID
 */
export async function deleteTeam(teamId: string): Promise<void> {
  await createApiClient<void>(`${API_BASE_URL}/api/v1/admin/teams/${teamId}`, {
    method: 'DELETE',
  });
}

// ============================================================================
// TEAM MEMBER MANAGEMENT API METHODS
// ============================================================================

export interface TeamMemberAdmin {
  userId: string;
  userName: string;
  email: string;
}

export interface TeamMembersResponse {
  members: TeamMemberAdmin[];
  total: number;
}

/**
 * Fetches members of a team
 */
export async function getTeamMembers(teamId: string): Promise<TeamMembersResponse> {
  return createApiClient<TeamMembersResponse>(
    `${API_BASE_URL}/api/v1/admin/teams/${teamId}/members`
  );
}

/**
 * Adds a member to a team
 */
export async function addTeamMember(teamId: string, userId: string): Promise<void> {
  await createApiClient<void>(`${API_BASE_URL}/api/v1/admin/teams/${teamId}/members`, {
    method: 'POST',
    body: JSON.stringify({ userId }),
  });
}

/**
 * Removes a member from a team
 */
export async function removeTeamMember(teamId: string, userId: string): Promise<void> {
  await createApiClient<void>(
    `${API_BASE_URL}/api/v1/admin/teams/${teamId}/members/${userId}`,
    { method: 'DELETE' }
  );
}

// ============================================================================
// SUPERVISOR CHAIN API METHODS
// ============================================================================

/**
 * Fetches the supervisor chain for a team
 *
 * @param teamId - Team ID
 * @returns Supervisor chain with user and level names
 */
export async function getSupervisorChain(teamId: string): Promise<SupervisorChainResponse> {
  return createApiClient<SupervisorChainResponse>(
    `${API_BASE_URL}/api/v1/admin/teams/${teamId}/supervisors`
  );
}

/**
 * Updates the supervisor chain for a team
 *
 * @param teamId - Team ID
 * @param request - New supervisor chain
 * @returns Updated supervisor chain
 */
export async function updateSupervisorChain(
  teamId: string,
  request: UpdateSupervisorChainRequest
): Promise<SupervisorChainResponse> {
  return createApiClient<SupervisorChainResponse>(
    `${API_BASE_URL}/api/v1/admin/teams/${teamId}/supervisors`,
    {
      method: 'PUT',
      body: JSON.stringify(request),
    }
  );
}

// ============================================================================
// HEALTH DIMENSIONS API METHODS
// ============================================================================

/**
 * Fetches all health dimensions
 *
 * @returns List of all health dimensions ordered by position
 */
export async function getDimensions(): Promise<DimensionsListResponse> {
  return createApiClient<DimensionsListResponse>(
    `${API_BASE_URL}/api/v1/admin/settings/dimensions`
  );
}

/**
 * Creates a new health dimension
 *
 * @param request - Dimension data
 * @returns Created dimension
 */
export async function createDimension(
  request: CreateDimensionRequest
): Promise<HealthDimension> {
  return createApiClient<HealthDimension>(
    `${API_BASE_URL}/api/v1/admin/settings/dimensions`,
    {
      method: 'POST',
      body: JSON.stringify(request),
    }
  );
}

/**
 * Updates a health dimension
 *
 * @param dimensionId - Dimension ID
 * @param request - Fields to update
 * @returns Updated dimension
 */
export async function updateDimension(
  dimensionId: string,
  request: UpdateDimensionRequest
): Promise<HealthDimension> {
  return createApiClient<HealthDimension>(
    `${API_BASE_URL}/api/v1/admin/settings/dimensions/${dimensionId}`,
    {
      method: 'PUT',
      body: JSON.stringify(request),
    }
  );
}

/**
 * Deletes a health dimension
 *
 * @param dimensionId - Dimension ID
 */
export async function deleteDimension(dimensionId: string): Promise<void> {
  await createApiClient<void>(
    `${API_BASE_URL}/api/v1/admin/settings/dimensions/${dimensionId}`,
    {
      method: 'DELETE',
    }
  );
}

// ============================================================================
// BRANDING SETTINGS API METHODS
// ============================================================================

export interface BrandingSettings {
  companyName: string;
  logoURL: string;
}

/**
 * Fetches branding settings
 */
export async function getBrandingSettings(): Promise<BrandingSettings> {
  return createApiClient<BrandingSettings>(
    `${API_BASE_URL}/api/v1/admin/settings/branding`
  );
}

/**
 * Updates branding settings (company name and optional logo)
 */
export async function updateBrandingSettings(
  request: BrandingSettings
): Promise<BrandingSettings> {
  return createApiClient<BrandingSettings>(
    `${API_BASE_URL}/api/v1/admin/settings/branding`,
    {
      method: 'PUT',
      body: JSON.stringify(request),
    }
  );
}

// ============================================================================
// NOTIFICATION SETTINGS API METHODS
// ============================================================================

/**
 * Fetches notification settings
 *
 * @returns Current notification settings
 */
export async function getNotificationSettings(): Promise<NotificationSettings> {
  return createApiClient<NotificationSettings>(
    `${API_BASE_URL}/api/v1/admin/settings/notifications`
  );
}

/**
 * Updates notification settings
 *
 * @param request - Fields to update
 * @returns Updated notification settings
 */
export async function updateNotificationSettings(
  request: UpdateNotificationSettingsRequest
): Promise<NotificationSettings> {
  return createApiClient<NotificationSettings>(
    `${API_BASE_URL}/api/v1/admin/settings/notifications`,
    {
      method: 'PUT',
      body: JSON.stringify(request),
    }
  );
}

// ============================================================================
// RETENTION POLICY API METHODS
// ============================================================================

/**
 * Fetches retention policy settings
 */
export async function getRetentionPolicy(): Promise<RetentionPolicy> {
  return createApiClient<RetentionPolicy>(
    `${API_BASE_URL}/api/v1/admin/settings/retention`
  );
}

/**
 * Updates retention policy settings
 */
export async function updateRetentionPolicy(
  request: UpdateRetentionPolicyRequest
): Promise<RetentionPolicy> {
  return createApiClient<RetentionPolicy>(
    `${API_BASE_URL}/api/v1/admin/settings/retention`,
    {
      method: 'PUT',
      body: JSON.stringify(request),
    }
  );
}

// ============================================================================
// CACHE MANAGEMENT
// ============================================================================

/**
 * Cache for admin data to avoid repeated API calls
 */
const adminCache = new Map<string, { data: unknown; timestamp: number }>();
const CACHE_TTL = 2 * 60 * 1000; // 2 minutes (shorter TTL for admin data)

/**
 * Generic cached getter
 *
 * @param key - Cache key
 * @param fetcher - Function to fetch data if not cached
 * @returns Cached or fresh data
 */
async function getCached<T>(key: string, fetcher: () => Promise<T>): Promise<T> {
  const cached = adminCache.get(key);
  const now = Date.now();

  if (cached && now - cached.timestamp < CACHE_TTL) {
    return cached.data as T;
  }

  const data = await fetcher();
  adminCache.set(key, { data, timestamp: now });
  return data;
}

/**
 * Cached hierarchy levels fetch
 */
export async function listHierarchyLevelsCached(): Promise<HierarchyLevel[]> {
  return getCached('hierarchy-levels', listHierarchyLevels);
}

/**
 * Cached users list fetch
 */
export async function listUsersCached(): Promise<UsersListResponse> {
  return getCached('users-list', listUsers);
}

/**
 * Cached teams list fetch
 */
export async function listAdminTeamsCached(): Promise<AdminTeamsListResponse> {
  return getCached('admin-teams-list', listAdminTeams);
}

/**
 * Cached dimensions fetch
 */
export async function getDimensionsCached(): Promise<DimensionsListResponse> {
  return getCached('dimensions-list', getDimensions);
}

/**
 * Clears the admin cache
 *
 * Call this after mutations (create/update/delete) to ensure fresh data
 */
export function clearAdminCache(): void {
  adminCache.clear();
}

/**
 * Clears specific cache entries
 *
 * @param keys - Cache keys to clear
 */
export function clearAdminCacheKeys(...keys: string[]): void {
  keys.forEach((key) => adminCache.delete(key));
}
