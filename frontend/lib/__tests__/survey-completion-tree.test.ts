import { describe, expect, it } from 'vitest';
import {
  buildFullyCompletedGroups,
  buildHierarchyAggregateIndex,
  buildVisibleGroups,
  buildVisibleHierarchyAggregateIndex,
  collectDescendantPods,
  computeHierarchyAggregate,
  computeVisibleGroupAggregate,
  countVisibleTeams,
  getPartialCompletionBadgeClass,
  getPodCompletionPercent,
  getPodDisplayStatus,
  isPaleRedPodRow,
  isParentStatusHiddenFor,
  isPersonSubtreeFullyCompleted,
  isPodFullyCompleted,
  isRemindable,
  isRowBackgroundHiddenFor,
  matchesCardFilter,
} from '@/lib/survey-completion-tree';
import type {
  SurveyCompletionGroup,
  SurveyCompletionPerson,
  SurveyCompletionPersonGroup,
  SurveyCompletionTeam,
} from '@/lib/api/survey-completion';

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

function person(id: string, name: string, level: string): SurveyCompletionPerson {
  return { id, name, level, levelId: level.toLowerCase().replace(/\s+/g, '-') };
}

// Mirrors the backend's own fixture: a Senior Director with a direct pod, a
// Senior Manager reporting straight to them (skipping the Director tier),
// and a Director with their own Manager nested underneath — plus a second,
// unrelated root and an unassigned pod under "Other".
const managerD: SurveyCompletionPersonGroup = {
  type: 'person',
  person: person('mgr-d', 'Manager D', 'Manager'),
  totalTeams: 1,
  optedInTeams: 1,
  completionPercent: 100,
  remindCount: 0,
  directTeams: [team({ teamId: 'gamma', teamName: 'Team Gamma' })],
  children: [],
};

const directorB: SurveyCompletionPersonGroup = {
  type: 'person',
  person: person('dir-b', 'Director B', 'Director'),
  totalTeams: 2,
  optedInTeams: 2,
  completionPercent: 50,
  remindCount: 1,
  directTeams: [team({ teamId: 'beta', teamName: 'Team Beta', status: 'in_progress', completed: 1, total: 4 })],
  children: [managerD],
};

const seniorManagerC: SurveyCompletionPersonGroup = {
  type: 'person',
  person: person('sm-c', 'Senior Manager C', 'Senior Manager'),
  totalTeams: 1,
  optedInTeams: 1,
  completionPercent: 0,
  remindCount: 1,
  directTeams: [team({ teamId: 'delta', teamName: 'Team Delta', status: 'not_started', completed: 0, total: 5 })],
  children: [],
};

const seniorDirectorA: SurveyCompletionPersonGroup = {
  type: 'person',
  person: person('sd-a', 'Senior Director A', 'Senior Director'),
  totalTeams: 4,
  optedInTeams: 4,
  completionPercent: 50,
  remindCount: 2,
  directTeams: [team({ teamId: 'alpha', teamName: 'Team Alpha' })],
  children: [directorB, seniorManagerC],
};

const seniorDirectorE: SurveyCompletionPersonGroup = {
  type: 'person',
  person: person('sd-e', 'Senior Director E', 'Senior Director'),
  totalTeams: 1,
  optedInTeams: 1,
  completionPercent: 100,
  remindCount: 0,
  directTeams: [team({ teamId: 'epsilon', teamName: 'Team Epsilon' })],
  children: [],
};

const groups: SurveyCompletionGroup[] = [
  seniorDirectorA,
  seniorDirectorE,
  {
    type: 'other',
    label: 'Other',
    totalTeams: 1,
    optedInTeams: 1,
    completionPercent: 0,
    remindCount: 1,
    teams: [
      team({
        teamId: 'zeta',
        teamName: 'Team Zeta',
        status: 'not_started',
        completed: 0,
        total: 1,
        teamLeadId: 'lead1',
        teamLeadName: 'Lead Zeta',
      }),
    ],
  },
];

