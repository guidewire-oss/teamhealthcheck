/**
 * Tests for the Team Member "Take Survey" flow on the Member Home page.
 *
 * Covers:
 *  - Automatic period selection based on the current date.
 *  - Rendering the bordered "Select assessment period" panel inside the blue Current Period card.
 *  - Opening/using the dropdown and selecting a previous period (e.g. H1 2026).
 *  - Clicking "Take Survey" opens the period-selection modal instead of navigating immediately.
 *  - Confirming the modal checks the six-month submission cooldown before navigating.
 *  - Cancel/close dismisses the modal without navigating.
 *  - A submission still within the six-month cooldown shows the amber info modal instead
 *    of opening the survey.
 *  - No Post-Workshop Survey button or functionality is rendered for Team Members.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import MemberHomePage from '../page';

const push = vi.fn();
const router = { push };

vi.mock('next/navigation', () => ({
  useRouter: () => router,
}));

vi.mock('@/lib/auth', () => ({
  getCurrentUser: () => ({
    id: 'user-1',
    username: 'demo',
    fullName: 'Demo User',
    hierarchyLevel: 'level-5',
    hierarchyLevelId: 'level-5',
    teamIds: ['team-1'],
  }),
  logout: vi.fn(),
  authenticatedFetch: vi.fn().mockResolvedValue({ ok: true, json: async () => ({ surveyHistory: [] }) }),
}));

vi.mock('@/lib/org-config', () => ({
  getOrgConfig: () => ({}),
  getHierarchyLevel: () => ({ id: 'level-5', name: 'Team Member' }),
}));

const getTeamInfoCached = vi.fn();
vi.mock('@/lib/api/teams', () => ({
  getTeamInfoCached: (...args: unknown[]) => getTeamInfoCached(...args),
}));

const checkSurveyEligibility = vi.fn();
vi.mock('@/lib/api/health-checks', () => ({
  checkSurveyEligibility: (...args: unknown[]) => checkSurveyEligibility(...args),
}));

vi.mock('@/components/OnboardingModal', () => ({
  default: () => null,
}));

const TEAM_INFO = {
  id: 'team-1',
  name: 'Falcons',
  cadence: 'half-yearly',
  members: [],
};

describe('Member Home: Take Survey flow', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    vi.setSystemTime(new Date(2026, 8, 22)); // Sep 22, 2026 -> auto period "2026 H2"
    getTeamInfoCached.mockResolvedValue(TEAM_INFO);
    checkSurveyEligibility.mockResolvedValue({ eligible: true });
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => ({}) }));
  });

  it('auto-selects the current-date period and renders it as the dropdown default', async () => {
    render(<MemberHomePage />);

    await waitFor(() => expect(screen.getByTestId('assessment-period-select')).toHaveValue('2026 H2'));
  });

  it('displays the auto-detected period in the Current Period label', async () => {
    render(<MemberHomePage />);

    await waitFor(() => expect(screen.getByTestId('current-period')).toHaveTextContent('Current Period: 2026 H2'));
  });

  it('keeps showing the auto-detected period in the Current Period label after the user overrides the dropdown', async () => {
    const user = userEvent.setup({ delay: null });
    render(<MemberHomePage />);

    await waitFor(() => expect(screen.getByTestId('assessment-period-select')).toHaveValue('2026 H2'));
    await user.selectOptions(screen.getByTestId('assessment-period-select'), '2026 H1');

    // The dropdown reflects the override...
    expect(screen.getByTestId('assessment-period-select')).toHaveValue('2026 H1');
    // ...but the Current Period label must remain the auto-detected value, never the selection
    expect(screen.getByTestId('current-period')).toHaveTextContent('Current Period: 2026 H2');
  });

  it('renders the bordered "Select assessment period" panel with a labeled dropdown and helper text', async () => {
    render(<MemberHomePage />);

    await waitFor(() => expect(screen.getByTestId('assessment-period-panel')).toBeInTheDocument());
    expect(screen.getByText('Select assessment period')).toBeInTheDocument();
    expect(screen.getByLabelText('Assessment period')).toBeInTheDocument();
    expect(screen.getByText(/selected automatically based on today's date/i)).toBeInTheDocument();
    expect(screen.getByTestId('take-survey-btn')).toBeInTheDocument();
  });

  it('lets the user select a previous period (e.g. H1 2026) from the dropdown', async () => {
    const user = userEvent.setup({ delay: null });
    render(<MemberHomePage />);

    await waitFor(() => expect(screen.getByTestId('assessment-period-select')).toHaveValue('2026 H2'));

    await user.selectOptions(screen.getByTestId('assessment-period-select'), '2026 H1');
    expect(screen.getByTestId('assessment-period-select')).toHaveValue('2026 H1');
  });

  it('does not show the period-selection modal on initial load', async () => {
    render(<MemberHomePage />);

    await waitFor(() => expect(screen.getByTestId('take-survey-btn')).toBeInTheDocument());
    expect(screen.queryByTestId('period-selection-modal')).not.toBeInTheDocument();
  });

  it('opens the period-selection modal (not the survey) when "Take Survey" is clicked', async () => {
    const user = userEvent.setup({ delay: null });
    render(<MemberHomePage />);

    await waitFor(() => expect(screen.getByTestId('take-survey-btn')).toBeInTheDocument());
    await user.click(screen.getByTestId('take-survey-btn'));

    const modal = screen.getByTestId('period-selection-modal');
    expect(modal).toBeInTheDocument();
    expect(within(modal).getByText('Select assessment period')).toBeInTheDocument();
    expect(screen.getByTestId('period-selection-confirm-button')).toHaveTextContent('Take Survey');
    expect(push).not.toHaveBeenCalled();
  });

  it('auto-selects the current-date period as the modal dropdown default', async () => {
    const user = userEvent.setup({ delay: null });
    render(<MemberHomePage />);

    await waitFor(() => expect(screen.getByTestId('take-survey-btn')).toBeInTheDocument());
    await user.click(screen.getByTestId('take-survey-btn'));

    await waitFor(() => expect(screen.getByTestId('take-survey-period-select')).toHaveValue('2026 H2'));
  });

  it('passes the selected (overridden) period into the survey via the query param on confirm', async () => {
    const user = userEvent.setup({ delay: null });
    render(<MemberHomePage />);

    await waitFor(() => expect(screen.getByTestId('take-survey-btn')).toBeInTheDocument());
    await user.click(screen.getByTestId('take-survey-btn'));

    await waitFor(() => expect(screen.getByTestId('take-survey-period-select')).toHaveValue('2026 H2'));
    await user.selectOptions(screen.getByTestId('take-survey-period-select'), '2026 H1');
    await user.click(screen.getByTestId('period-selection-confirm-button'));

    await waitFor(() => expect(push).toHaveBeenCalledWith('/survey?period=2026%20H1'));
    expect(checkSurveyEligibility).toHaveBeenCalledWith({
      surveyType: 'individual',
      assessmentPeriod: '2026 H1',
      userId: 'user-1',
    });
  });

  it('passes the auto-detected period into the survey when the dropdown is left unchanged (default behavior)', async () => {
    const user = userEvent.setup({ delay: null });
    render(<MemberHomePage />);

    await waitFor(() => expect(screen.getByTestId('take-survey-btn')).toBeInTheDocument());
    await user.click(screen.getByTestId('take-survey-btn'));
    await waitFor(() => expect(screen.getByTestId('take-survey-period-select')).toHaveValue('2026 H2'));
    await user.click(screen.getByTestId('period-selection-confirm-button'));

    await waitFor(() => expect(push).toHaveBeenCalledWith('/survey?period=2026%20H2'));
  });

  it('closes the modal without navigating when Cancel is clicked', async () => {
    const user = userEvent.setup({ delay: null });
    render(<MemberHomePage />);

    await waitFor(() => expect(screen.getByTestId('take-survey-btn')).toBeInTheDocument());
    await user.click(screen.getByTestId('take-survey-btn'));
    expect(screen.getByTestId('period-selection-modal')).toBeInTheDocument();

    await user.click(screen.getByTestId('period-selection-cancel-button'));

    expect(screen.queryByTestId('period-selection-modal')).not.toBeInTheDocument();
    expect(push).not.toHaveBeenCalled();
  });

  it('closes the modal without navigating when the close (X) icon is clicked', async () => {
    const user = userEvent.setup({ delay: null });
    render(<MemberHomePage />);

    await waitFor(() => expect(screen.getByTestId('take-survey-btn')).toBeInTheDocument());
    await user.click(screen.getByTestId('take-survey-btn'));
    expect(screen.getByTestId('period-selection-modal')).toBeInTheDocument();

    await user.click(screen.getByTestId('period-selection-close-button'));

    expect(screen.queryByTestId('period-selection-modal')).not.toBeInTheDocument();
    expect(push).not.toHaveBeenCalled();
  });

  describe('six-month submission cooldown', () => {
    it('shows an amber "already submitted" info modal instead of opening the survey when still within the cooldown', async () => {
      checkSurveyEligibility.mockResolvedValue({
        eligible: false,
        submittedPeriod: 'H1 2026',
        nextEligiblePeriod: 'H2 2026',
      });
      const user = userEvent.setup({ delay: null });
      render(<MemberHomePage />);

      await waitFor(() => expect(screen.getByTestId('take-survey-btn')).toBeInTheDocument());
      await user.click(screen.getByTestId('take-survey-btn'));
      await user.click(screen.getByTestId('period-selection-confirm-button'));

      await waitFor(() => expect(screen.getByTestId('duplicate-submission-modal')).toBeInTheDocument());
      expect(screen.queryByTestId('period-selection-modal')).not.toBeInTheDocument();
      expect(screen.getByTestId('duplicate-submission-message')).toHaveTextContent('Individual Survey');
      expect(screen.getByTestId('duplicate-submission-message')).toHaveTextContent('H1 2026');
      expect(screen.getByTestId('duplicate-submission-message')).toHaveTextContent('H2 2026');
      expect(screen.getByTestId('duplicate-submission-message').textContent).not.toMatch(/Q[1-4]/);
      expect(push).not.toHaveBeenCalled();
    });

    it('shows "Your next survey is scheduled for H1 <next year>" when the last submission was H2 (year rollover)', async () => {
      checkSurveyEligibility.mockResolvedValue({
        eligible: false,
        submittedPeriod: 'H2 2026',
        nextEligiblePeriod: 'H1 2027',
      });
      const user = userEvent.setup({ delay: null });
      render(<MemberHomePage />);

      await waitFor(() => expect(screen.getByTestId('take-survey-btn')).toBeInTheDocument());
      await user.click(screen.getByTestId('take-survey-btn'));
      await user.click(screen.getByTestId('period-selection-confirm-button'));

      await waitFor(() => expect(screen.getByTestId('duplicate-submission-modal')).toBeInTheDocument());
      expect(screen.getByTestId('duplicate-submission-message')).toHaveTextContent('Individual Survey');
      expect(screen.getByTestId('duplicate-submission-message')).toHaveTextContent('H2 2026');
      expect(screen.getByTestId('duplicate-submission-message')).toHaveTextContent('H1 2027');
      expect(screen.getByTestId('duplicate-submission-message').textContent).not.toMatch(/Q[1-4]/);
    });

    it('opens the survey normally when eligible', async () => {
      checkSurveyEligibility.mockResolvedValue({ eligible: true });
      const user = userEvent.setup({ delay: null });
      render(<MemberHomePage />);

      await waitFor(() => expect(screen.getByTestId('take-survey-btn')).toBeInTheDocument());
      await user.click(screen.getByTestId('take-survey-btn'));
      await user.click(screen.getByTestId('period-selection-confirm-button'));

      await waitFor(() => expect(push).toHaveBeenCalledWith('/survey?period=2026%20H2'));
      expect(screen.queryByTestId('duplicate-submission-modal')).not.toBeInTheDocument();
    });

    it('closes the info modal via the Close button without navigating', async () => {
      checkSurveyEligibility.mockResolvedValue({
        eligible: false,
        submittedPeriod: 'H1 2025',
        nextEligiblePeriod: 'H2 2025',
      });
      const user = userEvent.setup({ delay: null });
      render(<MemberHomePage />);

      await waitFor(() => expect(screen.getByTestId('take-survey-btn')).toBeInTheDocument());
      await user.click(screen.getByTestId('take-survey-btn'));
      await user.click(screen.getByTestId('period-selection-confirm-button'));
      await waitFor(() => expect(screen.getByTestId('duplicate-submission-modal')).toBeInTheDocument());

      await user.click(screen.getByTestId('duplicate-submission-close-button'));

      expect(screen.queryByTestId('duplicate-submission-modal')).not.toBeInTheDocument();
      expect(push).not.toHaveBeenCalled();
    });

    it('checks eligibility scoped to the logged-in Team Member only', async () => {
      checkSurveyEligibility.mockResolvedValue({ eligible: true });
      const user = userEvent.setup({ delay: null });
      render(<MemberHomePage />);

      await waitFor(() => expect(screen.getByTestId('take-survey-btn')).toBeInTheDocument());
      await user.click(screen.getByTestId('take-survey-btn'));
      await user.click(screen.getByTestId('period-selection-confirm-button'));

      await waitFor(() => expect(checkSurveyEligibility).toHaveBeenCalled());
      const call = checkSurveyEligibility.mock.calls[0][0];
      expect(call.surveyType).toBe('individual');
      expect(call.userId).toBe('user-1');
    });

    it('fails open and navigates to the survey if the eligibility check errors', async () => {
      checkSurveyEligibility.mockRejectedValue(new Error('network error'));
      const user = userEvent.setup({ delay: null });
      render(<MemberHomePage />);

      await waitFor(() => expect(screen.getByTestId('take-survey-btn')).toBeInTheDocument());
      await user.click(screen.getByTestId('take-survey-btn'));
      await user.click(screen.getByTestId('period-selection-confirm-button'));

      await waitFor(() => expect(push).toHaveBeenCalledWith('/survey?period=2026%20H2'));
      expect(screen.queryByTestId('duplicate-submission-modal')).not.toBeInTheDocument();
    });
  });

  describe('no Post-Workshop Survey functionality for Team Members', () => {
    it('does not render a Post-Workshop Survey button', async () => {
      render(<MemberHomePage />);

      await waitFor(() => expect(screen.getByTestId('take-survey-btn')).toBeInTheDocument());
      expect(screen.queryByTestId('post-workshop-survey-button')).not.toBeInTheDocument();
      expect(screen.queryByText(/post-workshop survey/i)).not.toBeInTheDocument();
    });

    it('never requests a post_workshop eligibility check from this page', async () => {
      const user = userEvent.setup({ delay: null });
      render(<MemberHomePage />);

      await waitFor(() => expect(screen.getByTestId('take-survey-btn')).toBeInTheDocument());
      await user.click(screen.getByTestId('take-survey-btn'));
      await user.click(screen.getByTestId('period-selection-confirm-button'));

      await waitFor(() => expect(checkSurveyEligibility).toHaveBeenCalled());
      for (const call of checkSurveyEligibility.mock.calls) {
        expect(call[0].surveyType).not.toBe('post_workshop');
      }
    });
  });
});
