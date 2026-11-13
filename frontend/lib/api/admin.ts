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

import { API_BASE_URL, APIError, APIRequestError, apiRequest, createApiClient, handleResponse } from './client';

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
  /** False for a soft-deactivated person, who stays listed for history. */
  isActive: boolean;
  deactivatedAt?: string | null;
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
  /** Limit to active or inactive people. Omit for everyone (inactive included). */
  status?: 'active' | 'inactive';
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
  /** False for a soft-deactivated team, which stays listed for history. */
  isActive: boolean;
  deactivatedAt?: string | null;
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
  const { page, pageSize, search, role, status, signal } = params;
  const query = new URLSearchParams();
  if (page !== undefined) query.set('page', String(page));
  if (pageSize !== undefined) query.set('pageSize', String(pageSize));
  if (search) query.set('search', search);
  if (role) query.set('role', role);
  if (status) query.set('status', status);

  const qs = query.toString();
  return createApiClient<UsersListResponse>(
    `${API_BASE_URL}/api/v1/admin/users${qs ? `?${qs}` : ''}`,
    signal ? { signal } : undefined
  );
}

/** The backend clamps pageSize to 100, so asking for more gains nothing. */
const ALL_USERS_PAGE_SIZE = 100;

/**
 * Fetches EVERY user matching the optional filters by walking all pages of
 * listUsers. Use this only where the full set is genuinely needed (e.g. an
 * add-member picker); listUsers itself stays a single, bounded page so
 * unrelated callers never trigger an unbounded fetch. Users are deduplicated
 * by id, since rows can shift between pages if the set changes mid-walk, and
 * the AbortSignal cancels both the in-flight page and any remaining pages.
 *
 * @returns Every matching user, in listing order
 */