describe('buildVisibleGroups', () => {
  it('with no query or filter, returns both Senior Director roots plus Other, each with their full subtree', () => {
    const visible = buildVisibleGroups(groups, '', 'all');
    expect(visible).toHaveLength(3);

    const sdA = visible[0];
    expect(sdA.type).toBe('person');
    if (sdA.type !== 'person') throw new Error('expected person');
    expect(sdA.visibleDirectTeams.map((t) => t.teamId)).toEqual(['alpha']);
    expect(sdA.visibleChildren.map((c) => c.name)).toEqual(['Director B', 'Senior Manager C']);

    const directorBVisible = sdA.visibleChildren[0];
    if (directorBVisible.type !== 'person') throw new Error('expected person');
    expect(directorBVisible.visibleDirectTeams.map((t) => t.teamId)).toEqual(['beta']);
    expect(directorBVisible.visibleChildren.map((c) => c.name)).toEqual(['Manager D']);

    expect(visible[1].name).toBe('Senior Director E');
    expect(visible[2].type).toBe('other');
  });

  it('matching a deeply nested leader (Manager D) auto-expands only that branch, pruning the unrelated sibling', () => {
    const visible = buildVisibleGroups(groups, 'Manager D', 'all');
    expect(visible).toHaveLength(1);
    const sdA = visible[0];
    if (sdA.type !== 'person') throw new Error('expected person');
    // Senior Director A itself doesn't match and has no matching direct pod.
    expect(sdA.visibleDirectTeams).toHaveLength(0);
    // Only the Director B -> Manager D path survives; Senior Manager C (no match) is pruned.
    expect(sdA.visibleChildren).toHaveLength(1);
    expect(sdA.visibleChildren[0].name).toBe('Director B');
    const directorBVisible = sdA.visibleChildren[0];
    if (directorBVisible.type !== 'person') throw new Error('expected person');
    expect(directorBVisible.visibleDirectTeams).toHaveLength(0);
    expect(directorBVisible.visibleChildren).toHaveLength(1);
    expect(directorBVisible.visibleChildren[0].name).toBe('Manager D');
  });

  it('a name match on a root shows its whole subtree, including a Senior Manager that skips the Director tier', () => {
    const visible = buildVisibleGroups(groups, 'Senior Director A', 'all');
    expect(visible).toHaveLength(1);
    const sdA = visible[0];
    if (sdA.type !== 'person') throw new Error('expected person');
    expect(sdA.visibleDirectTeams.map((t) => t.teamId)).toEqual(['alpha']);
    expect(sdA.visibleChildren).toHaveLength(2);
  });

  it('a query matching both Senior Directors by their shared title returns both roots in full', () => {
    const visible = buildVisibleGroups(groups, 'senior director', 'all');
    expect(visible).toHaveLength(2);
    expect(visible.map((g) => g.name)).toEqual(['Senior Director A', 'Senior Director E']);
  });

  it('matches the "Other" label', () => {
    const visible = buildVisibleGroups(groups, 'other', 'all');
    expect(visible).toHaveLength(1);
    expect(visible[0].type).toBe('other');
  });

  it('matches a pod name directly regardless of depth', () => {
    const visible = buildVisibleGroups(groups, 'Team Gamma', 'all');
    expect(visible).toHaveLength(1);
    const sdA = visible[0];
    if (sdA.type !== 'person') throw new Error('expected person');
    expect(sdA.visibleChildren[0].name).toBe('Director B');
  });

  it('returns nothing when the query matches no leader, "Other", or pod name', () => {
    expect(buildVisibleGroups(groups, 'nonexistent', 'all')).toHaveLength(0);
  });

  it('a card filter prunes non-matching pods at every depth, dropping any leader left with nothing visible', () => {
    const visible = buildVisibleGroups(groups, '', 'not_started');
    // Only Team Delta (under Senior Manager C) and Team Zeta (Other) are not_started.
    expect(visible).toHaveLength(2);

    const sdA = visible[0];
    if (sdA.type !== 'person') throw new Error('expected person');
    expect(sdA.visibleDirectTeams).toHaveLength(0); // alpha is complete
    expect(sdA.visibleChildren).toHaveLength(1); // Director B's whole branch is complete/in_progress, dropped
    expect(sdA.visibleChildren[0].name).toBe('Senior Manager C');

    expect(visible[1].type).toBe('other');
  });

  it("recomputes a leader's totalTeams/optedInTeams/remindCount from only the CURRENTLY VISIBLE teams, never the raw unfiltered group", () => {
    // Senior Director A's raw numbers (4 teams, 4 opted in, 2 remindable)
    // cover its WHOLE subtree (alpha, beta, gamma, delta) -- but under the
    // Not Started filter, only Team Delta (via Senior Manager C) survives;
    // alpha/beta/gamma are all pruned. The header must reflect that ONE
    // visible pod, not the raw four.
    const visible = buildVisibleGroups(groups, '', 'not_started');
    const sdA = visible[0];
    if (sdA.type !== 'person') throw new Error('expected person');

    expect(sdA.totalTeams).toBe(1);
    expect(sdA.optedInTeams).toBe(1);
    expect(sdA.remindCount).toBe(1);

    const smC = sdA.visibleChildren[0];
    if (smC.type !== 'person') throw new Error('expected person');
    expect(smC.totalTeams).toBe(1);
    expect(smC.optedInTeams).toBe(1);
    expect(smC.remindCount).toBe(1);
  });

  it('recomputes the "Other" group\'s totalTeams/optedInTeams/remindCount from only its visible teams too', () => {
    // Add a second, complete "Other" pod that a Not Started filter must hide.
    const groupsWithExtraOther: SurveyCompletionGroup[] = [
      seniorDirectorA,
      seniorDirectorE,
      {
        type: 'other',
        label: 'Other',
        totalTeams: 2,
        optedInTeams: 2,
        completionPercent: 50,
        remindCount: 1,
        teams: [
          team({ teamId: 'zeta', teamName: 'Team Zeta', status: 'not_started', completed: 0, total: 1 }),
          team({ teamId: 'eta-other', teamName: 'Other Eta', status: 'complete', completed: 2, total: 2 }),
        ],
      },
    ];
    const visible = buildVisibleGroups(groupsWithExtraOther, '', 'not_started');
    const other = visible.find((g) => g.type === 'other');
    if (!other || other.type !== 'other') throw new Error('expected an Other group');

    expect(other.visibleTeams).toHaveLength(1); // only zeta
    expect(other.totalTeams).toBe(1);
    expect(other.optedInTeams).toBe(1);
    expect(other.remindCount).toBe(1);
  });

  it('never renders any pod more than once across the whole visible tree', () => {
    const visible = buildVisibleGroups(groups, '', 'all');
    const seen = new Set<string>();

    const walk = (group: (typeof visible)[number]) => {
      if (group.type === 'person') {
        for (const t of group.visibleDirectTeams) {
          expect(seen.has(t.teamId)).toBe(false);
          seen.add(t.teamId);
        }
        group.visibleChildren.forEach(walk);
      } else {
        for (const t of group.visibleTeams) {
          expect(seen.has(t.teamId)).toBe(false);
          seen.add(t.teamId);
        }
      }
    };
    visible.forEach(walk);

    expect(seen.size).toBe(6); // alpha, beta, gamma, delta, epsilon, zeta
  });
});

