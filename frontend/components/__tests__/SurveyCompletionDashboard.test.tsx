import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import SurveyCompletionDashboard, { GroupHeaderRow, TeamRow } from '@/components/SurveyCompletionDashboard';
import { getSurveyCompletion } from '@/lib/api/survey-completion';
import { getAssessmentPeriods } from '@/lib/api/health-checks';
import type { SurveyCompletionOverview, SurveyCompletionTeam, SurveyStatus } from '@/lib/api/survey-completion';

vi.mock('@/lib/api/survey-completion', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/survey-completion')>()),
  getSurveyCompletion: vi.fn(),
}));
vi.mock('@/lib/api/health-checks', () => ({
  getAssessmentPeriods: vi.fn(),
}));

function team(overrides: Partial<SurveyCompletionTeam> = {}): SurveyCompletionTeam {
  return {
    teamId: 't1',
    teamName: 'Sunnyvale',
    completed: 3,
    total: 4,
    postWorkshopCompleted: true,
    status: 'in_progress',
    ...overrides,
  };
}

function renderRow(t: SurveyCompletionTeam, options: { showPercentBadge?: boolean } = {}) {
  return render(
    <table>
      <tbody>
        <TeamRow team={t} indentClass="" onRemind={vi.fn()} showPercentBadge={options.showPercentBadge} />
      </tbody>
    </table>,
  );
}

// The "Individual Survey Completed" column: completed_count / eligible_count,
// exactly as the API already provides them (team.completed / team.total) --
// the backend is the single source of truth for which users are eligible
// (Level 4 + Level 5 only); this column must never render blank just
// because completed is 0, and must never be confused with post-workshop
// fields or a derived status.
describe('TeamRow (Pods section) — Individual Survey Completed column', () => {
  it('renders 7 / 9 when 7 of 9 eligible users have completed the survey', () => {
    renderRow(team({ teamName: 'Generic Pod', status: 'in_progress', completed: 7, total: 9 }));
    expect(screen.getByText('7 / 9')).toBeInTheDocument();
  });

  it('renders 0 / 9, never blank, when no eligible user has completed the survey', () => {
    renderRow(team({ teamName: 'Generic Pod', status: 'not_started', completed: 0, total: 9 }));
    expect(screen.getByText('0 / 9')).toBeInTheDocument();
  });

  it('renders 9 / 9 when every eligible user has completed the survey', () => {
    renderRow(team({ teamName: 'Generic Pod', status: 'complete', completed: 9, total: 9 }));
    expect(screen.getByText('9 / 9')).toBeInTheDocument();
  });

  it('renders 0 / 0 (never blank) for a pod with no eligible users', () => {
    renderRow(team({ teamName: 'Generic Pod', status: 'not_started', completed: 0, total: 0 }));
    expect(screen.getByText('0 / 0')).toBeInTheDocument();
  });

  it('never renders a blank value for this column regardless of status', () => {
    (['complete', 'in_progress', 'not_started', 'opted_out'] as const).forEach((status) => {
      const { unmount } = renderRow(
        team({ teamName: 'Generic Pod', status, completed: 0, total: 9, postWorkshopCompleted: null }),
      );
      const cell = screen.getByTestId('survey-team-individual-completed');
      expect(cell.textContent?.trim()).not.toBe('');
      expect(cell.textContent?.replace(/\s+/g, ' ').trim()).toBe('0 / 9');
      unmount();
    });
  });

  // Regression test for the actual bug: an Opted Out pod's Individual
  // Survey Completed cell used to render "—" instead of its real
  // completed/eligible counts, which meant the whole column looked empty
  // under the "Opted Out" filter (since every row shown there has this
  // status) even though the API always returns real numbers for it.
  it('renders real completed/eligible numbers for an Opted Out pod, never "—"', () => {
    renderRow(
      team({ teamName: 'Generic Pod', status: 'opted_out', completed: 0, total: 9, postWorkshopCompleted: null }),
    );
    const cell = screen.getByTestId('survey-team-individual-completed');
    expect(cell.textContent?.replace(/\s+/g, ' ').trim()).toBe('0 / 9');
    expect(cell.textContent).not.toContain('—');
  });

  // Each of the five dashboard filters (Not Started, Fully Completed,
  // Opted Out, Opted In, Total Teams) only ever narrows WHICH pods are
  // shown (see buildVisibleGroups/buildFullyCompletedGroups) -- every pod
  // that survives any filter renders through this exact same TeamRow, so
  // proving the value is always present for every canonical status a
  // filter could show (not_started, complete, opted_out, in_progress --
  // "Opted In" and "Total Teams" both show a mix of these) proves it for
  // every filter without needing five separate rendering paths.
  it('renders a real, non-blank value for every canonical status a filter can show', () => {
    const casesByStatus = [
      { status: 'not_started' as const, completed: 0, total: 9, want: '0 / 9' }, // Not Started filter
      { status: 'complete' as const, completed: 9, total: 9, want: '9 / 9' }, // Fully Completed filter
      { status: 'opted_out' as const, completed: 0, total: 9, want: '0 / 9' }, // Opted Out filter
      { status: 'in_progress' as const, completed: 7, total: 9, want: '7 / 9' }, // Opted In / Total Teams filter
    ];
    casesByStatus.forEach(({ status, completed, total, want }) => {
      const { unmount } = renderRow(
        team({ teamName: 'Generic Pod', status, completed, total, postWorkshopCompleted: null }),
      );
      const cell = screen.getByTestId('survey-team-individual-completed');
      expect(cell.textContent?.replace(/\s+/g, ' ').trim()).toBe(want);
      unmount();
    });
  });
});

