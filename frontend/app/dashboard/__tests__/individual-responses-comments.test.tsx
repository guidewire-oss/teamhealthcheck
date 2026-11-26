/**
 * Individual Responses tab: comments are shown only in the Cards view.
 * The Matrix view must not render comment indicators or comment text (including tooltips).
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import DashboardPage from '../page';

vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: vi.fn() }),
}));

const COMMENT = 'Deploys are painful every release';

const authenticatedFetch = vi.fn();
// Stable references: the page re-fetches when `user` changes identity, so fresh objects
// per call would cause an endless fetch/render loop once the fetch returns data.
const USER = {
  id: 'lead-1',
  name: 'Taylor Lead',
  hierarchyLevelId: 'level-4',
  teamIds: ['team-1'],
};
const ORG_CONFIG = { companyName: 'Acme' };
const LEVEL = { id: 'level-4', name: 'Team Lead' };
const PERMISSIONS = {};

vi.mock('@/lib/auth', () => ({
  getCurrentUser: () => USER,
  logout: vi.fn(),
  authenticatedFetch: (...args: unknown[]) => authenticatedFetch(...args),
}));

vi.mock('@/lib/org-config', () => ({
  getOrgConfig: () => ORG_CONFIG,
  getHierarchyLevel: () => LEVEL,
  getUserPermissions: () => PERMISSIONS,
}));

vi.mock('@/lib/api/teams', () => ({
  getTeamInfoCached: vi.fn().mockResolvedValue({
    id: 'team-1',
    name: 'Falcons',
    cadence: 'half-yearly',
    members: [],
  }),
}));

vi.mock('@/lib/api/health-checks', () => ({
  getTeamSubmissionStatus: vi.fn().mockResolvedValue({
    teamId: 'team-1',
    assessmentPeriod: '2026 - 1st Half',
    totalMembers: 1,
    submittedMembers: 1,
    allSubmitted: true,
    postWorkshopExists: false,
  }),
  getAssessmentPeriods: vi.fn().mockResolvedValue([]),
  checkSurveyEligibility: vi.fn().mockResolvedValue({ eligible: true }),
}));

vi.mock('@/components/OnboardingModal', () => ({ default: () => null }));
vi.mock('@/components/ActionItemsTab', () => ({ default: () => null }));

const individualResponses = {
  responses: [
    {
      sessionId: 's1',
      userId: 'u1',
      userName: 'Alice',
      date: '2026-03-01',
      surveyType: 'individual',
      dimensions: [{ dimensionId: 'mission', score: 3, trend: 'improving', comment: COMMENT }],
    },
  ],
};

describe('Team Lead dashboard: Individual Responses comments', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    authenticatedFetch.mockImplementation(async (url: string) => {
      if (url.includes('/dashboard/individual-responses')) {
        return { ok: true, status: 200, json: async () => individualResponses };
      }
      return { ok: false, status: 404, json: async () => ({}) };
    });
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => ({}) }));
  });

  async function openResponsesTab() {
    render(<DashboardPage />);
    await userEvent.click(await screen.findByTestId('responses-tab'));
    await screen.findByTestId('matrix-view-btn');
  }

  it('Matrix view shows no comment indicator, text, or tooltip comment', async () => {
    await openResponsesTab();
    await userEvent.click(screen.getByTestId('matrix-view-btn'));

    const score = await screen.findByTestId('matrix-score-s1-mission');
    expect(screen.queryByTestId('matrix-comment-s1-mission')).toBeNull();
    expect(screen.queryByText(COMMENT)).toBeNull();

    fireEvent.mouseEnter(score.closest('td') as HTMLElement);
    expect(screen.queryByText(COMMENT)).toBeNull();
    expect(screen.queryByTestId('comment')).toBeNull();
  });

  it('Matrix view still shows score and trend', async () => {
    await openResponsesTab();
    await userEvent.click(screen.getByTestId('matrix-view-btn'));

    expect(await screen.findByTestId('matrix-score-s1-mission')).toBeTruthy();
    expect(screen.getByTestId('matrix-trend-s1-mission')).toBeTruthy();
  });

  it('Cards view still shows the comment', async () => {
    await openResponsesTab();
    await userEvent.click(screen.getByTestId('cards-view-btn'));

    await waitFor(() => expect(screen.getByText(COMMENT)).toBeTruthy());
    expect(screen.getByTestId('comment').textContent).toBe(COMMENT);
  });
});