describe('countVisibleTeams', () => {
  it('sums pods across every depth of every visible group', () => {
    const visible = buildVisibleGroups(groups, '', 'all');
    expect(countVisibleTeams(visible)).toBe(6);
  });

  it('reflects a card filter applied first', () => {
    const visible = buildVisibleGroups(groups, '', 'not_started');
    expect(countVisibleTeams(visible)).toBe(2); // delta, zeta
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

describe('getPodCompletionPercent', () => {
  it('computes completed/total * 100, rounded', () => {
    expect(getPodCompletionPercent(team({ completed: 45, total: 100 }))).toBe(45);
    expect(getPodCompletionPercent(team({ completed: 1, total: 3 }))).toBe(33); // 33.33... rounds to 33
    expect(getPodCompletionPercent(team({ completed: 4, total: 4 }))).toBe(100);
  });

  it('returns 0 (not NaN) when total is 0', () => {
    expect(getPodCompletionPercent(team({ completed: 0, total: 0 }))).toBe(0);
  });

  it('rounds up to 100 for a technically-incomplete pod very close to full (the display-status edge case)', () => {
    expect(getPodCompletionPercent(team({ completed: 199, total: 200 }))).toBe(100);
  });
});

describe('getPodDisplayStatus', () => {
  it('a 100%-complete pod displays as "complete"', () => {
    expect(getPodDisplayStatus(team({ status: 'complete', completed: 4, total: 4 }))).toBe('complete');
  });

  it('a partially-complete in_progress pod (e.g. 45%) keeps displaying as "in_progress"', () => {
    expect(getPodDisplayStatus(team({ status: 'in_progress', completed: 45, total: 100 }))).toBe('in_progress');
  });

  it('an in_progress pod whose rounded percentage reaches 100% displays as "complete", not "in_progress"', () => {
    // 199 of 200 -- backend still says in_progress (completed < total),
    // but rounds to 100%, so the display must show Completed, never a
    // contradictory "100% completed" next to an "In Progress" badge.
    expect(getPodDisplayStatus(team({ status: 'in_progress', completed: 199, total: 200 }))).toBe('complete');
  });

  it('never touches not_started or opted_out pods -- they keep their own status unchanged', () => {
    expect(getPodDisplayStatus(team({ status: 'not_started', completed: 0, total: 4 }))).toBe('not_started');
    expect(getPodDisplayStatus(team({ status: 'opted_out', completed: 0, total: 4 }))).toBe('opted_out');
  });

  it('never invents a status outside the existing four values', () => {
    const allStatuses: Array<SurveyCompletionTeam['status']> = ['complete', 'in_progress', 'not_started', 'opted_out'];
    for (const status of allStatuses) {
      const result = getPodDisplayStatus(team({ status, completed: 2, total: 4 }));
      expect(allStatuses).toContain(result);
    }
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

// Regression coverage for the reported "pods displayed twice" bug, using
// the exact pod names from that report, spread across a Level-2 root's
// direct pods, a nested Level-3 manager's pods, and "Other".
describe('buildVisibleGroups with the reported duplicate-pod names', () => {
  const managerA: SurveyCompletionPersonGroup = {
    type: 'person',
    person: person('mgr-a', 'Manager A', 'Manager'),
    totalTeams: 4,
    optedInTeams: 4,
    completionPercent: 50,
    remindCount: 2,
    directTeams: [
      team({ teamId: 't-biztech', teamName: 'BizTech-Salesforce-Team' }),
      team({ teamId: 't-anekal', teamName: 'Anekal', status: 'not_started', completed: 0, total: 2 }),
      team({ teamId: 't-bolinas', teamName: 'Bolinas', status: 'in_progress', completed: 1, total: 2 }),
      team({ teamId: 't-big-sur', teamName: 'big sur' }),
    ],
    children: [],
  };

  const l2Root: SurveyCompletionPersonGroup = {
    type: 'person',
    person: person('l2-root', 'L2 Root', 'Director'),
    totalTeams: 8,
    optedInTeams: 8,
    completionPercent: 50,
    remindCount: 4,
    directTeams: [
      team({ teamId: 't-andalusia', teamName: 'Andalusia' }),
      team({ teamId: 't-bobcaygeon', teamName: 'Bobcaygeon', status: 'in_progress', completed: 1, total: 2 }),
      team({ teamId: 't-biala', teamName: 'biala', status: 'not_started', completed: 0, total: 1 }),
      team({ teamId: 't-bay-view', teamName: 'bay view' }),
    ],
    children: [managerA],
  };

  const groups: SurveyCompletionGroup[] = [
    l2Root,
    {
      type: 'other',
      label: 'Other',
      totalTeams: 3,
      optedInTeams: 3,
      completionPercent: 33,
      remindCount: 2,
      teams: [
        team({ teamId: 't-atlanata', teamName: 'atlanata', status: 'not_started', completed: 0, total: 1, teamLeadId: 'tl-1', teamLeadName: 'Lead One' }),
        team({ teamId: 't-belfast', teamName: 'belfast' }),
        team({ teamId: 't-bandipur', teamName: 'bandipur', status: 'not_started', completed: 0, total: 1, teamLeadId: 'tl-3', teamLeadName: 'Lead Three' }),
      ],
    },
  ];

  it('renders each reported pod exactly once with no query or filter applied', () => {
    const visible = buildVisibleGroups(groups, '', 'all');
    const seen = new Set<string>();

    const walk = (group: (typeof visible)[number]) => {
      if (group.type === 'person') {
        for (const t of group.visibleDirectTeams) {
          expect(seen.has(t.teamId)).toBe(false);
          seen.add(t.teamId);
        }
        group.visibleChildren.forEach(walk);
      } else {
        for (const t of group.visibleTeams) {
          expect(seen.has(t.teamId)).toBe(false);
          seen.add(t.teamId);
        }
      }
    };
    visible.forEach(walk);

    expect(seen.size).toBe(11); // 4 direct + 4 under Manager A + 3 under Other
  });

  it('a name search on the nested manager surfaces that pod exactly once, not duplicated under the root', () => {
    const visible = buildVisibleGroups(groups, 'Manager A', 'all');
    expect(visible).toHaveLength(1);
    const root = visible[0];
    if (root.type !== 'person') throw new Error('expected person');
    expect(root.visibleDirectTeams).toHaveLength(0);
    expect(root.visibleChildren).toHaveLength(1);
    const managerAVisible = root.visibleChildren[0];
    if (managerAVisible.type !== 'person') throw new Error('expected person');
    expect(managerAVisible.visibleDirectTeams.map((t) => t.teamId)).toEqual([
      't-biztech', 't-anekal', 't-bolinas', 't-big-sur',
    ]);
  });

  it('a pod-name search (e.g. "bolinas") returns exactly one match, never two', () => {
    const visible = buildVisibleGroups(groups, 'bolinas', 'all');
    expect(visible).toHaveLength(1);
    const root = visible[0];
    if (root.type !== 'person') throw new Error('expected person');
    const managerAVisible = root.visibleChildren[0];
    if (managerAVisible.type !== 'person') throw new Error('expected person');
    expect(managerAVisible.visibleDirectTeams.map((t) => t.teamId)).toEqual(['t-bolinas']);
  });

  it('a card filter never leaves a reported pod appearing more than once', () => {
    const visible = buildVisibleGroups(groups, '', 'not_started');
    const seen = new Set<string>();
    const walk = (group: (typeof visible)[number]) => {
      if (group.type === 'person') {
        for (const t of group.visibleDirectTeams) {
          expect(seen.has(t.teamId)).toBe(false);
          seen.add(t.teamId);
        }
        group.visibleChildren.forEach(walk);
      } else {
        for (const t of group.visibleTeams) {
          expect(seen.has(t.teamId)).toBe(false);
          seen.add(t.teamId);
        }
      }
    };
    visible.forEach(walk);
    // not_started reported pods: biala, anekal, atlanata, bandipur.
    expect(seen).toEqual(new Set(['t-biala', 't-anekal', 't-atlanata', 't-bandipur']));
  });
});

// Regression test for the diagnosed root cause: two DIFFERENT team ids
// that happen to share the exact same name ("Aurora", one of the reported
// pods). These must render as two separate rows, keyed by teamId, never
// collapsed into one.
describe('buildVisibleGroups with two different team IDs sharing the same name', () => {
  const managerA: SurveyCompletionPersonGroup = {
    type: 'person',
    person: person('mgr-a', 'Manager A', 'Manager'),
    totalTeams: 1,
    optedInTeams: 1,
    completionPercent: 100,
    remindCount: 0,
    directTeams: [team({ teamId: 'aurora-1', teamName: 'Aurora' })],
    children: [],
  };
  const managerB: SurveyCompletionPersonGroup = {
    type: 'person',
    person: person('mgr-b', 'Manager B', 'Manager'),
    totalTeams: 1,
    optedInTeams: 1,
    completionPercent: 0,
    remindCount: 1,
    directTeams: [team({ teamId: 'aurora-2', teamName: 'Aurora', status: 'not_started', completed: 0, total: 3 })],
    children: [],
  };

  it('keeps both same-named pods visible and distinct when searching by that shared name', () => {
    const visible = buildVisibleGroups([managerA, managerB], 'aurora', 'all');
    expect(visible).toHaveLength(2);
    if (visible[0].type !== 'person' || visible[1].type !== 'person') throw new Error('expected person');
    expect(visible[0].visibleDirectTeams.map((t) => t.teamId)).toEqual(['aurora-1']);
    expect(visible[1].visibleDirectTeams.map((t) => t.teamId)).toEqual(['aurora-2']);
  });

  it('a status filter isolates the correct one of the two same-named pods by id, not by name', () => {
    const visible = buildVisibleGroups([managerA, managerB], '', 'not_started');
    expect(visible).toHaveLength(1);
    if (visible[0].type !== 'person') throw new Error('expected person');
    expect(visible[0].visibleDirectTeams.map((t) => t.teamId)).toEqual(['aurora-2']);
  });
});

// Fully Completed is hierarchy-aware: a leader qualifies only when EVERY
// descendant pod in their whole reporting subtree is at 100%, rolling up
// through however many reporting levels exist. Fixtures below use generic
// placeholder names ("Leader One", "Pod One", etc.) deliberately -- the
// helper must not special-case any particular person or pod name.
describe('isPodFullyCompleted / isPersonSubtreeFullyCompleted', () => {
  it('a pod at exactly 100% is fully completed; anything below is not', () => {
    expect(isPodFullyCompleted(team({ completed: 4, total: 4, status: 'complete' }))).toBe(true);
    // Rounds up to 100% even though backend status still says in_progress --
    // must agree with getPodCompletionPercent, the same helper the Status
    // column itself uses.
    expect(isPodFullyCompleted(team({ completed: 199, total: 200, status: 'in_progress' }))).toBe(true);
    expect(isPodFullyCompleted(team({ completed: 1, total: 4, status: 'in_progress' }))).toBe(false);
    expect(isPodFullyCompleted(team({ completed: 0, total: 4, status: 'not_started' }))).toBe(false);
  });

  it('a leader with two fully completed direct pods and no children qualifies', () => {
    const leader: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('leader-1', 'Leader One', 'Manager'),
      totalTeams: 2,
      optedInTeams: 2,
      completionPercent: 100,
      remindCount: 0,
      directTeams: [
        team({ teamId: 'pod-1', teamName: 'Pod One', completed: 4, total: 4, status: 'complete' }),
        team({ teamId: 'pod-2', teamName: 'Pod Two', completed: 5, total: 5, status: 'complete' }),
      ],
      children: [],
    };
    expect(isPersonSubtreeFullyCompleted(leader)).toBe(true);
  });

  it('a completed pod under a subordinate rolls up through the reporting chain to the top', () => {
    const subordinate: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('sub-1', 'Subordinate One', 'Senior Manager'),
      totalTeams: 1,
      optedInTeams: 1,
      completionPercent: 100,
      remindCount: 0,
      directTeams: [team({ teamId: 'pod-3', teamName: 'Pod Three', completed: 3, total: 3, status: 'complete' })],
      children: [],
    };
    const middleManager: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('mid-1', 'Middle Manager', 'Director'),
      totalTeams: 1,
      optedInTeams: 1,
      completionPercent: 100,
      remindCount: 0,
      directTeams: [],
      children: [subordinate],
    };
    const topLeader: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('top-1', 'Top Leader', 'Senior Director'),
      totalTeams: 1,
      optedInTeams: 1,
      completionPercent: 100,
      remindCount: 0,
      directTeams: [],
      children: [middleManager],
    };

    expect(isPersonSubtreeFullyCompleted(subordinate)).toBe(true);
    expect(isPersonSubtreeFullyCompleted(middleManager)).toBe(true);
    expect(isPersonSubtreeFullyCompleted(topLeader)).toBe(true);
  });

  it('one descendant pod below 100% prevents every affected ancestor from qualifying', () => {
    const subordinate: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('sub-2', 'Subordinate Two', 'Senior Manager'),
      totalTeams: 1,
      optedInTeams: 1,
      completionPercent: 50,
      remindCount: 1,
      directTeams: [
        team({ teamId: 'pod-4', teamName: 'Pod Four', completed: 2, total: 4, status: 'in_progress' }),
      ],
      children: [],
    };
    const middleManager: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('mid-2', 'Middle Manager Two', 'Director'),
      totalTeams: 1,
      optedInTeams: 1,
      completionPercent: 50,
      remindCount: 1,
      directTeams: [],
      children: [subordinate],
    };
    const topLeader: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('top-2', 'Top Leader Two', 'Senior Director'),
      totalTeams: 1,
      optedInTeams: 1,
      completionPercent: 50,
      remindCount: 1,
      directTeams: [],
      children: [middleManager],
    };

    expect(isPersonSubtreeFullyCompleted(subordinate)).toBe(false);
    expect(isPersonSubtreeFullyCompleted(middleManager)).toBe(false);
    expect(isPersonSubtreeFullyCompleted(topLeader)).toBe(false);
  });

  it('a leader with no direct pods and no children does not qualify (no relevant descendants to judge by)', () => {
    const emptyLeader: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('empty-1', 'Empty Leader', 'Director'),
      totalTeams: 0,
      optedInTeams: 0,
      completionPercent: 0,
      remindCount: 0,
      directTeams: [],
      children: [],
    };
    expect(isPersonSubtreeFullyCompleted(emptyLeader)).toBe(false);
  });
});