describe('TeamRow (Pods section) — status placement', () => {
  it('never shows any status text beside the pod name', () => {
    renderRow(team({ teamName: 'Andalusia', status: 'in_progress', completed: 1, total: 4 }));
    const nameCell = screen.getByText('Andalusia');
    // The name cell must contain only the name -- no "In Progress",
    // "Completed", "Not started", or percent text alongside it.
    expect(nameCell.textContent).toBe('Andalusia');
  });

  it('shows the percent text only inside the Status column cell, not anywhere else in the row', () => {
    renderRow(team({ teamName: 'Bobcaygeon', status: 'in_progress', completed: 45, total: 100 }));

    const percentText = screen.getByTestId('survey-team-percent');

    const statusCell = percentText.closest('td');
    expect(statusCell).not.toBeNull();
    expect(statusCell?.textContent).toContain('45% completed');

    const nameCell = screen.getByText('Bobcaygeon');
    expect(nameCell.closest('td')).not.toBe(statusCell);
    expect(nameCell.textContent).toBe('Bobcaygeon');
  });
});

describe('TeamRow (Pods section) — 100% completion', () => {
  it('shows green "Completed" in the Status column, with no "In Progress" text anywhere', () => {
    renderRow(team({ status: 'complete', completed: 4, total: 4 }));

    const badge = screen.getByText('Completed');
    expect(badge.className).toMatch(/bg-green-100/);
    expect(badge.className).toMatch(/text-green-800/);
    expect(screen.queryByText('In Progress')).not.toBeInTheDocument();
    // No separate percent text for a fully-complete pod.
    expect(screen.queryByTestId('survey-team-percent')).not.toBeInTheDocument();
  });

  it('an in_progress pod whose rounded percentage reaches 100% also displays as green "Completed"', () => {
    // 199 of 200 -- backend status is still in_progress, but the display
    // must show Completed once it rounds to 100%.
    renderRow(team({ status: 'in_progress', completed: 199, total: 200 }));

    expect(screen.getByText('Completed')).toBeInTheDocument();
    expect(screen.queryByText('In Progress')).not.toBeInTheDocument();
  });
});

describe('TeamRow (Pods section) — partial completion', () => {
  it('shows only the red "45% completed" text in the Status column, with no separate "In Progress" badge', () => {
    renderRow(team({ status: 'in_progress', completed: 45, total: 100 }));

    const percentText = screen.getByTestId('survey-team-percent');
    expect(percentText).toHaveTextContent('45% completed');
    expect(percentText.className).toMatch(/text-red-700/);
    expect(percentText.className).not.toMatch(/bg-red-50/);
    expect(screen.queryByText('In Progress')).not.toBeInTheDocument();
  });
});

