/**
 * Tests for the survey page's assessment-period behavior.
 *
 * Selection now happens on the Member Home page's "Select assessment period" panel and
 * arrives here via the `?period=` query param — the survey header is read-only. Covers:
 *  - Default (unchanged) path: auto-detected period is displayed and submitted when no
 *    period param is provided.
 *  - The survey header has no period selector/edit control (moved to Member Home) and
 *    displays the final selected period read-only.
 *  - A period passed in via the query param (e.g. a previous period like H1 2026) is used
 *    instead of today's auto-detected one.
 *  - Clicking "Submit Responses" opens a confirmation modal instead of submitting immediately.
 *  - The modal shows the default-path copy when the period matches auto-detection.
 *  - The modal shows the overridden-path copy, with dynamic period values, when it differs.
 *  - "Cancel" closes the modal without submitting or losing answers.
 *  - "Confirm and submit" submits the selected period (not a recomputed one).
 *  - Invalid/missing periods cannot be submitted.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import SurveyPage from '../page';

const push = vi.fn();
// Stable references: the real Next.js hooks return stable objects across renders.
// A fresh object/instance per call would retrigger any effect keyed on it (infinite loop).
const router = { push };
let searchParams = new URLSearchParams('');
let currentUser = {
  id: 'user-1',
  name: 'Taylor Lead',
  hierarchyLevelId: 'level-4',
  teamIds: ['team-1'],
};

vi.mock('next/navigation', () => ({
  useRouter: () => router,
  useSearchParams: () => searchParams,
}));

vi.mock('@/lib/auth', () => ({
  getCurrentUser: () => currentUser,
  logout: vi.fn(),
}));

vi.mock('@/lib/data', () => ({
  HEALTH_DIMENSIONS: [
    {
      id: 'mission',
      name: 'Mission',
      description: 'desc',
      goodDescription: 'good',
      badDescription: 'bad',
    },
    {
      id: 'value',
      name: 'Delivering Value',
      description: 'desc',
      goodDescription: 'good',
      badDescription: 'bad',
    },
  ],
}));

const getTeamInfoCached = vi.fn();
vi.mock('@/lib/api/teams', () => ({
  getTeamInfoCached: (...args: unknown[]) => getTeamInfoCached(...args),
}));

const submitHealthCheck = vi.fn();
vi.mock('@/lib/api/health-checks', () => ({
  submitHealthCheck: (...args: unknown[]) => submitHealthCheck(...args),
  formatDateForAPI: (d: Date) => d.toISOString(),
  HealthCheckAPIError: class HealthCheckAPIError extends Error {},
}));

const TEAM_INFO = {
  id: 'team-1',
  name: 'Falcons',
  cadence: 'half-yearly',
  members: [],
};

function getGreenScoreButton(): HTMLElement {
  const el = document.querySelector('[data-score="3"]');
  if (!el) throw new Error('score button not found');
  return el as HTMLElement;
}

async function answerAllDimensions(user: ReturnType<typeof userEvent.setup>) {
  await user.click(getGreenScoreButton());
  await user.click(screen.getByRole('button', { name: /next/i }));
  await user.click(getGreenScoreButton());
}

describe('Survey assessment period (read-only, sourced from Member Home)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    searchParams = new URLSearchParams('');
    currentUser = {
      id: 'user-1',
      name: 'Taylor Lead',
      hierarchyLevelId: 'level-4',
      teamIds: ['team-1'],
    };
    vi.setSystemTime(new Date(2026, 8, 22)); // Sep 22, 2026 -> auto period "2026 H2"
    getTeamInfoCached.mockResolvedValue(TEAM_INFO);
    submitHealthCheck.mockResolvedValue({ id: 'session-123' });
  });

  it('auto-selects and displays the current-date period on load when no period param is given', async () => {
    render(<SurveyPage />);

    await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H2 2026'));
  });

  it('does not render an editable period selector/edit control in the survey header (moved to Member Home)', async () => {
    render(<SurveyPage />);

    await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H2 2026'));

    expect(screen.queryByTestId('edit-period-button')).not.toBeInTheDocument();
    expect(screen.queryByTestId('period-selector')).not.toBeInTheDocument();
    expect(screen.queryByTestId('done-period-button')).not.toBeInTheDocument();
    // No override indicator when the period matches auto-detection
    expect(screen.queryByTestId('period-overridden-label')).not.toBeInTheDocument();
  });

  it('uses the period passed via the query param (e.g. a previous period like H1 2026) instead of auto-detection, and does not overwrite it on mount', async () => {
    searchParams = new URLSearchParams('period=2026 H1');
    render(<SurveyPage />);

    await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H1 2026'));
    // It must stay "2026 H1" and not be recalculated back to today's auto-detected "2026 H2"
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(screen.getByTestId('selected-period')).toHaveTextContent('H1 2026');
  });

  it('shows the override indicator with the auto-detected period when the selected period differs', async () => {
    searchParams = new URLSearchParams('period=2026 H1');
    render(<SurveyPage />);

    await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H1 2026'));
    expect(screen.getByTestId('period-overridden-label')).toHaveTextContent(
      'Overridden — auto-detected period: H2 2026'
    );
  });

  it('does not overwrite the period passed from Member Home with a stale draft saved under the previous default selection', async () => {
    // Simulate a draft saved earlier this same auto-detected period window (e.g. an earlier visit
    // where the user took the default period), which must NOT clobber a fresh override.
    localStorage.setItem(
      'surveyDraft:user-1:team-1',
      JSON.stringify({
        responses: [{ dimensionId: 'mission', score: 3, trend: 'stable', comment: '' }],
        currentDimension: 0,
        assessmentPeriod: '2026 H2',
        selectedAssessmentPeriod: '2026 H2',
        savedAt: new Date().toISOString(),
      })
    );
    searchParams = new URLSearchParams('period=2026 H1');

    render(<SurveyPage />);

    await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H1 2026'));
    // The stale draft (for a different period) must not have been restored either
    expect(screen.queryByTestId('draft-restored-banner')).not.toBeInTheDocument();
  });

  it('falls back to the auto-detected period when the query param is invalid', async () => {
    searchParams = new URLSearchParams('period=not-a-real-period');
    render(<SurveyPage />);

    await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H2 2026'));
  });

  it('shows the default confirmation modal when the period matches auto-detection', async () => {
    const user = userEvent.setup({ delay: null });
    render(<SurveyPage />);

    await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H2 2026'));

    await answerAllDimensions(user);
    await user.click(screen.getByRole('button', { name: /submit responses/i }));

    expect(submitHealthCheck).not.toHaveBeenCalled();
    expect(screen.getByTestId('submit-confirm-modal')).toBeInTheDocument();
    expect(screen.getByTestId('submit-confirm-title')).toHaveTextContent('Confirm health check submission');
    expect(screen.getByTestId('submit-confirm-message')).toHaveTextContent(
      'Your health check will be submitted for H2 2026. Please verify the assessment period before continuing.'
    );
  });

  it('shows the overridden confirmation modal, with dynamic period values, when a previous period was passed in', async () => {
    searchParams = new URLSearchParams('period=2026 H1');
    const user = userEvent.setup({ delay: null });
    render(<SurveyPage />);

    await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H1 2026'));

    await answerAllDimensions(user);
    await user.click(screen.getByRole('button', { name: /submit responses/i }));

    expect(screen.getByTestId('submit-confirm-title')).toHaveTextContent('Confirm assessment period');
    expect(screen.getByTestId('submit-confirm-message')).toHaveTextContent(
      'You selected H1 2026 instead of the automatically detected period H2 2026. ' +
        'After submission, this health check will be recorded for H1 2026. ' +
        'Please confirm that this is the correct assessment period.'
    );
  });

  it('cancels the modal without submitting or losing answers', async () => {
    searchParams = new URLSearchParams('period=2026 H1');
    const user = userEvent.setup({ delay: null });
    render(<SurveyPage />);

    await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H1 2026'));

    await answerAllDimensions(user);
    await user.click(screen.getByRole('button', { name: /submit responses/i }));
    await user.click(screen.getByTestId('submit-confirm-cancel'));

    expect(screen.queryByTestId('submit-confirm-modal')).not.toBeInTheDocument();
    expect(submitHealthCheck).not.toHaveBeenCalled();
    // Answers and the passed-in period are preserved
    expect(screen.getByTestId('selected-period')).toHaveTextContent('H1 2026');
    expect(getGreenScoreButton()).toHaveClass('border-green-500');
  });

  it('submits the auto-detected period when confirmed, unchanged from the default behavior', async () => {
    const user = userEvent.setup({ delay: null });
    render(<SurveyPage />);

    await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H2 2026'));

    await answerAllDimensions(user);
    await user.click(screen.getByRole('button', { name: /submit responses/i }));
    await user.click(screen.getByTestId('submit-confirm-accept'));

    await waitFor(() => expect(submitHealthCheck).toHaveBeenCalledTimes(1));
    expect(submitHealthCheck.mock.calls[0][0]).toMatchObject({
      teamId: 'team-1',
      userId: 'user-1',
      assessmentPeriod: '2026 H2',
    });
  });

  it('submits and persists the period passed from Member Home (e.g. H1 2026) once confirmed, not a recomputed one', async () => {
    searchParams = new URLSearchParams('period=2026 H1');
    const user = userEvent.setup({ delay: null });
    render(<SurveyPage />);

    await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H1 2026'));

    await answerAllDimensions(user);
    await user.click(screen.getByRole('button', { name: /submit responses/i }));
    await user.click(screen.getByTestId('submit-confirm-accept'));

    await waitFor(() => expect(submitHealthCheck).toHaveBeenCalledTimes(1));
    expect(submitHealthCheck.mock.calls[0][0]).toMatchObject({ assessmentPeriod: '2026 H1' });
  });

  it('lets a Team Lead override the period on the Post-Workshop Survey and submits it', async () => {
    searchParams = new URLSearchParams('type=post_workshop&period=2026 H1');
    const user = userEvent.setup({ delay: null });
    render(<SurveyPage />);

    await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H1 2026'));
    expect(screen.getByTestId('period-overridden-label')).toHaveTextContent(
      'Overridden — auto-detected period: H2 2026'
    );

    await answerAllDimensions(user);
    await user.click(screen.getByRole('button', { name: /submit responses/i }));
    await user.click(screen.getByTestId('submit-confirm-accept'));

    await waitFor(() => expect(submitHealthCheck).toHaveBeenCalledTimes(1));
    expect(submitHealthCheck.mock.calls[0][0]).toMatchObject({
      assessmentPeriod: '2026 H1',
      surveyType: 'post_workshop',
    });
  });

  it('does not show any period selector/edit control for a Team Member on the Post-Workshop Survey', async () => {
    currentUser = {
      id: 'member-1',
      name: 'Alex Member',
      hierarchyLevelId: 'level-5',
      teamIds: ['team-1'],
    };
    searchParams = new URLSearchParams('type=post_workshop&period=2026 H1');
    render(<SurveyPage />);

    await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H1 2026'));

    expect(screen.queryByTestId('edit-period-button')).not.toBeInTheDocument();
    expect(screen.queryByTestId('period-selector')).not.toBeInTheDocument();
    expect(screen.queryByTestId('done-period-button')).not.toBeInTheDocument();
  });

  it('blocks submission with a validation error when responses are incomplete', async () => {
    const user = userEvent.setup({ delay: null });
    render(<SurveyPage />);

    await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H2 2026'));

    // Only answer the first of two dimensions, then try to jump straight to the last-dimension submit button
    await user.click(getGreenScoreButton());
    await user.click(screen.getByRole('button', { name: /next/i }));
    await user.click(screen.getByRole('button', { name: /submit responses/i }));

    expect(screen.queryByTestId('submit-confirm-modal')).not.toBeInTheDocument();
    expect(submitHealthCheck).not.toHaveBeenCalled();
    expect(screen.getByText(/please fill out all health check dimensions/i)).toBeInTheDocument();
  });
});