describe('buildFullyCompletedGroups', () => {
  const fullyCompletedSubordinate: SurveyCompletionPersonGroup = {
    type: 'person',
    person: person('fc-sub', 'Fully Completed Subordinate', 'Manager'),
    totalTeams: 1,
    optedInTeams: 1,
    completionPercent: 100,
    remindCount: 0,
    directTeams: [team({ teamId: 'fc-pod-1', teamName: 'Fully Completed Pod One', completed: 2, total: 2, status: 'complete' })],
    children: [],
  };
  const incompleteSubordinate: SurveyCompletionPersonGroup = {
    type: 'person',
    person: person('inc-sub', 'Incomplete Subordinate', 'Manager'),
    totalTeams: 1,
    optedInTeams: 1,
    completionPercent: 25,
    remindCount: 1,
    directTeams: [
      team({ teamId: 'inc-pod-1', teamName: 'Incomplete Pod One', completed: 1, total: 4, status: 'in_progress' }),
    ],
    children: [],
  };
  // A director whose own subtree is NOT fully completed (one subordinate is
  // incomplete), but who has one subordinate that independently qualifies.
  const mixedDirector: SurveyCompletionPersonGroup = {
    type: 'person',
    person: person('mixed-dir', 'Mixed Director', 'Director'),
    totalTeams: 2,
    optedInTeams: 2,
    completionPercent: 63,
    remindCount: 1,
    directTeams: [],
    children: [fullyCompletedSubordinate, incompleteSubordinate],
  };
  const fullyCompletedRoot: SurveyCompletionPersonGroup = {
    type: 'person',
    person: person('fc-root', 'Fully Completed Root', 'Senior Director'),
    totalTeams: 1,
    optedInTeams: 1,
    completionPercent: 100,
    remindCount: 0,
    directTeams: [team({ teamId: 'fc-pod-2', teamName: 'Fully Completed Pod Two', completed: 1, total: 1, status: 'complete' })],
    children: [],
  };
  const otherGroupWithMixedPods: SurveyCompletionGroup = {
    type: 'other',
    label: 'Other',
    totalTeams: 2,
    optedInTeams: 2,
    completionPercent: 50,
    remindCount: 1,
    teams: [
      team({ teamId: 'other-fc', teamName: 'Other Fully Completed Pod', completed: 1, total: 1, status: 'complete' }),
      team({ teamId: 'other-inc', teamName: 'Other Incomplete Pod', completed: 0, total: 1, status: 'not_started' }),
    ],
  };

  it('a pod below 100% never appears, and a fully completed root appears with only its own pods', () => {
    const visible = buildFullyCompletedGroups([fullyCompletedRoot], '');
    expect(visible).toHaveLength(1);
    const root = visible[0];
    if (root.type !== 'person') throw new Error('expected person');
    expect(root.visibleDirectTeams.map((t) => t.teamId)).toEqual(['fc-pod-2']);
  });

  it('a parent with even one incomplete descendant does not appear, but a subordinate that independently qualifies is promoted to a top-level result', () => {
    const visible = buildFullyCompletedGroups([mixedDirector], '');
    // Mixed Director itself must not appear -- Incomplete Subordinate fails it.
    expect(visible.some((g) => g.name === 'Mixed Director')).toBe(false);
    // Fully Completed Subordinate qualifies on its own and is surfaced.
    expect(visible).toHaveLength(1);
    expect(visible[0].name).toBe('Fully Completed Subordinate');
    if (visible[0].type !== 'person') throw new Error('expected person');
    expect(visible[0].visibleDirectTeams.map((t) => t.teamId)).toEqual(['fc-pod-1']);
    // Incomplete Subordinate never appears anywhere in the results.
    expect(visible.some((g) => g.name === 'Incomplete Subordinate')).toBe(false);
  });

  it('an "other" group only shows the pods within it that are individually at 100%', () => {
    const visible = buildFullyCompletedGroups([otherGroupWithMixedPods], '');
    expect(visible).toHaveLength(1);
    const other = visible[0];
    if (other.type !== 'other') throw new Error('expected other');
    expect(other.visibleTeams.map((t) => t.teamId)).toEqual(['other-fc']);
  });

  it('search narrows results to matching subtrees without affecting the fully-completed classification itself', () => {
    const visible = buildFullyCompletedGroups([fullyCompletedRoot, mixedDirector], 'Fully Completed Root');
    expect(visible).toHaveLength(1);
    expect(visible[0].name).toBe('Fully Completed Root');
  });

  it('never uses a person or pod name as a special-case condition -- renaming every fixture leaves the same qualification result', () => {
    const renamed: SurveyCompletionPersonGroup = {
      ...fullyCompletedRoot,
      person: person('fc-root', 'Zzyzx Nine', 'Senior Director'),
      directTeams: [team({ teamId: 'fc-pod-2', teamName: 'Qwerty Pod', completed: 1, total: 1, status: 'complete' })],
    };
    expect(isPersonSubtreeFullyCompleted(renamed)).toBe(true);
    const visible = buildFullyCompletedGroups([renamed], '');
    expect(visible).toHaveLength(1);
    expect(visible[0].name).toBe('Zzyzx Nine');
  });
});

