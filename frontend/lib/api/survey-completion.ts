/**
 * Survey Completion API Client
 *
 * Backend: GET /api/v1/admin/survey-completion
 * (backend/interfaces/api/v1/survey_completion_admin_handler.go)
 */

import { API_BASE_URL, apiRequest, handleResponse } from './client';

export type SurveyStatus = 'complete' | 'in_progress' | 'not_started' | 'opted_out';

export interface SurveyCompletionTeam {
  teamId: string;
  teamName: string;
  completed: number;
  total: number;
  // null when the team is opted out (health_check_enabled = false) — the
  // backend has no post-workshop data to report in that case.
  postWorkshopCompleted: boolean | null;
  status: SurveyStatus;
  // Set whenever the team has a Level-4 team lead — used both for the
  // "Other" group's escalation target and to label this pod's leaf in every
  // reminder-target tree.
  teamLeadId?: string;
  teamLeadName?: string;
}

export interface SurveyCompletionPerson {
  id: string;
  name: string;
  email?: string;
  // hierarchy_level_id — schema-driven, not a display string.
  levelId: string;
  // hierarchy_levels.name — the human-readable role/sub-level label, e.g.
  // "Senior Director", "Director", "Senior Manager", "Manager". Never
  // invented client-side; always whatever the organization's own hierarchy
  // configuration calls that level.
  level: string;
}

/**
 * A leadership person's row in the recursive hierarchy — their own direct
 * pods and, recursively, their own children (always other person groups;
 * "Other" is exclusively a top-level catch-all, never nested under a
 * leader). There is no separate type per tier — the same shape nests to
 * whatever depth an organization's reports_to chains actually go.
 */
export interface SurveyCompletionPersonGroup {
  type: 'person';
  person: SurveyCompletionPerson;
  totalTeams: number;
  optedInTeams: number;
  completionPercent: number;
  remindCount: number;
  directTeams: SurveyCompletionTeam[];
  children: SurveyCompletionPersonGroup[];
}

/** The catch-all top-level group for pods with no resolvable leadership owner. */
export interface SurveyCompletionOtherGroup {
  type: 'other';
  label: string;
  totalTeams: number;
  optedInTeams: number;
  completionPercent: number;
  remindCount: number;
  teams: SurveyCompletionTeam[];
}

/** One top-level row in the survey-completion table: a leader, or "Other". */
export type SurveyCompletionGroup = SurveyCompletionPersonGroup | SurveyCompletionOtherGroup;

export interface SurveyCompletionTrendPoint {
  label: string;
  completion: number;
}

export interface SurveyCompletionOverview {
  assessmentPeriod: string;
  overallCompletion: number;
  totalTeams: number;
  optedIn: number;
  fullyComplete: number;
  inProgress: number;
  notStarted: number;
  optedOut: number;
  groups: SurveyCompletionGroup[];
  timeSeries: SurveyCompletionTrendPoint[];
}

/**
 * Fetches the org-wide survey completion overview for one assessment
 * period. Omit assessmentPeriod to let the backend pick the most recent
 * period with submitted data.
 */
export async function getSurveyCompletion(assessmentPeriod?: string): Promise<SurveyCompletionOverview> {
  const params = new URLSearchParams();
  if (assessmentPeriod) {
    params.append('assessmentPeriod', assessmentPeriod);
  }

  const url = `${API_BASE_URL}/api/v1/admin/survey-completion${
    params.toString() ? `?${params.toString()}` : ''
  }`;

  const response = await apiRequest(url);
  return handleResponse<SurveyCompletionOverview>(response);
}
