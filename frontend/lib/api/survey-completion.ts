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

/** Who a reminder is for. The server works out the recipients from this. */
export type SurveyReminderRequest =
  | { scope: 'team'; teamId: string; assessmentPeriod?: string }
  | { scope: 'person'; personId: string; assessmentPeriod?: string }
  | { scope: 'org'; assessmentPeriod?: string };

export interface SurveyReminderResult {
  assessmentPeriod: string;
  // 'log' means email delivery is switched off on the server (EMAIL_MODE=log):
  // nothing was sent, and each reminder was written to the server log instead.
  mode: 'send' | 'log';
  // Set when the server limited recipients to one test pod (EMAIL_TEST_POD).
  testPod?: string;
  // The test addresses every email was delivered to INSTEAD of its real
  // recipient (EMAIL_REDIRECT_TO). Empty when emails went to real recipients.
  redirectedTo?: string[];
  sent: number;
  sentTo: string[];
  // Who the reminder would have gone to, in log mode.
  logged: number;
  loggedFor: string[];
  // Recipients with no email address on file.
  skippedNoEmail: string[];
  // Recipients the mail server rejected.
  failed: string[];
  // Lagging pods that have no team lead to remind.
  podsWithoutTeamLead: string[];
}

/**
 * Emails the people behind a Remind button. The server decides who from the
 * same data the dashboard shows, so the reminder reaches the people the
 * preview listed.
 *
 * Backend: POST /api/v1/admin/survey-completion/reminders
 */
export async function sendSurveyReminders(request: SurveyReminderRequest): Promise<SurveyReminderResult> {
  const response = await apiRequest(`${API_BASE_URL}/api/v1/admin/survey-completion/reminders`, {
    method: 'POST',
    body: JSON.stringify(request),
  });
  return handleResponse<SurveyReminderResult>(response);
}