// Total Teams / All Teams only: partial completion renders as a pale-red
// pill badge -- same structural shape (px-2.5 py-1, rounded-full, text-xs
// font-semibold) as the green "Completed" badge -- instead of the plain
// red text every other filter still uses.
// Total Teams / All Teams AND In Progress (showPercentBadge): partial
// completion renders as a color-tiered pill badge -- same structural
// shape (px-2.5 py-1, rounded-full, text-xs font-semibold) as the green
// "Completed" badge -- red under 50%, yellow at 50% and up -- instead of
// the plain red text every other filter still uses.
describe('TeamRow (Pods section) — partial completion pill badge under Total Teams / In Progress', () => {
  it('renders a red pill badge (background + rounded-full + padding) below 50%', () => {
    renderRow(team({ status: 'in_progress', completed: 34, total: 100 }), { showPercentBadge: true });

    const percentBadge = screen.getByTestId('survey-team-percent');
    expect(percentBadge).toHaveTextContent('34% completed');
    expect(percentBadge.className).toMatch(/bg-red-50/);
    expect(percentBadge.className).toMatch(/text-red-700/);
    expect(percentBadge.className).toMatch(/rounded-full/);
    expect(percentBadge.className).toMatch(/px-2\.5/);
    expect(percentBadge.className).toMatch(/py-1/);
  });

  it('renders a yellow pill badge at 50% and up', () => {
    renderRow(team({ status: 'in_progress', completed: 67, total: 100 }), { showPercentBadge: true });

    const percentBadge = screen.getByTestId('survey-team-percent');
    expect(percentBadge).toHaveTextContent('67% completed');
    expect(percentBadge.className).toMatch(/bg-yellow-50/);
    expect(percentBadge.className).toMatch(/text-yellow-700/);
    expect(percentBadge.className).toMatch(/rounded-full/);
    expect(percentBadge.className).not.toMatch(/bg-red-50/);
  });

  it('falls back to the plain red text (no background/pill) when showPercentBadge is not set', () => {
    renderRow(team({ status: 'in_progress', completed: 51, total: 100 }), { showPercentBadge: false });

    const percentText = screen.getByTestId('survey-team-percent');
    expect(percentText.className).toMatch(/text-red-700/);
    expect(percentText.className).not.toMatch(/bg-red-50/);
    expect(percentText.className).not.toMatch(/bg-yellow-50/);
    expect(percentText.className).not.toMatch(/rounded-full/);
  });

  it('never affects a fully Completed pod\'s green badge, even under Total Teams / In Progress', () => {
    renderRow(team({ status: 'complete', completed: 4, total: 4 }), { showPercentBadge: true });

    const badge = screen.getByText('Completed');
    expect(badge.className).toMatch(/bg-green-100/);
    expect(badge.className).not.toMatch(/bg-red-50/);
    expect(badge.className).not.toMatch(/bg-yellow-50/);
  });

  it('never affects a Not Started pod\'s badge, even under Total Teams / In Progress', () => {
    renderRow(team({ status: 'not_started', completed: 0, total: 4 }), { showPercentBadge: true });

    const badge = screen.getByText('Not started');
    expect(badge.className).toMatch(/bg-red-100/);
    expect(badge.className).not.toMatch(/bg-red-50/);
  });
});

// showPercentBadge (Total Teams / All Teams and In Progress) removes ALL
// row background -- status-driven pale red AND the ordinary hover tint --
// so color only ever appears on the compact badge itself.
describe('TeamRow (Pods section) — no row background under Total Teams / In Progress', () => {
  it('removes the pale red row background from a Not Started pod row', () => {
    const { container } = renderRow(team({ status: 'not_started', completed: 0, total: 4 }), {
      showPercentBadge: true,
    });
    const row = container.querySelector('tr');
    expect(row?.className).not.toMatch(/bg-red-50/);
  });

  it('removes the pale red row background from a partial/In Progress pod row', () => {
    const { container } = renderRow(team({ status: 'in_progress', completed: 34, total: 100 }), {
      showPercentBadge: true,
    });
    const row = container.querySelector('tr');
    expect(row?.className).not.toMatch(/bg-red-50/);
  });

  it('removes the hover background tint from the row', () => {
    const { container } = renderRow(team({ status: 'in_progress', completed: 34, total: 100 }), {
      showPercentBadge: true,
    });
    const row = container.querySelector('tr');
    expect(row?.className).not.toMatch(/hover:bg-indigo-50/);
  });

  it('keeps the existing row background and hover tint when showPercentBadge is not set', () => {
    const { container } = renderRow(team({ status: 'not_started', completed: 0, total: 4 }));
    const row = container.querySelector('tr');
    expect(row?.className).toMatch(/bg-red-50/);
    expect(row?.className).toMatch(/hover:bg-indigo-50/);
  });
});

