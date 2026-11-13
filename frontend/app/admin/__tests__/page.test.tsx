import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import AdminPage from '../page';
import { deleteUser, deleteTeam } from '@/lib/api/admin';

// Tabs other than "users" render heavy child components with their own data
// fetching; they're irrelevant to this suite so we stub them out.
vi.mock('@/components/HierarchyConfig', () => ({ default: () => null }));
vi.mock('@/components/DimensionConfig', () => ({ default: () => null }));
vi.mock('@/components/SupervisorChainModal', () => ({ default: () => null }));
vi.mock('@/components/TeamMembersModal', () => ({ default: () => null }));
vi.mock('@/components/DocsLink', () => ({ default: () => null }));

const mockRouter = { push: vi.fn() };
vi.mock('next/navigation', () => ({
  useRouter: () => mockRouter,
}));

vi.mock('@/lib/auth', () => ({
  getCurrentUser: () => ({
    id: 'admin',
    username: 'admin',
    name: 'Admin User',
    isAdmin: true,
  }),
  logout: vi.fn(),
}));

const listUsers = vi.fn();
const listUsersLite = vi.fn();
const listHierarchyLevels = vi.fn();
const listAdminTeams = vi.fn();

vi.mock('@/lib/api/admin', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/admin')>('@/lib/api/admin');
  return {
    ...actual,
    listUsers: (...args: any[]) => listUsers(...args),
    listUsersLite: (...args: any[]) => listUsersLite(...args),
    listHierarchyLevels: (...args: any[]) => listHierarchyLevels(...args),
    listAdminTeams: (...args: any[]) => listAdminTeams(...args),
    createUser: vi.fn(),
    updateUser: vi.fn(),
    deleteUser: vi.fn(),
    deleteTeam: vi.fn(),
    clearAdminCache: vi.fn(),
    getBrandingSettings: vi.fn().mockResolvedValue({ companyName: 'Acme', logoURL: '' }),
    updateBrandingSettings: vi.fn(),
    getNotificationSettings: vi.fn().mockResolvedValue({
      emailEnabled: false,
      slackEnabled: false,
      notifyOnSubmission: false,
      notifyManagers: false,
      reminderDaysBefore: 0,
      reminderRecipients: [],
      smtpConfigured: false,
    }),
    updateNotificationSettings: vi.fn(),
    getRetentionPolicy: vi.fn().mockResolvedValue({
      keepSessionsMonths: 12,
      archiveEnabled: false,
      anonymizeAfterDays: 0,
    }),
    updateRetentionPolicy: vi.fn(),
  };
});

function makeUser(id: string) {
  return {
    id,
    username: id,
    email: `${id}@test.com`,
    fullName: `User ${id}`,
    hierarchyLevel: 'level-5',
    reportsTo: null,
    teamIds: [],
    authType: 'local' as const,
    createdAt: '2024-01-01T00:00:00Z',
    updatedAt: '2024-01-01T00:00:00Z',
    isActive: true,
  };
}

function paginationFor(page: number, pageSize: number, totalItems: number) {
  const totalPages = Math.max(1, Math.ceil(totalItems / pageSize));
  return {
    page,
    pageSize,
    totalItems,
    totalPages,
    hasNextPage: page < totalPages,
    hasPreviousPage: page > 1,
  };
}

async function openUsersTab() {
  render(<AdminPage />);
  const user = userEvent.setup();
  await user.click(await screen.findByTestId('users-tab'));
  return user;
}

beforeEach(() => {
  vi.clearAllMocks();
  listHierarchyLevels.mockResolvedValue([]);
  listAdminTeams.mockResolvedValue({ teams: [] });
  listUsersLite.mockResolvedValue({ users: [] });
});

