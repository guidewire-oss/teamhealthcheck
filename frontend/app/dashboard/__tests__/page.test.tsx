/**
 * Tests for the Team Lead assessment-period selection modal, shown before the
 * "Take Survey" and "Post-Workshop Survey" buttons open the actual survey.
 *
 * Covers:
 *  - Clicking either button opens the period-selection modal instead of navigating immediately.
 *  - Automatic period selection is the modal's dropdown default.
 *  - The Team Lead can select a previous period.
 *  - Confirming "Take Survey" passes the selected period into the survey via the query param.
 *  - Confirming "Take Post-Workshop Survey" passes the selected period (and survey type).
 *  - Default (unchanged) behavior when the dropdown is left at auto-detection.
 *  - Cancel/close dismisses the modal without navigating.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import DashboardPage from '../page';

const push = vi.fn();
const router = { push };

vi.mock('next/navigation', () => ({
  useRouter: () => router,
}));

vi.mock('@/lib/auth', () => ({
  getCurrentUser: () => ({
    id: 'lead-1',
    name: 'Taylor Lead',
    hierarchyLevelId: 'level-4',
    teamIds: ['team-1'],
  }),
  logout: vi.fn(),
  authenticatedFetch: vi.fn().mockResolvedValue({ ok: false, status: 404 }),
}));

vi.mock('@/lib/org-config', () => ({
  getOrgConfig: () => ({ companyName: 'Acme' }),
  getHierarchyLevel: () => ({ id: 'level-4', name: 'Team Lead' }),
  getUserPermissions: () => ({}),
}));

const getTeamInfoCached = vi.fn();
vi.mock('@/lib/api/teams', () => ({
  getTeamInfoCached: (...args: unknown[]) => getTeamInfoCached(...args),
}));

const checkSurveyEligibility = vi.fn();
vi.mock('@/lib/api/health-checks', () => ({
  getTeamSubmissionStatus: vi.fn().mockResolvedValue({
    teamId: 'team-1',
    assessmentPeriod: '2026 H2',
    totalMembers: 1,
    submittedMembers: 0,
    allSubmitted: false,
    postWorkshopExists: false,
  }),
  getAssessmentPeriods: vi.fn().mockResolvedValue([]),
  checkSurveyEligibility: (...args: unknown[]) => checkSurveyEligibility(...args),
}));

vi.mock('@/components/OnboardingModal', () => ({ default: () => null }));
vi.mock('@/components/ActionItemsTab', () => ({ default: () => null }));

const TEAM_INFO = {
  id: 'team-1',
  name: 'Falcons',
  cadence: 'half-yearly',
  members: [],
};

describe('Team Lead dashboard: assessment period selection modal', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.setSystemTime(new Date(2026, 8, 22)); // Sep 22, 2026 -> auto period "2026 H2"
    getTeamInfoCached.mockResolvedValue(TEAM_INFO);
    checkSurveyEligibility.mockResolvedValue({ eligible: true });
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => ({}) }));
  });

  it('does not show the period-selection modal on initial load', async () => {
    render(<DashboardPage />);

    await waitFor(() => expect(screen.getByTestId('take-survey-button')).toBeInTheDocument());
    expect(screen.queryByTestId('period-selection-modal')).not.toBeInTheDocument();
  });

  it('opens the period-selection modal (not the survey) when "Take Survey" is clicked', async () => {
    const user = userEvent.setup({ delay: null });
    render(<DashboardPage />);

    await waitFor(() => expect(screen.getByTestId('take-survey-button')).toBeInTheDocument());
    await user.click(screen.getByTestId('take-survey-button'));

    expect(screen.getByTestId('period-selection-modal')).toBeInTheDocument();
    expect(screen.getByText('Select assessment period')).toBeInTheDocument();
    expect(push).not.toHaveBeenCalled();
  });

  it('opens the period-selection modal (not the survey) when "Post-Workshop Survey" is clicked', async () => {
    const user = userEvent.setup({ delay: null });
    render(<DashboardPage />);

    await waitFor(() => expect(screen.getByTestId('post-workshop-survey-button')).toBeInTheDocument());
    await user.click(screen.getByTestId('post-workshop-survey-button'));

    expect(screen.getByTestId('period-selection-modal')).toBeInTheDocument();
    expect(push).not.toHaveBeenCalled();
  });

  it('auto-selects the current-date period as the modal dropdown default', async () => {
    const user = userEvent.setup({ delay: null });
    render(<DashboardPage />);

    await waitFor(() => expect(screen.getByTestId('take-survey-button')).toBeInTheDocument());
    await user.click(screen.getByTestId('take-survey-button'));

    await waitFor(() => expect(screen.getByTestId('take-survey-period-select')).toHaveValue('2026 H2'));
  });

  it('lets the Team Lead select a previous period (e.g. H1 2026)', async () => {
    const user = userEvent.setup({ delay: null });
    render(<DashboardPage />);

    await waitFor(() => expect(screen.getByTestId('take-survey-button')).toBeInTheDocument());
    await user.click(screen.getByTestId('take-survey-button'));

    await waitFor(() => expect(screen.getByTestId('take-survey-period-select')).toHaveValue('2026 H2'));
    await user.selectOptions(screen.getByTestId('take-survey-period-select'), '2026 H1');

    expect(screen.getByTestId('take-survey-period-select')).toHaveValue('2026 H1');
  });

  it('shows a "Take Survey" confirm button in the modal for the regular survey flow', async () => {
    const user = userEvent.setup({ delay: null });
    render(<DashboardPage />);

    await waitFor(() => expect(screen.getByTestId('take-survey-button')).toBeInTheDocument());
    await user.click(screen.getByTestId('take-survey-button'));

    expect(screen.getByTestId('period-selection-confirm-button')).toHaveTextContent('Take Survey');
  });

  it('shows a "Take Post-Workshop Survey" confirm button in the modal for the post-workshop flow', async () => {
    const user = userEvent.setup({ delay: null });
    render(<DashboardPage />);

    await waitFor(() => expect(screen.getByTestId('post-workshop-survey-button')).toBeInTheDocument());
    await user.click(screen.getByTestId('post-workshop-survey-button'));

    expect(screen.getByTestId('period-selection-confirm-button')).toHaveTextContent('Take Post-Workshop Survey');
  });

  it('passes the selected period into the survey when confirming "Take Survey"', async () => {
    const user = userEvent.setup({ delay: null });
    render(<DashboardPage />);

    await waitFor(() => expect(screen.getByTestId('take-survey-button')).toBeInTheDocument());
    await user.click(screen.getByTestId('take-survey-button'));

    await waitFor(() => expect(screen.getByTestId('take-survey-period-select')).toHaveValue('2026 H2'));
    await user.selectOptions(screen.getByTestId('take-survey-period-select'), '2026 H1');
    await user.click(screen.getByTestId('period-selection-confirm-button'));

    await waitFor(() => expect(push).toHaveBeenCalledWith('/survey?team=team-1&period=2026+H1'));
    expect(checkSurveyEligibility).toHaveBeenCalledWith({
      surveyType: 'individual',
      assessmentPeriod: '2026 H1',
      teamId: undefined,
      userId: 'lead-1',
    });
  });

  it('passes the selected period and survey type into the survey when confirming "Take Post-Workshop Survey"', async () => {
    const user = userEvent.setup({ delay: null });
    render(<DashboardPage />);

    await waitFor(() => expect(screen.getByTestId('post-workshop-survey-button')).toBeInTheDocument());
    await user.click(screen.getByTestId('post-workshop-survey-button'));

    await waitFor(() => expect(screen.getByTestId('take-survey-period-select')).toHaveValue('2026 H2'));
    await user.selectOptions(screen.getByTestId('take-survey-period-select'), '2026 H1');
    await user.click(screen.getByTestId('period-selection-confirm-button'));

    await waitFor(() => expect(push).toHaveBeenCalledWith('/survey?team=team-1&type=post_workshop&period=2026+H1'));
    expect(checkSurveyEligibility).toHaveBeenCalledWith({
      surveyType: 'post_workshop',
      assessmentPeriod: '2026 H1',
      teamId: 'team-1',
      userId: undefined,
    });
  });

  it('passes the auto-detected period into both surveys when the dropdown is left unchanged (default behavior)', async () => {
    const user = userEvent.setup({ delay: null });
    render(<DashboardPage />);

    await waitFor(() => expect(screen.getByTestId('take-survey-button')).toBeInTheDocument());

    await user.click(screen.getByTestId('take-survey-button'));
    await waitFor(() => expect(screen.getByTestId('take-survey-period-select')).toHaveValue('2026 H2'));
    await user.click(screen.getByTestId('period-selection-confirm-button'));
    await waitFor(() => expect(push).toHaveBeenCalledWith('/survey?team=team-1&period=2026+H2'));

    await user.click(screen.getByTestId('post-workshop-survey-button'));
    await waitFor(() => expect(screen.getByTestId('take-survey-period-select')).toHaveValue('2026 H2'));
    await user.click(screen.getByTestId('period-selection-confirm-button'));
    await waitFor(() => expect(push).toHaveBeenCalledWith('/survey?team=team-1&type=post_workshop&period=2026+H2'));
  });

  it('closes the modal without navigating when Cancel is clicked', async () => {
    const user = userEvent.setup({ delay: null });
    render(<DashboardPage />);

    await waitFor(() => expect(screen.getByTestId('take-survey-button')).toBeInTheDocument());
    await user.click(screen.getByTestId('take-survey-button'));
    expect(screen.getByTestId('period-selection-modal')).toBeInTheDocument();

    await user.click(screen.getByTestId('period-selection-cancel-button'));

    expect(screen.queryByTestId('period-selection-modal')).not.toBeInTheDocument();
    expect(push).not.toHaveBeenCalled();
  });

  it('closes the modal without navigating when the close (X) button is clicked', async () => {
    const user = userEvent.setup({ delay: null });
    render(<DashboardPage />);

    await waitFor(() => expect(screen.getByTestId('post-workshop-survey-button')).toBeInTheDocument());
    await user.click(screen.getByTestId('post-workshop-survey-button'));
    expect(screen.getByTestId('period-selection-modal')).toBeInTheDocument();

    await user.click(screen.getByTestId('period-selection-close-button'));

    expect(screen.queryByTestId('period-selection-modal')).not.toBeInTheDocument();
    expect(push).not.toHaveBeenCalled();
  });

  it('does not render the take-survey period dropdown on the main page until a button is clicked', async () => {
    render(<DashboardPage />);

    await waitFor(() => expect(screen.getByTestId('take-survey-button')).toBeInTheDocument());

    // The take-survey period dropdown only exists inside the period-selection modal.
    expect(screen.queryByTestId('take-survey-period-select')).not.toBeInTheDocument();
  });

  describe('six-month submission cooldown', () => {
    it('shows the "already submitted" modal instead of opening the survey when Individual Survey is ineligible', async () => {
      checkSurveyEligibility.mockResolvedValue({
        eligible: false,
        submittedPeriod: 'H1 2026',
        nextEligiblePeriod: 'H2 2026',
      });
      const user = userEvent.setup({ delay: null });
      render(<DashboardPage />);

      await waitFor(() => expect(screen.getByTestId('take-survey-button')).toBeInTheDocument());
      await user.click(screen.getByTestId('take-survey-button'));
      await user.click(screen.getByTestId('period-selection-confirm-button'));

      await waitFor(() => expect(screen.getByTestId('duplicate-submission-modal')).toBeInTheDocument());
      expect(screen.queryByTestId('period-selection-modal')).not.toBeInTheDocument();
      expect(screen.getByTestId('duplicate-submission-message')).toHaveTextContent('Individual Survey');
      expect(screen.getByTestId('duplicate-submission-message')).toHaveTextContent('H1 2026');
      expect(screen.getByTestId('duplicate-submission-message')).toHaveTextContent('H2 2026');
      expect(push).not.toHaveBeenCalled();
    });

    it('shows the "already submitted" modal instead of opening the survey when Post-Workshop Survey is ineligible', async () => {
      checkSurveyEligibility.mockResolvedValue({
        eligible: false,
        submittedPeriod: 'H1 2026',
        nextEligiblePeriod: 'H2 2026',
      });
      const user = userEvent.setup({ delay: null });
      render(<DashboardPage />);

      await waitFor(() => expect(screen.getByTestId('post-workshop-survey-button')).toBeInTheDocument());
      await user.click(screen.getByTestId('post-workshop-survey-button'));
      await user.click(screen.getByTestId('period-selection-confirm-button'));

      await waitFor(() => expect(screen.getByTestId('duplicate-submission-modal')).toBeInTheDocument());
      expect(screen.getByTestId('duplicate-submission-message')).toHaveTextContent('Post-Workshop Survey');
      expect(screen.getByTestId('duplicate-submission-message')).toHaveTextContent('H1 2026');
      expect(screen.getByTestId('duplicate-submission-message')).toHaveTextContent('H2 2026');
      expect(push).not.toHaveBeenCalled();
    });

    it('shows "Your next survey is scheduled for H1 <next year>" for the Individual Survey when the last submission was H2 (year rollover)', async () => {
      checkSurveyEligibility.mockResolvedValue({
        eligible: false,
        submittedPeriod: 'H2 2026',
        nextEligiblePeriod: 'H1 2027',
      });
      const user = userEvent.setup({ delay: null });
      render(<DashboardPage />);

      await waitFor(() => expect(screen.getByTestId('take-survey-button')).toBeInTheDocument());
      await user.click(screen.getByTestId('take-survey-button'));
      await user.click(screen.getByTestId('period-selection-confirm-button'));

      await waitFor(() => expect(screen.getByTestId('duplicate-submission-modal')).toBeInTheDocument());
      expect(screen.getByTestId('duplicate-submission-message')).toHaveTextContent('Individual Survey');
      expect(screen.getByTestId('duplicate-submission-message')).toHaveTextContent('H2 2026');
      expect(screen.getByTestId('duplicate-submission-message')).toHaveTextContent('H1 2027');
      expect(screen.getByTestId('duplicate-submission-message').textContent).not.toMatch(/Q[1-4]/);
    });

    it('shows "Your next survey is scheduled for H1 <next year>" for the Post-Workshop Survey when the last submission was H2 (year rollover)', async () => {
      checkSurveyEligibility.mockResolvedValue({
        eligible: false,
        submittedPeriod: 'H2 2026',
        nextEligiblePeriod: 'H1 2027',
      });
      const user = userEvent.setup({ delay: null });
      render(<DashboardPage />);

      await waitFor(() => expect(screen.getByTestId('post-workshop-survey-button')).toBeInTheDocument());
      await user.click(screen.getByTestId('post-workshop-survey-button'));
      await user.click(screen.getByTestId('period-selection-confirm-button'));

      await waitFor(() => expect(screen.getByTestId('duplicate-submission-modal')).toBeInTheDocument());
      expect(screen.getByTestId('duplicate-submission-message')).toHaveTextContent('Post-Workshop Survey');
      expect(screen.getByTestId('duplicate-submission-message')).toHaveTextContent('H2 2026');
      expect(screen.getByTestId('duplicate-submission-message')).toHaveTextContent('H1 2027');
      expect(screen.getByTestId('duplicate-submission-message').textContent).not.toMatch(/Q[1-4]/);
    });

    it('opens the survey normally when eligible', async () => {
      checkSurveyEligibility.mockResolvedValue({ eligible: true });
      const user = userEvent.setup({ delay: null });
      render(<DashboardPage />);

      await waitFor(() => expect(screen.getByTestId('take-survey-button')).toBeInTheDocument());
      await user.click(screen.getByTestId('take-survey-button'));
      await user.click(screen.getByTestId('period-selection-confirm-button'));

      await waitFor(() => expect(push).toHaveBeenCalledWith('/survey?team=team-1&period=2026+H2'));
      expect(screen.queryByTestId('duplicate-submission-modal')).not.toBeInTheDocument();
    });

    it('closes the duplicate-submission modal via the Close button', async () => {
      checkSurveyEligibility.mockResolvedValue({
        eligible: false,
        submittedPeriod: 'H1 2026',
        nextEligiblePeriod: 'H2 2026',
      });
      const user = userEvent.setup({ delay: null });
      render(<DashboardPage />);

      await waitFor(() => expect(screen.getByTestId('take-survey-button')).toBeInTheDocument());
      await user.click(screen.getByTestId('take-survey-button'));
      await user.click(screen.getByTestId('period-selection-confirm-button'));
      await waitFor(() => expect(screen.getByTestId('duplicate-submission-modal')).toBeInTheDocument());

      await user.click(screen.getByTestId('duplicate-submission-close-button'));

      expect(screen.queryByTestId('duplicate-submission-modal')).not.toBeInTheDocument();
      expect(push).not.toHaveBeenCalled();
    });

    it('checks the individual survey eligibility by the logged-in user only, regardless of team', async () => {
      checkSurveyEligibility.mockResolvedValue({ eligible: true });
      const user = userEvent.setup({ delay: null });
      render(<DashboardPage />);

      await waitFor(() => expect(screen.getByTestId('take-survey-button')).toBeInTheDocument());
      await user.click(screen.getByTestId('take-survey-button'));
      await user.click(screen.getByTestId('period-selection-confirm-button'));

      await waitFor(() => expect(checkSurveyEligibility).toHaveBeenCalled());
      const call = checkSurveyEligibility.mock.calls[0][0];
      expect(call.surveyType).toBe('individual');
      expect(call.userId).toBe('lead-1');
      expect(call.teamId).toBeUndefined();
    });

    it('checks the post-workshop eligibility by the selected team', async () => {
      checkSurveyEligibility.mockResolvedValue({ eligible: true });
      const user = userEvent.setup({ delay: null });
      render(<DashboardPage />);

      await waitFor(() => expect(screen.getByTestId('post-workshop-survey-button')).toBeInTheDocument());
      await user.click(screen.getByTestId('post-workshop-survey-button'));
      await user.click(screen.getByTestId('period-selection-confirm-button'));

      await waitFor(() => expect(checkSurveyEligibility).toHaveBeenCalled());
      const call = checkSurveyEligibility.mock.calls[0][0];
      expect(call.surveyType).toBe('post_workshop');
      expect(call.teamId).toBe('team-1');
      expect(call.userId).toBeUndefined();
    });

    it('fails open (opens the survey) if the eligibility check itself errors', async () => {
      checkSurveyEligibility.mockRejectedValue(new Error('network error'));
      const user = userEvent.setup({ delay: null });
      render(<DashboardPage />);

      await waitFor(() => expect(screen.getByTestId('take-survey-button')).toBeInTheDocument());
      await user.click(screen.getByTestId('take-survey-button'));
      await user.click(screen.getByTestId('period-selection-confirm-button'));

      await waitFor(() => expect(push).toHaveBeenCalledWith('/survey?team=team-1&period=2026+H2'));
      expect(screen.queryByTestId('duplicate-submission-modal')).not.toBeInTheDocument();
    });
  });
});
