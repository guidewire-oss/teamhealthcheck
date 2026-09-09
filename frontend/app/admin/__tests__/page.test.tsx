import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import AdminPage from '../page';

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
      pagination: paginationFor(2, 25, 60),
    });

    const user = await openUsersTab();
    await waitFor(() => expect(listUsers).toHaveBeenCalled());

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
      pagination: paginationFor(1, 25, 1),
    });

    const user = await openUsersTab();
    await waitFor(() => expect(listUsers).toHaveBeenCalled());

    listUsers.mockClear();
    listUsers.mockResolvedValue({
      users: [makeUser('bob')],
      pagination: paginationFor(1, 25, 1),
    });

    const roleFilter = await screen.findByTestId('user-role-filter');
    await user.selectOptions(roleFilter, 'level-1');

    await waitFor(() => {
      expect(listUsers).toHaveBeenCalledWith(
        expect.objectContaining({ page: 1, role: 'level-1' })
      );
    });
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