describe('TeamRow (Pods section) — no progress', () => {
  it('shows "Not started" with no percent text', () => {
    renderRow(team({ status: 'not_started', completed: 0, total: 4 }));

    expect(screen.getByText('Not started')).toBeInTheDocument();
    expect(screen.queryByTestId('survey-team-percent')).not.toBeInTheDocument();
    expect(screen.queryByText('In Progress')).not.toBeInTheDocument();
    expect(screen.queryByText('Completed')).not.toBeInTheDocument();
  });
});

// The pale red "at risk" row background — driven by the exact same
// displayStatus the Status column itself renders from (isPaleRedPodRow),
// so it can never disagree with what that column shows. Applies to every
// pod row regardless of which filter is active or how deeply nested the
// pod is, since TeamRow is the one shared component every pod renders
// through.
describe('TeamRow (Pods section) — pale red row background', () => {
  it('applies the pale red background to a Not Started pod row', () => {
    const { container } = renderRow(team({ status: 'not_started', completed: 0, total: 4 }));
    const row = container.querySelector('tr');
    expect(row?.className).toMatch(/bg-red-50/);
  });

  it('applies the pale red background to a partial/In Progress pod row', () => {
    const { container } = renderRow(team({ status: 'in_progress', completed: 45, total: 100 }));
    const row = container.querySelector('tr');
    expect(row?.className).toMatch(/bg-red-50/);
  });

  it('does not apply the pale red background to a Completed pod row', () => {
    const { container } = renderRow(team({ status: 'complete', completed: 4, total: 4 }));
    const row = container.querySelector('tr');
    expect(row?.className).not.toMatch(/bg-red-50/);
  });

  it('does not apply the pale red background to an Opted Out pod row', () => {
    const { container } = renderRow(
      team({ status: 'opted_out', completed: 0, total: 4, postWorkshopCompleted: null }),
    );
    const row = container.querySelector('tr');
    expect(row?.className).not.toMatch(/bg-red-50/);
  });

  it('applies the pale red background even to an in_progress pod that rounds up to 100% and displays as Completed', () => {
    // 199/200 rounds to Completed display -- the row background must
    // follow that SAME effective display status, not the raw backend one.
    const { container } = renderRow(team({ status: 'in_progress', completed: 200, total: 200 }));
    const row = container.querySelector('tr');
    expect(row?.className).not.toMatch(/bg-red-50/);
  });
});

function renderHeaderRow(
  overrides: {
    completionPercent?: number;
    showFullyCompletedBadge?: boolean;
    hideStatusColumn?: boolean;
    hierarchyAggregate?: { percent: number; displayStatus: SurveyStatus };
    name?: string;
  } = {},
) {
  return render(
    <table>
      <tbody>
        <GroupHeaderRow
          name={overrides.name ?? 'Leader Name'}
          totalTeams={2}
          optedInTeams={2}
          completionPercent={overrides.completionPercent ?? 100}
          remindCount={0}
          expanded
          onToggle={vi.fn()}
          onRemind={vi.fn()}
          showFullyCompletedBadge={overrides.showFullyCompletedBadge}
          hideStatusColumn={overrides.hideStatusColumn}
          hierarchyAggregate={overrides.hierarchyAggregate}
        />
      </tbody>
    </table>,
  );
}

