import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ManagerPage from '../page';
import { getCurrentUser, authenticatedFetch } from '@/lib/auth';
import { getAssessmentPeriods } from '@/lib/api/health-checks';
import { listManagerTeamsActionSummary } from '@/lib/api/action-items';

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn() }),
}));

vi.mock('@/lib/auth', () => ({
  getCurrentUser: vi.fn(),
  logout: vi.fn().mockResolvedValue(undefined),
  authenticatedFetch: vi.fn(),
}));

vi.mock('@/lib/api/health-checks', () => ({
  getAssessmentPeriods: vi.fn(),
}));

vi.mock('@/lib/api/action-items', () => ({
  listManagerTeamsActionSummary: vi.fn(),
  listActionItems: vi.fn(),
}));

const mockUser = {
  id: 'user-1',
  username: 'manager1',
  name: 'Test Manager',
  fullName: 'Test Manager',
  hierarchyLevel: 'level-3',
  hierarchyLevelId: 'level-3',
  teamIds: ['team-1'],
};

const dashboardResponse = {
  managerId: 'user-1',
  teams: [
    {
      teamId: 'team-1',
      teamName: 'Team Alpha',
      overallHealth: 2.5,
      submissionCount: 5,
      dimensions: [],
    },
  ],
  totalTeams: 1,
};

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.setItem('onboarding_complete:user-1', 'true');

  (getCurrentUser as unknown as ReturnType<typeof vi.fn>).mockReturnValue(mockUser);
  (getAssessmentPeriods as unknown as ReturnType<typeof vi.fn>).mockResolvedValue([]);
  (listManagerTeamsActionSummary as unknown as ReturnType<typeof vi.fn>).mockResolvedValue([]);

  (authenticatedFetch as unknown as ReturnType<typeof vi.fn>).mockImplementation((url: string) => {
    if (url.includes('/subordinates')) {
      return Promise.resolve({ ok: true, json: async () => ({ subordinates: [] }) });
    }
    if (url.includes('/teams/health')) {
      return Promise.resolve({ ok: true, json: async () => dashboardResponse });
    }
    return Promise.resolve({ ok: true, json: async () => ({}) });
  });

  global.fetch = vi.fn().mockResolvedValue({
    ok: true,
    json: async () => ({}),
  }) as unknown as typeof fetch;
});

describe('ManagerPage tab labels', () => {
  it('renders the renamed tabs and not the old labels', async () => {
    render(<ManagerPage />);

    expect(await screen.findByRole('button', { name: 'Post Workshop Survey Results' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Individual Survey Summary' })).toBeInTheDocument();

    expect(screen.queryByText('Team Cards')).not.toBeInTheDocument();
    expect(screen.queryByText('Hierarchy View')).not.toBeInTheDocument();
  });

  it('shows the organization hierarchy content when Individual Survey Summary is selected', async () => {
    const user = userEvent.setup();
    render(<ManagerPage />);

    const tab = await screen.findByRole('button', { name: 'Individual Survey Summary' });
    await user.click(tab);

    expect(await screen.findByText('Organization Hierarchy')).toBeInTheDocument();
    expect(screen.getByTestId('hierarchy-tree')).toBeInTheDocument();
  });

  it('shows team health data under Post Workshop Survey Results by default', async () => {
    render(<ManagerPage />);

    expect(await screen.findByText('Team Alpha')).toBeInTheDocument();
  });
});