// Not Started and Opted Out must never be conflated: opted_out comes purely
// from the pod's canonical `status` field (never inferred from a 0%
// completion percentage), and matchesCardFilter/buildVisibleGroups compare
// against that same canonical field for both filters. Fixtures below use
// generic placeholder names deliberately -- neither filter special-cases
// any person or pod name.
describe('matchesCardFilter — not_started vs opted_out', () => {
  it('a not_started pod matches only the not_started filter, never opted_out', () => {
    const pod = team({ status: 'not_started', completed: 0, total: 4 });
    expect(matchesCardFilter(pod, 'not_started')).toBe(true);
    expect(matchesCardFilter(pod, 'opted_out')).toBe(false);
  });

  it('an opted_out pod at 0% completion matches only the opted_out filter, never not_started', () => {
    // Same 0% completion as the not_started pod above -- the only thing
    // that must decide the outcome is the canonical status field, not the
    // percentage, which is identical for both pods here.
    const pod = team({ status: 'opted_out', completed: 0, total: 4, postWorkshopCompleted: null });
    expect(matchesCardFilter(pod, 'opted_out')).toBe(true);
    expect(matchesCardFilter(pod, 'not_started')).toBe(false);
  });
});

describe('buildVisibleGroups — Not Started and Opted Out filters', () => {
  const nestedLeader: SurveyCompletionPersonGroup = {
    type: 'person',
    person: person('nested-1', 'Nested Leader', 'Manager'),
    totalTeams: 2,
    optedInTeams: 1,
    completionPercent: 0,
    remindCount: 1,
    directTeams: [
      team({ teamId: 'pod-nested-ns', teamName: 'Pod Nested Not Started', status: 'not_started', completed: 0, total: 3 }),
      team({
        teamId: 'pod-nested-oo',
        teamName: 'Pod Nested Opted Out',
        status: 'opted_out',
        completed: 0,
        total: 3,
        postWorkshopCompleted: null,
      }),
    ],
    children: [],
  };
  const rootLeader: SurveyCompletionPersonGroup = {
    type: 'person',
    person: person('root-1', 'Root Leader', 'Director'),
    totalTeams: 4,
    optedInTeams: 2,
    completionPercent: 25,
    remindCount: 2,
    directTeams: [
      team({ teamId: 'pod-ns', teamName: 'Pod Not Started', status: 'not_started', completed: 0, total: 4 }),
      team({
        teamId: 'pod-oo',
        teamName: 'Pod Opted Out',
        status: 'opted_out',
        completed: 0,
        total: 4,
        postWorkshopCompleted: null,
      }),
      team({ teamId: 'pod-complete', teamName: 'Pod Complete', status: 'complete', completed: 5, total: 5 }),
    ],
    children: [nestedLeader],
  };
  const groups: SurveyCompletionGroup[] = [rootLeader];

  it('the Not Started filter returns only not_started pods, at every level of the hierarchy', () => {
    const visible = buildVisibleGroups(groups, '', 'not_started');
    expect(visible).toHaveLength(1);
    const root = visible[0];
    if (root.type !== 'person') throw new Error('expected person');
    expect(root.visibleDirectTeams.map((t) => t.teamId)).toEqual(['pod-ns']);
    expect(root.visibleChildren).toHaveLength(1);
    const nested = root.visibleChildren[0];
    if (nested.type !== 'person') throw new Error('expected person');
    expect(nested.visibleDirectTeams.map((t) => t.teamId)).toEqual(['pod-nested-ns']);
  });

  it('the Opted Out filter returns only opted_out pods, at every level of the hierarchy', () => {
    const visible = buildVisibleGroups(groups, '', 'opted_out');
    expect(visible).toHaveLength(1);
    const root = visible[0];
    if (root.type !== 'person') throw new Error('expected person');
    expect(root.visibleDirectTeams.map((t) => t.teamId)).toEqual(['pod-oo']);
    // The filter narrows WHICH pods are visible -- it must never strip or
    // null out a surviving pod's own individual-survey completion fields.
    expect(root.visibleDirectTeams[0].completed).toBe(0);
    expect(root.visibleDirectTeams[0].total).toBe(4);
    expect(root.visibleChildren).toHaveLength(1);
    const nested = root.visibleChildren[0];
    if (nested.type !== 'person') throw new Error('expected person');
    expect(nested.visibleDirectTeams.map((t) => t.teamId)).toEqual(['pod-nested-oo']);
  });

  it('opted_out pods never appear under the Not Started filter and vice versa', () => {
    const collectTeamIds = (visible: ReturnType<typeof buildVisibleGroups>): Set<string> => {
      const ids = new Set<string>();
      const walk = (group: (typeof visible)[number]) => {
        if (group.type === 'person') {
          group.visibleDirectTeams.forEach((t) => ids.add(t.teamId));
          group.visibleChildren.forEach(walk);
        } else {
          group.visibleTeams.forEach((t) => ids.add(t.teamId));
        }
      };
      visible.forEach(walk);
      return ids;
    };

    const notStartedIds = collectTeamIds(buildVisibleGroups(groups, '', 'not_started'));
    expect(notStartedIds.has('pod-oo')).toBe(false);
    expect(notStartedIds.has('pod-nested-oo')).toBe(false);

    const optedOutIds = collectTeamIds(buildVisibleGroups(groups, '', 'opted_out'));
    expect(optedOutIds.has('pod-ns')).toBe(false);
    expect(optedOutIds.has('pod-nested-ns')).toBe(false);
  });

  it('the Fully Completed filter continues to work unaffected by this fixture', () => {
    // Neither root nor nested leader qualifies (they have not_started/opted_out
    // descendants), and no subtree here independently qualifies either.
    expect(buildFullyCompletedGroups(groups, '')).toHaveLength(0);
  });
});

