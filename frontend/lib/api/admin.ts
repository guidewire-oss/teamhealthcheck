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

export interface PaginationMeta {
  page: number;
  pageSize: number;
  totalItems: number;
  totalPages: number;
  hasNextPage: boolean;
  hasPreviousPage: boolean;
}

export interface UsersListResponse {
  users: AdminUser[];
  pagination: PaginationMeta;
}

export interface ListUsersParams {
  page?: number;
  pageSize?: number;
  search?: string;
  role?: string;
  signal?: AbortSignal;
}

export interface UserLite {
  id: string;
  username: string;
  fullName: string;
  hierarchyLevel: string;
}

export interface UsersLiteResponse {
  users: UserLite[];
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
 * Fetches one page of users, with optional search/role filtering applied
 * server-side.
 *
 * @param params - Pagination (page, pageSize), filters (search, role), and an
 *   optional AbortSignal to cancel a stale in-flight request
 * @returns The requested page of users plus pagination metadata
 */
export async function listUsers(params: ListUsersParams = {}): Promise<UsersListResponse> {
  const { page, pageSize, search, role, signal } = params;
  const query = new URLSearchParams();
  if (page !== undefined) query.set('page', String(page));
  if (pageSize !== undefined) query.set('pageSize', String(pageSize));
  if (search) query.set('search', search);
  if (role) query.set('role', role);

  const qs = query.toString();
  return createApiClient<UsersListResponse>(
    `${API_BASE_URL}/api/v1/admin/users${qs ? `?${qs}` : ''}`,
    signal ? { signal } : undefined
  );
}

/**
 * Fetches minimal data (id, username, fullName, hierarchyLevel) for every
 * user. Intended for dropdowns/pickers (team lead, reports-to) that need the
 * full user set without the cost of the paginated listing's team-membership
 * joins.
 *
 * @returns All users in minimal form
 */
export async function listUsersLite(): Promise<UsersLiteResponse> {
  return createApiClient<UsersLiteResponse>(`${API_BASE_URL}/api/v1/admin/users/lite`);
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

// ============================================================================
// ORGANIZATION PROVIDER API METHODS
// ============================================================================

/**
 * Whether the external organization-data provider is configured.
 *
 * There is no token field: the provider credential lives only in the
 * backend's environment configuration, never in the database or this response.
 */
export interface OrganizationProviderSettings {
  provider: string;
  /** DATA_PROVIDER_BASE_URL is set on the backend and passed construction-time validation. */
  baseUrlConfigured: boolean;
  /** DATA_PROVIDER_API_TOKEN is set on the backend. */
  tokenConfigured: boolean;
  /** Every prerequisite is met, so a sync can run. */
  readyToSync: boolean;
}

/** A snapshot user that could not be imported, and why. */
export interface SkippedProviderUser {
  userId: string;
  username: string;
  hierarchyLevelId: string;
  reason: string;
}

/** The outcome of one synchronization run. */
export interface OrganizationSyncResult {
  status: string;
  teamsSynced: number;
  usersSynced: number;
  membershipsSynced: number;
  membershipsRemoved: number;
  healthChecksDisabled: number;
  healthChecksEnabled: number;
  usersDeleted: number;
  teamsDeleted: number;
  /** Action items removed as a side effect of the deletions above (cascaded, not held back). */
  actionItemsDeleted: number;
  usersSkipped: number;
  skippedUsers?: SkippedProviderUser[];
  managerLinksCleared: number;
  teamLeadsCleared: number;
  membershipsDiscarded: number;
  /** True when this run only completed because an admin waived the mass-deletion hold. */
  massDeletionOverridden?: boolean;
  /** The counts that were waived, present only alongside massDeletionOverridden. */
  massDeletion?: MassDeletionReport;
  startedAt: string;
  completedAt: string;
}

/**
 * Fetches organization provider configuration readiness.
 *
 * There is no token field anywhere in this response: the provider credential
 * lives only in the backend's environment configuration
 * (DATA_PROVIDER_BASE_URL / DATA_PROVIDER_API_TOKEN) and is never entered,
 * stored, or displayed through this UI.
 */
export async function getOrganizationProviderSettings(): Promise<OrganizationProviderSettings> {
  return createApiClient<OrganizationProviderSettings>(
    `${API_BASE_URL}/api/v1/admin/settings/organization-provider`
  );
}

/**
 * The mass-deletion threshold in force for the organization sync, and where it
 * came from. Carries no provider credentials -- it is a safety percentage.
 */
export interface OrgSyncDeletionThreshold {
  /** The percentage actually applied by the guard. */
  maxDeletePercent: number;
  /** 'admin' (saved here), 'environment' (ORG_SYNC_MAX_DELETE_PERCENT), or 'default'. */
  source: 'admin' | 'environment' | 'default';
  /** The recommended value, shown as a hint rather than hardcoded in the UI. */
  defaultPercent: number;
  minPercent: number;
  maxPercent: number;
  /**
   * True while a running or held sync freezes the threshold. The backend
   * enforces this independently -- the disabled controls are a courtesy, not
   * the guarantee.
   */
  locked: boolean;
  /** Why it is frozen: 'syncing' or 'held'. Absent when editable. */
  lockReason?: 'syncing' | 'held';
  /** The value in force for the run holding the lock, when one is. */
  activeSyncThreshold?: number;
}

/** ErrorResponse.code returned when a threshold update is refused by the lock. */
export const THRESHOLD_LOCKED_CODE = 'threshold_locked';

/**
 * Reads the mass-deletion threshold in force, resolved by the same precedence
 * the sync itself uses.
 */
export async function getOrgSyncDeletionThreshold(): Promise<OrgSyncDeletionThreshold> {
  return createApiClient<OrgSyncDeletionThreshold>(
    `${API_BASE_URL}/api/v1/admin/settings/organization-provider/deletion-threshold`
  );
}

/**
 * Saves the mass-deletion threshold. Only finite values from 1 to 100 are
 * accepted; the backend validates independently of this client.
 */
export async function updateOrgSyncDeletionThreshold(
  maxDeletePercent: number
): Promise<OrgSyncDeletionThreshold> {
  return createApiClient<OrgSyncDeletionThreshold>(
    `${API_BASE_URL}/api/v1/admin/settings/organization-provider/deletion-threshold`,
    {
      method: 'PUT',
      body: JSON.stringify({ maxDeletePercent }),
    }
  );
}

/**
 * Resolves a mass-deletion hold without applying it, which unfreezes the
 * threshold. It applies nothing and deletes nothing -- the held sync stays
 * unapplied. Overriding a hold is the separate, explicit Sync Anyway action.
 */
export async function dismissMassDeletionHold(): Promise<OrgSyncDeletionThreshold> {
  return createApiClient<OrgSyncDeletionThreshold>(
    `${API_BASE_URL}/api/v1/admin/organization-provider/sync/hold`,
    { method: 'DELETE' }
  );
}

/** The three fixed automatic-sync cadences. Never raw cron syntax in the UI. */
export type OrgSyncFrequency = 'daily' | 'weekly' | 'monthly';

/** The admin-configured automatic organization-sync schedule. */
export interface OrgSyncSchedule {
  enabled: boolean;
  /** Absent while never configured. */
  frequency?: OrgSyncFrequency;
  /** ISO 8601, absent while disabled. */
  nextRunAt?: string;
  /** The fixed set of choices to render -- served by the backend, not hardcoded here either. */
  availableFrequencies: OrgSyncFrequency[];
  /** Used only on the very first ever enable; a later re-enable reuses the last saved frequency. */
  defaultFrequency: OrgSyncFrequency;
}

/** Reads the automatic organization-sync schedule. */
export async function getOrgSyncSchedule(): Promise<OrgSyncSchedule> {
  return createApiClient<OrgSyncSchedule>(
    `${API_BASE_URL}/api/v1/admin/settings/organization-provider/schedule`
  );
}

/**
 * Enables, disables, or changes the frequency of the automatic sync.
 * Disabling preserves the saved frequency (so re-enabling remembers the last
 * choice) but clears the next-run date.
 */
export async function updateOrgSyncSchedule(
  enabled: boolean,
  frequency?: OrgSyncFrequency
): Promise<OrgSyncSchedule> {
  return createApiClient<OrgSyncSchedule>(
    `${API_BASE_URL}/api/v1/admin/settings/organization-provider/schedule`,
    {
      method: 'PUT',
      body: JSON.stringify({ enabled, frequency }),
    }
  );
}

/** Tri-state report of whether a sync attempt's destructive writes landed. */
export type OrgSyncWritesStatus = 'none' | 'applied' | 'unknown';

/** One sync attempt that ran to a conclusion (success, blocked, or failed). */
export interface OrgSyncAttempt {
  trigger: 'manual' | 'scheduled';
  status: 'success' | 'blocked' | 'failed';
  startedAt: string;
  finishedAt: string;
  thresholdPercent?: number;
  usersExisting?: number;
  usersDeleting?: number;
  usersPercent?: number;
  teamsExisting?: number;
  teamsDeleting?: number;
  teamsPercent?: number;
  /**
   * 'applied' means the transaction committed, not that any row changed --
   * always shown alongside the counts above, never alone. 'unknown' means the
   * outcome could not be confirmed either way; verify current counts directly.
   */
  writesStatus: OrgSyncWritesStatus;
  overrideUsed: boolean;
  usersSynced?: number;
  teamsSynced?: number;
  message: string;
}

/** The most recent scheduled occurrence that was skipped rather than attempted. */
export interface OrgSyncSkip {
  reason: 'manual_sync_running' | 'scheduled_sync_running' | 'hold_unresolved' | 'overdue_catch_up_skipped';
  at: string;
}

/**
 * The persisted result of the organization sync's most recent activity.
 * lastAttempt and lastSkip are independently absent: a skip never overwrites
 * the last attempt that actually ran, so both can be shown together --
 * headline result plus a secondary "most recently skipped" line.
 */
export interface OrgSyncLastRun {
  lastAttempt?: OrgSyncAttempt;
  lastSkip?: OrgSyncSkip;
}

/** Reads the persisted last-attempt/last-skip result, surviving refresh, logout/login, and a process restart. */
export async function getOrgSyncLastRun(): Promise<OrgSyncLastRun> {
  return createApiClient<OrgSyncLastRun>(
    `${API_BASE_URL}/api/v1/admin/organization-provider/sync/last-run`
  );
}

/** How a deletion metric's rows are removed. */
export type DeletionMetricKind = 'deleted' | 'cascaded';

/** One entity type's share of the deletions a sync proposes. */
export interface DeletionMetric {
  /** 'deleted' for rows the sync removes directly, 'cascaded' for rows the database removes with them. */
  kind: DeletionMetricKind;
  /** Denominator: eligible (non-protected) rows currently in Team360. */
  existing: number;
  /**
   * How many records of this type the provider's snapshot actually contained.
   * Not part of the threshold math -- it is the number that tells an admin
   * whether a surprising hold is a real mass departure or just an incomplete
   * provider payload.
   */
  incoming: number;
  /** Numerator: rows this sync proposes to remove. */
  deleting: number;
  /** deleting / existing * 100, as the backend guard computes it. */
  percent: number;
  /** The configured maximum percentage (default 20). */
  threshold: number;
  exceedsThreshold: boolean;
  /** False for metrics reported for information only (memberships). */
  contributesToHold: boolean;
}

/** The counts behind a sync the mass-deletion guard held. Aggregates only. */
export interface MassDeletionReport {
  threshold: number;
  users: DeletionMetric;
  teams: DeletionMetric;
  /** Informational cascade metric; absent when the backend could not measure it. */
  memberships?: DeletionMetric;
}

/** The typed `code` the backend sets on a held sync. */
export const MASS_DELETION_HOLD_CODE = 'mass_deletion_hold';

/**
 * Extracts the mass-deletion report from a failed sync, or null if the failure
 * was something else.
 *
 * Recognition is by the typed `code` field, not by matching message text.
 */
export function getMassDeletionHold(error: unknown): MassDeletionReport | null {
  const apiError = (error as APIRequestError | undefined)?.apiError as
    | { code?: string; massDeletion?: MassDeletionReport }
    | undefined;
  if (!apiError || apiError.code !== MASS_DELETION_HOLD_CODE) return null;
  return apiError.massDeletion ?? null;
}

/** The four counts an admin reviewed on a held sync's response, echoed back
 * to confirm an override. See syncOrganizationProvider. */
export interface ConfirmedMassDeletion {
  usersExisting: number;
  usersDeleting: number;
  teamsExisting: number;
  teamsDeleting: number;
}

/**
 * Triggers a manual organization sync
 *
 * Rewrites users, teams and memberships, so callers must clear the admin cache
 * on success or the UI will keep serving pre-sync counts for up to two minutes.
 *
 * `overrideMassDeletion` waives the backend's mass-deletion hold for this one
 * request. It is only ever sent after an admin has reviewed the held counts and
 * explicitly confirmed; the backend re-checks admin privileges and never trusts
 * this flag on its own. A plain sync sends no body at all, so the normal request
 * is byte-for-byte what it was before this option existed.
 *
 * `confirmedMassDeletion` is required alongside `overrideMassDeletion`: every
 * sync re-fetches the provider snapshot fresh, so a bare override flag would
 * waive the guard for whatever that fresh fetch turns up, which may no longer
 * be the deletion the admin actually reviewed. Pass the exact counts from the
 * `MassDeletionReport` the held response carried -- the backend refuses the
 * override (returning a fresh hold instead) if they no longer match.
 */
export async function syncOrganizationProvider(
  options: { overrideMassDeletion?: boolean; confirmedMassDeletion?: ConfirmedMassDeletion } = {}
): Promise<OrganizationSyncResult> {
  const request: RequestInit = { method: 'POST' };
  if (options.overrideMassDeletion) {
    request.body = JSON.stringify({
      overrideMassDeletion: true,
      confirmedMassDeletion: options.confirmedMassDeletion,
    });
  }
  return createApiClient<OrganizationSyncResult>(
    `${API_BASE_URL}/api/v1/admin/organization-provider/sync`,
    request
  );
}
