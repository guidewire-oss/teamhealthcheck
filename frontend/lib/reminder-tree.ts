/**
 * Pure builder for the "reminder target tree" shown when an admin clicks a
 * Remind button on the survey-completion dashboard. Mirrors the same
 * selection logic the real reminder endpoint will eventually use, so the
 * tree is always an accurate preview of who gets mailed.
 *
 * Kept dependency-free from React so it can be unit-tested without
 * rendering anything — see lib/__tests__/reminder-tree.test.ts.
 */

import { isRemindable } from '@/lib/survey-completion-tree';
import { STATUS_ORDER } from '@/lib/status-colors';
import type {
  SurveyCompletionGroup,
  SurveyCompletionManagerGroup,
  SurveyCompletionPerson,
  SurveyCompletionTeam,
  SurveyStatus,
} from '@/lib/api/survey-completion';

export type ReminderTreeNode =
  | { type: 'person'; id: string; name: string; role: 'director' | 'manager'; children: ReminderTreeNode[] }
  | { type: 'team'; id: string; name: string; teamLeadName: string; status: SurveyStatus };

export interface ReminderPlan {
  root: ReminderTreeNode;
  podCount: number;
  subtitle: string;
  toastMessage: string;
}

function teamNode(team: SurveyCompletionTeam): ReminderTreeNode {
  return {
    type: 'team',
    id: team.teamId,
    name: team.teamName,
    teamLeadName: team.teamLeadName ?? 'Unassigned',
    status: team.status,
  };
}

function personNode(
  person: SurveyCompletionPerson,
  role: 'director' | 'manager',
  children: ReminderTreeNode[],
): ReminderTreeNode {
  return { type: 'person', id: person.id, name: person.name, role, children };
}

function plural(count: number, noun: string): string {
  return `${count} ${noun}${count === 1 ? '' : 's'}`;
}

/**
 * Team-level plan (single pod row's Remind button). The root is the highest
 * person who'll be reminded — the pod's manager if it has one, else its
 * director, else there's no supervisor and the tree is just the pod itself.
 */
export function buildTeamReminderPlan(
  team: SurveyCompletionTeam,
  context: { manager?: SurveyCompletionPerson; director?: SurveyCompletionPerson },
): ReminderPlan {
  const leaf = teamNode(team);
  const supervisor = context.manager ?? context.director;
  const root = supervisor ? personNode(supervisor, context.manager ? 'manager' : 'director', [leaf]) : leaf;

  return {
    root,
    podCount: 1,
    subtitle: supervisor
      ? `Reminding 1 pod under ${supervisor.name}`
      : `Reminding the team lead for ${team.teamName}`,
    toastMessage: `Reminder sent to the team lead for ${team.teamName}.`,
  };
}

/**
 * Manager-level plan (Remind (N) next to a manager row). Root is the
 * manager; one leaf per pending pod under them.
 */
export function buildManagerReminderPlan(managerGroup: SurveyCompletionManagerGroup): ReminderPlan {
  const pendingTeams = (managerGroup.teams ?? []).filter(isRemindable);
  const root = personNode(managerGroup.manager, 'manager', pendingTeams.map(teamNode));
  const podCount = pendingTeams.length;

  return {
    root,
    podCount,
    subtitle: `Reminding ${plural(podCount, 'pod')} under ${managerGroup.manager.name}`,
    toastMessage: `Reminder sent to ${managerGroup.manager.name} and ${plural(podCount, 'team lead')}.`,
  };
}

type DirectorGroup = Extract<SurveyCompletionGroup, { type: 'director' }>;

/**
 * Director-level plan (Remind (N) next to a director row). Root is the
 * director; children are one node per manager under them who has at least
 * one pending pod (with that manager's pending pods nested underneath), plus
 * a leaf for each pending pod reporting directly to the director.
 */
export function buildDirectorReminderPlan(directorGroup: DirectorGroup): ReminderPlan {
  const directPending = (directorGroup.directTeams ?? []).filter(isRemindable);

  const managerNodes: ReminderTreeNode[] = [];
  let managerCount = 0;
  let podCount = directPending.length;

  for (const managerGroup of directorGroup.managers ?? []) {
    const pendingTeams = (managerGroup.teams ?? []).filter(isRemindable);
    if (pendingTeams.length === 0) continue;
    managerCount += 1;
    podCount += pendingTeams.length;
    managerNodes.push(personNode(managerGroup.manager, 'manager', pendingTeams.map(teamNode)));
  }

  const root = personNode(directorGroup.director, 'director', [...managerNodes, ...directPending.map(teamNode)]);

  return {
    root,
    podCount,
    subtitle: `Reminding ${plural(podCount, 'pod')} under ${directorGroup.director.name}`,
    toastMessage: `Reminder sent to ${directorGroup.director.name}, ${plural(managerCount, 'manager')}, and ${plural(podCount, 'team lead')}.`,
  };
}

/**
 * Distinct statuses that appear among the team leaves of a reminder tree, in
 * the canonical legend order — used to show only the legend entries that are
 * actually relevant to the tree on screen.
 */
export function getStatusesInTree(node: ReminderTreeNode): SurveyStatus[] {
  const seen = new Set<SurveyStatus>();

  const visit = (n: ReminderTreeNode) => {
    if (n.type === 'team') {
      seen.add(n.status);
    } else {
      n.children.forEach(visit);
    }
  };
  visit(node);

  return STATUS_ORDER.filter((status) => seen.has(status));
}