// isParentStatusHiddenFor is the single shared rule deciding whether a
// leader (Manager/Director/Level-2) row's own Status column shows anything
// at all. Both Not Started and Opted Out hide it entirely.
describe('isParentStatusHiddenFor', () => {
  it('returns true for the Not Started filter', () => {
    expect(isParentStatusHiddenFor('not_started')).toBe(true);
  });

  it('returns true for the Opted Out filter', () => {
    expect(isParentStatusHiddenFor('opted_out')).toBe(true);
  });

  it('returns false for every other filter (all, optedIn, complete, in_progress)', () => {
    expect(isParentStatusHiddenFor('all')).toBe(false);
    expect(isParentStatusHiddenFor('optedIn')).toBe(false);
    expect(isParentStatusHiddenFor('complete')).toBe(false);
    expect(isParentStatusHiddenFor('in_progress')).toBe(false);
  });
});

describe('isPaleRedPodRow', () => {
  it('is true for Not Started and partial/In Progress, false for Completed and Opted Out', () => {
    expect(isPaleRedPodRow('not_started')).toBe(true);
    expect(isPaleRedPodRow('in_progress')).toBe(true);
    expect(isPaleRedPodRow('complete')).toBe(false);
    expect(isPaleRedPodRow('opted_out')).toBe(false);
  });
});

// Total Teams / All Teams hierarchical aggregation. Fixtures use generic
// placeholder names deliberately -- neither collectDescendantPods nor
// computeHierarchyAggregate ever special-cases a person or pod name.
describe('collectDescendantPods', () => {
  it('collects direct pods plus every descendant pod, each exactly once, at any depth', () => {
    const grandchild: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('gc-1', 'Grandchild Leader', 'Manager'),
      totalTeams: 1,
      optedInTeams: 1,
      completionPercent: 100,
      remindCount: 0,
      directTeams: [team({ teamId: 'gc-pod', teamName: 'Grandchild Pod' })],
      children: [],
    };
    const child: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('child-1', 'Child Leader', 'Senior Manager'),
      totalTeams: 2,
      optedInTeams: 2,
      completionPercent: 100,
      remindCount: 0,
      directTeams: [team({ teamId: 'child-pod', teamName: 'Child Pod' })],
      children: [grandchild],
    };
    const root: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('root-1', 'Root Leader', 'Director'),
      totalTeams: 3,
      optedInTeams: 3,
      completionPercent: 100,
      remindCount: 0,
      directTeams: [team({ teamId: 'root-pod', teamName: 'Root Pod' })],
      children: [child],
    };

    const pods = collectDescendantPods(root);
    expect(pods.map((p) => p.teamId).sort()).toEqual(['child-pod', 'gc-pod', 'root-pod']);
  });

  it('returns an empty array for a leader with no direct pods and no children', () => {
    const empty: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('empty-1', 'Empty Leader', 'Manager'),
      totalTeams: 0,
      optedInTeams: 0,
      completionPercent: 0,
      remindCount: 0,
      directTeams: [],
      children: [],
    };
    expect(collectDescendantPods(empty)).toEqual([]);
  });
});

