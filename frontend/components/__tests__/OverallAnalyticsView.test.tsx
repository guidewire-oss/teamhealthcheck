import { describe, expect, it } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import OverallAnalyticsView from '@/components/OverallAnalyticsView';
import type { SurveyCompletionData } from '@/components/SurveyCompletionDashboard';

function buildData(teamStats: SurveyCompletionData['teamStats']): SurveyCompletionData {
  return {
    overallCompletion: 50,
    totalTeams: teamStats.length,
    optedIn: teamStats.filter((t) => t.status !== 'opted_out').length,
    fullyComplete: teamStats.filter((t) => t.status === 'complete').length,
    inProgress: teamStats.filter((t) => t.status === 'in_progress').length,
    notStarted: teamStats.filter((t) => t.status === 'not_started').length,
    optedOut: teamStats.filter((t) => t.status === 'opted_out').length,
    teamStats,
    timeSeries: [],
  };
}

function team(name: string, overrides: Partial<SurveyCompletionData['teamStats'][number]> = {}) {
  return { teamId: name, teamName: name, completed: 2, total: 4, status: 'in_progress' as const, ...overrides };
}

// Detailed status-filter/search/empty-state/scroll-area behavior is tested
// once, thoroughly, against CompletionByTeamPanel directly (see
// CompletionByTeamPanel.test.tsx) -- both the card here and the fullscreen
// modal render that exact same component, so this file only needs to
// confirm the wiring: card chrome, the expand button, and the fullscreen
// overlay's own open/close/Escape/scroll-lock behavior.
// Regression coverage: these labels used to hardcode "this half", which was
// wrong for any non-half-yearly cadence (monthly/quarterly/yearly) and for
// any period other than the current one selected from the dropdown -- the
// label must reflect whatever period the caller actually passes in.
describe('OverallAnalyticsView — period-specific labels', () => {
  it("shows the actual selected assessment period, never a hardcoded 'this half'", () => {
    render(
      <OverallAnalyticsView data={buildData([team('Sunnyvale')])} assessmentPeriod="2026 Q1" onBack={() => {}} />,
    );

    expect(screen.getByText(/the trend so far in 2026 Q1\./)).toBeInTheDocument();
    expect(screen.getByText('Completion trend — 2026 Q1')).toBeInTheDocument();
    expect(screen.queryByText(/this half/i)).not.toBeInTheDocument();
  });

  it('updates the label when a different period is passed', () => {
    render(
      <OverallAnalyticsView data={buildData([team('Sunnyvale')])} assessmentPeriod="2025 - 2nd Half" onBack={() => {}} />,
    );

    expect(screen.getByText('Completion trend — 2025 - 2nd Half')).toBeInTheDocument();
  });
});

describe('OverallAnalyticsView — Completion by team card', () => {
  it('gives the two chart cards the identical fixed height class, so they always match', () => {
    render(<OverallAnalyticsView data={buildData([team('Sunnyvale')])} assessmentPeriod="2026 H1" onBack={() => {}} />);
    const breakdownCard = screen.getByTestId('chart-card-breakdown');
    const teamCard = screen.getByTestId('chart-card-team-bars');
    expect(breakdownCard.className).toMatch(/h-\[22rem\]/);
    expect(teamCard.className).toMatch(/h-\[22rem\]/);
  });

  it('renders the shared CompletionByTeamPanel inside the card (search box, scroll area present)', () => {
    render(<OverallAnalyticsView data={buildData([team('Sunnyvale')])} assessmentPeriod="2026 H1" onBack={() => {}} />);
    expect(screen.getByTestId('team-bars-search-input')).toBeInTheDocument();
    expect(screen.getByTestId('team-bars-scroll-area')).toBeInTheDocument();
  });

  it('shows an expand button in the card', () => {
    render(<OverallAnalyticsView data={buildData([team('Sunnyvale')])} assessmentPeriod="2026 H1" onBack={() => {}} />);
    expect(screen.getByTestId('team-bars-expand-button')).toBeInTheDocument();
  });

  it('does not show the fullscreen overlay before the expand button is clicked', () => {
    render(<OverallAnalyticsView data={buildData([team('Sunnyvale')])} assessmentPeriod="2026 H1" onBack={() => {}} />);
    expect(screen.queryByTestId('team-bars-fullscreen-overlay')).not.toBeInTheDocument();
  });
});

