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
import { render, screen, waitFor, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import SurveyPage from '../page';
import { TeamsAPIError } from '@/lib/api/teams';

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
  // The page does `err instanceof TeamsAPIError` when a team lookup fails, so the mocked module
  // has to provide the class -- otherwise that check throws a TypeError instead of running.
  TeamsAPIError: class TeamsAPIError extends Error {},
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

  describe('post-submission Thank You page', () => {
    it('shows the Thank You page after a successful submission', async () => {
      const user = userEvent.setup({ delay: null });
      render(<SurveyPage />);

      await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H2 2026'));
      await answerAllDimensions(user);
      await user.click(screen.getByRole('button', { name: /submit responses/i }));
      await user.click(screen.getByTestId('submit-confirm-accept'));

      await waitFor(() => expect(screen.getByTestId('thank-you-page')).toBeInTheDocument());
      expect(screen.getByText('Thank You!')).toBeInTheDocument();
    });

    it('keeps the Thank You page visible with no automatic navigation away', async () => {
      const user = userEvent.setup({ delay: null });
      render(<SurveyPage />);

      await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H2 2026'));
      await answerAllDimensions(user);
      await user.click(screen.getByRole('button', { name: /submit responses/i }));
      await user.click(screen.getByTestId('submit-confirm-accept'));

      await waitFor(() => expect(screen.getByTestId('thank-you-page')).toBeInTheDocument());
      expect(push).not.toHaveBeenCalled();

      // Give any stray timer/effect a chance to fire, then confirm nothing navigated away.
      await new Promise((resolve) => setTimeout(resolve, 1500));
      expect(screen.getByTestId('thank-you-page')).toBeInTheDocument();
      expect(push).not.toHaveBeenCalled();
    });

    it('navigates a Team Member to /home when the explicit button is clicked', async () => {
      currentUser = {
        id: 'member-1',
        name: 'Alex Member',
        hierarchyLevelId: 'level-5',
        teamIds: ['team-1'],
      };
      const user = userEvent.setup({ delay: null });
      render(<SurveyPage />);

      await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H2 2026'));
      await answerAllDimensions(user);
      await user.click(screen.getByRole('button', { name: /submit responses/i }));
      await user.click(screen.getByTestId('submit-confirm-accept'));

      await waitFor(() => expect(screen.getByTestId('thank-you-page')).toBeInTheDocument());
      expect(push).not.toHaveBeenCalled();

      await user.click(screen.getByTestId('thank-you-nav-button'));
      expect(push).toHaveBeenCalledWith('/home');
    });

    it('navigates a Team Lead to /dashboard when the explicit button is clicked', async () => {
      const user = userEvent.setup({ delay: null }); // default currentUser is a Team Lead (level-4)
      render(<SurveyPage />);

      await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H2 2026'));
      await answerAllDimensions(user);
      await user.click(screen.getByRole('button', { name: /submit responses/i }));
      await user.click(screen.getByTestId('submit-confirm-accept'));

      await waitFor(() => expect(screen.getByTestId('thank-you-page')).toBeInTheDocument());
      expect(push).not.toHaveBeenCalled();

      await user.click(screen.getByTestId('thank-you-nav-button'));
      expect(push).toHaveBeenCalledWith('/dashboard');
    });

    it('does not show the Thank You page and shows an error when submission fails', async () => {
      submitHealthCheck.mockRejectedValueOnce(new Error('boom'));
      const user = userEvent.setup({ delay: null });
      render(<SurveyPage />);

      await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H2 2026'));
      await answerAllDimensions(user);
      await user.click(screen.getByRole('button', { name: /submit responses/i }));
      await user.click(screen.getByTestId('submit-confirm-accept'));

      await waitFor(() => expect(screen.getByText(/submission failed/i)).toBeInTheDocument());
      expect(screen.queryByTestId('thank-you-page')).not.toBeInTheDocument();
      expect(push).not.toHaveBeenCalled();
    });

    it('calls the API only once when the confirm button is clicked twice before React re-renders', async () => {
      let resolveSubmit: (value: { id: string }) => void;
      submitHealthCheck.mockImplementationOnce(
        () => new Promise((resolve) => { resolveSubmit = resolve; })
      );
      const user = userEvent.setup({ delay: null });
      render(<SurveyPage />);

      await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H2 2026'));
      await answerAllDimensions(user);
      await user.click(screen.getByRole('button', { name: /submit responses/i }));

      // performSubmit closes the confirm modal as soon as it runs, so a second click made after
      // a re-render lands on a detached button and never reaches the handler -- that would not
      // test the guard at all. Two native clicks inside one act() are batched: the modal is
      // still mounted for the second, which therefore reaches the real handler, and only the
      // in-flight guard (submitInFlightRef) stops it from submitting twice.
      const confirmButton = screen.getByTestId('submit-confirm-accept');
      act(() => {
        confirmButton.click();
        confirmButton.click();
      });

      await waitFor(() => expect(submitHealthCheck).toHaveBeenCalledTimes(1));

      resolveSubmit!({ id: 'session-999' });
      await waitFor(() => expect(screen.getByTestId('thank-you-page')).toBeInTheDocument());
      expect(submitHealthCheck).toHaveBeenCalledTimes(1);
    });
  });
});

