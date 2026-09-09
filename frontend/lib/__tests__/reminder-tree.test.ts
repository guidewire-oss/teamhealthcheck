import { describe, expect, it } from 'vitest';
import {
  buildOrgReminderPlan,
  buildPersonReminderPlan,
  buildTeamReminderPlan,
  getStatusesInTree,
} from '@/lib/reminder-tree';
import type {
  SurveyCompletionGroup,
  SurveyCompletionPerson,
  SurveyCompletionPersonGroup,
  SurveyCompletionTeam,
} from '@/lib/api/survey-completion';

function team(overrides: Partial<SurveyCompletionTeam> = {}): SurveyCompletionTeam {
  return {
    teamId: 't1',
    teamName: 'Ajanta',
    completed: 1,
    total: 3,
    postWorkshopCompleted: false,
    status: 'not_started',
    teamLeadName: 'Alice',
    ...overrides,
  };
}

function person(id: string, name: string, level: string): SurveyCompletionPerson {
  return { id, name, level, levelId: level.toLowerCase().replace(/\s+/g, '-') };
}

describe('buildTeamReminderPlan', () => {
  it("roots the tree at the pod's direct owner when it has one", () => {
    const owner = person('mgr1', 'Sreejith Rajasekaran', 'Manager');
    const plan = buildTeamReminderPlan(team({ teamName: 'Goa' }), { owner });

    expect(plan.root).toEqual({
      type: 'person',
      id: 'mgr1',
      name: 'Sreejith Rajasekaran',
      level: 'Manager',
      children: [{ type: 'team', id: 't1', name: 'Goa', teamLeadName: 'Alice', status: 'not_started' }],
    });
    expect(plan.podCount).toBe(1);
    expect(plan.subtitle).toBe('Reminding 1 pod under Sreejith Rajasekaran');
    // The preview tree shows the owner as a node alongside the pod, so the
    // toast must name them too, matching what the preview just showed.
    expect(plan.toastMessage).toBe('Reminder sent to Sreejith Rajasekaran and the team lead for Goa.');
  });

  it('has no supervisor root when the pod has no owner at all (the "Other" case)', () => {
    const plan = buildTeamReminderPlan(team({ teamName: 'Test Team' }), {});

    expect(plan.root).toEqual({
      type: 'team',
      id: 't1',
      name: 'Test Team',
      teamLeadName: 'Alice',
      status: 'not_started',
    });
    expect(plan.subtitle).toBe('Reminding the team lead for Test Team');
  });

  it('labels an unassigned team lead', () => {
    const plan = buildTeamReminderPlan(team({ teamLeadName: undefined }), {});
    expect(plan.root).toMatchObject({ teamLeadName: 'Unassigned' });
  });
});

