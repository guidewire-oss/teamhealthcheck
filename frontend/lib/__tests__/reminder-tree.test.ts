import { describe, expect, it } from 'vitest';
import {
  buildDirectorReminderPlan,
  buildManagerReminderPlan,
  buildTeamReminderPlan,
  getStatusesInTree,
} from '@/lib/reminder-tree';
import type {
  SurveyCompletionGroup,
  SurveyCompletionManagerGroup,
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

describe('buildTeamReminderPlan', () => {
  it('roots the tree at the manager when the pod has one', () => {
    const plan = buildTeamReminderPlan(team({ teamName: 'Goa' }), {
      manager: { id: 'mgr1', name: 'Sreejith Rajasekaran' },
      director: { id: 'dir1', name: 'Satish Patil' },
    });

    expect(plan.root).toEqual({
      type: 'person',
      id: 'mgr1',
      name: 'Sreejith Rajasekaran',
      role: 'manager',
      children: [{ type: 'team', id: 't1', name: 'Goa', teamLeadName: 'Alice', status: 'not_started' }],
    });
    expect(plan.podCount).toBe(1);
    expect(plan.subtitle).toBe('Reminding 1 pod under Sreejith Rajasekaran');
    expect(plan.toastMessage).toBe('Reminder sent to the team lead for Goa.');
  });

  it('falls back to the director when the pod has no manager', () => {
    const plan = buildTeamReminderPlan(team({ teamName: 'Alcatraz' }), {
      director: { id: 'dir1', name: 'Satish Patil' },
    });

    expect(plan.root.type).toBe('person');
    expect(plan.root).toMatchObject({ role: 'director', name: 'Satish Patil' });
  });

  it('has no supervisor root when the pod has neither manager nor director', () => {
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

describe('buildManagerReminderPlan', () => {
  const managerGroup: SurveyCompletionManagerGroup = {
    manager: { id: 'mgr1', name: 'Sreejith Rajasekaran' },
    totalTeams: 3,
    optedInTeams: 3,
    completionPercent: 33,
    remindCount: 2,
    teams: [
      team({ teamId: 'a', teamName: 'Ajanta', status: 'not_started' }),
      team({ teamId: 'g', teamName: 'Gokarna', status: 'in_progress' }),
      team({ teamId: 'c', teamName: 'Complete Pod', status: 'complete' }),
    ],
  };

  it('roots at the manager with only pending pods as children', () => {
    const plan = buildManagerReminderPlan(managerGroup);

    expect(plan.root.type).toBe('person');
    expect(plan.root).toMatchObject({ id: 'mgr1', role: 'manager' });
    const root = plan.root as Extract<typeof plan.root, { type: 'person' }>;
    expect(root.children.map((c) => (c as { name: string }).name)).toEqual(['Ajanta', 'Gokarna']);
    expect(plan.podCount).toBe(2);
    expect(plan.subtitle).toBe('Reminding 2 pods under Sreejith Rajasekaran');
    expect(plan.toastMessage).toBe('Reminder sent to Sreejith Rajasekaran and 2 team leads.');
  });

  it('uses singular wording for exactly one pending pod', () => {
    const single: SurveyCompletionManagerGroup = {
      ...managerGroup,
      teams: [team({ teamId: 'a', teamName: 'Ajanta', status: 'not_started' })],
    };
    const plan = buildManagerReminderPlan(single);
    expect(plan.subtitle).toBe('Reminding 1 pod under Sreejith Rajasekaran');
    expect(plan.toastMessage).toBe('Reminder sent to Sreejith Rajasekaran and 1 team lead.');
  });
});

describe('buildDirectorReminderPlan', () => {
  const directorGroup: SurveyCompletionGroup = {
    type: 'director',
    director: { id: 'dir1', name: 'Satish Patil' },
    totalTeams: 5,
    optedInTeams: 5,
    completionPercent: 20,
    remindCount: 5,
    directTeams: [
      team({ teamId: 'alcatraz', teamName: 'Alcatraz', status: 'not_started' }),
      team({ teamId: 'complete-direct', teamName: 'Complete Direct', status: 'complete' }),
    ],
    managers: [
      {
        manager: { id: 'mgr1', name: 'Sreejith Rajasekaran' },
        totalTeams: 2,
        optedInTeams: 2,
        completionPercent: 0,
        remindCount: 2,
        teams: [
          team({ teamId: 'ajanta', teamName: 'Ajanta', status: 'not_started' }),
          team({ teamId: 'gokarna', teamName: 'Gokarna', status: 'in_progress' }),
        ],
      },
      {
        manager: { id: 'mgr2', name: 'All Complete Manager' },
        totalTeams: 1,
        optedInTeams: 1,
        completionPercent: 100,
        remindCount: 0,
        teams: [team({ teamId: 'done', teamName: 'Done Pod', status: 'complete' })],
      },
    ],
  };

  it('nests managers with pending pods and attaches direct pending pods as leaves', () => {
    const plan = buildDirectorReminderPlan(directorGroup);

    expect(plan.root.type).toBe('person');
    const root = plan.root as Extract<typeof plan.root, { type: 'person' }>;
    expect(root).toMatchObject({ id: 'dir1', role: 'director' });
    // Only the manager with pending pods (mgr1) appears; the all-complete
    // manager (mgr2) is dropped entirely, and the completed direct team
    // never shows up either.
    expect(root.children).toHaveLength(2);
    expect(root.children[0]).toMatchObject({ type: 'person', id: 'mgr1' });
    const managerNode = root.children[0] as Extract<typeof root.children[0], { type: 'person' }>;
    expect(managerNode.children.map((c) => (c as { name: string }).name)).toEqual(['Ajanta', 'Gokarna']);
    expect(root.children[1]).toMatchObject({ type: 'team', name: 'Alcatraz' });
  });

  it('counts only pending pods across the whole subtree for the badge/subtitle/toast', () => {
    const plan = buildDirectorReminderPlan(directorGroup);
    expect(plan.podCount).toBe(3); // Alcatraz, Ajanta, Gokarna
    expect(plan.subtitle).toBe('Reminding 3 pods under Satish Patil');
    expect(plan.toastMessage).toBe('Reminder sent to Satish Patil, 1 manager, and 3 team leads.');
  });
});

describe('getStatusesInTree', () => {
  it('returns only the statuses actually present, in canonical legend order', () => {
    const plan = buildTeamReminderPlan(team({ status: 'not_started' }), {
      manager: { id: 'mgr1', name: 'Sreejith Rajasekaran' },
    });
    expect(getStatusesInTree(plan.root)).toEqual(['not_started']);
  });

  it('dedupes and orders statuses across a multi-pod tree as in_progress, not_started, complete, opted_out', () => {
    const managerGroup: SurveyCompletionManagerGroup = {
      manager: { id: 'mgr1', name: 'Sreejith Rajasekaran' },
      totalTeams: 4,
      optedInTeams: 4,
      completionPercent: 0,
      remindCount: 2,
      // buildManagerReminderPlan only keeps pending (not_started/in_progress)
      // teams as leaves, so both appear even though the source list mixes
      // in every status.
      teams: [
        team({ teamId: 'a', status: 'not_started' }),
        team({ teamId: 'b', status: 'in_progress' }),
        team({ teamId: 'c', status: 'complete' }),
        team({ teamId: 'd', status: 'opted_out' }),
      ],
    };
    const plan = buildManagerReminderPlan(managerGroup);
    expect(getStatusesInTree(plan.root)).toEqual(['in_progress', 'not_started']);
  });

  it('returns an empty array for a tree with no team leaves', () => {
    const emptyManagerGroup: SurveyCompletionManagerGroup = {
      manager: { id: 'mgr1', name: 'Sreejith Rajasekaran' },
      totalTeams: 0,
      optedInTeams: 0,
      completionPercent: 0,
      remindCount: 0,
      teams: [],
    };
    const plan = buildManagerReminderPlan(emptyManagerGroup);
    expect(getStatusesInTree(plan.root)).toEqual([]);
  });
});