describe('computeHierarchyAggregate', () => {
  // Two pods directly under one manager: 32% and 100%. Averaged (NOT
  // weighted by member count) = (32 + 100) / 2 = 66%.
  const manager: SurveyCompletionPersonGroup = {
    type: 'person',
    person: person('mgr-1', 'Manager One', 'Senior Manager'),
    totalTeams: 2,
    optedInTeams: 2,
    completionPercent: 0,
    remindCount: 1,
    directTeams: [
      team({ teamId: 'pod-a', teamName: 'Pod A', status: 'in_progress', completed: 32, total: 100 }),
      team({ teamId: 'pod-b', teamName: 'Pod B', status: 'complete', completed: 20, total: 20 }),
    ],
    children: [],
  };

  it("combines a manager's own two direct pods into an unweighted average of their own percentages, never double-counted", () => {
    const result = computeHierarchyAggregate(manager);
    expect(result).not.toBeNull();
    expect(result?.percent).toBe(66); // round((32 + 100) / 2)
    expect(result?.displayStatus).toBe('in_progress');
  });

  it('rolls up through a parent who owns two more pods directly, averaging all four leaf pods\' own percentages', () => {
    const higherParent: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('dir-1', 'Director One', 'Director'),
      totalTeams: 4,
      optedInTeams: 4,
      completionPercent: 0,
      remindCount: 2,
      directTeams: [
        team({ teamId: 'pod-c', teamName: 'Pod C', status: 'not_started', completed: 0, total: 10 }),
        team({ teamId: 'pod-d', teamName: 'Pod D', status: 'complete', completed: 5, total: 5 }),
      ],
      children: [manager],
    };
    // Pod percentages: A=32, B=100, C=0, D=100 -> average = 232/4 = 58.
    const result = computeHierarchyAggregate(higherParent);
    expect(result).not.toBeNull();
    expect(result?.percent).toBe(58);
    expect(result?.displayStatus).toBe('in_progress');
  });

  it("never re-averages a subordinate's own already-computed percentage -- rolls up the same raw leaf-pod percentages instead", () => {
    // If the higher parent's calculation incorrectly averaged its own pods'
    // percentage with the manager's own pre-computed 66%, the result would
    // differ from averaging every leaf pod's own raw percentage once.
    const higherParent: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('dir-2', 'Director Two', 'Director'),
      totalTeams: 2,
      optedInTeams: 2,
      completionPercent: 0,
      remindCount: 1,
      directTeams: [],
      children: [manager],
    };
    const leafPods = collectDescendantPods(higherParent);
    const expectedPercent = Math.round(
      leafPods.reduce((sum, p) => sum + getPodCompletionPercent(p), 0) / leafPods.length,
    );
    const result = computeHierarchyAggregate(higherParent);
    expect(result?.percent).toBe(expectedPercent);
  });

  it("a leader whose two children's aggregates are 70% and 60% must read 64% (the average of all five leaf pods), never 65% (the average of the two children's own aggregates)", () => {
    // Reproduces the reported acceptance scenario exactly: Bharath (Pod A
    // 60%, Pod B 80%) and Ravi (Pod C 40%, Pod D 60%, Pod E 80%) report to
    // Murali. Bharath's own aggregate is 70%, Ravi's is 60% -- averaging
    // those two numbers gives the WRONG answer, 65%. Murali's aggregate
    // must instead average all five leaf pods directly: 64%.
    const bharath: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('bharath', 'Bharath', 'Manager'),
      totalTeams: 2,
      optedInTeams: 2,
      completionPercent: 0,
      remindCount: 2,
      directTeams: [
        team({ teamId: 'pod-a2', teamName: 'Pod A', status: 'in_progress', completed: 6, total: 10 }),
        team({ teamId: 'pod-b2', teamName: 'Pod B', status: 'in_progress', completed: 8, total: 10 }),
      ],
      children: [],
    };
    const ravi: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('ravi', 'Ravi', 'Manager'),
      totalTeams: 3,
      optedInTeams: 3,
      completionPercent: 0,
      remindCount: 3,
      directTeams: [
        team({ teamId: 'pod-c2', teamName: 'Pod C', status: 'in_progress', completed: 4, total: 10 }),
        team({ teamId: 'pod-d2', teamName: 'Pod D', status: 'in_progress', completed: 6, total: 10 }),
        team({ teamId: 'pod-e2', teamName: 'Pod E', status: 'in_progress', completed: 8, total: 10 }),
      ],
      children: [],
    };
    const murali: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('murali', 'Murali', 'Director'),
      totalTeams: 5,
      optedInTeams: 5,
      completionPercent: 0,
      remindCount: 5,
      directTeams: [],
      children: [bharath, ravi],
    };

    expect(computeHierarchyAggregate(bharath)?.percent).toBe(70);
    expect(computeHierarchyAggregate(ravi)?.percent).toBe(60);
    const muraliResult = computeHierarchyAggregate(murali);
    expect(muraliResult?.percent).toBe(64);
    expect(muraliResult?.percent).not.toBe(65);
    expect(muraliResult?.displayStatus).toBe('in_progress');
  });

  it('aggregates correctly through three or more nested hierarchy levels, from the actual leaf pods rather than any intermediate percentage', () => {
    // Parent
    // ├── Team A: Pod 1 (100%), Pod 2 (50%)
    // ├── Team B: Pod 3 (0%), Team C: Pod 4 (80%), Pod 5 (50%)
    // └── Team D: Pod 6 (25%)
    //
    // Four leader levels deep (Parent -> Team B -> Team C), and Parent's own
    // percent must come from all six pods (51%), never from averaging Team
    // A/B/D's own percentages (75, 43, 25 -> 48, a different, wrong number).
    const teamC: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('team-c', 'Team C', 'Team Lead'),
      totalTeams: 2,
      optedInTeams: 2,
      completionPercent: 0,
      remindCount: 0,
      directTeams: [
        team({ teamId: 'pod-4', teamName: 'Pod 4', status: 'complete', completed: 4, total: 5 }),
        team({ teamId: 'pod-5', teamName: 'Pod 5', status: 'in_progress', completed: 1, total: 2 }),
      ],
      children: [],
    };
    const teamB: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('team-b', 'Team B', 'Manager'),
      totalTeams: 3,
      optedInTeams: 3,
      completionPercent: 0,
      remindCount: 0,
      directTeams: [team({ teamId: 'pod-3', teamName: 'Pod 3', status: 'not_started', completed: 0, total: 4 })],
      children: [teamC],
    };
    const teamA: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('team-a', 'Team A', 'Manager'),
      totalTeams: 2,
      optedInTeams: 2,
      completionPercent: 0,
      remindCount: 0,
      directTeams: [
        team({ teamId: 'pod-1', teamName: 'Pod 1', status: 'complete', completed: 5, total: 5 }),
        team({ teamId: 'pod-2', teamName: 'Pod 2', status: 'in_progress', completed: 3, total: 6 }),
      ],
      children: [],
    };
    const teamD: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('team-d', 'Team D', 'Manager'),
      totalTeams: 1,
      optedInTeams: 1,
      completionPercent: 0,
      remindCount: 0,
      directTeams: [team({ teamId: 'pod-6', teamName: 'Pod 6', status: 'in_progress', completed: 2, total: 8 })],
      children: [],
    };
    const parent: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('parent', 'Parent', 'Director'),
      totalTeams: 6,
      optedInTeams: 6,
      completionPercent: 0,
      remindCount: 0,
      directTeams: [],
      children: [teamA, teamB, teamD],
    };

    // Pod percentages: 100, 50, 0, 80, 50, 25 -> sum 305 / 6 = 50.83 -> 51.
    expect(computeHierarchyAggregate(parent)?.percent).toBe(51);
    // Team-level percentages this must NOT be derived from: A=75, B=43, D=25.
    expect(computeHierarchyAggregate(teamA)?.percent).toBe(75);
    expect(computeHierarchyAggregate(teamB)?.percent).toBe(43);
    expect(computeHierarchyAggregate(teamD)?.percent).toBe(25);
    const wrongAverageOfChildPercentages = Math.round((75 + 43 + 25) / 3);
    expect(computeHierarchyAggregate(parent)?.percent).not.toBe(wrongAverageOfChildPercentages);
  });

  it('displays green Completed when every descendant pod is at 100%', () => {
    const allComplete: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('all-complete', 'Fully Done Leader', 'Manager'),
      totalTeams: 2,
      optedInTeams: 2,
      completionPercent: 100,
      remindCount: 0,
      directTeams: [
        team({ teamId: 'ac-1', teamName: 'AC One', status: 'complete', completed: 4, total: 4 }),
        team({ teamId: 'ac-2', teamName: 'AC Two', status: 'complete', completed: 10, total: 10 }),
      ],
      children: [],
    };
    const result = computeHierarchyAggregate(allComplete);
    expect(result?.percent).toBe(100);
    expect(result?.displayStatus).toBe('complete');
  });

  it('excludes opted-out pods entirely from both the numerator and denominator', () => {
    const withOptedOut: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('with-oo', 'Leader With Opted Out', 'Manager'),
      totalTeams: 2,
      optedInTeams: 1,
      completionPercent: 100,
      remindCount: 0,
      directTeams: [
        team({ teamId: 'oo-real', teamName: 'Real Pod', status: 'complete', completed: 4, total: 4 }),
        team({
          teamId: 'oo-excluded',
          teamName: 'Opted Out Pod',
          status: 'opted_out',
          completed: 0,
          total: 50,
          postWorkshopCompleted: null,
        }),
      ],
      children: [],
    };
    const result = computeHierarchyAggregate(withOptedOut);
    // If the opted-out pod's 50 members were counted, this would not be 100%.
    expect(result?.percent).toBe(100);
    expect(result?.displayStatus).toBe('complete');
  });

  it('returns null when every descendant pod is opted out (nothing meaningful to compute)', () => {
    const allOptedOut: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('all-oo', 'All Opted Out Leader', 'Manager'),
      totalTeams: 1,
      optedInTeams: 0,
      completionPercent: 0,
      remindCount: 0,
      directTeams: [
        team({ teamId: 'oo-1', teamName: 'OO One', status: 'opted_out', completed: 0, total: 10, postWorkshopCompleted: null }),
      ],
      children: [],
    };
    expect(computeHierarchyAggregate(allOptedOut)).toBeNull();
  });

  it('returns null for a leader with no descendant pods at all', () => {
    const empty: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('empty-2', 'Empty Leader Two', 'Manager'),
      totalTeams: 0,
      optedInTeams: 0,
      completionPercent: 0,
      remindCount: 0,
      directTeams: [],
      children: [],
    };
    expect(computeHierarchyAggregate(empty)).toBeNull();
  });

  it('displays not_started when every opted-in descendant pod has zero completion', () => {
    const allNotStarted: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('all-ns', 'All Not Started Leader', 'Manager'),
      totalTeams: 2,
      optedInTeams: 2,
      completionPercent: 0,
      remindCount: 2,
      directTeams: [
        team({ teamId: 'ns-1', teamName: 'NS One', status: 'not_started', completed: 0, total: 4 }),
        team({ teamId: 'ns-2', teamName: 'NS Two', status: 'not_started', completed: 0, total: 6 }),
      ],
      children: [],
    };
    const result = computeHierarchyAggregate(allNotStarted);
    expect(result?.percent).toBe(0);
    expect(result?.displayStatus).toBe('not_started');
  });
});

