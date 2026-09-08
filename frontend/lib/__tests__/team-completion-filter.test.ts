import { describe, expect, it } from 'vitest';
import {
  buildCompletionBars,
  buildInProgressBars,
  filterTeamStats,
  filterTeamsByStatus,
} from '@/lib/team-completion-filter';
import type { SurveyCompletionData } from '@/components/SurveyCompletionDashboard';

function stats(): SurveyCompletionData['teamStats'] {
  return [
    { teamId: 'Sunnyvale', teamName: 'Sunnyvale', completed: 3, total: 3, status: 'complete' },
    { teamId: 'Riverside', teamName: 'Riverside', completed: 1, total: 4, status: 'in_progress' },
    { teamId: 'Foothill', teamName: 'Foothill', completed: 0, total: 2, status: 'not_started' },
    { teamId: 'Lakeside', teamName: 'Lakeside', completed: 0, total: 3, status: 'opted_out' },
  ];
}

describe('filterTeamStats', () => {
  it('returns every team unchanged when the query is empty', () => {
    expect(filterTeamStats(stats(), '')).toEqual(stats());
  });

  it('returns every team unchanged when the query is only whitespace', () => {
    expect(filterTeamStats(stats(), '   ')).toEqual(stats());
  });

  it('matches case-insensitively', () => {
    const result = filterTeamStats(stats(), 'RIVER');
    expect(result.map((t) => t.teamName)).toEqual(['Riverside']);
  });

  it('matches a substring anywhere in the name, not just a prefix', () => {
    const result = filterTeamStats(stats(), 'side');
    expect(result.map((t) => t.teamName)).toEqual(['Riverside', 'Lakeside']);
  });

  it('trims leading/trailing whitespace from the query before matching', () => {
    const result = filterTeamStats(stats(), '  sunny  ');
    expect(result.map((t) => t.teamName)).toEqual(['Sunnyvale']);
  });

  it('returns an empty array when nothing matches', () => {
    expect(filterTeamStats(stats(), 'nonexistent')).toEqual([]);
  });

  it('preserves each team\'s completed/total/status fields unchanged', () => {
    const result = filterTeamStats(stats(), 'riverside');
    expect(result[0]).toEqual({ teamId: 'Riverside', teamName: 'Riverside', completed: 1, total: 4, status: 'in_progress' });
  });
});

describe('filterTeamsByStatus', () => {
  it('"in_progress" keeps only teams whose status is in_progress', () => {
    const result = filterTeamsByStatus(stats(), 'in_progress');
    expect(result.map((t) => t.teamName)).toEqual(['Riverside']);
  });

  it('"complete" keeps only teams whose status is complete', () => {
    const result = filterTeamsByStatus(stats(), 'complete');
    expect(result.map((t) => t.teamName)).toEqual(['Sunnyvale']);
  });

  it('"all" keeps complete and in_progress teams, excluding not_started and opted_out', () => {
    const result = filterTeamsByStatus(stats(), 'all');
    expect(result.map((t) => t.teamName).sort()).toEqual(['Riverside', 'Sunnyvale']);
  });

  it('never mixes teams from other statuses into a selected filter', () => {
    const inProgressOnly = filterTeamsByStatus(stats(), 'in_progress');
    expect(inProgressOnly.every((t) => t.status === 'in_progress')).toBe(true);

    const completeOnly = filterTeamsByStatus(stats(), 'complete');
    expect(completeOnly.every((t) => t.status === 'complete')).toBe(true);
  });

  it('returns an empty array when no team matches the filter', () => {
    const noInProgress = stats().filter((t) => t.status !== 'in_progress');
    expect(filterTeamsByStatus(noInProgress, 'in_progress')).toEqual([]);
  });

  it('preserves each surviving team\'s fields unchanged', () => {
    const result = filterTeamsByStatus(stats(), 'in_progress');
    expect(result[0]).toEqual({ teamId: 'Riverside', teamName: 'Riverside', completed: 1, total: 4, status: 'in_progress' });
  });
});

describe('filterTeamStats composed with filterTeamsByStatus', () => {
  it('search only ever matches within the status-filtered subset, never an excluded team', () => {
    const withMatchingCompleteTeam = [
      ...stats(),
      { teamId: 'Riverside Annex', teamName: 'Riverside Annex', completed: 5, total: 5, status: 'complete' as const },
    ];
    const inProgressOnly = filterTeamsByStatus(withMatchingCompleteTeam, 'in_progress');
    const result = filterTeamStats(inProgressOnly, 'river');
    // "Riverside Annex" matches the search text too, but it's complete, not
    // in_progress, so it must never appear here.
    expect(result.map((t) => t.teamName)).toEqual(['Riverside']);
  });
});

