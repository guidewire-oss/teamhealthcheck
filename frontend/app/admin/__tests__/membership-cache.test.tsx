import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import AdminPage from '../page';

// A stand-in for the real modal that exposes its two callbacks as buttons, so
// the test drives exactly the contract AdminPage relies on: a membership
// change is announced when it is saved, and closing is a separate event.
vi.mock('@/components/TeamMembersModal', () => ({
  default: ({ onClose, onMembershipChange }: { onClose: () => void; onMembershipChange?: () => void }) => (
    <div data-testid="members-modal-stub">
      <button data-testid="stub-save-membership" onClick={() => onMembershipChange?.()}>save</button>
      <button data-testid="stub-close" onClick={onClose}>close</button>
    </div>
  ),
}));
vi.mock('@/components/HierarchyConfig', () => ({ default: () => null }));
vi.mock('@/components/DimensionConfig', () => ({ default: () => null }));
vi.mock('@/components/SupervisorChainModal', () => ({ default: () => null }));
vi.mock('@/components/DocsLink', () => ({ default: () => null }));
// The router object must be referentially stable: AdminPage has an effect keyed
// on it, so a fresh object per render would re-run that effect forever.
const mockRouter = { push: vi.fn() };
vi.mock('next/navigation', () => ({ useRouter: () => mockRouter }));
const mockAdmin = { id: 'admin', username: 'admin', name: 'Admin User', isAdmin: true };
vi.mock('@/lib/auth', () => ({ getCurrentUser: () => mockAdmin, logout: vi.fn() }));

const listUsers = vi.fn();
const listUsersLite = vi.fn();
const listHierarchyLevels = vi.fn();
const listAdminTeams = vi.fn();

vi.mock('@/lib/api/admin', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/admin')>('@/lib/api/admin');
  return {
    ...actual,
    listUsers: (...a: unknown[]) => listUsers(...a),
    listUsersLite: (...a: unknown[]) => listUsersLite(...a),
    listHierarchyLevels: (...a: unknown[]) => listHierarchyLevels(...a),
    listAdminTeams: (...a: unknown[]) => listAdminTeams(...a),
    clearAdminCache: vi.fn(),
    getBrandingSettings: vi.fn().mockResolvedValue({ companyName: 'Acme', logoURL: '' }),
  };
});

const POD = { id: 'pod-a', name: 'Pod A', teamLeadId: null, teamLeadName: null, memberCount: 0, cadence: 'monthly' };

function userWithTeams(teamIds: string[]) {
  return {
    id: 'alice',
    username: 'alice',
    email: 'alice@test.com',
    fullName: 'User alice',
    hierarchyLevel: 'level-5',
    reportsTo: null,
    teamIds,
    authType: 'local' as const,
    createdAt: '2024-01-01T00:00:00Z',
    updatedAt: '2024-01-01T00:00:00Z',
  };
}

const page = (users: ReturnType<typeof userWithTeams>[]) => ({
  users,
  pagination: { page: 1, pageSize: 25, totalItems: users.length, totalPages: 1, hasNextPage: false, hasPreviousPage: false },
});

beforeEach(() => {
  vi.clearAllMocks();
  listHierarchyLevels.mockResolvedValue([]);
  listAdminTeams.mockResolvedValue({ teams: [POD] });
  listUsersLite.mockResolvedValue({ users: [] });
});

describe('Admin Users view after a team-membership change', () => {
  it('refetches the users page instead of showing the cached, stale team membership', async () => {
    // First visit: alice is on no team. After the modal adds her, the server says Pod A.
    listUsers.mockResolvedValueOnce(page([userWithTeams([])])).mockResolvedValue(page([userWithTeams(['pod-a'])]));
    const user = userEvent.setup();
    render(<AdminPage />);

    await user.click(await screen.findByTestId('users-tab'));
    await screen.findByText('User alice');
    expect(listUsers).toHaveBeenCalledTimes(1);

    // Leave for Teams, change membership via the modal, come back to Users.
    await user.click(screen.getByTestId('teams-tab'));
    await user.click(await screen.findByTestId('manage-members-btn'));
    await user.click(screen.getByTestId('stub-save-membership'));
    await user.click(screen.getByTestId('stub-close'));
    await user.click(screen.getByTestId('users-tab'));

    await waitFor(() => expect(listUsers).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(screen.getAllByTestId('user-row')[0]).toHaveTextContent('Pod A'));
  });

  it('keeps using the cached page when membership did not change (modal opened and closed)', async () => {
    listUsers.mockResolvedValue(page([userWithTeams([])]));
    const user = userEvent.setup();
    render(<AdminPage />);

    await user.click(await screen.findByTestId('users-tab'));
    await screen.findByText('User alice');
    await user.click(screen.getByTestId('teams-tab'));
    await user.click(await screen.findByTestId('manage-members-btn'));
    await user.click(screen.getByTestId('stub-close'));
    await user.click(screen.getByTestId('users-tab'));

    await screen.findByText('User alice');
    expect(listUsers).toHaveBeenCalledTimes(1); // no unnecessary refetch
  });
});
