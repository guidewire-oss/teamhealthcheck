import { describe, expect, it } from 'vitest';
import { buildVisibleGroups, isRemindable, matchesCardFilter } from '@/lib/survey-completion-tree';
import type { SurveyCompletionGroup, SurveyCompletionTeam } from '@/lib/api/survey-completion';

function team(overrides: Partial<SurveyCompletionTeam> = {}): SurveyCompletionTeam {
  return {
    teamId: 't1',
    teamName: 'Sunnyvale',
    completed: 3,
    total: 3,
    postWorkshopCompleted: true,
    status: 'complete',
    ...overrides,
  };
}

const groups: SurveyCompletionGroup[] = [
  {
    type: 'director',
    director: { id: 'dir1', name: 'Dana Director' },
    totalTeams: 2,
    optedInTeams: 2,
    completionPercent: 50,
    remindCount: 1,
    directTeams: [team({ teamId: 'a', teamName: 'Sunnyvale' })],
    managers: [
      {
        manager: { id: 'mgr1', name: 'Zed Manager' },
        totalTeams: 1,
        optedInTeams: 1,
        completionPercent: 0,
        remindCount: 1,
        teams: [team({ teamId: 'b', teamName: 'Riverside', status: 'in_progress' })],
      },
    ],
  },
  {
    type: 'manager',
    manager: { id: 'mgr2', name: 'Mo Manager' },
    totalTeams: 1,
    optedInTeams: 1,
    completionPercent: 100,
    remindCount: 0,
    teams: [team({ teamId: 'c', teamName: 'Lakeside' })],
  },
  {
    type: 'other',
    label: 'Other',
    totalTeams: 1,
    optedInTeams: 1,
    completionPercent: 0,
    remindCount: 1,
    teams: [team({ teamId: 'd', teamName: 'Foothill', status: 'not_started', teamLeadId: 'lead1', teamLeadName: 'Lee Lead' })],
  },
];

describe('buildVisibleGroups', () => {
  it('with no query or filter, returns all three groups with their teams intact', () => {
    const visible = buildVisibleGroups(groups, '', 'all');
    expect(visible).toHaveLength(3);

    const director = visible[0];
    expect(director.type).toBe('director');
    if (director.type !== 'director') throw new Error('expected director');
    expect(director.visibleDirectTeams.map((t) => t.teamId)).toEqual(['a']);
    expect(director.visibleManagers).toHaveLength(1);
    expect(director.visibleManagers[0].visibleTeams.map((t) => t.teamId)).toEqual(['b']);

    const manager = visible[1];
    expect(manager.type).toBe('manager');
    const other = visible[2];
    expect(other.type).toBe('other');
  });

  it('matches by director name and shows all of that director\'s teams, including nested manager teams', () => {
    const visible = buildVisibleGroups(groups, 'Dana', 'all');
    expect(visible).toHaveLength(1);
    const director = visible[0];
    if (director.type !== 'director') throw new Error('expected director');
    expect(director.visibleDirectTeams.map((t) => t.teamId)).toEqual(['a']);
    expect(director.visibleManagers[0].visibleTeams.map((t) => t.teamId)).toEqual(['b']);
  });

  it('matches by nested manager name and shows only that manager\'s subtree, not the director\'s direct teams', () => {
    const visible = buildVisibleGroups(groups, 'Zed', 'all');
    expect(visible).toHaveLength(1);
    const director = visible[0];
    if (director.type !== 'director') throw new Error('expected director');
    expect(director.visibleDirectTeams).toHaveLength(0);
    expect(director.visibleManagers).toHaveLength(1);
  });

  it('matches by top-level manager name', () => {
    const visible = buildVisibleGroups(groups, 'Mo Manager', 'all');
    expect(visible).toHaveLength(1);
    expect(visible[0].type).toBe('manager');
  });

  it('matches the "Other" label', () => {
    const visible = buildVisibleGroups(groups, 'other', 'all');
    expect(visible).toHaveLength(1);
    expect(visible[0].type).toBe('other');
  });

  it('matches a team name directly, regardless of which group it is in', () => {
    const visible = buildVisibleGroups(groups, 'Foothill', 'all');
    expect(visible).toHaveLength(1);
    expect(visible[0].type).toBe('other');
  });

  it('drops groups left with no visible teams after a card filter is applied', () => {
    const visible = buildVisibleGroups(groups, '', 'not_started');
    // Only the "Other" group's team is not_started.
    expect(visible).toHaveLength(1);
    expect(visible[0].type).toBe('other');
  });

  it('returns nothing when the query matches no director, manager, "Other", or team name', () => {
    expect(buildVisibleGroups(groups, 'nonexistent', 'all')).toHaveLength(0);
  });
});

describe('isRemindable', () => {
  it('is true for in_progress and not_started, false otherwise', () => {
    expect(isRemindable(team({ status: 'in_progress' }))).toBe(true);
    expect(isRemindable(team({ status: 'not_started' }))).toBe(true);
    expect(isRemindable(team({ status: 'complete' }))).toBe(false);
    expect(isRemindable(team({ status: 'opted_out' }))).toBe(false);
  });
});

describe('matchesCardFilter', () => {
  it('optedIn excludes opted_out teams only', () => {
    expect(matchesCardFilter(team({ status: 'opted_out' }), 'optedIn')).toBe(false);
    expect(matchesCardFilter(team({ status: 'complete' }), 'optedIn')).toBe(true);
  });

  it('a specific status filter matches only that status', () => {
    expect(matchesCardFilter(team({ status: 'complete' }), 'complete')).toBe(true);
    expect(matchesCardFilter(team({ status: 'in_progress' }), 'complete')).toBe(false);
  });
});