describe('buildPersonReminderPlan', () => {
  it('roots at a leaf leader with only pending pods as children', () => {
    const managerD: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('mgr-d', 'Manager D', 'Manager'),
      totalTeams: 3,
      optedInTeams: 3,
      completionPercent: 33,
      remindCount: 2,
      directTeams: [
        team({ teamId: 'a', teamName: 'Ajanta', status: 'not_started' }),
        team({ teamId: 'g', teamName: 'Gokarna', status: 'in_progress' }),
        team({ teamId: 'c', teamName: 'Complete Pod', status: 'complete' }),
      ],
      children: [],
    };

    const plan = buildPersonReminderPlan(managerD);

    expect(plan.root.type).toBe('person');
    expect(plan.root).toMatchObject({ id: 'mgr-d', level: 'Manager' });
    const root = plan.root as Extract<typeof plan.root, { type: 'person' }>;
    expect(root.children.map((c) => (c as { name: string }).name)).toEqual(['Ajanta', 'Gokarna']);
    expect(plan.podCount).toBe(2);
    expect(plan.subtitle).toBe('Reminding 2 pods under Manager D');
    expect(plan.toastMessage).toBe('Reminder sent to Manager D and 2 team leads.');
  });

  it('rolls pending pods up through three levels, nesting a Director under a Senior Director with a Manager under the Director', () => {
    const managerD: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('mgr-d', 'Manager D', 'Manager'),
      totalTeams: 1,
      optedInTeams: 1,
      completionPercent: 0,
      remindCount: 1,
      directTeams: [team({ teamId: 'gamma', teamName: 'Team Gamma', status: 'not_started' })],
      children: [],
    };
    const directorB: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('dir-b', 'Director B', 'Director'),
      totalTeams: 2,
      optedInTeams: 2,
      completionPercent: 0,
      remindCount: 2,
      directTeams: [team({ teamId: 'beta', teamName: 'Team Beta', status: 'in_progress' })],
      children: [managerD],
    };
    const allCompleteManager: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('mgr-x', 'All Complete Manager', 'Manager'),
      totalTeams: 1,
      optedInTeams: 1,
      completionPercent: 100,
      remindCount: 0,
      directTeams: [team({ teamId: 'done', teamName: 'Done Pod', status: 'complete' })],
      children: [],
    };
    const seniorDirectorA: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('sd-a', 'Senior Director A', 'Senior Director'),
      totalTeams: 4,
      optedInTeams: 4,
      completionPercent: 0,
      remindCount: 3,
      directTeams: [
        team({ teamId: 'alcatraz', teamName: 'Alcatraz', status: 'not_started' }),
        team({ teamId: 'complete-direct', teamName: 'Complete Direct', status: 'complete' }),
      ],
      children: [directorB, allCompleteManager],
    };

    const plan = buildPersonReminderPlan(seniorDirectorA);

    expect(plan.root.type).toBe('person');
    const root = plan.root as Extract<typeof plan.root, { type: 'person' }>;
    expect(root).toMatchObject({ id: 'sd-a', level: 'Senior Director' });
    // The all-complete manager is dropped entirely (no pending pods anywhere
    // in their subtree); the completed direct pod never shows up either.
    expect(root.children).toHaveLength(2);
    expect(root.children[0]).toMatchObject({ type: 'person', id: 'dir-b' });
    const directorNode = root.children[0] as Extract<typeof root.children[0], { type: 'person' }>;
    expect(directorNode.children).toHaveLength(2); // Manager D's node + Beta leaf
    expect(directorNode.children[0]).toMatchObject({ type: 'person', id: 'mgr-d' });
    const managerNode = directorNode.children[0] as Extract<typeof directorNode.children[0], { type: 'person' }>;
    expect(managerNode.children.map((c) => (c as { name: string }).name)).toEqual(['Team Gamma']);
    expect(directorNode.children[1]).toMatchObject({ type: 'team', name: 'Team Beta' });
    expect(root.children[1]).toMatchObject({ type: 'team', name: 'Alcatraz' });

    // 3 pending pods: Alcatraz (direct), Beta (under Director B), Gamma (under Director B -> Manager D).
    expect(plan.podCount).toBe(3);
    expect(plan.subtitle).toBe('Reminding 3 pods under Senior Director A');
    // 2 descendant leaders have pending pods: Director B and Manager D.
    expect(plan.toastMessage).toBe('Reminder sent to Senior Director A, 2 leaders, and 3 team leads.');
  });

  it('uses singular wording for exactly one pending pod and one leader', () => {
    const managerD: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('mgr-d', 'Manager D', 'Manager'),
      totalTeams: 1,
      optedInTeams: 1,
      completionPercent: 0,
      remindCount: 1,
      directTeams: [team({ teamId: 'gamma', teamName: 'Team Gamma', status: 'not_started' })],
      children: [],
    };
    const directorB: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('dir-b', 'Director B', 'Director'),
      totalTeams: 1,
      optedInTeams: 1,
      completionPercent: 0,
      remindCount: 1,
      directTeams: [],
      children: [managerD],
    };

    const plan = buildPersonReminderPlan(directorB);
    expect(plan.podCount).toBe(1);
    expect(plan.subtitle).toBe('Reminding 1 pod under Director B');
    expect(plan.toastMessage).toBe('Reminder sent to Director B, 1 leader, and 1 team lead.');
  });
});

