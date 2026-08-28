"use client";

import { Fragment, useEffect, useRef, useState } from "react";
import {
  Search,
  X,
  Send,
  LayoutGrid,
  CheckCircle2,
  TrendingUp,
  ShieldCheck,
  Clock,
  Circle,
  MinusCircle,
  ChevronDown,
  ChevronRight,
} from "lucide-react";
import OverallAnalyticsView from "./OverallAnalyticsView";
import ReminderTreeModal from "./ReminderTreeModal";
import { getAssessmentPeriods } from "@/lib/api/health-checks";
import {
  getSurveyCompletion,
  type SurveyCompletionGroup,
  type SurveyCompletionManagerGroup,
  type SurveyCompletionOverview,
  type SurveyCompletionPerson,
  type SurveyCompletionTeam,
  type SurveyStatus,
} from "@/lib/api/survey-completion";
import {
  buildVisibleGroups,
  isRemindable,
  type CardFilter,
  type VisibleGroup,
} from "@/lib/survey-completion-tree";
import {
  buildDirectorReminderPlan,
  buildManagerReminderPlan,
  buildTeamReminderPlan,
  type ReminderPlan,
} from "@/lib/reminder-tree";
import { STATUS_BADGE_CLASS, STATUS_LABEL } from "@/lib/status-colors";

/**
 * Survey completion dashboard for the admin console.
 *
 * All data is fetched from the backend (GET /api/v1/admin/survey-completion,
 * plus GET /api/v1/assessment-periods for the period dropdown) — there is no
 * mock data or local fixture here. Whatever database the backend's
 * DATABASE_URL points at is what this dashboard shows.
 *
 * The table groups teams into a Director (Level 2) -> Manager (Level 3) ->
 * Team hierarchy (see lib/survey-completion-tree.ts for the filtering
 * logic). A manager with no resolvable director stands alone as a top-level
 * row; a team with neither falls into the catch-all "Other" group.
 *
 * The "Remind" buttons are UI-only for now (a local toast) — there is no
 * backend endpoint for sending reminders yet.
 *
 * --- How to verify it's reading the DB behind DATABASE_URL ---
 * With the backend pointed at teams360_dummy, flip a team's status and
 * reload this page to see the change:
 *
 *   docker exec teams360-db psql -U postgres -d teams360_dummy -c \
 *     "UPDATE public.teams SET health_check_enabled = FALSE WHERE id = 'sunnyvale';"
 *
 * Reload the dashboard — 'sunnyvale' should now show as "Opted out" and
 * disappear from the completion percentages. Flip it back with
 * health_check_enabled = TRUE to undo.
 */

/**
 * Shared data contract for the metric cards and the "Overall completion
 * analytics" view they open — both read from the same derived object so the
 * numbers never drift apart. Field names/shape are unchanged from the
 * pre-API version so OverallAnalyticsView and the chart components below it
 * don't need to change.
 */
export type { SurveyStatus };

export interface SurveyCompletionData {
  overallCompletion: number;
  totalTeams: number;
  optedIn: number;
  fullyComplete: number;
  inProgress: number;
  notStarted: number;
  optedOut: number;
  teamStats: {
    teamName: string;
    completed: number;
    total: number;
    status: SurveyStatus;
  }[];
  timeSeries: { label: string; completion: number }[];
}

const FILTER_LABELS: Record<CardFilter, string> = {
  all: "All teams",
  optedIn: "Opted in",
  complete: "Fully complete",
  in_progress: "In progress",
  not_started: "Not started",
  opted_out: "Opted out",
};

function getInitials(name: string): string {
  return name
    .split(" ")
    .map((part) => part[0])
    .join("")
    .toUpperCase();
}

function teamsInGroup(group: SurveyCompletionOverview["groups"][number]): SurveyCompletionTeam[] {
  // Defensive against a malformed/older API response — the backend always
  // sends [] rather than omitting these, but don't let a shape mismatch
  // white-screen the whole dashboard.
  if (group.type === "director") {
    return [...(group.directTeams ?? []), ...(group.managers ?? []).flatMap((m) => m.teams ?? [])];
  }
  return group.teams ?? [];
}

function toSurveyCompletionData(overview: SurveyCompletionOverview): SurveyCompletionData {
  return {
    overallCompletion: overview.overallCompletion,
    totalTeams: overview.totalTeams,
    optedIn: overview.optedIn,
    fullyComplete: overview.fullyComplete,
    inProgress: overview.inProgress,
    notStarted: overview.notStarted,
    optedOut: overview.optedOut,
    teamStats: overview.groups.flatMap((group) =>
      teamsInGroup(group).map((team) => ({
        teamName: team.teamName,
        completed: team.completed,
        total: team.total,
        status: team.status,
      })),
    ),
    timeSeries: overview.timeSeries,
  };
}

