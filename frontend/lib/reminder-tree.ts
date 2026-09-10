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
  SurveyCompletionPerson,
  SurveyCompletionPersonGroup,
  SurveyCompletionTeam,
  SurveyStatus,
} from '@/lib/api/survey-completion';

export type ReminderTreeNode =
  | { type: 'person'; id: string; name: string; level: string; children: ReminderTreeNode[] }
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

function personNode(person: SurveyCompletionPerson, children: ReminderTreeNode[]): ReminderTreeNode {
  return { type: 'person', id: person.id, name: person.name, level: person.level, children };
}

function plural(count: number, noun: string): string {
  return `${count} ${noun}${count === 1 ? '' : 's'}`;
}

/**
 * Team-level plan (single pod row's Remind button). The root is the pod's
 * direct owner if it has one, else there's no supervisor and the tree is
 * just the pod itself.
 */
export function buildTeamReminderPlan(
  team: SurveyCompletionTeam,
  context: { owner?: SurveyCompletionPerson },
): ReminderPlan {
  const leaf = teamNode(team);
  const root = context.owner ? personNode(context.owner, [leaf]) : leaf;

  // The preview tree shows the owner as a node right alongside the pod when
  // there is one, which reads as "this leader is also part of who gets
  // notified" — the toast must name them too, or it under-reports who the
  // preview just showed as a recipient.
  return {
    root,
    podCount: 1,
    subtitle: context.owner
      ? `Reminding 1 pod under ${context.owner.name}`
      : `Reminding the team lead for ${team.teamName}`,
    toastMessage: context.owner
      ? `Reminder sent to ${context.owner.name} and the team lead for ${team.teamName}.`
      : `Reminder sent to the team lead for ${team.teamName}.`,
  };
}

interface Subtree {
  node: ReminderTreeNode;
  pendingCount: number;
  // Number of descendant leaders (not including this node) that have at
  // least one pending pod anywhere in their own subtree.
  leaderCountInSubtree: number;
}

/**
 * Recursively builds this leader's reminder subtree: their own pending
 * direct pods, plus (only) the children who themselves have at least one
 * pending pod anywhere below them — an all-complete branch is dropped
 * entirely, at any depth, exactly like the old two-tier behavior generalized
 * to arbitrary depth.
 */
function buildSubtree(group: SurveyCompletionPersonGroup): Subtree {
  const directPending = (group.directTeams ?? []).filter(isRemindable);

  const childNodes: ReminderTreeNode[] = [];
  let leaderCount = 0;
  let podCount = directPending.length;

  for (const child of group.children ?? []) {
    const sub = buildSubtree(child);
    if (sub.pendingCount === 0) continue;
    leaderCount += 1 + sub.leaderCountInSubtree;
    podCount += sub.pendingCount;
    childNodes.push(sub.node);
  }

  return {
    node: personNode(group.person, [...childNodes, ...directPending.map(teamNode)]),
    pendingCount: podCount,
    leaderCountInSubtree: leaderCount,
  };
}

/**
 * Leader-level plan (Remind (N) next to any person row, at any depth).
 * Root is that leader; children are one node per descendant leader who has
 * at least one pending pod (nested arbitrarily deep, same as the real
 * hierarchy), plus a leaf for each of the leader's own pending direct pods.
 */
export function buildPersonReminderPlan(group: SurveyCompletionPersonGroup): ReminderPlan {
  const { node, pendingCount, leaderCountInSubtree } = buildSubtree(group);

  return {
    root: node,
    podCount: pendingCount,
    subtitle: `Reminding ${plural(pendingCount, 'pod')} under ${group.person.name}`,
    toastMessage:
      leaderCountInSubtree > 0
        ? `Reminder sent to ${group.person.name}, ${plural(leaderCountInSubtree, 'leader')}, and ${plural(pendingCount, 'team lead')}.`
        : `Reminder sent to ${group.person.name} and ${plural(pendingCount, 'team lead')}.`,
  };
}

/**
 * Org-wide bulk plan (the "Remind N lagging leaders" button above the
 * table). Previously this button skipped the preview modal entirely and
 * fired a canned toast straight away, and its count only ever looked at
 * person-owned root groups — a pending pod with no resolvable owner (the
 * "Other" bucket) was silently never remindable from this button at all.
 * This builds one real plan, exactly like every other Remind button: one
 * child node per root leader with at least one pending pod (via
 * buildSubtree, unchanged), PLUS the "Other" group's own pending pods as
 * sibling leaves — so Other's pending teams are always included in both
 * the preview and the count, never dropped.
 */
export function buildOrgReminderPlan(groups: SurveyCompletionGroup[]): ReminderPlan {
  const children: ReminderTreeNode[] = [];
  let podCount = 0;
  let leaderCount = 0;

  for (const group of groups) {
    if (group.type !== 'person') continue;
    const sub = buildSubtree(group);
    if (sub.pendingCount === 0) continue;
    leaderCount += 1 + sub.leaderCountInSubtree;
    podCount += sub.pendingCount;
    children.push(sub.node);
  }

  const otherPending = groups
    .filter((group): group is Extract<SurveyCompletionGroup, { type: 'other' }> => group.type === 'other')
    .flatMap((group) => (group.teams ?? []).filter(isRemindable));
  podCount += otherPending.length;
  children.push(...otherPending.map(teamNode));

  return {
    root: { type: 'person', id: 'org-remind-all', name: 'All lagging teams', level: 'Org-wide', children },
    podCount,
    subtitle:
      leaderCount > 0
        ? `Reminding ${plural(podCount, 'pod')} across ${plural(leaderCount, 'leader')}`
        : `Reminding ${plural(podCount, 'pod')} with no assigned leader`,
    toastMessage:
      leaderCount > 0
        ? `Reminder sent to ${plural(leaderCount, 'leader')} and ${plural(podCount, 'team lead')}.`
        : `Reminder sent to ${plural(podCount, 'team lead')}.`,
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