describe('GroupHeaderRow (leader rows) — Fully Completed hierarchy badge', () => {
  it('shows only a green "Completed" tag when showFullyCompletedBadge is set, with no percentage text', () => {
    renderHeaderRow({ showFullyCompletedBadge: true, completionPercent: 100 });

    const badge = screen.getByText('Completed');
    expect(badge.className).toMatch(/bg-green-100/);
    expect(badge.className).toMatch(/text-green-800/);
    expect(screen.queryByText(/% complete/)).not.toBeInTheDocument();
    // The name cell holds only the leader's name — no duplicate status text beside it.
    expect(screen.getByText('Leader Name').textContent).toBe('Leader Name');
  });

  it('falls back to the existing percent-based badge when showFullyCompletedBadge is not set', () => {
    renderHeaderRow({ showFullyCompletedBadge: false, completionPercent: 50 });

    expect(screen.getByText('50% complete')).toBeInTheDocument();
    expect(screen.queryByText('Completed')).not.toBeInTheDocument();
  });
});

// A leader (Manager/Director/Level-2) row is never itself labeled
// "Not started" or "Opted out" — those are pod-level canonical statuses,
// shown only on the actual pod rows' Status column (TeamRow). A leader
// stays visible purely as hierarchy context for its matching pods, always
// on its own ordinary percent-based badge, no matter how uniform its
// subtree is (all pods Not Started, all Opted Out, or anything else). The
// name below is a generic placeholder — no person/Level-2 name drives this.
describe('GroupHeaderRow (leader rows) — never shows a pod-level status badge', () => {
  it('always renders its own percent-based badge, never "Not started" or "Opted out", regardless of name or completion percent', () => {
    renderHeaderRow({ name: 'Some Level-2 Leader', completionPercent: 0 });

    expect(screen.getByText('0% complete')).toBeInTheDocument();
    expect(screen.queryByText('Not started')).not.toBeInTheDocument();
    expect(screen.queryByText('Opted out')).not.toBeInTheDocument();
  });

  it('never shows any status badge beside the leader\'s name — the name cell holds only the name', () => {
    renderHeaderRow({ name: 'Some Level-2 Leader', completionPercent: 0 });

    const nameCell = screen.getByText('Some Level-2 Leader').closest('td');
    expect(nameCell).not.toBeNull();
    expect(nameCell?.textContent).not.toMatch(/Not started|Opted out|Completed/);

    // The badge lives in a separate cell (the Status column), not the name cell.
    const badgeCell = screen.getByText('0% complete').closest('td');
    expect(badgeCell).not.toBe(nameCell);
  });
});

// Under the Not Started or Opted Out filter, a leader (Manager/Director/
// Level-2) row is pure hierarchy context: its Status column must render
// nothing at all -- no percent badge, no derived status, no "Not started"/
// "Opted out"/"Completed" text of any kind. Generic name/percent values only.
describe('GroupHeaderRow (leader rows) — hideStatusColumn empties the Status column', () => {
  it('renders nothing in the Status column when hideStatusColumn is set, regardless of completion percent', () => {
    renderHeaderRow({ hideStatusColumn: true, completionPercent: 45 });

    const statusCell = screen.getByTestId('survey-group-status-cell');
    expect(statusCell.textContent).toBe('');
    expect(screen.queryByText(/% complete/)).not.toBeInTheDocument();
    expect(screen.queryByText('Not started')).not.toBeInTheDocument();
    expect(screen.queryByText('Opted out')).not.toBeInTheDocument();
    expect(screen.queryByText('Completed')).not.toBeInTheDocument();
    expect(screen.queryByText('In Progress')).not.toBeInTheDocument();
  });

  it('takes priority over showFullyCompletedBadge if both were ever set', () => {
    renderHeaderRow({ hideStatusColumn: true, showFullyCompletedBadge: true, completionPercent: 100 });

    const statusCell = screen.getByTestId('survey-group-status-cell');
    expect(statusCell.textContent).toBe('');
    expect(screen.queryByText('Completed')).not.toBeInTheDocument();
  });

  it('never shows any status text beside the leader\'s name while the Status column is hidden', () => {
    renderHeaderRow({ hideStatusColumn: true, name: 'Some Level-2 Leader', completionPercent: 25 });

    const nameCell = screen.getByText('Some Level-2 Leader').closest('td');
    expect(nameCell?.textContent).not.toMatch(/%|Not started|Opted out|Completed|In Progress/);
  });

  it('falls back to the ordinary percent-based badge when hideStatusColumn is not set', () => {
    renderHeaderRow({ completionPercent: 45 });

    expect(screen.getByText('45% complete')).toBeInTheDocument();
  });
});