describe('Admin Users tab — pagination', () => {
  it('requests the first page with default page size on initial load', async () => {
    listUsers.mockResolvedValue({
      users: [makeUser('alice')],
      pagination: paginationFor(1, 25, 1),
    });

    await openUsersTab();

    await waitFor(() => {
      expect(listUsers).toHaveBeenCalledWith(
        expect.objectContaining({ page: 1, pageSize: 25 })
      );
    });
  });

  it('renders only the users returned for the current page', async () => {
    listUsers.mockResolvedValue({
      users: [makeUser('alice'), makeUser('bob')],
      pagination: paginationFor(1, 25, 2),
    });

    await openUsersTab();

    expect(await screen.findByText('User alice')).toBeInTheDocument();
    expect(screen.getByText('User bob')).toBeInTheDocument();
    expect(screen.getAllByTestId('user-row')).toHaveLength(2);
  });

  it('shows an empty state when there are no users', async () => {
    listUsers.mockResolvedValue({ users: [], pagination: paginationFor(1, 25, 0) });

    await openUsersTab();

    expect(await screen.findByTestId('users-empty-state')).toHaveTextContent(
      'No users found'
    );
  });

  it('shows an error state when the request fails', async () => {
    listUsers.mockRejectedValue(new Error('Network error'));

    await openUsersTab();

    expect(await screen.findByText('Network error')).toBeInTheDocument();
  });

  it('disables Previous on the first page and enables Next when more pages exist', async () => {
    listUsers.mockResolvedValue({
      users: [makeUser('alice')],
      pagination: paginationFor(1, 25, 50),
    });

    await openUsersTab();

    const prevBtn = await screen.findByTestId('users-prev-page-btn');
    const nextBtn = screen.getByTestId('users-next-page-btn');
    expect(prevBtn).toBeDisabled();
    expect(nextBtn).not.toBeDisabled();
  });

  it('disables Next on the last page and enables Previous', async () => {
    listUsers.mockResolvedValue({
      users: [makeUser('alice')],
      pagination: paginationFor(2, 25, 30),
    });

    await openUsersTab();

    const prevBtn = await screen.findByTestId('users-prev-page-btn');
    const nextBtn = screen.getByTestId('users-next-page-btn');
    expect(prevBtn).not.toBeDisabled();
    expect(nextBtn).toBeDisabled();
  });

  it('requests the next page when Next is clicked', async () => {
    listUsers.mockResolvedValue({
      users: [makeUser('alice')],
      pagination: paginationFor(1, 25, 60),
    });

    const user = await openUsersTab();
    await waitFor(() => expect(listUsers).toHaveBeenCalled());

    listUsers.mockResolvedValue({
      users: [makeUser('bob')],
      pagination: paginationFor(2, 25, 60),
    });
    await user.click(screen.getByTestId('users-next-page-btn'));

    await waitFor(() => {
      expect(listUsers).toHaveBeenCalledWith(
        expect.objectContaining({ page: 2 })
      );
    });
    expect(await screen.findByText('User bob')).toBeInTheDocument();
  });

  it('resets to page 1 when the search query changes', async () => {
    listUsers.mockResolvedValue({
      users: [makeUser('alice')],
      pagination: paginationFor(1, 25, 60),
    });

    const user = await openUsersTab();
    await waitFor(() => expect(listUsers).toHaveBeenCalled());

    // Actually drive the component to page 2 via the Next button, rather
    // than asserting against mocked pagination metadata the component never
    // produced itself.
    listUsers.mockResolvedValue({
      users: [makeUser('bob')],
      pagination: paginationFor(2, 25, 60),
    });
    await user.click(screen.getByTestId('users-next-page-btn'));
    await waitFor(() => {
      expect(listUsers).toHaveBeenCalledWith(expect.objectContaining({ page: 2 }));
    });
    expect(await screen.findByText('User bob')).toBeInTheDocument();

    listUsers.mockClear();
    listUsers.mockResolvedValue({
      users: [makeUser('carl')],
      pagination: paginationFor(1, 25, 1),
    });

    const searchInput = screen.getByTestId('user-search-input');
    await user.type(searchInput, 'carl');

    await waitFor(
      () => {
        expect(listUsers).toHaveBeenCalledWith(
          expect.objectContaining({ page: 1, search: 'carl' })
        );
      },
      { timeout: 2000 }
    );
    expect(await screen.findByText('User carl')).toBeInTheDocument();
  });

  it('resets to page 1 when the role filter changes', async () => {
    listHierarchyLevels.mockResolvedValue([
      {
        id: 'level-1',
        name: 'VP',
        position: 1,
        permissions: {
          canViewAllTeams: true,
          canEditTeams: true,
          canManageUsers: true,
          canTakeSurvey: false,
          canViewAnalytics: true,
        },
        createdAt: '2024-01-01T00:00:00Z',
        updatedAt: '2024-01-01T00:00:00Z',
      },
    ]);
    listUsers.mockResolvedValue({
      users: [makeUser('alice')],
      pagination: paginationFor(1, 25, 60),
    });

    const user = await openUsersTab();
    await waitFor(() => expect(listUsers).toHaveBeenCalled());

    // Actually drive the component to page 2 via the Next button, rather
    // than asserting against mocked pagination metadata the component never
    // produced itself.
    listUsers.mockResolvedValue({
      users: [makeUser('bob')],
      pagination: paginationFor(2, 25, 60),
    });
    await user.click(screen.getByTestId('users-next-page-btn'));
    await waitFor(() => {
      expect(listUsers).toHaveBeenCalledWith(expect.objectContaining({ page: 2 }));
    });
    expect(await screen.findByText('User bob')).toBeInTheDocument();

    listUsers.mockClear();
    listUsers.mockResolvedValue({
      users: [makeUser('dana')],
      pagination: paginationFor(1, 25, 1),
    });

    const roleFilter = await screen.findByTestId('user-role-filter');
    await user.selectOptions(roleFilter, 'level-1');

    await waitFor(() => {
      expect(listUsers).toHaveBeenCalledWith(
        expect.objectContaining({ page: 1, role: 'level-1' })
      );
    });
    expect(await screen.findByText('User dana')).toBeInTheDocument();
  });

  it('ignores a stale response that resolves after a newer request supersedes it', async () => {
    listHierarchyLevels.mockResolvedValue([
      {
        id: 'level-1',
        name: 'VP',
        position: 1,
        permissions: {
          canViewAllTeams: true,
          canEditTeams: true,
          canManageUsers: true,
          canTakeSurvey: false,
          canViewAnalytics: true,
        },
        createdAt: '2024-01-01T00:00:00Z',
        updatedAt: '2024-01-01T00:00:00Z',
      },
    ]);
    listUsers.mockResolvedValue({
      users: [makeUser('alice')],
      pagination: paginationFor(1, 25, 60),
    });

    const user = await openUsersTab();
    const nextBtn = await screen.findByTestId('users-next-page-btn');

    // Clicking Next kicks off a page-2 request that we deliberately leave
    // pending, simulating a slow response.
    let resolveStale!: (value: any) => void;
    listUsers.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveStale = resolve;
        })
    );
    await user.click(nextBtn);

    // Before the stale request resolves, the role filter changes — this
    // resets to page 1 and fires a newer request that resolves immediately.
    listUsers.mockResolvedValueOnce({
      users: [makeUser('newer')],
      pagination: paginationFor(1, 25, 1),
    });
    const roleFilter = screen.getByTestId('user-role-filter');
    await user.selectOptions(roleFilter, 'level-1');

    expect(await screen.findByText('User newer')).toBeInTheDocument();

    // Now the stale page-2 response finally resolves — it must NOT overwrite
    // the newer result that's already rendered.
    resolveStale({
      users: [makeUser('stale')],
      pagination: paginationFor(2, 25, 60),
    });

    await new Promise((r) => setTimeout(r, 50));
    expect(screen.queryByText('User stale')).not.toBeInTheDocument();
    expect(screen.getByText('User newer')).toBeInTheDocument();
  });
});