describe('getStatusesInTree', () => {
  it('returns only the statuses actually present, in canonical legend order', () => {
    const owner = person('mgr1', 'Sreejith Rajasekaran', 'Manager');
    const plan = buildTeamReminderPlan(team({ status: 'not_started' }), { owner });
    expect(getStatusesInTree(plan.root)).toEqual(['not_started']);
  });

  it('dedupes and orders statuses across a multi-pod tree as in_progress, not_started, complete, opted_out', () => {
    const group: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('mgr1', 'Sreejith Rajasekaran', 'Manager'),
      totalTeams: 4,
      optedInTeams: 4,
      completionPercent: 0,
      remindCount: 2,
      // buildPersonReminderPlan only keeps pending (not_started/in_progress)
      // teams as leaves, so both appear even though the source list mixes
      // in every status.
      directTeams: [
        team({ teamId: 'a', status: 'not_started' }),
        team({ teamId: 'b', status: 'in_progress' }),
        team({ teamId: 'c', status: 'complete' }),
        team({ teamId: 'd', status: 'opted_out' }),
      ],
      children: [],
    };
    const plan = buildPersonReminderPlan(group);
    expect(getStatusesInTree(plan.root)).toEqual(['in_progress', 'not_started']);
  });

  it('returns an empty array for a tree with no team leaves', () => {
    const emptyGroup: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('mgr1', 'Sreejith Rajasekaran', 'Manager'),
      totalTeams: 0,
      optedInTeams: 0,
      completionPercent: 0,
      remindCount: 0,
      directTeams: [],
      children: [],
    };
    const plan = buildPersonReminderPlan(emptyGroup);
    expect(getStatusesInTree(plan.root)).toEqual([]);
  });
});

describe('buildOrgReminderPlan', () => {
  it('includes a pending "Other" pod alongside a leader\'s own pending pods, previously dropped entirely', () => {
    const managerD: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('mgr-d', 'Manager D', 'Manager'),
      totalTeams: 1,
      optedInTeams: 1,
      completionPercent: 0,
      remindCount: 1,
      directTeams: [team({ teamId: 'gamma', teamName: 'Team Gamma', status: 'not_started' })],
      children: [],
    };
    const groups: SurveyCompletionGroup[] = [
      managerD,
      {
        type: 'other',
        label: 'Other',
        totalTeams: 2,
        optedInTeams: 2,
        completionPercent: 0,
        remindCount: 1,
        teams: [
          team({ teamId: 'orphan-pending', teamName: 'Orphan Pending', status: 'in_progress' }),
          team({ teamId: 'orphan-done', teamName: 'Orphan Done', status: 'complete' }),
        ],
      },
    ];

    const plan = buildOrgReminderPlan(groups);

    expect(plan.root.type).toBe('person');
    const root = plan.root as Extract<typeof plan.root, { type: 'person' }>;
    // One node for Manager D's own subtree, one leaf for the Other pod.
    expect(root.children).toHaveLength(2);
    expect(root.children[0]).toMatchObject({ type: 'person', id: 'mgr-d' });
    expect(root.children[1]).toMatchObject({ type: 'team', id: 'orphan-pending', name: 'Orphan Pending' });
    // The completed Other pod never appears -- only pending ones do.
    expect(root.children.some((c) => 'id' in c && c.id === 'orphan-done')).toBe(false);

    expect(plan.podCount).toBe(2); // gamma + orphan-pending
    expect(plan.subtitle).toBe('Reminding 2 pods across 1 leader');
    expect(plan.toastMessage).toBe('Reminder sent to 1 leader and 2 team leads.');
  });

  it('still builds a usable plan when every pending pod is in "Other" and no leader has any', () => {
    const groups: SurveyCompletionGroup[] = [
      {
        type: 'other',
        label: 'Other',
        totalTeams: 1,
        optedInTeams: 1,
        completionPercent: 0,
        remindCount: 1,
        teams: [team({ teamId: 'orphan-only', teamName: 'Orphan Only', status: 'not_started' })],
      },
    ];

    const plan = buildOrgReminderPlan(groups);

    const root = plan.root as Extract<typeof plan.root, { type: 'person' }>;
    expect(root.children).toEqual([
      { type: 'team', id: 'orphan-only', name: 'Orphan Only', teamLeadName: 'Alice', status: 'not_started' },
    ]);
    expect(plan.podCount).toBe(1);
    expect(plan.subtitle).toBe('Reminding 1 pod with no assigned leader');
    expect(plan.toastMessage).toBe('Reminder sent to 1 team lead.');
  });

  it('returns a zero-pod plan when nothing anywhere is pending', () => {
    const allDone: SurveyCompletionPersonGroup = {
      type: 'person',
      person: person('mgr-x', 'All Complete Manager', 'Manager'),
      totalTeams: 1,
      optedInTeams: 1,
      completionPercent: 100,
      remindCount: 0,
      directTeams: [team({ teamId: 'done', teamName: 'Done Pod', status: 'complete' })],
      children: [],
    };
    const plan = buildOrgReminderPlan([allDone]);
    expect(plan.podCount).toBe(0);
    const root = plan.root as Extract<typeof plan.root, { type: 'person' }>;
    expect(root.children).toEqual([]);
  });
});