export async function listAllUsers(
  params: Pick<ListUsersParams, 'search' | 'role' | 'signal'> = {}
): Promise<AdminUser[]> {
  const { signal } = params;
  const byId = new Map<string, AdminUser>();
  let page = 1;
  let totalPages = 1;

  do {
    if (signal?.aborted) {
      throw new DOMException('Request aborted', 'AbortError');
    }
    const response = await listUsers({ ...params, page, pageSize: ALL_USERS_PAGE_SIZE });
    for (const user of response.users ?? []) {
      if (!byId.has(user.id)) byId.set(user.id, user);
    }
    totalPages = response.pagination?.totalPages ?? 1;
    page += 1;
  } while (page <= totalPages);

  return Array.from(byId.values());
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

/** Persisted lifecycle state of one organization sync run. */
export type SyncRunStatus = 'PENDING' | 'RUNNING' | 'SUCCESS' | 'FAILED';

/**
 * One organization sync run as the backend persists it. This is the single
 * source of truth for the Sync Now screen, so a refreshed page re-reads it
 * instead of guessing.
 */
export interface SyncRun {
  runId: string;
  status: SyncRunStatus;
  /** Current step, e.g. UPSERTING_USERS; FAILED once the run has failed. */
  stage: string;
  startedAt: string | null;
  completedAt: string | null;
  createdAt: string;
  /** Added = newly inserted. Updated = existing and a business field really changed. */
  insertedUsers: number;
  updatedUsers: number;
  /** Existing and identical: never counted as updated. null = not recorded for this run. */
  unchangedUsers: number | null;
  insertedTeams: number;
  updatedTeams: number;
  unchangedTeams: number | null;
  /** Real membership additions, reactivations and removals only. */
  membershipChanges: number;
  /** People and teams marked inactive. Nothing is ever hard-deleted. */
  deactivatedRecords: number;
  /** Teams whose health checks this sync switched off / on. */
  healthChecksDisabled: number;
  healthChecksEnabled: number;
  /** True when an administrator waived the mass-deletion hold for this run. */
  massDeletionOverridden: boolean;
  errorCode: string | null;
  /** Safe to show an administrator; never contains credentials. */
  errorMessage: string | null;
  /** The stage that failed, present only when status is FAILED. */
  failedStage: string | null;
  /** Counts behind a mass-deactivation hold, when that is why the run failed. */
  massDeletion?: MassDeletionReport;
  /** True when a start request was answered with a run already in flight. */
  alreadyActive?: boolean;
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

/** The typed `errorCode` a run carries when the mass-deactivation guard held it. */
export const MASS_DEACTIVATION_HOLD_CODE = 'MASS_DEACTIVATION_HOLD';

/**
 * The counts behind a held run, or null if the run failed for another reason.
 * Recognition is by the typed errorCode, not by matching message text.
 */
export function getMassDeletionHold(run: SyncRun | null | undefined): MassDeletionReport | null {
  if (!run || run.status !== 'FAILED' || run.errorCode !== MASS_DEACTIVATION_HOLD_CODE) return null;
  return run.massDeletion ?? null;
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
): Promise<SyncRun> {
  const request: RequestInit = { method: 'POST' };
  if (options.overrideMassDeletion) {
    request.body = JSON.stringify({
      overrideMassDeletion: true,
      confirmedMassDeletion: options.confirmedMassDeletion,
    });
  }
  // The backend answers 202 with the PENDING run; the work itself continues in
  // the background and is followed with getSyncRun.
  return createApiClient<SyncRun>(
    `${API_BASE_URL}/api/v1/admin/organization-provider/sync`,
    request
  );
}

/** Reads one run's persisted state. */
export async function getSyncRun(runId: string): Promise<SyncRun> {
  return createApiClient<SyncRun>(
    `${API_BASE_URL}/api/v1/admin/organization-provider/sync/runs/${encodeURIComponent(runId)}`
  );
}

/**
 * The most recent run, or null if no sync has ever run (the backend answers
 * 204 with no body, which the shared JSON helper cannot parse).
 */
export async function getLatestSyncRun(): Promise<SyncRun | null> {
  const response = await apiRequest(`${API_BASE_URL}/api/v1/admin/organization-provider/sync/runs/latest`);
  if (response.status === 204) return null;
  return handleResponse<SyncRun>(response);
}

/** How often an automatic organization sync runs. */
export type AutoSyncFrequency = 'DAILY' | 'WEEKLY' | 'MONTHLY';

/** One automatic run. `startedAt` is when the attempt was recorded (UTC). */
export interface AutoSyncAttempt {
  runId: string;
  status: SyncRunStatus;
  startedAt: string;
  completedAt: string | null;
  /** Safe to show an administrator; never contains credentials. */
  errorMessage: string | null;
}

/** Saved automatic-sync schedule plus automatic-run history. All times are UTC. */
export interface AutoSyncSchedule {
  enabled: boolean;
  frequency: AutoSyncFrequency;
  /** e.g. "Weekly on Monday at 02:00 UTC" */
  description: string;
  timezone: string;
  /** null when auto-sync is disabled. */
  nextRunAt: string | null;
  lastAttempt: AutoSyncAttempt | null;
  /** Completion time of the last SUCCESSFUL automatic sync. */
  lastSuccessAt: string | null;
}

export async function getAutoSyncSchedule(): Promise<AutoSyncSchedule> {
  return createApiClient<AutoSyncSchedule>(
    `${API_BASE_URL}/api/v1/admin/organization-provider/auto-sync`
  );
}

/** Saves the schedule. This never starts a sync. */
export async function updateAutoSyncSchedule(
  enabled: boolean,
  frequency: AutoSyncFrequency
): Promise<AutoSyncSchedule> {
  return createApiClient<AutoSyncSchedule>(
    `${API_BASE_URL}/api/v1/admin/organization-provider/auto-sync`,
    { method: 'PUT', body: JSON.stringify({ enabled, frequency }) }
  );
}

// ============================================================================
// SYNC HISTORY (read-only)
// ============================================================================

/** What started a run (org_sync_runs.trigger_type). */
export type SyncRunTrigger = 'MANUAL' | 'SCHEDULED';

/** Whether the provider snapshot was complete (org_sync_runs.snapshot_type). */
export type SyncRunSnapshotType = 'COMPLETE' | 'PARTIAL';

/**
 * One row of sync history, as the admin-only history endpoint returns it. It is
 * a deliberate subset of the persisted run: no idempotency key, worker id,
 * heartbeat or raw hold report.
 */
export interface SyncRunHistoryItem {
  runId: string;
  snapshotType: SyncRunSnapshotType;
  trigger: SyncRunTrigger;
  status: SyncRunStatus;
  stage: string;
  /** Administrator user id, or "scheduler" for an automatic run. */
  initiatedBy: string;
  /** When the run was accepted/queued -- not when work began. */
  createdAt: string;
  /** When a worker began processing; null while queued or if it failed first. */
  startedAt: string | null;
  /** When the run reached SUCCESS or FAILED; null while active. */
  completedAt: string | null;
  /** When the provider generated the applied snapshot; only for SUCCESS runs. */
  sourceGeneratedAt: string | null;
  /** startedAt -> completedAt; null when it cannot be calculated accurately. */
  durationSeconds: number | null;
  /** Failed runs only. */
  failedStage: string | null;
  errorCode: string | null;
  /** Failed runs only; sanitized by the server. */
  errorMessage: string | null;
  /**
   * What this run committed, same meaning as the Settings "Sync complete"
   * summary. All null unless status is SUCCESS (a queued, running or failed run
   * committed nothing). null = not available; 0 is a real zero.
   */
  insertedUsers: number | null;
  updatedUsers: number | null;
  /** Also null for older runs that never recorded it. */
  unchangedUsers: number | null;
  insertedTeams: number | null;
  updatedTeams: number | null;
  unchangedTeams: number | null;
  membershipChanges: number | null;
}

export interface SyncRunHistoryResponse {
  runs: SyncRunHistoryItem[];
  pagination: PaginationMeta;
}

export interface ListSyncRunHistoryParams {
  page?: number;
  pageSize?: number;
  status?: SyncRunStatus;
  trigger?: SyncRunTrigger;
  snapshotType?: SyncRunSnapshotType;
  signal?: AbortSignal;
}

/** Fetches one page of sync-run history, newest first. Admin only. */
export async function listSyncRunHistory(
  params: ListSyncRunHistoryParams = {}
): Promise<SyncRunHistoryResponse> {
  const { page, pageSize, status, trigger, snapshotType, signal } = params;
  const query = new URLSearchParams();
  if (page !== undefined) query.set('page', String(page));
  if (pageSize !== undefined) query.set('pageSize', String(pageSize));
  if (status) query.set('status', status);
  if (trigger) query.set('trigger', trigger);
  if (snapshotType) query.set('snapshotType', snapshotType);
  const qs = query.toString();
  return createApiClient<SyncRunHistoryResponse>(
    `${API_BASE_URL}/api/v1/admin/organization-provider/sync/runs${qs ? `?${qs}` : ''}`,
    signal ? { signal } : undefined
  );
}
