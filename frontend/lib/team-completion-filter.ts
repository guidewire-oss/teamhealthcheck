/**
 * Pure filtering logic for the "Completion by team" card's search box.
 * Split out from OverallAnalyticsView so it can be unit-tested without
 * rendering anything — same pattern as lib/survey-completion-tree.ts.
 */

import type { SurveyCompletionData, SurveyStatus } from '@/components/SurveyCompletionDashboard';

// Same palette used for the status pills/badges in the "All teams" table
// and the status breakdown pie chart.
const STATUS_CHART_COLOR: Record<SurveyStatus, string> = {
  complete: '#10B981',
  in_progress: '#F59E0B',
  not_started: '#EF4444',
  opted_out: '#9CA3AF',
};

/**
 * Case-insensitive, whitespace-trimmed substring match on team name. An
 * empty (or whitespace-only) query returns every team unchanged.
 */
export function filterTeamStats(
  teamStats: SurveyCompletionData['teamStats'],
  query: string,
): SurveyCompletionData['teamStats'] {
  const q = query.trim().toLowerCase();
  if (!q) return teamStats;
  return teamStats.filter((team) => team.teamName.toLowerCase().includes(q));
}

/**
 * The "Completion by team" card/fullscreen view's status filter. "all"
 * means "every status this chart can meaningfully show a percentage for"
 * -- complete and in_progress -- not literally every backend status: the
 * chart has no dedicated filter for not_started or opted_out, and
 * buildCompletionBars already excludes opted_out (and zero-member teams)
 * regardless of this filter, so showing them under "All" here would be
 * misleading (no way to isolate or exclude them again).
 */
export type TeamStatusFilter = 'all' | 'complete' | 'in_progress';

/**
 * Applies the selected status filter -- the first step of the pipeline,
 * run before search and before buildCompletionBars' own sort. Complete,
 * not-started, and opted-out teams are excluded entirely (not just
 * visually hidden) whenever they don't match the filter, so search and
 * the visible count both operate on this narrowed set.
 */
export function filterTeamsByStatus(
  teamStats: SurveyCompletionData['teamStats'],
  filter: TeamStatusFilter,
): SurveyCompletionData['teamStats'] {
  if (filter === 'all') {
    return teamStats.filter((team) => team.status === 'complete' || team.status === 'in_progress');
  }
  return teamStats.filter((team) => team.status === filter);
}

// The only two statuses this chart's rows ever display -- "Completion by
// team" only ever shows complete/in_progress teams to begin with (see
// filterTeamsByStatus), and buildCompletionBars' own effective-status rule
// below never produces anything else.
export type CompletionBarStatus = 'complete' | 'in_progress';

export interface CompletionBar {
  teamId: string;
  teamName: string;
  percent: number;
  completed: number;
  total: number;
  status: CompletionBarStatus;
  color: string;
}

/**
 * Builds the "Completion by team" list's data from whatever teams the
 * caller has already filtered down to (status filter, then search) --
 * excludes opted-out teams and teams with no members (no meaningful
 * percentage to show), computes each team's rounded completion percentage,
 * and sorts the result by that percentage ascending (lowest first,
 * highest last). Sorting runs last, after every exclusion above, so it
 * always orders exactly the set that ends up on screen.
 *
 * `status` here is an EFFECTIVE display status, derived purely from the
 * rounded percentage (percent >= 100 -> "complete", otherwise
 * "in_progress") -- not copied straight from the team's own backend
 * status. This matters for one edge case: rounding can make a team that's
 * technically still in_progress (e.g. 199 of 200 members complete, which
 * rounds to 100%) display as fully done. Showing "100% completed" next to
 * an "In Progress" badge would look like a contradiction, so that case is
 * treated the same as a genuinely complete team -- both the badge and the
 * bar color flip to green. This never changes an already-complete team
 * (completed === total already rounds to exactly 100) and never invents a
 * new status: the result is always one of the two existing values.
 */
export function buildCompletionBars(
  teamStats: SurveyCompletionData['teamStats'],
): CompletionBar[] {
  return teamStats
    .filter((team) => team.status !== 'opted_out' && team.total > 0)
    .map((team) => {
      const percent = Math.round((team.completed / team.total) * 100);
      const status: CompletionBarStatus = percent >= 100 ? 'complete' : 'in_progress';
      return {
        teamId: team.teamId,
        teamName: team.teamName,
        percent,
        completed: team.completed,
        total: team.total,
        status,
        color: STATUS_CHART_COLOR[status],
      };
    })
    .sort((a, b) => a.percent - b.percent);
}

/**
 * The "Completion by team/pod" list's own bars, narrowed to ONLY
 * genuinely in-progress teams.
 *
 * First filters to canonical status "in_progress" (filterTeamsByStatus) --
 * never inferred from a percentage alone, so a Not Started team (which is
 * always at 0% but is NOT "in progress") and an Opted Out team are both
 * excluded by their own canonical status, exactly like every other status
 * display in this app. Then reuses buildCompletionBars' own
 * effective-status rule (the same 100%-rounding edge case applied
 * everywhere else) to additionally drop a raw in_progress team whose
 * percentage happens to round up to 100% (e.g. 199 of 200 members) --
 * excluded here exactly like a genuinely complete team, instead of
 * showing up mislabeled green.
 */
export function buildInProgressBars(
  teamStats: SurveyCompletionData['teamStats'],
): CompletionBar[] {
  const inProgressOnly = filterTeamsByStatus(teamStats, 'in_progress');
  return buildCompletionBars(inProgressOnly).filter((bar) => bar.status === 'in_progress');
}
