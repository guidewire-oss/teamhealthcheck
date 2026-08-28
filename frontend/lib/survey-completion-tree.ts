/**
 * Pure filtering/search logic for the survey-completion table's
 * director -> manager -> team hierarchy. Split out of
 * SurveyCompletionDashboard.tsx so it can be unit-tested without rendering
 * anything.
 */

import type {
  SurveyCompletionGroup,
  SurveyCompletionManagerGroup,
  SurveyCompletionTeam,
  SurveyStatus,
} from '@/lib/api/survey-completion';

export type CardFilter = 'all' | 'optedIn' | SurveyStatus;

export function isRemindable(team: SurveyCompletionTeam): boolean {
  return team.status === 'in_progress' || team.status === 'not_started';
}

export function matchesCardFilter(team: SurveyCompletionTeam, filter: CardFilter): boolean {
  switch (filter) {
    case 'optedIn':
      return team.status !== 'opted_out';
    case 'complete':
    case 'in_progress':
    case 'not_started':
    case 'opted_out':
      return team.status === filter;
    default:
      return true;
  }
}

export interface VisibleManagerGroup {
  id: string;
  name: string;
  totalTeams: number;
  optedInTeams: number;
  completionPercent: number;
  remindCount: number;
  visibleTeams: SurveyCompletionTeam[];
}

export type VisibleGroup =
  | {
      type: 'director';
      key: string;
      id: string;
      name: string;
      totalTeams: number;
      optedInTeams: number;
      completionPercent: number;
      remindCount: number;
      visibleDirectTeams: SurveyCompletionTeam[];
      visibleManagers: VisibleManagerGroup[];
    }
  | {
      type: 'manager';
      key: string;
      id: string;
      name: string;
      totalTeams: number;
      optedInTeams: number;
      completionPercent: number;
      remindCount: number;
      visibleTeams: SurveyCompletionTeam[];
    }
  | {
      type: 'other';
      key: string;
      id: string;
      name: string;
      totalTeams: number;
      optedInTeams: number;
      completionPercent: number;
      remindCount: number;
      visibleTeams: SurveyCompletionTeam[];
    };

function teamMatches(team: SurveyCompletionTeam, query: string): boolean {
  return team.teamName.toLowerCase().includes(query);
}

function visibleTeamsFor(
  teams: SurveyCompletionTeam[],
  filter: CardFilter,
  query: string,
  ancestorMatches: boolean,
): SurveyCompletionTeam[] {
  return teams.filter(
    (team) => matchesCardFilter(team, filter) && (!query || ancestorMatches || teamMatches(team, query)),
  );
}

function toVisibleManagerGroup(
  manager: SurveyCompletionManagerGroup,
  filter: CardFilter,
  query: string,
  directorMatches: boolean,
): VisibleManagerGroup {
  const managerMatches = !!query && manager.manager.name.toLowerCase().includes(query);
  return {
    id: manager.manager.id,
    name: manager.manager.name,
    totalTeams: manager.totalTeams,
    optedInTeams: manager.optedInTeams,
    completionPercent: manager.completionPercent,
    remindCount: manager.remindCount,
    visibleTeams: visibleTeamsFor(manager.teams ?? [], filter, query, directorMatches || managerMatches),
  };
}

/**
 * Filters the group hierarchy by the active metric-card filter and search
 * query, dropping any group/manager left with no visible teams. When a
 * search query is present, a group/manager whose own name matches the query
 * shows all of its (filter-matching) teams, not just the ones whose name
 * also matches — matching director/manager/"Other"/team name is exactly
 * the contract the search box promises.
 */
export function buildVisibleGroups(
  groups: SurveyCompletionGroup[],
  searchQuery: string,
  filter: CardFilter,
): VisibleGroup[] {
  const query = searchQuery.trim().toLowerCase();

  const visible: VisibleGroup[] = [];

  for (const group of groups) {
    if (group.type === 'director') {
      const directorMatches = !!query && group.director.name.toLowerCase().includes(query);
      const visibleDirectTeams = visibleTeamsFor(group.directTeams ?? [], filter, query, directorMatches);
      const visibleManagers = (group.managers ?? [])
        .map((m) => toVisibleManagerGroup(m, filter, query, directorMatches))
        .filter((m) => m.visibleTeams.length > 0);

      if (visibleDirectTeams.length === 0 && visibleManagers.length === 0) continue;

      visible.push({
        type: 'director',
        key: `director:${group.director.id}`,
        id: group.director.id,
        name: group.director.name,
        totalTeams: group.totalTeams,
        optedInTeams: group.optedInTeams,
        completionPercent: group.completionPercent,
        remindCount: group.remindCount,
        visibleDirectTeams,
        visibleManagers,
      });
    } else if (group.type === 'manager') {
      const managerMatches = !!query && group.manager.name.toLowerCase().includes(query);
      const visibleTeams = visibleTeamsFor(group.teams ?? [], filter, query, managerMatches);
      if (visibleTeams.length === 0) continue;

      visible.push({
        type: 'manager',
        key: `manager:${group.manager.id}`,
        id: group.manager.id,
        name: group.manager.name,
        totalTeams: group.totalTeams,
        optedInTeams: group.optedInTeams,
        completionPercent: group.completionPercent,
        remindCount: group.remindCount,
        visibleTeams,
      });
    } else {
      const labelMatches = !!query && group.label.toLowerCase().includes(query);
      const visibleTeams = visibleTeamsFor(group.teams ?? [], filter, query, labelMatches);
      if (visibleTeams.length === 0) continue;

      visible.push({
        type: 'other',
        key: 'other',
        id: 'other',
        name: group.label,
        totalTeams: group.totalTeams,
        optedInTeams: group.optedInTeams,
        completionPercent: group.completionPercent,
        remindCount: group.remindCount,
        visibleTeams,
      });
    }
  }

  return visible;
}