describe('buildHierarchyAggregateIndex', () => {
  it('indexes every person id at every depth by its own hierarchy-wide aggregate', () => {
    const child: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('idx-child', 'Indexed Child', 'Manager'),
      totalTeams: 1,
      optedInTeams: 1,
      completionPercent: 100,
      remindCount: 0,
      directTeams: [team({ teamId: 'idx-pod', teamName: 'Indexed Pod', status: 'complete', completed: 5, total: 5 })],
      children: [],
    };
    const root: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('idx-root', 'Indexed Root', 'Director'),
      totalTeams: 1,
      optedInTeams: 1,
      completionPercent: 100,
      remindCount: 0,
      directTeams: [],
      children: [child],
    };
    const index = buildHierarchyAggregateIndex([root]);
    expect(index.get('idx-root')).toEqual({ percent: 100, displayStatus: 'complete' });
    expect(index.get('idx-child')).toEqual({ percent: 100, displayStatus: 'complete' });
  });
});

describe('computeVisibleGroupAggregate / buildVisibleHierarchyAggregateIndex', () => {
  // Mirrors the reported example: a leader (Murali) with three direct pods
  // -- Danville (5/9, in_progress), Sausalito (5/9, in_progress), and a
  // third, already-complete pod that the In Progress filter would prune.
  // Danville + Sausalito combined = 10/18 = 56%, and the pruned complete
  // pod must never dilute that number.
  const murali: SurveyCompletionPersonGroup = {
    type: 'person',
    person: person('murali', 'Murali', 'Manager'),
    totalTeams: 3,
    optedInTeams: 3,
    completionPercent: 29,
    remindCount: 2,
    directTeams: [
      team({ teamId: 'danville', teamName: 'Danville', status: 'in_progress', completed: 5, total: 9 }),
      team({ teamId: 'sausalito', teamName: 'Sausalito', status: 'in_progress', completed: 5, total: 9 }),
      team({ teamId: 'other-pod', teamName: 'Other Pod', status: 'complete', completed: 6, total: 6 }),
    ],
    children: [],
  };

  it('sums only the CURRENTLY VISIBLE (in_progress) pods, ignoring a hidden complete sibling', () => {
    const visible = buildVisibleGroups([murali], '', 'in_progress');
    expect(visible).toHaveLength(1);
    const visibleMurali = visible[0] as Extract<(typeof visible)[number], { type: 'person' }>;
    // The complete pod was pruned by the filter -- only Danville/Sausalito remain.
    expect(visibleMurali.visibleDirectTeams.map((t) => t.teamId)).toEqual(['danville', 'sausalito']);

    const result = computeVisibleGroupAggregate(visibleMurali);
    expect(result).toEqual({ percent: 56, displayStatus: 'in_progress' }); // round(10/18*100) = 56
  });

  it('buildVisibleHierarchyAggregateIndex indexes every visible leader at every depth', () => {
    const visible = buildVisibleGroups([murali], '', 'in_progress');
    const index = buildVisibleHierarchyAggregateIndex(visible);
    expect(index.get('murali')).toEqual({ percent: 56, displayStatus: 'in_progress' });
  });

  it('returns null when a leader has no visible pod left after filtering', () => {
    const allComplete: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('all-complete-leader', 'All Complete Leader', 'Manager'),
      totalTeams: 1,
      optedInTeams: 1,
      completionPercent: 100,
      remindCount: 0,
      directTeams: [team({ teamId: 'ac-1', teamName: 'AC One', status: 'complete', completed: 4, total: 4 })],
      children: [],
    };
    const visible = buildVisibleGroups([allComplete], '', 'in_progress');
    expect(visible).toHaveLength(0);
  });
});

describe('isRowBackgroundHiddenFor', () => {
  it('returns true for Total Teams (all) and In Progress', () => {
    expect(isRowBackgroundHiddenFor('all')).toBe(true);
    expect(isRowBackgroundHiddenFor('in_progress')).toBe(true);
  });

  it('returns false for every other filter', () => {
    expect(isRowBackgroundHiddenFor('optedIn')).toBe(false);
    expect(isRowBackgroundHiddenFor('complete')).toBe(false);
    expect(isRowBackgroundHiddenFor('not_started')).toBe(false);
    expect(isRowBackgroundHiddenFor('opted_out')).toBe(false);
  });
});

describe('getPartialCompletionBadgeClass', () => {
  it('is red below 50%, including a genuine 0%', () => {
    expect(getPartialCompletionBadgeClass(0)).toBe('bg-red-50 text-red-700');
    expect(getPartialCompletionBadgeClass(34)).toBe('bg-red-50 text-red-700');
    expect(getPartialCompletionBadgeClass(49)).toBe('bg-red-50 text-red-700');
  });

  it('is yellow at 50% and up', () => {
    expect(getPartialCompletionBadgeClass(50)).toBe('bg-yellow-50 text-yellow-700');
    expect(getPartialCompletionBadgeClass(67)).toBe('bg-yellow-50 text-yellow-700');
    expect(getPartialCompletionBadgeClass(99)).toBe('bg-yellow-50 text-yellow-700');
  });
});