describe('Admin Users tab — Active / Inactive status', () => {
  const active = () => ({ ...makeUser('alice'), isActive: true });
  const inactive = () => ({ ...makeUser('bob'), isActive: false, deactivatedAt: '2024-06-01T00:00:00Z' });
  const rowFor = (name: string) =>
    screen.getAllByTestId('user-row').find((r) => r.textContent?.includes(name))!;

  beforeEach(() => {
    listUsers.mockResolvedValue({ users: [active(), inactive()], pagination: paginationFor(1, 25, 2) });
  });

  it('shows Active for active people and Inactive for deactivated people', async () => {
    await openUsersTab();
    await screen.findAllByTestId('user-row');
    expect(within(rowFor('User alice')).getByTestId('user-status-badge')).toHaveTextContent('Active');
    expect(within(rowFor('User bob')).getByTestId('user-status-badge')).toHaveTextContent('Inactive');
  });

  it('puts the status immediately before the name', async () => {
    await openUsersTab();
    await screen.findAllByTestId('user-row');
    const badge = within(rowFor('User bob')).getByTestId('user-status-badge');
    expect(badge.nextSibling?.textContent).toBe('User bob');
  });

  it('keeps inactive people listed alongside active ones by default (nothing is filtered out)', async () => {
    await openUsersTab();
    await screen.findAllByTestId('user-row');
    expect(screen.getAllByTestId('user-row')).toHaveLength(2);
    expect(listUsers).toHaveBeenCalledWith(expect.objectContaining({ status: undefined }));
  });

  it('keeps inactive people searchable', async () => {
    const user = await openUsersTab();
    await screen.findAllByTestId('user-row');
    listUsers.mockResolvedValue({ users: [inactive()], pagination: paginationFor(1, 25, 1) });
    await user.type(screen.getByTestId('user-search-input'), 'bob');
    await waitFor(() =>
      expect(listUsers).toHaveBeenCalledWith(expect.objectContaining({ search: 'bob', status: undefined }))
    );
    expect(await screen.findByText('User bob')).toBeInTheDocument();
    expect(within(rowFor('User bob')).getByTestId('user-status-badge')).toHaveTextContent('Inactive');
  });

  it('offers an Active/Inactive filter that is sent to the server', async () => {
    const user = await openUsersTab();
    await screen.findAllByTestId('user-row');
    const filter = screen.getByTestId('user-status-filter');
    expect(within(filter).getAllByRole('option').map((o) => o.textContent)).toEqual(['All Statuses', 'Active', 'Inactive']);

    listUsers.mockResolvedValue({ users: [inactive()], pagination: paginationFor(1, 25, 1) });
    await user.selectOptions(filter, 'inactive');
    await waitFor(() => expect(listUsers).toHaveBeenCalledWith(expect.objectContaining({ status: 'inactive' })));
    expect(screen.getAllByTestId('user-row')).toHaveLength(1);

    listUsers.mockResolvedValue({ users: [active()], pagination: paginationFor(1, 25, 1) });
    await user.selectOptions(filter, 'active');
    await waitFor(() => expect(listUsers).toHaveBeenCalledWith(expect.objectContaining({ status: 'active' })));

    // Back to everyone: the already-loaded "all" page is reused from the page
    // cache, so what matters is that inactive people are listed again.
    await user.selectOptions(filter, '');
    await waitFor(() => expect(screen.getAllByTestId('user-row')).toHaveLength(2));
    expect(within(rowFor('User bob')).getByTestId('user-status-badge')).toHaveTextContent('Inactive');
  });

  it('says so when the status filter matches nobody', async () => {
    const user = await openUsersTab();
    await screen.findAllByTestId('user-row');
    listUsers.mockResolvedValue({ users: [], pagination: paginationFor(1, 25, 0) });
    await user.selectOptions(screen.getByTestId('user-status-filter'), 'inactive');
    expect(await screen.findByTestId('users-empty-state')).toHaveTextContent('No users match');
  });

  it('only offers to deactivate people who are still active', async () => {
    await openUsersTab();
    await screen.findAllByTestId('user-row');
    expect(within(rowFor('User alice')).getByTestId('deactivate-user-btn')).toBeEnabled();
    expect(within(rowFor('User bob')).getByTestId('deactivate-user-btn')).toBeDisabled();
  });

  it('deactivates instead of deleting: confirms with soft-deactivation wording', async () => {
    const user = await openUsersTab();
    await screen.findAllByTestId('user-row');
    await user.click(within(rowFor('User alice')).getByTestId('deactivate-user-btn'));

    expect(screen.getByText('Confirm Deactivate')).toBeInTheDocument();
    expect(screen.getByText(/remain visible as Inactive/)).toBeInTheDocument();
    expect(screen.queryByText(/cannot be undone/)).toBeNull();
    expect(screen.queryByText('Delete User')).toBeNull();

    (deleteUser as any).mockResolvedValue(undefined);
    listUsers.mockResolvedValue({
      users: [{ ...active(), isActive: false }, inactive()],
      pagination: paginationFor(1, 25, 2),
    });
    await user.click(screen.getByTestId('confirm-deactivate'));

    await waitFor(() => expect(deleteUser).toHaveBeenCalledWith('alice'));
    await waitFor(() =>
      expect(within(rowFor('User alice')).getByTestId('user-status-badge')).toHaveTextContent('Inactive')
    );
    expect(screen.getAllByTestId('user-row')).toHaveLength(2); // still listed, not removed
  });
});