function MetricCard({
  label,
  value,
  icon,
  iconClass,
  active,
  onClick,
  footer,
  testId,
}: {
  label: string;
  value: string | number;
  icon: React.ReactNode;
  iconClass: string;
  active?: boolean;
  onClick?: () => void;
  footer?: React.ReactNode;
  testId?: string;
}) {
  const clickable = !!onClick;

  return (
    <button
      type="button"
      onClick={onClick}
      disabled={!clickable}
      data-testid={testId}
      className={`text-left bg-white rounded-xl border p-4 shadow-sm transition-all focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-1 ${
        clickable ? "hover:shadow-md hover:-translate-y-0.5 hover:scale-[1.01] cursor-pointer" : "cursor-default"
      } ${active ? "border-indigo-500 ring-1 ring-indigo-200" : "border-gray-200"}`}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-semibold text-gray-500">{label}</span>
        <span className={`w-8 h-8 rounded-full flex items-center justify-center flex-shrink-0 ${iconClass}`}>
          {icon}
        </span>
      </div>
      <div className="text-3xl font-semibold text-gray-900 mt-3 tabular-nums">{value}</div>
      {footer}
    </button>
  );
}

function StatusBadge({ status }: { status: SurveyStatus }) {
  return (
    <span
      className={`inline-flex items-center justify-center w-full max-w-[120px] px-2.5 py-1 rounded-full text-xs font-semibold ${STATUS_BADGE_CLASS[status]}`}
    >
      {STATUS_LABEL[status]}
    </span>
  );
}

function ChevronToggle({ expanded }: { expanded: boolean }) {
  return expanded ? (
    <ChevronDown className="w-4 h-4 text-gray-500 flex-shrink-0" />
  ) : (
    <ChevronRight className="w-4 h-4 text-gray-500 flex-shrink-0" />
  );
}

function TeamRow({
  team,
  indentClass,
  onRemind,
}: {
  team: SurveyCompletionTeam;
  indentClass: string;
  onRemind: () => void;
}) {
  return (
    <tr className="border-b border-gray-100 last:border-b-0 hover:bg-indigo-50/40 transition-colors">
      <td className={`px-5 py-3 text-sm text-gray-900 ${indentClass}`}>{team.teamName}</td>
      <td
        className={`px-5 py-3 text-sm font-semibold ${
          team.status === "opted_out"
            ? "text-gray-900"
            : team.completed === team.total
              ? "text-green-700"
              : "text-gray-900"
        }`}
      >
        {team.status === "opted_out" ? "—" : `${team.completed} / ${team.total}`}
      </td>
      <td className="px-5 py-3 text-sm text-gray-900">
        {team.postWorkshopCompleted === null ? "—" : team.postWorkshopCompleted ? "Yes" : "No"}
      </td>
      <td className="px-5 py-3">
        <div className="flex justify-center">
          <StatusBadge status={team.status} />
        </div>
      </td>
      <td className="px-5 py-3 text-right">
        <button
          type="button"
          onClick={onRemind}
          className={`px-3 py-1.5 border border-gray-300 rounded-lg text-xs font-semibold text-gray-700 hover:border-indigo-400 hover:text-indigo-600 transition-colors focus:outline-none focus:ring-2 focus:ring-indigo-500 whitespace-nowrap ${
            isRemindable(team) ? "" : "invisible"
          }`}
        >
          Remind
        </button>
      </td>
    </tr>
  );
}