// Regression coverage for the reported "pods displayed twice" bug: a
// reminder plan built for one manager must never pull in a pod owned by a
// different manager, even when both pods share the exact same name
// (again using "Aurora", one of the reported names) -- proving the
// reminder tree keys strictly on ownership/teamId, never on name.
describe('buildPersonReminderPlan with two different team IDs sharing the same name', () => {
  const managerA: SurveyCompletionPersonGroup = {
    type: 'person',
    person: person('mgr-a', 'Manager A', 'Manager'),
    totalTeams: 1,
    optedInTeams: 1,
    completionPercent: 0,
    remindCount: 1,
    directTeams: [team({ teamId: 'aurora-1', teamName: 'Aurora', status: 'not_started', completed: 0, total: 2 })],
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

  it("Manager A's reminder plan includes only aurora-1, never Manager B's aurora-2", () => {
    const plan = buildPersonReminderPlan(managerA);
    expect(plan.podCount).toBe(1);
    const root = plan.root as Extract<typeof plan.root, { type: 'person' }>;
    expect(root.children).toHaveLength(1);
    expect(root.children[0]).toMatchObject({ type: 'team', id: 'aurora-1', name: 'Aurora' });
  });

  it("Manager B's reminder plan includes only aurora-2, never Manager A's aurora-1", () => {
    const plan = buildPersonReminderPlan(managerB);
    expect(plan.podCount).toBe(1);
    const root = plan.root as Extract<typeof plan.root, { type: 'person' }>;
    expect(root.children).toHaveLength(1);
    expect(root.children[0]).toMatchObject({ type: 'team', id: 'aurora-2', name: 'Aurora' });
  });
});

// Regression coverage rolling a Level-2 root's reminder plan up through a
// nested manager, using several of the exact reported pod names, and
// confirming none of them appear twice in the resulting tree.
describe('buildPersonReminderPlan with the reported duplicate-pod names', () => {
  const managerA: SurveyCompletionPersonGroup = {
    type: 'person',
    person: person('mgr-a', 'Manager A', 'Manager'),
    totalTeams: 2,
    optedInTeams: 2,
    completionPercent: 0,
    remindCount: 2,
    directTeams: [
      team({ teamId: 't-anekal', teamName: 'Anekal', status: 'not_started', completed: 0, total: 2 }),
      team({ teamId: 't-bolinas', teamName: 'Bolinas', status: 'in_progress', completed: 1, total: 2 }),
    ],
    children: [],
  };
  const l2Root: SurveyCompletionPersonGroup = {
    type: 'person',
    person: person('l2-root', 'L2 Root', 'Director'),
    totalTeams: 3,
    optedInTeams: 3,
    completionPercent: 0,
    remindCount: 3,
    directTeams: [team({ teamId: 't-bobcaygeon', teamName: 'Bobcaygeon', status: 'in_progress', completed: 1, total: 2 })],
    children: [managerA],
  };

  it('rolls up Manager A\'s pending pods under the root with no duplication and an accurate leader count', () => {
    const plan = buildPersonReminderPlan(l2Root);
    expect(plan.podCount).toBe(3);
    expect(plan.toastMessage).toBe('Reminder sent to L2 Root, 1 leader, and 3 team leads.');

    const root = plan.root as Extract<typeof plan.root, { type: 'person' }>;
    expect(root.children).toHaveLength(2); // Manager A's node + Bobcaygeon leaf
    const managerNode = root.children[0] as Extract<typeof root.children[0], { type: 'person' }>;
    const leafIds = managerNode.children.map((c) => (c as { id: string }).id);
    expect(leafIds).toEqual(['t-anekal', 't-bolinas']);
    expect(root.children[1]).toMatchObject({ type: 'team', id: 't-bobcaygeon' });

    // Confirm no pod id appears twice anywhere in the tree.
    const allLeafIds: string[] = [...leafIds, (root.children[1] as { id: string }).id];
    expect(new Set(allLeafIds).size).toBe(allLeafIds.length);
  });
});