describe('Admin Teams tab — Active / Inactive status', () => {
  const makeTeam = (id: string, name: string, isActive: boolean, extra: Record<string, any> = {}) => ({
    id,
    name,
    teamLeadId: null,
    teamLeadName: null,
    cadence: 'monthly',
    memberCount: isActive ? 3 : 0,
    createdAt: '2024-01-01T00:00:00Z',
    updatedAt: '2024-01-01T00:00:00Z',
    isActive,
    ...extra,
  });
  const active = () => makeTeam('t-hope', 'Hopefarm', true);
  const inactive = () => makeTeam('t-old', 'Old Squad', false, { deactivatedAt: '2024-06-01T00:00:00Z' });
  const rowFor = (name: string) =>
    screen.getAllByTestId('team-row').find((r) => r.textContent?.includes(name))!;

  async function openTeamsTab() {
    render(<AdminPage />);
    const user = userEvent.setup();
    await user.click(await screen.findByTestId('teams-tab'));
    await screen.findAllByTestId('team-row');
    return user;
  }

  beforeEach(() => {
    listAdminTeams.mockResolvedValue({ teams: [active(), inactive()], total: 2 });
    listUsers.mockResolvedValue({ users: [], pagination: paginationFor(1, 25, 0) });
  });

  it('shows Active for active teams and Inactive for deactivated teams', async () => {
    await openTeamsTab();
    expect(within(rowFor('Hopefarm')).getByTestId('team-status-badge')).toHaveTextContent('Active');
    expect(within(rowFor('Old Squad')).getByTestId('team-status-badge')).toHaveTextContent('Inactive');
  });

  it('puts the status immediately before the team name', async () => {
    await openTeamsTab();
    const badge = within(rowFor('Old Squad')).getByTestId('team-status-badge');
    expect(badge.nextSibling?.textContent).toBe('Old Squad');
  });

  it('keeps inactive teams listed by default', async () => {
    await openTeamsTab();
    expect(screen.getAllByTestId('team-row')).toHaveLength(2);
  });

  it('keeps inactive teams searchable', async () => {
    const user = await openTeamsTab();
    await user.type(screen.getByTestId('team-search-input'), 'old');
    expect(screen.getAllByTestId('team-row')).toHaveLength(1);
    expect(within(rowFor('Old Squad')).getByTestId('team-status-badge')).toHaveTextContent('Inactive');
  });

  it('offers an Active/Inactive filter', async () => {
    const user = await openTeamsTab();
    const filter = screen.getByTestId('team-status-filter');
    expect(within(filter).getAllByRole('option').map((o) => o.textContent)).toEqual(['All Statuses', 'Active', 'Inactive']);

    await user.selectOptions(filter, 'inactive');
    expect(screen.getAllByTestId('team-row')).toHaveLength(1);
    expect(rowFor('Old Squad')).toBeTruthy();

    await user.selectOptions(filter, 'active');
    expect(screen.getAllByTestId('team-row')).toHaveLength(1);
    expect(rowFor('Hopefarm')).toBeTruthy();

    await user.selectOptions(filter, '');
    expect(screen.getAllByTestId('team-row')).toHaveLength(2);
  });

  it('combines the status filter with search, and says so when nothing matches', async () => {
    const user = await openTeamsTab();
    await user.selectOptions(screen.getByTestId('team-status-filter'), 'inactive');
    await user.type(screen.getByTestId('team-search-input'), 'hope'); // Hopefarm is active, so it is filtered out
    expect(screen.queryAllByTestId('team-row')).toHaveLength(0);
    expect(screen.getByTestId('teams-empty-filtered')).toHaveTextContent('No teams match');
  });

  it('only offers to deactivate teams that are still active', async () => {
    await openTeamsTab();
    expect(within(rowFor('Hopefarm')).getByTestId('deactivate-team-btn')).toBeEnabled();
    expect(within(rowFor('Old Squad')).getByTestId('deactivate-team-btn')).toBeDisabled();
  });

  it('deactivates instead of deleting, and the team stays listed as Inactive', async () => {
    const user = await openTeamsTab();
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
    (deleteTeam as any).mockResolvedValue(undefined);
    listAdminTeams.mockResolvedValue({
      teams: [makeTeam('t-hope', 'Hopefarm', false, { deactivatedAt: '2024-07-01T00:00:00Z' }), inactive()],
      total: 2,
    });

    await user.click(within(rowFor('Hopefarm')).getByTestId('deactivate-team-btn'));

    expect(confirmSpy.mock.calls[0][0]).toMatch(/deactivate this team/);
    expect(confirmSpy.mock.calls[0][0]).not.toMatch(/cannot be undone/);
    await waitFor(() => expect(deleteTeam).toHaveBeenCalledWith('t-hope'));
    await waitFor(() =>
      expect(within(rowFor('Hopefarm')).getByTestId('team-status-badge')).toHaveTextContent('Inactive')
    );
    expect(screen.getAllByTestId('team-row')).toHaveLength(2); // still listed, not removed
    confirmSpy.mockRestore();
  });

  it('does not deactivate when the admin cancels the confirmation', async () => {
    const user = await openTeamsTab();
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false);
    await user.click(within(rowFor('Hopefarm')).getByTestId('deactivate-team-btn'));
    expect(deleteTeam).not.toHaveBeenCalled();
    confirmSpy.mockRestore();
  });

  it('treats a team without the status flag (older API) as active', async () => {
    listAdminTeams.mockResolvedValue({ teams: [{ ...active(), isActive: undefined }], total: 1 });
    await openTeamsTab();
    expect(within(rowFor('Hopefarm')).getByTestId('team-status-badge')).toHaveTextContent('Active');
  });
});

describe('AdminPage Sync History tab', () => {
  it('is a top-level tab after Settings and opens Sync History outside Settings', async () => {
    listUsers.mockResolvedValue({ users: [], pagination: paginationFor(1, 25, 0) });
    render(<AdminPage />);
    const user = userEvent.setup();
    const tab = await screen.findByTestId('sync-history-tab');
    expect(tab).toHaveTextContent('Sync History');
    expect(screen.getByTestId('settings-tab').compareDocumentPosition(tab) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(screen.queryByTestId('sync-history')).not.toBeInTheDocument();

    await user.click(tab);
    expect(await screen.findByTestId('sync-history')).toBeInTheDocument();
    expect(screen.queryByTestId('data-provider-settings')).not.toBeInTheDocument();

    await user.click(screen.getByTestId('settings-tab'));
    await screen.findByTestId('data-provider-settings');
    expect(screen.queryByTestId('sync-history')).not.toBeInTheDocument();
  });
});
