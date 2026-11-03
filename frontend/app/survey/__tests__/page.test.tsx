import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import SurveyPage from '../page';

const mockRouter = { push: vi.fn() };
vi.mock('next/navigation', () => ({
  useRouter: () => mockRouter,
  useSearchParams: () => new URLSearchParams(),
}));

vi.mock('@/lib/auth', () => ({
  getCurrentUser: () => ({
    id: 'user-1',
    username: 'user1',
    name: 'User One',
    hierarchyLevelId: 'level-5',
    teamIds: ['team-1'],
  }),
  logout: vi.fn(),
}));

const getTeamInfoCached = vi.fn();
vi.mock('@/lib/api/teams', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/teams')>('@/lib/api/teams');
  return {
    ...actual,
    getTeamInfoCached: (...args: any[]) => getTeamInfoCached(...args),
  };
});

const getHealthDimensions = vi.fn();
vi.mock('@/lib/api/health-checks', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/health-checks')>('@/lib/api/health-checks');
  return {
    ...actual,
    getHealthDimensions: (...args: any[]) => getHealthDimensions(...args),
  };
});

function getDraftKey(userId: string, teamId: string) {
  return `surveyDraft:${userId}:${teamId}`;
}

const TEAM_INFO = {
  id: 'team-1',
  name: 'Team One',
  cadence: 'quarterly',
  members: [],
};

// Only two of the three dimensions the draft references are still active.
const ACTIVE_DIMENSIONS = [
  { id: 'dim-1', name: 'Mission', description: 'd', goodDescription: 'good', badDescription: 'bad' },
  { id: 'dim-2', name: 'Fun', description: 'd', goodDescription: 'good', badDescription: 'bad' },
];

describe('SurveyPage draft restoration', () => {
  beforeEach(() => {
    localStorage.clear();
    getTeamInfoCached.mockResolvedValue(TEAM_INFO);
    getHealthDimensions.mockResolvedValue(ACTIVE_DIMENSIONS);
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('clamps an out-of-range currentDimension and drops responses for deactivated dimensions', async () => {
    const currentPeriod = (await import('@/lib/assessment-period')).getAssessmentPeriod(
      new Date(),
      (await import('@/lib/assessment-period')).toCadence(TEAM_INFO.cadence)
    );

    const draft = {
      responses: [
        { dimensionId: 'dim-1', score: 3, trend: 'stable', comment: '' },
        { dimensionId: 'dim-removed', score: 2, trend: 'stable', comment: 'stale' },
      ],
      // Points past the end of the now-shorter active dimension list.
      currentDimension: 5,
      assessmentPeriod: currentPeriod,
      savedAt: new Date().toISOString(),
    };
    localStorage.setItem(getDraftKey('user-1', 'team-1'), JSON.stringify(draft));

    render(<SurveyPage />);

    // Renders the last valid dimension instead of crashing on an undefined one.
    await waitFor(() => {
      expect(screen.getByText('Fun')).toBeInTheDocument();
    });

    // The stale response for the deactivated dimension must not resurrect it.
    expect(screen.queryByText('dim-removed')).not.toBeInTheDocument();
  });

  it('handles an empty active-dimension list without rendering an undefined dimension', async () => {
    getHealthDimensions.mockResolvedValue([]);
    localStorage.setItem(
      getDraftKey('user-1', 'team-1'),
      JSON.stringify({
        responses: [{ dimensionId: 'dim-1', score: 1, trend: 'stable', comment: '' }],
        currentDimension: 2,
        assessmentPeriod: 'whatever',
        savedAt: new Date().toISOString(),
      })
    );

    render(<SurveyPage />);

    await waitFor(() => {
      expect(screen.getByText('Unable to Load Health Dimensions')).toBeInTheDocument();
    });
  });
});