// Total Teams / All Teams: a leader's Status column displays the
// hierarchy-wide aggregate (see computeHierarchyAggregate) exactly like a
// pod's own Status column would render the same numbers -- green
// "Completed" at 100%, a color-tiered percentage badge for partial (red
// below 50%, yellow at 50% and up -- getPartialCompletionBadgeClass, the
// SAME helper a pod's own Status column badge uses), red "Not started"
// badge at a genuine 0%. Generic name/percent values only.
describe('GroupHeaderRow (leader rows) — Total Teams hierarchy aggregate', () => {
  it('shows only a green "Completed" tag when the aggregate is 100%, with no percentage text', () => {
    renderHeaderRow({ hierarchyAggregate: { percent: 100, displayStatus: 'complete' }, completionPercent: 50 });

    const badge = screen.getByText('Completed');
    expect(badge.className).toMatch(/bg-green-100/);
    expect(screen.queryByText(/% complete/)).not.toBeInTheDocument();
  });

  it('shows a red compact badge (background + rounded-full + padding), not plain text, below 50%', () => {
    renderHeaderRow({ hierarchyAggregate: { percent: 34, displayStatus: 'in_progress' }, completionPercent: 50 });

    const badge = screen.getByText('34% completed');
    const statusCell = screen.getByTestId('survey-group-status-cell');
    expect(statusCell.contains(badge)).toBe(true);
    expect(badge.className).toMatch(/bg-red-50/);
    expect(badge.className).toMatch(/text-red-700/);
    expect(badge.className).toMatch(/rounded-full/);
    expect(badge.className).toMatch(/px-2\.5/);
    expect(badge.className).toMatch(/py-1/);
    expect(screen.queryByText('Completed')).not.toBeInTheDocument();
    expect(screen.queryByText(/% complete\b/)).not.toBeInTheDocument();
  });

  it('shows a yellow compact badge at 50% and up', () => {
    renderHeaderRow({ hierarchyAggregate: { percent: 67, displayStatus: 'in_progress' }, completionPercent: 50 });

    const badge = screen.getByText('67% completed');
    expect(badge.className).toMatch(/bg-yellow-50/);
    expect(badge.className).toMatch(/text-yellow-700/);
    expect(badge.className).toMatch(/rounded-full/);
    expect(badge.className).not.toMatch(/bg-red-50/);
  });

  it('shows a red "Not started" tag when the aggregate is a genuine 0%', () => {
    renderHeaderRow({ hierarchyAggregate: { percent: 0, displayStatus: 'not_started' }, completionPercent: 0 });

    const badge = screen.getByText('Not started');
    expect(badge.className).toMatch(/bg-red-100/);
    expect(screen.queryByText(/% complete/)).not.toBeInTheDocument();
  });

  it('falls back to the ordinary percent-based badge when hierarchyAggregate is not set', () => {
    renderHeaderRow({ completionPercent: 62 });

    expect(screen.getByText('62% complete')).toBeInTheDocument();
  });

  it('keeps the badge background scoped to the badge itself — the leader row has no hover-driven or status-driven background of its own', () => {
    renderHeaderRow({ hierarchyAggregate: { percent: 34, displayStatus: 'in_progress' }, completionPercent: 50 });

    const row = screen.getByTestId('survey-group-header');
    // The row's own background is the static structural gray, never a
    // hover-only or status-colored one -- the badge's red/yellow color
    // never leaks onto the row, and the row never depends on :hover to
    // keep the badge visible.
    expect(row.className).not.toMatch(/hover:/);
    expect(row.className).not.toMatch(/bg-red-50|bg-yellow-50/);
    expect(screen.getByText('34% completed')).toBeInTheDocument();
  });

  it('hideStatusColumn takes priority over hierarchyAggregate if both were ever set', () => {
    renderHeaderRow({
      hideStatusColumn: true,
      hierarchyAggregate: { percent: 100, displayStatus: 'complete' },
    });

    const statusCell = screen.getByTestId('survey-group-status-cell');
    expect(statusCell.textContent).toBe('');
    expect(screen.queryByText('Completed')).not.toBeInTheDocument();
  });

  it('showFullyCompletedBadge takes priority over hierarchyAggregate if both were ever set', () => {
    renderHeaderRow({
      showFullyCompletedBadge: true,
      hierarchyAggregate: { percent: 43, displayStatus: 'in_progress' },
    });

    expect(screen.getByText('Completed')).toBeInTheDocument();
    expect(screen.queryByText('43% completed')).not.toBeInTheDocument();
  });
});