function GroupHeaderRow({
  name,
  badge,
  totalTeams,
  optedInTeams,
  completionPercent,
  remindCount,
  expanded,
  onToggle,
  onRemind,
  indentClass,
}: {
  name: string;
  badge?: string;
  totalTeams: number;
  optedInTeams: number;
  completionPercent: number;
  remindCount: number;
  expanded: boolean;
  onToggle: () => void;
  onRemind: () => void;
  indentClass?: string;
}) {
  return (
    <tr className="bg-gray-50 border-b border-gray-200">
      <td colSpan={3} className="px-5 py-3">
        <button
          type="button"
          onClick={onToggle}
          className={`flex items-center gap-3 text-left ${indentClass ?? ""}`}
        >
          <ChevronToggle expanded={expanded} />
          <span className="w-7 h-7 rounded-full bg-indigo-600 text-white text-[11px] font-bold flex items-center justify-center flex-shrink-0">
            {getInitials(name)}
          </span>
          <div>
            <span className="text-sm font-bold text-gray-900">{name}</span>
            {badge && (
              <span className="ml-2 px-1.5 py-0.5 rounded text-[10px] font-bold uppercase tracking-wide bg-indigo-100 text-indigo-700 align-middle">
                {badge}
              </span>
            )}
            <span className="text-sm text-gray-500 ml-2">
              {totalTeams} teams &middot; {optedInTeams} opted in
            </span>
          </div>
        </button>
      </td>
      <td className="px-5 py-3 text-center">
        <span
          className={`inline-flex items-center justify-center w-full max-w-[120px] px-2.5 py-1 rounded-full text-xs font-bold ${
            completionPercent >= 50 ? "bg-amber-100 text-amber-800" : "bg-red-100 text-red-800"
          }`}
        >
          {completionPercent}% complete
        </span>
      </td>
      <td className="px-5 py-3 text-right">
        <button
          type="button"
          onClick={onRemind}
          data-testid="survey-remind-leader"
          className={`px-3 py-1.5 border border-gray-300 rounded-lg text-xs font-semibold text-gray-700 hover:border-indigo-400 hover:text-indigo-600 transition-colors focus:outline-none focus:ring-2 focus:ring-indigo-500 whitespace-nowrap ${
            remindCount === 0 ? "invisible" : ""
          }`}
        >
          Remind ({remindCount})
        </button>
      </td>
    </tr>
  );
}

function OtherHeaderRow({ name, expanded, onToggle }: { name: string; expanded: boolean; onToggle: () => void }) {
  return (
    <tr className="bg-gray-50 border-b border-gray-200">
      <td colSpan={5} className="px-5 py-3">
        <button type="button" onClick={onToggle} className="flex items-center gap-3 text-left">
          <ChevronToggle expanded={expanded} />
          <span className="text-sm font-bold text-gray-900">{name}</span>
        </button>
      </td>
    </tr>
  );
}