describe('buildCompletionBars', () => {
  function inProgressTeams(): SurveyCompletionData['teamStats'] {
    return [
      { teamId: 'Alpha', teamName: 'Alpha', completed: 1, total: 4, status: 'in_progress' }, // 25%
      { teamId: 'Bravo', teamName: 'Bravo', completed: 3, total: 4, status: 'in_progress' }, // 75%
      { teamId: 'Charlie', teamName: 'Charlie', completed: 1, total: 2, status: 'in_progress' }, // 50%
    ];
  }

  it('sorts ascending by completion percentage -- lowest first, highest last', () => {
    const bars = buildCompletionBars(inProgressTeams());
    expect(bars.map((b) => b.teamName)).toEqual(['Alpha', 'Charlie', 'Bravo']);
    expect(bars.map((b) => b.percent)).toEqual([25, 50, 75]);
  });

  it('does not mutate the input array (returns a new sorted array)', () => {
    const input = inProgressTeams();
    const originalOrder = input.map((t) => t.teamName);
    buildCompletionBars(input);
    expect(input.map((t) => t.teamName)).toEqual(originalOrder);
  });

  it('excludes opted-out teams and teams with zero members', () => {
    const withExclusions: SurveyCompletionData['teamStats'] = [
      ...inProgressTeams(),
      { teamId: 'OptedOut', teamName: 'OptedOut', completed: 0, total: 4, status: 'opted_out' },
      { teamId: 'NoMembers', teamName: 'NoMembers', completed: 0, total: 0, status: 'in_progress' },
    ];
    const bars = buildCompletionBars(withExclusions);
    expect(bars.map((b) => b.teamName)).not.toContain('OptedOut');
    expect(bars.map((b) => b.teamName)).not.toContain('NoMembers');
    expect(bars).toHaveLength(3);
  });

  it('sorts the full pipeline result: in-progress filter, then search, then ascending sort', () => {
    const withOthers: SurveyCompletionData['teamStats'] = [
      ...inProgressTeams(),
      { teamId: 'Delta', teamName: 'Delta', completed: 4, total: 4, status: 'complete' }, // would be 100%, excluded by status
      { teamId: 'Alpha Annex', teamName: 'Alpha Annex', completed: 0, total: 4, status: 'in_progress' }, // 0%, excluded by search
    ];
    const inProgressOnly = filterTeamsByStatus(withOthers, 'in_progress');
    const searched = filterTeamStats(inProgressOnly, 'alpha');
    const bars = buildCompletionBars(searched);
    // "Delta" is excluded by status (not in_progress) before search even
    // runs. "Alpha" and "Alpha Annex" both match the search text and are
    // both in_progress, so both survive -- ordered ascending by percent.
    expect(bars.map((b) => b.teamName)).toEqual(['Alpha Annex', 'Alpha']);
    expect(bars.map((b) => b.percent)).toEqual([0, 25]);
  });
});

// The "Completion by team/pod" section's own bars: genuinely in-progress
// teams only, reusing buildCompletionBars' effective-status rule rather
// than re-deriving it. Fixtures use generic placeholder names.
describe('buildInProgressBars', () => {
  it('includes an in_progress team', () => {
    const bars = buildInProgressBars([{ teamId: 'Alpha', teamName: 'Alpha', completed: 1, total: 4, status: 'in_progress' }]);
    expect(bars.map((b) => b.teamName)).toEqual(['Alpha']);
    expect(bars[0].status).toBe('in_progress');
  });

  it('excludes a genuinely complete team', () => {
    const bars = buildInProgressBars([
      { teamId: 'Alpha', teamName: 'Alpha', completed: 1, total: 4, status: 'in_progress' },
      { teamId: 'Beta', teamName: 'Beta', completed: 4, total: 4, status: 'complete' },
    ]);
    expect(bars.map((b) => b.teamName)).toEqual(['Alpha']);
  });

  it('excludes an in_progress team whose percentage rounds up to exactly 100%', () => {
    const bars = buildInProgressBars([
      { teamId: 'Alpha', teamName: 'Alpha', completed: 1, total: 4, status: 'in_progress' },
      // Raw status in_progress (completed < total), but rounds to 100%.
      { teamId: 'RoundsUp', teamName: 'RoundsUp', completed: 199, total: 200, status: 'in_progress' },
    ]);
    expect(bars.map((b) => b.teamName)).toEqual(['Alpha']);
  });

  it('excludes not_started and opted_out teams', () => {
    const bars = buildInProgressBars([
      { teamId: 'Alpha', teamName: 'Alpha', completed: 1, total: 4, status: 'in_progress' },
      { teamId: 'NotStarted', teamName: 'NotStarted', completed: 0, total: 4, status: 'not_started' },
      { teamId: 'OptedOut', teamName: 'OptedOut', completed: 0, total: 4, status: 'opted_out' },
    ]);
    expect(bars.map((b) => b.teamName)).toEqual(['Alpha']);
  });

  it('returns an empty array when nothing is genuinely in progress', () => {
    const bars = buildInProgressBars([
      { teamId: 'Beta', teamName: 'Beta', completed: 4, total: 4, status: 'complete' },
      { teamId: 'RoundsUp', teamName: 'RoundsUp', completed: 199, total: 200, status: 'in_progress' },
    ]);
    expect(bars).toEqual([]);
  });

  it('keeps ascending-percent sort across multiple in-progress teams', () => {
    const bars = buildInProgressBars([
      { teamId: 'High', teamName: 'High', completed: 90, total: 100, status: 'in_progress' },
      { teamId: 'Low', teamName: 'Low', completed: 10, total: 100, status: 'in_progress' },
      { teamId: 'Mid', teamName: 'Mid', completed: 50, total: 100, status: 'in_progress' },
    ]);
    expect(bars.map((b) => b.teamName)).toEqual(['Low', 'Mid', 'High']);
  });
});