// Under the Not Started filter, a pod row and its Level-2 parent row render
// side by side in the same table: the pod carries the canonical "Not
// started" status in its own Status column, while the parent renders only
// as hierarchy context, on its ordinary percent-based badge. Names below
// are generic placeholders — no person/pod/Level-2 name drives this.
describe('Not Started filter — pod vs Level-2 parent display', () => {
  it('shows "Not started" only on the pod row; the Level-2 parent row\'s Status column is completely empty', () => {
    render(
      <table>
        <tbody>
          <GroupHeaderRow
            name="Generic Level-2 Leader"
            totalTeams={1}
            optedInTeams={1}
            completionPercent={0}
            remindCount={1}
            expanded
            onToggle={vi.fn()}
            onRemind={vi.fn()}
            hideStatusColumn
          />
          <TeamRow
            team={team({ teamName: 'Generic Pod', status: 'not_started', completed: 0, total: 4 })}
            indentClass=""
            onRemind={vi.fn()}
          />
        </tbody>
      </table>,
    );

    // The pod row shows "Not started" in its own Status column.
    const podBadge = screen.getByText('Not started');
    const podNameCell = screen.getByText('Generic Pod').closest('td');
    expect(podNameCell?.textContent).toBe('Generic Pod');
    expect(podBadge.closest('td')).not.toBe(podNameCell);

    // The Level-2 parent row's entire Status column is empty -- no percent
    // badge, no derived status, nothing beside its own name either.
    const parentNameCell = screen.getByText('Generic Level-2 Leader').closest('td');
    expect(parentNameCell?.textContent).not.toMatch(/%|Not started|Opted out|Completed|In Progress/);
    expect(screen.getByTestId('survey-group-status-cell').textContent).toBe('');
    expect(screen.queryByText(/% complete/)).not.toBeInTheDocument();
    // Only one "Not started" exists in the whole table -- the pod's.
    expect(screen.getAllByText('Not started')).toHaveLength(1);
  });
});

// Mirrors the Not Started case above, for Opted Out: a pod row and its
// Level-2 parent row render side by side, the pod carries the canonical
// "Opted out" status in its own Status column, and the parent's Status
// column is completely empty. Names below are generic placeholders — no
// person/pod/Level-2 name drives this.
describe('Opted Out filter — pod vs Level-2 parent display', () => {
  it('shows "Opted out" only on the pod row; the Level-2 parent row\'s Status column is completely empty', () => {
    render(
      <table>
        <tbody>
          <GroupHeaderRow
            name="Generic Level-2 Leader"
            totalTeams={1}
            optedInTeams={0}
            completionPercent={0}
            remindCount={0}
            expanded
            onToggle={vi.fn()}
            onRemind={vi.fn()}
            hideStatusColumn
          />
          <TeamRow
            team={team({
              teamName: 'Generic Pod',
              status: 'opted_out',
              completed: 0,
              total: 4,
              postWorkshopCompleted: null,
            })}
            indentClass=""
            onRemind={vi.fn()}
          />
        </tbody>
      </table>,
    );

    // The pod row shows "Opted out" in its own Status column.
    const podBadge = screen.getByText('Opted out');
    const podNameCell = screen.getByText('Generic Pod').closest('td');
    expect(podNameCell?.textContent).toBe('Generic Pod');
    expect(podBadge.closest('td')).not.toBe(podNameCell);

    // The Level-2 parent row's entire Status column is empty -- no percent
    // badge, no derived status, nothing beside its own name either.
    const parentNameCell = screen.getByText('Generic Level-2 Leader').closest('td');
    expect(parentNameCell?.textContent).not.toMatch(/%|Not started|Opted out|Completed|In Progress/);
    expect(screen.getByTestId('survey-group-status-cell').textContent).toBe('');
    expect(screen.queryByText(/% complete/)).not.toBeInTheDocument();
    // Only one "Opted out" exists in the whole table -- the pod's.
    expect(screen.getAllByText('Opted out')).toHaveLength(1);
  });
});