describe('OverallAnalyticsView — fullscreen expansion', () => {
  it('opens the fullscreen overlay when the expand button is clicked', async () => {
    const user = userEvent.setup();
    render(<OverallAnalyticsView data={buildData([team('Sunnyvale')])} assessmentPeriod="2026 H1" onBack={() => {}} />);

    await user.click(screen.getByTestId('team-bars-expand-button'));

    expect(screen.getByTestId('team-bars-fullscreen-overlay')).toBeInTheDocument();
  });

  it('exposes dialog semantics, moves focus in on open, and restores it to the expand button on close', async () => {
    const user = userEvent.setup();
    render(<OverallAnalyticsView data={buildData([team('Sunnyvale')])} assessmentPeriod="2026 H1" onBack={() => {}} />);

    const expandButton = screen.getByTestId('team-bars-expand-button');
    await user.click(expandButton);

    const dialog = screen.getByRole('dialog');
    expect(dialog).toHaveAttribute('aria-modal', 'true');
    expect(dialog).toHaveAccessibleName('Completion by team');
    expect(document.activeElement).toBe(screen.getByTestId('team-bars-fullscreen-close'));

    await user.click(screen.getByTestId('team-bars-fullscreen-close'));

    expect(document.activeElement).toBe(expandButton);
  });

  it('the fullscreen view includes the same panel: search box and sorted rows, in-progress-only', async () => {
    const user = userEvent.setup();
    render(
      <OverallAnalyticsView
        data={buildData([team('Sunnyvale'), team('Riverside', { completed: 4, total: 4, status: 'complete' })])}
        assessmentPeriod="2026 H1"
        onBack={() => {}}
      />,
    );

    await user.click(screen.getByTestId('team-bars-expand-button'));

    // The card behind the overlay stays mounted (it's just visually
    // covered), so these testids now exist twice in the DOM -- scope every
    // lookup to inside the overlay specifically.
    const overlay = screen.getByTestId('team-bars-fullscreen-overlay');
    const withinOverlay = within(overlay);
    expect(withinOverlay.getByTestId('team-bars-search-input')).toBeInTheDocument();
    expect(withinOverlay.getByTestId('team-bars-scroll-area')).toBeInTheDocument();
    // The Completed team never appears in the fullscreen view either --
    // filtering is applied consistently between the card and the overlay.
    expect(withinOverlay.getByText('Sunnyvale')).toBeInTheDocument();
    expect(withinOverlay.queryByText('Riverside')).not.toBeInTheDocument();
  });

  it('closes the fullscreen overlay via its close button', async () => {
    const user = userEvent.setup();
    render(<OverallAnalyticsView data={buildData([team('Sunnyvale')])} assessmentPeriod="2026 H1" onBack={() => {}} />);

    await user.click(screen.getByTestId('team-bars-expand-button'));
    expect(screen.getByTestId('team-bars-fullscreen-overlay')).toBeInTheDocument();

    await user.click(screen.getByTestId('team-bars-fullscreen-close'));
    expect(screen.queryByTestId('team-bars-fullscreen-overlay')).not.toBeInTheDocument();
  });

  it('closes the fullscreen overlay when Escape is pressed', async () => {
    const user = userEvent.setup();
    render(<OverallAnalyticsView data={buildData([team('Sunnyvale')])} assessmentPeriod="2026 H1" onBack={() => {}} />);

    await user.click(screen.getByTestId('team-bars-expand-button'));
    expect(screen.getByTestId('team-bars-fullscreen-overlay')).toBeInTheDocument();

    await user.keyboard('{Escape}');
    expect(screen.queryByTestId('team-bars-fullscreen-overlay')).not.toBeInTheDocument();
  });

  it('locks page-level scrolling while open and restores it on close', async () => {
    const user = userEvent.setup();
    render(<OverallAnalyticsView data={buildData([team('Sunnyvale')])} assessmentPeriod="2026 H1" onBack={() => {}} />);

    expect(document.body.style.overflow).not.toBe('hidden');

    await user.click(screen.getByTestId('team-bars-expand-button'));
    expect(document.body.style.overflow).toBe('hidden');

    await user.click(screen.getByTestId('team-bars-fullscreen-close'));
    expect(document.body.style.overflow).not.toBe('hidden');
  });

  it('the fullscreen panel keeps its own independent internal scroll area (does not affect page scroll)', async () => {
    const user = userEvent.setup();
    const manyTeams = Array.from({ length: 40 }, (_, i) => team(`Team ${i}`));
    render(<OverallAnalyticsView data={buildData(manyTeams)} assessmentPeriod="2026 H1" onBack={() => {}} />);

    await user.click(screen.getByTestId('team-bars-expand-button'));

    const overlay = screen.getByTestId('team-bars-fullscreen-overlay');
    const scrollArea = within(overlay).getByTestId('team-bars-scroll-area');
    expect(scrollArea.className).toMatch(/overflow-y-auto/);
  });
});
