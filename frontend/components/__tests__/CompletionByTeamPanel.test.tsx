import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import CompletionByTeamPanel from '@/components/CompletionByTeamPanel';
import type { SurveyCompletionData } from '@/components/SurveyCompletionDashboard';

function team(name: string, overrides: Partial<SurveyCompletionData['teamStats'][number]> = {}) {
  return { teamId: name, teamName: name, completed: 2, total: 4, status: 'in_progress' as const, ...overrides };
}

// This section always shows ONLY genuinely in-progress teams/pods now --
// there is no status filter to switch away from that. Fixtures use
// generic placeholder names deliberately.
describe('CompletionByTeamPanel — in-progress-only filtering', () => {
  it('displays an in_progress team', () => {
    render(<CompletionByTeamPanel teamStats={[team('Alpha Pod')]} />);

    expect(screen.getByText('Alpha Pod')).toBeInTheDocument();
    expect(screen.queryByTestId('team-bars-empty-state')).not.toBeInTheDocument();
  });

  it('excludes a Completed team', () => {
    render(
      <CompletionByTeamPanel
        teamStats={[
          team('Alpha Pod'),
          team('Complete Pod', { status: 'complete', completed: 4, total: 4 }),
        ]}
      />,
    );

    expect(screen.getByText('Alpha Pod')).toBeInTheDocument();
    expect(screen.queryByText('Complete Pod')).not.toBeInTheDocument();
  });

  it('excludes an in_progress team whose percentage rounds up to exactly 100%', () => {
    render(
      <CompletionByTeamPanel
        teamStats={[
          team('Alpha Pod'),
          // Raw canonical status is still in_progress (completed < total),
          // but 199/200 rounds to 100% -- must be excluded just like a
          // genuinely complete team, never shown as a green "Completed" row.
          team('Rounds To Complete Pod', { status: 'in_progress', completed: 199, total: 200 }),
        ]}
      />,
    );

    expect(screen.getByText('Alpha Pod')).toBeInTheDocument();
    expect(screen.queryByText('Rounds To Complete Pod')).not.toBeInTheDocument();
  });

  it('excludes a Not Started team', () => {
    render(
      <CompletionByTeamPanel
        teamStats={[team('Alpha Pod'), team('Not Started Pod', { status: 'not_started', completed: 0, total: 4 })]}
      />,
    );

    expect(screen.getByText('Alpha Pod')).toBeInTheDocument();
    expect(screen.queryByText('Not Started Pod')).not.toBeInTheDocument();
  });

  it('excludes an Opted Out team', () => {
    render(
      <CompletionByTeamPanel
        teamStats={[team('Alpha Pod'), team('Opted Out Pod', { status: 'opted_out', completed: 0, total: 4 })]}
      />,
    );

    expect(screen.getByText('Alpha Pod')).toBeInTheDocument();
    expect(screen.queryByText('Opted Out Pod')).not.toBeInTheDocument();
  });

  it('shows the in-progress empty state when there are no in_progress teams at all', () => {
    render(
      <CompletionByTeamPanel
        teamStats={[
          team('Complete Pod', { status: 'complete', completed: 4, total: 4 }),
          team('Not Started Pod', { status: 'not_started', completed: 0, total: 4 }),
          team('Opted Out Pod', { status: 'opted_out', completed: 0, total: 4 }),
        ]}
      />,
    );

    expect(screen.getByTestId('team-bars-empty-state')).toHaveTextContent('No teams currently in progress.');
  });

  it('shows the in-progress empty state when the only in_progress team rounds up to 100%', () => {
    render(<CompletionByTeamPanel teamStats={[team('Rounds To Complete Pod', { completed: 199, total: 200 })]} />);

    expect(screen.getByTestId('team-bars-empty-state')).toHaveTextContent('No teams currently in progress.');
  });

  it('does not offer any status filter tabs any more', () => {
    render(<CompletionByTeamPanel teamStats={[team('Alpha Pod')]} />);

    expect(screen.queryByTestId('team-status-filter-all')).not.toBeInTheDocument();
    expect(screen.queryByTestId('team-status-filter-complete')).not.toBeInTheDocument();
    expect(screen.queryByTestId('team-status-filter-in_progress')).not.toBeInTheDocument();
  });
});

// Regression coverage: two DIFFERENT teams (different canonical ids) can
// share the exact same display name -- rendering both must never collide on
// React's own list `key` (which was previously bar.teamName), which would
// otherwise cause React to warn and, worse, only render/update one of them.
describe('CompletionByTeamPanel — duplicate team names, distinct ids', () => {
  it('renders both same-named teams as separate rows, keyed by their own distinct ids', () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
    try {
      render(
        <CompletionByTeamPanel
          teamStats={[
            team('Aurora', { teamId: 'aurora-1', completed: 1, total: 4 }),
            team('Aurora', { teamId: 'aurora-2', completed: 3, total: 4 }),
          ]}
        />,
      );

      expect(screen.getAllByText('Aurora')).toHaveLength(2);
      expect(screen.getAllByTestId('team-completion-row')).toHaveLength(2);
      const keyWarning = consoleError.mock.calls.some((args) =>
        args.some((arg) => typeof arg === 'string' && arg.includes('same key')),
      );
      expect(keyWarning).toBe(false);
    } finally {
      consoleError.mockRestore();
    }
  });
});