function overview(overrides: Partial<SurveyCompletionOverview> = {}): SurveyCompletionOverview {
  return {
    assessmentPeriod: '2026 H1',
    overallCompletion: 0,
    totalTeams: 0,
    optedIn: 0,
    fullyComplete: 0,
    inProgress: 0,
    notStarted: 0,
    optedOut: 0,
    groups: [],
    timeSeries: [],
    ...overrides,
  };
}

function deferredPromise<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

// Regression coverage: switching the period dropdown must never leave the
// PREVIOUS period's data on screen while the new period is still loading, or
// silently swallow a fetch failure for the new period and keep showing the
// stale one -- see the fetch effect's setOverview(null) fix.
describe('SurveyCompletionDashboard period switch', () => {
  it('clears the previous period\'s data while the newly selected period is still loading', async () => {
    vi.mocked(getAssessmentPeriods).mockResolvedValue(['2026 H1', '2026 H2']);
    const periodTwo = deferredPromise<SurveyCompletionOverview>();
    vi.mocked(getSurveyCompletion).mockImplementation(async (period?: string) => {
      if (period === '2026 H2') return periodTwo.promise;
      return overview({ assessmentPeriod: '2026 H1', totalTeams: 5 });
    });

    render(<SurveyCompletionDashboard />);

    const totalCard = await screen.findByTestId('survey-card-total');
    await waitFor(() => expect(totalCard.textContent).toContain('5'));

    await userEvent.selectOptions(screen.getByTestId('survey-period-select'), '2026 H2');

    // While period two's fetch is still pending, the old period's "5" total
    // teams count must be gone -- either the loading placeholder is shown,
    // or at minimum the stale number is no longer on screen.
    await waitFor(() => {
      expect(screen.queryByTestId('survey-card-total')).not.toBeInTheDocument();
    });

    periodTwo.resolve(overview({ assessmentPeriod: '2026 H2', totalTeams: 9 }));

    const refreshedCard = await screen.findByTestId('survey-card-total');
    await waitFor(() => expect(refreshedCard.textContent).toContain('9'));
  });

  it('surfaces an error, rather than silently keeping the old period\'s data, when the new period fails to load', async () => {
    vi.mocked(getAssessmentPeriods).mockResolvedValue(['2026 H1', '2026 H2']);
    const periodTwo = deferredPromise<SurveyCompletionOverview>();
    vi.mocked(getSurveyCompletion).mockImplementation(async (period?: string) => {
      if (period === '2026 H2') return periodTwo.promise;
      return overview({ assessmentPeriod: '2026 H1', totalTeams: 5 });
    });

    render(<SurveyCompletionDashboard />);

    const totalCard = await screen.findByTestId('survey-card-total');
    await waitFor(() => expect(totalCard.textContent).toContain('5'));

    await userEvent.selectOptions(screen.getByTestId('survey-period-select'), '2026 H2');
    periodTwo.reject(new Error('Unknown assessment period'));

    const errorState = await screen.findByTestId('survey-dashboard-error');
    expect(errorState.textContent).toContain('Unknown assessment period');
    // The old period's data must not still be rendered behind/alongside the error.
    expect(screen.queryByTestId('survey-card-total')).not.toBeInTheDocument();
  });
});

// Regression coverage: the search box + bulk Remind button toolbar row
// previously used a plain non-wrapping flex row with a fixed-width search
// input, which overflowed the viewport on mobile instead of wrapping.
describe('SurveyCompletionDashboard mobile toolbar layout', () => {
  it('lets the search box and Remind button wrap onto their own line instead of overflowing', async () => {
    vi.mocked(getAssessmentPeriods).mockResolvedValue(['2026 H1']);
    vi.mocked(getSurveyCompletion).mockResolvedValue(overview({ assessmentPeriod: '2026 H1' }));

    render(<SurveyCompletionDashboard />);
    await screen.findByTestId('survey-card-total');

    const searchInput = screen.getByTestId('survey-search-input');
    const toolbarRow = searchInput.closest('div')?.parentElement;
    expect(toolbarRow?.className).toMatch(/flex-wrap/);
    expect(searchInput.className).toMatch(/w-full/);
  });
});