describe('Survey: team lookup failure', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    searchParams = new URLSearchParams('');
    currentUser = { id: 'user-1', name: 'Taylor Lead', hierarchyLevelId: 'level-4', teamIds: ['team-1'] };
    vi.setSystemTime(new Date(2026, 8, 22));
  });

  it('shows the team error from the API when the lookup fails with a TeamsAPIError', async () => {
    getTeamInfoCached.mockRejectedValue(new TeamsAPIError('Team not found'));
    render(<SurveyPage />);

    await waitFor(() => expect(screen.getByText('Team not found')).toBeInTheDocument());
  });

  it('shows a generic message when the lookup fails for any other reason', async () => {
    getTeamInfoCached.mockRejectedValue(new Error('socket hang up'));
    render(<SurveyPage />);

    await waitFor(() => expect(screen.getByText('Failed to load team information')).toBeInTheDocument());
    expect(screen.queryByText('socket hang up')).not.toBeInTheDocument();
  });
});

describe('Survey: resuming a saved draft', () => {
  const draftKey = 'surveyDraft:user-1:team-1';
  const answers = [{ dimensionId: 'mission', score: 3, trend: 'stable' }];

  function saveDraft(fields: { assessmentPeriod: string; selectedAssessmentPeriod?: string }) {
    localStorage.setItem(
      draftKey,
      JSON.stringify({ responses: answers, currentDimension: 1, savedAt: '2026-08-01T00:00:00Z', ...fields }),
    );
  }

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    searchParams = new URLSearchParams('');
    currentUser = { id: 'user-1', name: 'Taylor Lead', hierarchyLevelId: 'level-4', teamIds: ['team-1'] };
    getTeamInfoCached.mockResolvedValue(TEAM_INFO);
  });

  it('resumes a draft saved under a manually selected earlier period even after today\'s period has moved on', async () => {
    // Saved in H2 2026 while working on H1 2026 (a catch-up). Today the auto period is H1 2027,
    // so the auto-detected period no longer matches the one recorded in the draft.
    saveDraft({ assessmentPeriod: '2026 H2', selectedAssessmentPeriod: '2026 H1' });
    vi.setSystemTime(new Date(2027, 0, 10));
    searchParams = new URLSearchParams('period=2026 H1');
    render(<SurveyPage />);

    await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H1 2026'));
    await waitFor(() => expect(screen.getByTestId('draft-restored-banner')).toBeInTheDocument());
  });

  it('does not resume a draft saved under the auto-detected period once that period has ended', async () => {
    saveDraft({ assessmentPeriod: '2026 H2', selectedAssessmentPeriod: '2026 H2' });
    vi.setSystemTime(new Date(2027, 0, 10)); // auto period is now 2027 H1
    render(<SurveyPage />);

    await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H1 2027'));
    expect(screen.queryByTestId('draft-restored-banner')).not.toBeInTheDocument();
  });

  it('does not resume a draft that belongs to a different period than the one being shown', async () => {
    saveDraft({ assessmentPeriod: '2026 H2', selectedAssessmentPeriod: '2026 H1' });
    vi.setSystemTime(new Date(2026, 8, 22));
    render(<SurveyPage />); // no ?period=, so the survey opens for the auto period, 2026 H2

    await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H2 2026'));
    expect(screen.queryByTestId('draft-restored-banner')).not.toBeInTheDocument();
  });
});

describe('Survey: switching teams keeps the chosen period', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    searchParams = new URLSearchParams('');
    currentUser = { id: 'user-1', name: 'Taylor Lead', hierarchyLevelId: 'level-4', teamIds: ['team-1', 'team-2'] };
    vi.setSystemTime(new Date(2026, 8, 22)); // auto period 2026 H2
    getTeamInfoCached.mockImplementation(async (id: string) => ({ ...TEAM_INFO, id, name: id }));
  });

  it('keeps a period chosen on Member Home when the user switches to another team', async () => {
    searchParams = new URLSearchParams('period=2026 H1');
    const user = userEvent.setup({ delay: null });
    render(<SurveyPage />);

    await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H1 2026'));
    await user.selectOptions(await screen.findByTestId('team-selector'), 'team-2');

    await waitFor(() => expect(getTeamInfoCached).toHaveBeenCalledWith('team-2'));
    await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H1 2026'));
  });

  it('uses the new team\'s own current period when the user was on the auto-detected one', async () => {
    const user = userEvent.setup({ delay: null });
    render(<SurveyPage />);

    await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H2 2026'));
    await user.selectOptions(await screen.findByTestId('team-selector'), 'team-2');

    await waitFor(() => expect(getTeamInfoCached).toHaveBeenCalledWith('team-2'));
    await waitFor(() => expect(screen.getByTestId('selected-period')).toHaveTextContent('H2 2026'));
  });
});