export default function SurveyCompletionDashboard() {
  const [periods, setPeriods] = useState<string[]>([]);
  const [timePeriod, setTimePeriod] = useState<string | undefined>(undefined);
  const [overview, setOverview] = useState<SurveyCompletionOverview | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState("");
  const [activeFilter, setActiveFilter] = useState<CardFilter>("all");
  const [showOverallAnalytics, setShowOverallAnalytics] = useState(false);
  const [toast, setToast] = useState<string | null>(null);
  const toastTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set());
  const [expandedManagerRows, setExpandedManagerRows] = useState<Set<string>>(new Set());
  const [reminderPlan, setReminderPlan] = useState<ReminderPlan | null>(null);

  // Load the list of assessment periods once, for the period dropdown.
  useEffect(() => {
    getAssessmentPeriods()
      .then((fetched) => setPeriods(fetched))
      .catch(() => setPeriods([]));
  }, []);

  // Fetch the overview whenever the selected period changes. Passing
  // `undefined` lets the backend pick the most recent period with data,
  // which is also how the period dropdown gets its initial selection.
  useEffect(() => {
    let cancelled = false;
    setIsLoading(true);
    setError(null);

    getSurveyCompletion(timePeriod)
      .then((data) => {
        if (cancelled) return;
        setOverview(data);
        setTimePeriod((current) => current ?? data.assessmentPeriod);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(err instanceof Error ? err.message : "Failed to load survey completion data");
      })
      .finally(() => {
        if (!cancelled) setIsLoading(false);
      });

    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [timePeriod]);

  const showToast = (message: string) => {
    // Placeholder feedback only — wire this to a real reminder endpoint later.
    console.log("[SurveyCompletionDashboard]", message);
    setToast(message);
    if (toastTimer.current) clearTimeout(toastTimer.current);
    toastTimer.current = setTimeout(() => setToast(null), 2200);
  };

  const toggleFilter = (filter: CardFilter) => {
    // These cards filter the "All teams" table — make sure it's actually on
    // screen (leaving the analytics view if it was open) so the filter is
    // visible, same as every other card's existing behavior.
    setShowOverallAnalytics(false);
    setActiveFilter((current) => (current === filter ? "all" : filter));
  };

  const toggleGroup = (key: string) => {
    setExpandedGroups((current) => {
      const next = new Set(current);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  const toggleManagerRow = (managerId: string) => {
    setExpandedManagerRows((current) => {
      const next = new Set(current);
      if (next.has(managerId)) next.delete(managerId);
      else next.add(managerId);
      return next;
    });
  };

  const findDirectorGroup = (directorId: string): Extract<SurveyCompletionGroup, { type: "director" }> | undefined => {
    return overview?.groups.find(
      (g): g is Extract<SurveyCompletionGroup, { type: "director" }> => g.type === "director" && g.director.id === directorId,
    );
  };

  const findManagerGroup = (managerId: string): SurveyCompletionManagerGroup | undefined => {
    for (const g of overview?.groups ?? []) {
      if (g.type === "manager" && g.manager.id === managerId) {
        return { manager: g.manager, totalTeams: g.totalTeams, optedInTeams: g.optedInTeams, completionPercent: g.completionPercent, remindCount: g.remindCount, teams: g.teams };
      }
      if (g.type === "director") {
        const mg = (g.managers ?? []).find((m) => m.manager.id === managerId);
        if (mg) return mg;
      }
    }
    return undefined;
  };

  const openTeamReminder = (
    team: SurveyCompletionTeam,
    context: { manager?: SurveyCompletionPerson; director?: SurveyCompletionPerson },
  ) => {
    setReminderPlan(buildTeamReminderPlan(team, context));
  };

  const openManagerReminder = (managerId: string) => {
    const managerGroup = findManagerGroup(managerId);
    if (!managerGroup) return;
    setReminderPlan(buildManagerReminderPlan(managerGroup));
  };

  const openDirectorReminder = (directorId: string) => {
    const directorGroup = findDirectorGroup(directorId);
    if (!directorGroup) return;
    setReminderPlan(buildDirectorReminderPlan(directorGroup));
  };

  const handleSendReminder = () => {
    if (!reminderPlan) return;
    showToast(reminderPlan.toastMessage);
    setReminderPlan(null);
  };

  if (isLoading && !overview) {
    return (
      <div className="bg-white rounded-xl border border-gray-200 p-12 text-center text-sm text-gray-500" data-testid="survey-dashboard-loading">
        Loading survey completion data…
      </div>
    );
  }

  if (error && !overview) {
    return (
      <div
        className="bg-white rounded-xl border border-red-200 p-12 text-center text-sm text-red-600"
        data-testid="survey-dashboard-error"
      >
        Failed to load survey completion data: {error}
      </div>
    );
  }

  if (!overview) {
    return null;
  }

  const data = toSurveyCompletionData(overview);
  const laggingGroupCount = overview.groups.filter(
    (group) => group.type !== "other" && group.remindCount > 0,
  ).length;

  const searchActive = searchQuery.trim().length > 0;
  const visibleGroups = buildVisibleGroups(overview.groups, searchQuery, activeFilter);
  const visibleCount = visibleGroups.reduce((sum, group) => {
    if (group.type === "director") {
      return sum + group.visibleDirectTeams.length + group.visibleManagers.reduce((s, m) => s + m.visibleTeams.length, 0);
    }
    return sum + group.visibleTeams.length;
  }, 0);
  const noResults = searchActive && visibleCount === 0;

  const isGroupExpanded = (group: VisibleGroup) => searchActive || expandedGroups.has(group.key);
  const isManagerExpanded = (managerId: string) => searchActive || expandedManagerRows.has(managerId);

  return (
    <div className="space-y-6" data-testid="survey-dashboard">
      {/* Header */}
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h2 className="text-xl font-semibold text-gray-900">Survey completion</h2>
          <p className="text-sm text-gray-500 mt-1">
            Admin only &middot; PDO department &middot; participation metadata &mdash; no individual responses
          </p>
        </div>
        <label className="flex items-center gap-2 text-sm font-medium text-gray-700">
          Time period
          <select
            value={timePeriod ?? overview.assessmentPeriod}
            onChange={(e) => setTimePeriod(e.target.value)}
            data-testid="survey-period-select"
            className="px-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
          >
            {periods.map((period) => (
              <option key={period} value={period}>
                {period}
              </option>
            ))}
          </select>
        </label>
      </div>

      {/* Summary metric cards */}
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-7 gap-4">
        <MetricCard
          label="Total teams"
          value={data.totalTeams}
          icon={<LayoutGrid className="w-4 h-4" />}
          iconClass="bg-indigo-50 text-indigo-600"
          active={activeFilter === "all" && !showOverallAnalytics}
          onClick={() => {
            setShowOverallAnalytics(false);
            setActiveFilter("all");
          }}
          testId="survey-card-total"
        />
        <MetricCard
          label="Opted in"
          value={data.optedIn}
          icon={<CheckCircle2 className="w-4 h-4" />}
          iconClass="bg-green-50 text-green-700"
          active={activeFilter === "optedIn" && !showOverallAnalytics}
          onClick={() => toggleFilter("optedIn")}
          testId="survey-card-opted-in"
        />
        <MetricCard
          label="Overall completion"
          value={`${data.overallCompletion}%`}
          icon={<TrendingUp className="w-4 h-4" />}
          iconClass="bg-indigo-50 text-indigo-600"
          active={showOverallAnalytics}
          onClick={() => setShowOverallAnalytics(true)}
          testId="survey-card-completion"
          footer={
            <div className="mt-2.5 h-1.5 rounded-full bg-gray-100 overflow-hidden">
              <div
                className="h-full bg-indigo-600 rounded-full transition-all"
                style={{ width: `${data.overallCompletion}%` }}
              />
            </div>
          }
        />
        <MetricCard
          label="Fully complete"
          value={data.fullyComplete}
          icon={<ShieldCheck className="w-4 h-4" />}
          iconClass="bg-green-50 text-green-700"
          active={activeFilter === "complete" && !showOverallAnalytics}
          onClick={() => toggleFilter("complete")}
          testId="survey-card-complete"
        />
        <MetricCard
          label="In progress"
          value={data.inProgress}
          icon={<Clock className="w-4 h-4" />}
          iconClass="bg-amber-50 text-amber-700"
          active={activeFilter === "in_progress" && !showOverallAnalytics}
          onClick={() => toggleFilter("in_progress")}
          testId="survey-card-in-progress"
        />
        <MetricCard
          label="Not started"
          value={data.notStarted}
          icon={<Circle className="w-4 h-4" strokeDasharray="3 3" />}
          iconClass="bg-red-50 text-red-700"
          active={activeFilter === "not_started" && !showOverallAnalytics}
          onClick={() => toggleFilter("not_started")}
          testId="survey-card-not-started"
        />
        <MetricCard
          label="Opted out"
          value={data.optedOut}
          icon={<MinusCircle className="w-4 h-4" />}
          iconClass="bg-gray-100 text-gray-500"
          active={activeFilter === "opted_out" && !showOverallAnalytics}
          onClick={() => toggleFilter("opted_out")}
          testId="survey-card-opted-out"
        />
      </div>

      {/* Clicking "Overall completion" swaps this whole area for the
          analytics view; "Back to teams list" swaps it back. */}
      {showOverallAnalytics ? (
        <OverallAnalyticsView data={data} onBack={() => setShowOverallAnalytics(false)} />
      ) : (
      <div className="bg-white rounded-xl shadow-sm border overflow-hidden">
        <div className="flex flex-wrap items-center justify-between gap-3 p-5 border-b">
          <div className="flex items-center gap-2 flex-wrap">
            <h3 className="text-base font-semibold text-gray-900">All teams</h3>
            <span className="px-2.5 py-0.5 rounded-full text-xs font-bold bg-indigo-50 text-indigo-700">
              {visibleCount}
            </span>
            {activeFilter !== "all" && (
              <button
                type="button"
                onClick={() => setActiveFilter("all")}
                data-testid="survey-clear-filter"
                className="flex items-center gap-1 pl-2.5 pr-2 py-1 rounded-full text-xs font-semibold bg-indigo-50 text-indigo-700 hover:bg-indigo-100 transition-colors focus:outline-none focus:ring-2 focus:ring-indigo-500"
              >
                {FILTER_LABELS[activeFilter]}
                <X className="w-3 h-3" />
              </button>
            )}
          </div>

          <div className="flex items-center gap-3">
            <div className="relative">
              <Search className="w-4 h-4 text-gray-400 absolute left-3 top-1/2 -translate-y-1/2 pointer-events-none" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Search teams or leaders"
                data-testid="survey-search-input"
                className="w-56 pl-9 pr-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
              />
            </div>
            <button
              type="button"
              onClick={() =>
                showToast(
                  `Reminders sent to ${laggingGroupCount} lagging leader${
                    laggingGroupCount === 1 ? "" : "s"
                  }`,
                )
              }
              disabled={laggingGroupCount === 0}
              data-testid="survey-remind-all"
              className="flex items-center gap-2 px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed whitespace-nowrap"
            >
              <Send className="w-4 h-4" />
              Remind {laggingGroupCount} lagging leader{laggingGroupCount === 1 ? "" : "s"}
            </button>
          </div>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full min-w-[760px]" data-testid="survey-table">
            <thead>
              <tr className="bg-gray-50 border-b border-gray-200">
                <th scope="col" className="px-5 py-2.5 text-left text-xs font-semibold text-gray-500 uppercase tracking-wider">
                  Team name
                </th>
                <th scope="col" className="px-5 py-2.5 text-left text-xs font-semibold text-gray-500 uppercase tracking-wider">
                  Individual survey completed
                </th>
                <th scope="col" className="px-5 py-2.5 text-left text-xs font-semibold text-gray-500 uppercase tracking-wider">
                  Post workshop completed
                </th>
                <th scope="col" className="px-5 py-2.5 text-center text-xs font-semibold text-gray-500 uppercase tracking-wider">
                  Status
                </th>
                <th scope="col" className="px-5 py-2.5">
                  <span className="sr-only">Actions</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {noResults && (
                <tr>
                  <td colSpan={5} className="px-5 py-12 text-center text-sm text-gray-500">
                    No teams match your search.
                  </td>
                </tr>
              )}
              {visibleGroups.map((group) => (
                <Fragment key={group.key}>
                  {group.type === "other" ? (
                    <OtherHeaderRow
                      name={group.name}
                      expanded={isGroupExpanded(group)}
                      onToggle={() => toggleGroup(group.key)}
                    />
                  ) : (
                    <GroupHeaderRow
                      name={group.name}
                      badge={group.type === "manager" ? "Manager" : undefined}
                      totalTeams={group.totalTeams}
                      optedInTeams={group.optedInTeams}
                      completionPercent={group.completionPercent}
                      remindCount={group.remindCount}
                      expanded={isGroupExpanded(group)}
                      onToggle={() => toggleGroup(group.key)}
                      onRemind={() =>
                        group.type === "director" ? openDirectorReminder(group.id) : openManagerReminder(group.id)
                      }
                    />
                  )}

                  {isGroupExpanded(group) &&
                    (group.type === "director" ? (
                      <>
                        {group.visibleDirectTeams.map((team) => (
                          <TeamRow
                            key={team.teamId}
                            team={team}
                            indentClass="pl-12"
                            onRemind={() => openTeamReminder(team, { director: { id: group.id, name: group.name } })}
                          />
                        ))}
                        {group.visibleManagers.map((manager) => (
                          <Fragment key={manager.id}>
                            <GroupHeaderRow
                              name={manager.name}
                              totalTeams={manager.totalTeams}
                              optedInTeams={manager.optedInTeams}
                              completionPercent={manager.completionPercent}
                              remindCount={manager.remindCount}
                              expanded={isManagerExpanded(manager.id)}
                              onToggle={() => toggleManagerRow(manager.id)}
                              onRemind={() => openManagerReminder(manager.id)}
                              indentClass="pl-6"
                            />
                            {isManagerExpanded(manager.id) &&
                              manager.visibleTeams.map((team) => (
                                <TeamRow
                                  key={team.teamId}
                                  team={team}
                                  indentClass="pl-20"
                                  onRemind={() => openTeamReminder(team, { manager: { id: manager.id, name: manager.name } })}
                                />
                              ))}
                          </Fragment>
                        ))}
                      </>
                    ) : (
                      group.visibleTeams.map((team) => (
                        <TeamRow
                          key={team.teamId}
                          team={team}
                          indentClass="pl-12"
                          onRemind={() =>
                            openTeamReminder(
                              team,
                              group.type === "manager" ? { manager: { id: group.id, name: group.name } } : {},
                            )
                          }
                        />
                      ))
                    ))}
                </Fragment>
              ))}
            </tbody>
          </table>
        </div>

        <p className="px-5 py-4 text-xs text-gray-500 italic border-t">
          Reminders go to each team&apos;s director or manager about teams that aren&apos;t fully complete;
          &quot;Other&quot; teams remind their team lead directly. Opted-out teams are never included.
        </p>
      </div>
      )}

      {toast && (
        <div
          role="status"
          data-testid="survey-toast"
          className="fixed bottom-6 right-6 bg-gray-900 text-white text-sm font-medium px-5 py-3 rounded-lg shadow-lg z-50"
        >
          {toast}
        </div>
      )}

      {reminderPlan && (
        <ReminderTreeModal
          plan={reminderPlan}
          onCancel={() => setReminderPlan(null)}
          onSend={handleSendReminder}
        />
      )}
    </div>
  );
}