describe('CompletionByTeamPanel — search', () => {
  it('filters the team list as the user types, case-insensitively', async () => {
    const user = userEvent.setup();
    render(
      <CompletionByTeamPanel teamStats={[team('Sunnyvale'), team('Riverside'), team('Foothill')]} />,
    );

    await user.type(screen.getByTestId('team-bars-search-input'), 'river');

    expect(screen.queryByTestId('team-bars-empty-state')).not.toBeInTheDocument();
    expect(screen.getByTestId('team-bars-search-input')).toHaveValue('river');
    expect(screen.getByText('Riverside')).toBeInTheDocument();
    expect(screen.queryByText('Sunnyvale')).not.toBeInTheDocument();
  });

  it('shows the empty state when no team matches the search text', async () => {
    const user = userEvent.setup();
    render(<CompletionByTeamPanel teamStats={[team('Sunnyvale'), team('Riverside')]} />);

    await user.type(screen.getByTestId('team-bars-search-input'), 'nonexistent-team-name');

    expect(screen.getByTestId('team-bars-empty-state')).toHaveTextContent('No teams match your search.');
  });

  it('does not show the empty state before any search text is entered, even with teams present', () => {
    render(<CompletionByTeamPanel teamStats={[team('Sunnyvale')]} />);
    expect(screen.queryByTestId('team-bars-empty-state')).not.toBeInTheDocument();
  });

  it('clears the search text via the clear button, restoring the full list', async () => {
    const user = userEvent.setup();
    render(<CompletionByTeamPanel teamStats={[team('Sunnyvale'), team('Riverside')]} />);

    const input = screen.getByTestId('team-bars-search-input');
    await user.type(input, 'nonexistent');
    expect(screen.getByTestId('team-bars-empty-state')).toBeInTheDocument();

    await user.click(screen.getByTestId('team-bars-search-clear'));
    expect(input).toHaveValue('');
    expect(screen.queryByTestId('team-bars-empty-state')).not.toBeInTheDocument();
  });

  it('does not render the clear button when the search field is empty', () => {
    render(<CompletionByTeamPanel teamStats={[team('Sunnyvale')]} />);
    expect(screen.queryByTestId('team-bars-search-clear')).not.toBeInTheDocument();
  });

  it('applies search after the in-progress filter -- a same-named team from an excluded status never appears', async () => {
    const user = userEvent.setup();
    render(
      <CompletionByTeamPanel
        teamStats={[
          team('Riverside'),
          team('Riverside Annex', { status: 'complete', completed: 4, total: 4 }),
        ]}
      />,
    );

    // Searching "riverside" must only ever match the in_progress
    // "Riverside", never the complete "Riverside Annex", even though both
    // names match the search text.
    await user.type(screen.getByTestId('team-bars-search-input'), 'riverside');

    expect(screen.getByText('Riverside')).toBeInTheDocument();
    expect(screen.queryByText('Riverside Annex')).not.toBeInTheDocument();
  });
});

describe('CompletionByTeamPanel — sorting', () => {
  it('keeps ascending-percent sort on the filtered (in-progress-only) list', () => {
    render(
      <CompletionByTeamPanel
        teamStats={[
          team('High Pod', { completed: 90, total: 100 }),
          team('Low Pod', { completed: 10, total: 100 }),
          team('Mid Pod', { completed: 50, total: 100 }),
          team('Complete Pod', { status: 'complete', completed: 4, total: 4 }),
        ]}
      />,
    );

    const rows = screen.getAllByTestId('team-completion-row').map((row) => row.textContent);
    const lowIndex = rows.findIndex((text) => text?.includes('Low Pod'));
    const midIndex = rows.findIndex((text) => text?.includes('Mid Pod'));
    const highIndex = rows.findIndex((text) => text?.includes('High Pod'));

    expect(lowIndex).toBeGreaterThanOrEqual(0);
    expect(lowIndex).toBeLessThan(midIndex);
    expect(midIndex).toBeLessThan(highIndex);
  });
});

describe('CompletionByTeamPanel — scroll area', () => {
  it('keeps the team list inside its own internal scroll area, separate from the fixed header', () => {
    render(<CompletionByTeamPanel teamStats={[team('Sunnyvale')]} />);
    const scrollArea = screen.getByTestId('team-bars-scroll-area');
    expect(scrollArea.className).toMatch(/overflow-y-auto/);
    expect(scrollArea).not.toContainElement(screen.getByTestId('team-bars-search-input'));
  });

  it('keeps exactly one scroll area regardless of how many teams there are', () => {
    const manyTeams = Array.from({ length: 40 }, (_, i) => team(`Team ${i}`));
    render(<CompletionByTeamPanel teamStats={manyTeams} />);
    expect(screen.getAllByTestId('team-bars-scroll-area')).toHaveLength(1);
  });
});
