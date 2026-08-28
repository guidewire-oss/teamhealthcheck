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
  // Set only for teams in the "Other" group (no Level-2/Level-3 owner) —
  // the Level-4 team lead reminders escalate to.
  teamLeadId?: string;
  teamLeadName?: string;
}

export interface SurveyCompletionPerson {
  id: string;
  name: string;
  email?: string;
}

export interface SurveyCompletionManagerGroup {
  manager: SurveyCompletionPerson;
  totalTeams: number;
  optedInTeams: number;
  completionPercent: number;
  remindCount: number;
  teams: SurveyCompletionTeam[];
}

/**
 * One top-level row in the survey-completion table:
 * - "director": a Level-2 director, with direct teams and nested Level-3 managers.
 * - "manager": a Level-3 manager with no resolvable director, standing alone.
 * - "other": the catch-all group for teams with neither a director nor a manager.
 */
export type SurveyCompletionGroup =
  | {
      type: 'director';
      director: SurveyCompletionPerson;
      totalTeams: number;
      optedInTeams: number;
      completionPercent: number;
      remindCount: number;
      directTeams: SurveyCompletionTeam[];
      managers: SurveyCompletionManagerGroup[];
    }
  | {
      type: 'manager';
      manager: SurveyCompletionPerson;
      totalTeams: number;
      optedInTeams: number;
      completionPercent: number;
      remindCount: number;
      teams: SurveyCompletionTeam[];
    }
  | {
      type: 'other';
      label: string;
      totalTeams: number;
      optedInTeams: number;
      completionPercent: number;
      remindCount: number;
      teams: SurveyCompletionTeam[];
    };

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
