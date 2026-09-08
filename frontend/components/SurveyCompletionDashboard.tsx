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
  type SurveyCompletionOverview,
  type SurveyCompletionPersonGroup,
  type SurveyCompletionTeam,
  type SurveyStatus,
} from "@/lib/api/survey-completion";
import {
  buildFullyCompletedGroups,
  buildHierarchyAggregateIndex,
  buildVisibleGroups,
  buildVisibleHierarchyAggregateIndex,
  countVisibleTeams,
  getPartialCompletionBadgeClass,
  getPodCompletionPercent,
  getPodDisplayStatus,
  isPaleRedPodRow,
  isParentStatusHiddenFor,
  isRemindable,
  isRowBackgroundHiddenFor,
  type CardFilter,
  type VisibleGroup,
} from "@/lib/survey-completion-tree";
import {
  buildOrgReminderPlan,
  buildPersonReminderPlan,
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
 * The table renders a fully recursive leadership hierarchy (see
 * lib/survey-completion-tree.ts for the filtering logic): a leader nests
 * under whichever leader their own reports_to chain resolves to, to
 * whatever depth the organization's own data actually goes — there is no
 * fixed two-tier assumption. A leader with no resolvable parent stands
 * alone as a top-level row; a team with no resolvable owner falls into the
 * catch-all "Other" group.
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
    teamId: string;
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

// Fixed Tailwind class palettes for indenting nested rows — Tailwind's JIT
// scanner needs literal class names, not dynamically interpolated ones, so
// depth is capped rather than computing "pl-" + n. Capping visually is fine
// too: a chain nested this deep is already an edge case.
const HEADER_INDENT_CLASSES = ["", "pl-6", "pl-12", "pl-16", "pl-20", "pl-24"];
const TEAM_INDENT_CLASSES = ["pl-12", "pl-16", "pl-20", "pl-24", "pl-28", "pl-32"];

function indentClassFor(classes: string[], depth: number): string {
  return classes[Math.min(depth, classes.length - 1)];
}

function getInitials(name: string): string {
  return name
    .split(" ")
    .map((part) => part[0])
    .join("")
    .toUpperCase();
}

function teamsInGroup(group: SurveyCompletionGroup): SurveyCompletionTeam[] {
  // Defensive against a malformed/older API response — the backend always
  // sends [] rather than omitting these, but don't let a shape mismatch
  // white-screen the whole dashboard.
  if (group.type === "person") {
    return [...(group.directTeams ?? []), ...(group.children ?? []).flatMap(teamsInGroup)];
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
        teamId: team.teamId,
        teamName: team.teamName,
        completed: team.completed,
        total: team.total,
        status: team.status,
      })),
    ),
    timeSeries: overview.timeSeries,
  };
}

/** Recursively finds a leader's raw (unfiltered) group anywhere in the tree. */
function findPersonGroup(groups: SurveyCompletionGroup[], id: string): SurveyCompletionPersonGroup | undefined {
  for (const g of groups) {
    if (g.type !== "person") continue;
    if (g.person.id === id) return g;
    const found = findPersonGroup(g.children, id);
    if (found) return found;
  }
  return undefined;
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

// StatusBadge is used only for pod rows (TeamRow) in this file — the exact
// wording requested for pods ("Completed", "In Progress") differs slightly
// from lib/status-colors.ts's shared STATUS_LABEL ("Complete", "In
// progress"), which is still used verbatim everywhere else (reminder tree,
// other dashboards). This override is scoped to this one component so
// nothing outside the Pods section is affected; colors still come from the
// shared STATUS_BADGE_CLASS palette.
const POD_STATUS_LABEL: Record<SurveyStatus, string> = {
  ...STATUS_LABEL,
  complete: "Completed",
  in_progress: "In Progress",
};

function StatusBadge({ status }: { status: SurveyStatus }) {
  return (
    <span
      className={`inline-flex items-center justify-center w-full max-w-[120px] px-2.5 py-1 rounded-full text-xs font-semibold ${STATUS_BADGE_CLASS[status]}`}
    >
      {POD_STATUS_LABEL[status]}
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

// Exported for isolated testing of the pod row's status/percentage
// rendering — see components/__tests__/SurveyCompletionDashboard.test.tsx.
// Not used outside this module in application code.
export function TeamRow({
  team,
  indentClass,
  onRemind,
  showPercentBadge,
}: {
  team: SurveyCompletionTeam;
  indentClass: string;
  onRemind: () => void;
  // True only while the "Total Teams" / "All Teams" or "In Progress"
  // filter is active (see isRowBackgroundHiddenFor) — scoped purely to
  // this pod row: no row background at all (status-driven or hover), and
  // partial completion renders as a color-tiered badge instead of plain
  // red text. Every other filter keeps the existing row background/hover
  // and plain-text behavior unchanged.
  showPercentBadge?: boolean;
}) {
  // Pods only ever display an EFFECTIVE status/percentage here, never the
  // raw backend status directly — see getPodDisplayStatus for why (it only
  // ever differs from team.status for the 100%-by-rounding edge case on an
  // in_progress pod). This is scoped to pod rows alone: Director/Manager
  // header rows (GroupHeaderRow) are untouched and keep using their own
  // backend-computed completionPercent/remindCount as before.
  //
  // Both the percent text and the status badge live together in the
  // Status column only (below) — the team-name cell never carries any
  // status/percent text, and the "Individual survey completed" column
  // shows the raw submitted-count, not a derived percentage.
  const displayStatus = getPodDisplayStatus(team);
  const percent = getPodCompletionPercent(team);
  // Not Started and partial/In Progress pods get a consistent pale red row
  // background — driven by the exact same displayStatus the Status column
  // itself renders from, never a second, independently-guessed condition.
  // Under Total Teams/In Progress (showPercentBadge), the row itself never
  // gets any background — color lives only in the Status-column badge.
  const rowBackgroundClass = !showPercentBadge && isPaleRedPodRow(displayStatus) ? "bg-red-50" : "";
  const rowHoverClass = showPercentBadge ? "" : "hover:bg-indigo-50/40";

  return (
    <tr
      className={`border-b border-gray-100 last:border-b-0 transition-colors ${rowHoverClass} ${rowBackgroundClass}`}
    >
      <td className={`px-5 py-3 text-sm text-gray-900 ${indentClass}`}>{team.teamName}</td>
      <td
        className={`px-5 py-3 text-sm font-semibold ${
          displayStatus === "complete" ? "text-green-700" : "text-gray-900"
        }`}
        data-testid="survey-team-individual-completed"
      >
        {/* Individual Survey Completed: completed_count / eligible_count,
            straight from the API's individual-survey fields, for EVERY
            pod row regardless of canonical status or which filter is
            currently active -- an Opted Out pod's eligible members still
            have a real (usually 0) completed count, and 0 is never
            rendered as blank/"—". Only the Post-workshop column (below)
            has a genuine null case, because that field is itself nullable
            for an opted-out team; this field is a plain number pair and
            is never null. */}
        {team.completed} / {team.total}
      </td>
      <td className="px-5 py-3 text-sm text-gray-900">
        {team.postWorkshopCompleted === null ? "—" : team.postWorkshopCompleted ? "Yes" : "No"}
      </td>
      <td className="px-5 py-3">
        <div className="flex flex-col items-center gap-1">
          {displayStatus === "in_progress" ? (
            showPercentBadge ? (
              // Total Teams / All Teams and In Progress: partial completion
              // renders as a pill badge — same structural shape as the
              // green "Completed" badge (StatusBadge) — colored red below
              // 50% and yellow at 50% and up (getPartialCompletionBadgeClass)
              // — instead of the plain red text used everywhere else.
              <span
                className={`inline-flex items-center justify-center w-full max-w-[120px] px-2.5 py-1 rounded-full text-xs font-semibold ${getPartialCompletionBadgeClass(percent)}`}
                data-testid="survey-team-percent"
              >
                {percent}% completed
              </span>
            ) : (
              // Every other filter: partial completion shows only the red
              // percentage — no separate "In Progress" badge alongside it.
              <span className="text-xs font-bold text-red-700" data-testid="survey-team-percent">
                {percent}% completed
              </span>
            )
          ) : (
            <StatusBadge status={displayStatus} />
          )}
        </div>
      </td>
      <td className="px-5 py-3 text-right">
        <button
          type="button"
          onClick={onRemind}
          className={`px-3 py-1.5 border border-gray-300 rounded-lg text-xs font-semibold text-gray-700 hover:border-indigo-400 hover:text-indigo-600 transition-colors focus:outline-none focus:ring-2 focus:ring-indigo-500 whitespace-nowrap ${
            isRemindable(team) ? "" : "invisible"
          }`}
          data-testid="survey-remind-team"
        >
          Remind
        </button>
      </td>
    </tr>
  );
}

// Exported for isolated testing of the Fully-Completed leader badge — see
// components/__tests__/SurveyCompletionDashboard.test.tsx. Not used outside
// this module in application code.
export function GroupHeaderRow({
  name,
  totalTeams,
  optedInTeams,
  completionPercent,
  remindCount,
  expanded,
  onToggle,
  onRemind,
  indentClass,
  showFullyCompletedBadge,
  hideStatusColumn,
  hierarchyAggregate,
}: {
  name: string;
  totalTeams: number;
  optedInTeams: number;
  completionPercent: number;
  remindCount: number;
  expanded: boolean;
  onToggle: () => void;
  onRemind: () => void;
  indentClass?: string;
  // True only when rendering under the "Fully Completed" filter. Every
  // leader shown there is, by construction (see
  // isPersonSubtreeFullyCompleted / buildFullyCompletedGroups), guaranteed
  // to have every descendant pod at 100% — so this always swaps in the
  // same green "Completed" tag pods use, never a percentage or "In
  // Progress", and never needs a per-render re-check of its own.
  //
  // Not Started and Opted Out never swap this badge either — see
  // hideStatusColumn below, which hides the whole Status column for those
  // filters instead. A leader row never shows "Not started"/"Opted out" of
  // its own, no matter how uniform its subtree is; those pod statuses
  // appear only on the actual pod rows' Status column (see TeamRow).
  showFullyCompletedBadge?: boolean;
  // True only under the "Not Started" or "Opted Out" filters (see
  // isParentStatusHiddenFor) — every leader row's Status column renders
  // completely empty: no percent badge, no derived status, nothing beside
  // its name. A leader shown there is pure hierarchy context for its
  // matching pods; only the actual pod rows ever show "Not started" /
  // "Opted out".
  hideStatusColumn?: boolean;
  // Set under any filter that shows a plain percent badge at all: "Total
  // Teams" / "All Teams" and "Opted In" (see computeHierarchyAggregate,
  // aggregated from EVERY leaf pod in the leader's whole reporting subtree)
  // or "In Progress" (see computeVisibleGroupAggregate, aggregated from only
  // the teams currently listed under this leader in the filtered view).
  // When present, this always wins over the plain completionPercent-based
  // badge below (but never over showFullyCompletedBadge/hideStatusColumn,
  // which only ever apply under a different filter and so never coexist
  // with this one in practice).
  hierarchyAggregate?: { percent: number; displayStatus: SurveyStatus };
}) {
  return (
    <tr className="bg-gray-50 border-b border-gray-200" data-testid="survey-group-header">
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
            <span className="text-sm text-gray-500 ml-2">
              {totalTeams} teams &middot; {optedInTeams} opted in
            </span>
          </div>
        </button>
      </td>
      <td className="px-5 py-3 text-center" data-testid="survey-group-status-cell">
        {hideStatusColumn ? null : showFullyCompletedBadge ? (
          <StatusBadge status="complete" />
        ) : hierarchyAggregate ? (
          hierarchyAggregate.displayStatus === "in_progress" ? (
            // Same compact badge shape/coloring a pod's own partial
            // completion uses under Total Teams / In Progress
            // (getPartialCompletionBadgeClass) — red below 50%, yellow at
            // 50% and up — never plain text, so a Level-2 row's badge
            // always matches the Status column's existing pod styling.
            <span
              className={`inline-flex items-center justify-center w-full max-w-[120px] px-2.5 py-1 rounded-full text-xs font-semibold ${getPartialCompletionBadgeClass(hierarchyAggregate.percent)}`}
            >
              {hierarchyAggregate.percent}% completed
            </span>
          ) : (
            <StatusBadge status={hierarchyAggregate.displayStatus} />
          )
        ) : (
          <span
            className={`inline-flex items-center justify-center w-full max-w-[120px] px-2.5 py-1 rounded-full text-xs font-bold ${
              completionPercent >= 50 ? "bg-amber-100 text-amber-800" : "bg-red-100 text-red-800"
            }`}
          >
            {completionPercent}% complete
          </span>
        )}
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

type VisiblePersonGroup = Extract<VisibleGroup, { type: "person" }>;

/**
 * Renders one leader's header row, its direct pods, and — recursively —
 * every child leader's own rows underneath. Collapsing this leader's row
 * hides this whole subtree, since the recursive calls simply don't happen
 * when `expanded` is false.
 */
function PersonGroupRows({
  group,
  depth,
  isExpanded,
  onToggle,
  onRemindLeader,
  onRemindTeam,
  showFullyCompletedBadge,
  hideStatusColumn,
  hierarchyAggregateIndex,
  showPercentBadge,
}: {
  group: VisiblePersonGroup;
  depth: number;
  isExpanded: (key: string) => boolean;
  onToggle: (key: string) => void;
  onRemindLeader: (id: string) => void;
  onRemindTeam: (team: SurveyCompletionTeam, ownerId: string) => void;
  // True only while the "Fully Completed" filter is active. Every group
  // reaching this component was built by buildFullyCompletedGroups, so
  // every leader (and every pod under them) is guaranteed fully completed —
  // this just controls the badge rendered, not which rows appear.
  showFullyCompletedBadge?: boolean;
  // True only while the "Not Started" or "Opted Out" filter is active (see
  // isParentStatusHiddenFor) — every leader row's Status column renders
  // completely empty, regardless of depth.
  hideStatusColumn?: boolean;
  // Set while "Total Teams" / "All Teams" or "Opted In" is active — every
  // person id's whole-subtree completion aggregate (see
  // buildHierarchyAggregateIndex), computed once from the raw tree — OR
  // while "In Progress" is active — every person id's aggregate over only
  // the teams currently listed under them in this filtered view (see
  // buildVisibleHierarchyAggregateIndex). Looked up per node below either
  // way.
  hierarchyAggregateIndex?: Map<string, { percent: number; displayStatus: SurveyStatus } | null>;
  // True only while the "Total Teams" / "All Teams" or "In Progress" filter
  // is active (see isRowBackgroundHiddenFor) — passed straight through to
  // every pod row (TeamRow) at any depth so its own row background/hover
  // is removed and its partial-completion display switches to the
  // color-tiered badge style.
  showPercentBadge?: boolean;
}) {
  const expanded = isExpanded(group.key);
  const hierarchyAggregate = hierarchyAggregateIndex?.get(group.id) ?? undefined;

  return (
    <Fragment>
      <GroupHeaderRow
        name={group.name}
        totalTeams={group.totalTeams}
        optedInTeams={group.optedInTeams}
        completionPercent={group.completionPercent}
        remindCount={group.remindCount}
        expanded={expanded}
        onToggle={() => onToggle(group.key)}
        onRemind={() => onRemindLeader(group.id)}
        indentClass={indentClassFor(HEADER_INDENT_CLASSES, depth)}
        showFullyCompletedBadge={showFullyCompletedBadge}
        hideStatusColumn={hideStatusColumn}
        hierarchyAggregate={hierarchyAggregate}
      />
      {expanded &&
        group.visibleDirectTeams.map((team) => (
          <TeamRow
            key={team.teamId}
            team={team}
            indentClass={indentClassFor(TEAM_INDENT_CLASSES, depth)}
            onRemind={() => onRemindTeam(team, group.id)}
            showPercentBadge={showPercentBadge}
          />
        ))}
      {expanded &&
        group.visibleChildren.map((child) => (
          <PersonGroupRows
            key={child.key}
            group={child as VisiblePersonGroup}
            depth={depth + 1}
            isExpanded={isExpanded}
            onToggle={onToggle}
            onRemindLeader={onRemindLeader}
            onRemindTeam={onRemindTeam}
            showFullyCompletedBadge={showFullyCompletedBadge}
            hideStatusColumn={hideStatusColumn}
            hierarchyAggregateIndex={hierarchyAggregateIndex}
            showPercentBadge={showPercentBadge}
          />
        ))}
    </Fragment>
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
    // Clear the previous period's data immediately, before the new fetch
    // resolves. Without this, overview stays populated from the last
    // successful load, so the `isLoading && !overview` / `error && !overview`
    // guards below never fire again on a period switch — a slow or failed
    // fetch for the newly selected period would otherwise leave the OLD
    // period's data on screen, silently mislabeled under the new period
    // (and any fetch error would be swallowed entirely, with no visible
    // indication anything went wrong).
    setOverview(null);

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

  // Reminder plans are always built from the raw, unfiltered tree — not the
  // search/filter-narrowed VisibleGroup — so the plan and its counts always
  // match the real dataset regardless of what's currently on screen.
  const openTeamReminder = (team: SurveyCompletionTeam, ownerId?: string) => {
    const owner = ownerId ? findPersonGroup(overview?.groups ?? [], ownerId)?.person : undefined;
    setReminderPlan(buildTeamReminderPlan(team, { owner }));
  };

  const openPersonReminder = (id: string) => {
    const group = findPersonGroup(overview?.groups ?? [], id);
    if (!group) return;
    setReminderPlan(buildPersonReminderPlan(group));
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
  // The org-wide bulk plan, built once per render from the raw, unfiltered
  // tree (same "always the real dataset" rule the single team/leader plans
  // already follow) — includes the "Other" group's own pending pods, which
  // the previous laggingGroupCount-only-counts-owned-leaders logic silently
  // dropped from both the button's count and its (nonexistent) preview.
  const orgReminderPlan = buildOrgReminderPlan(overview.groups);

  const searchActive = searchQuery.trim().length > 0;
  // The "Fully Completed" filter is hierarchy-aware: a leader qualifies
  // only when every descendant pod in their whole reporting subtree is at
  // 100% (see isPersonSubtreeFullyCompleted), not merely "has a matching
  // pod somewhere below them" like every other filter. It therefore needs
  // its own builder, sharing the same underlying pod/subtree helpers so the
  // rows shown here can never disagree with what a pod's own Status column
  // displays.
  const isFullyCompletedFilter = activeFilter === "complete";
  const visibleGroups = isFullyCompletedFilter
    ? buildFullyCompletedGroups(overview.groups, searchQuery)
    : buildVisibleGroups(overview.groups, searchQuery, activeFilter);
  const visibleCount = countVisibleTeams(visibleGroups);
  const noResults = searchActive && visibleCount === 0;

  // Under the Not Started or Opted Out filter, a leader row is pure
  // hierarchy context for its matching pods -- its Status column renders
  // nothing at all (see isParentStatusHiddenFor, the single shared rule
  // for this).
  const hideParentStatusColumn = isParentStatusHiddenFor(activeFilter);

  // Every leader's percentage/status is a hierarchy-wide aggregate,
  // recalculated from EVERY relevant leaf pod in their whole reporting
  // subtree (see computeHierarchyAggregate) rather than the backend's own
  // per-node completionPercent (a different metric -- "what fraction of
  // this subtree's PODS are fully complete" -- not a Status %). This applies
  // under every filter that shows a plain percent badge at all: Total Teams
  // / All Teams and Opted In both read from the FULL unfiltered subtree
  // (buildHierarchyAggregateIndex) -- opted_out pods are already excluded by
  // computeHierarchyAggregate itself, so both filters agree on the same
  // number for the same leader.
  //
  // In Progress instead reads from only the teams CURRENTLY LISTED under
  // each leader in this filtered view (buildVisibleHierarchyAggregateIndex,
  // over visibleGroups) -- e.g. a manager showing Danville (5/9 = 56%) and
  // Sausalito (5/9 = 56%) here reads 56%, not a number diluted by that
  // manager's other, hidden (complete/not_started/opted_out) pods.
  //
  // Fully Completed (its own green badge, never a percent) and Not
  // Started/Opted Out (status column hidden entirely) never need this index.
  const isInProgressFilter = activeFilter === "in_progress";
  const usesRawHierarchyAggregate = !hideParentStatusColumn && !isFullyCompletedFilter && !isInProgressFilter;
  const hierarchyAggregateIndex = isInProgressFilter
    ? buildVisibleHierarchyAggregateIndex(visibleGroups)
    : usesRawHierarchyAggregate
      ? buildHierarchyAggregateIndex(overview.groups)
      : undefined;

  // Under Total Teams / All Teams AND In Progress, every pod row's Status
  // column becomes the only place color appears -- no row background at
  // all (status-driven or hover) -- and partial completion renders as a
  // color-tiered badge instead of plain red text (see
  // isRowBackgroundHiddenFor, the single shared rule for this).
  const showPercentBadge = isRowBackgroundHiddenFor(activeFilter);

  const isGroupExpanded = (key: string) => searchActive || expandedGroups.has(key);

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
        <OverallAnalyticsView
          data={data}
          assessmentPeriod={overview.assessmentPeriod}
          onBack={() => setShowOverallAnalytics(false)}
        />
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

          <div className="flex flex-wrap items-center gap-3">
            <div className="relative">
              <Search className="w-4 h-4 text-gray-400 absolute left-3 top-1/2 -translate-y-1/2 pointer-events-none" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Search teams or leaders"
                data-testid="survey-search-input"
                className="w-full sm:w-56 pl-9 pr-3 py-2 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
              />
            </div>
            <button
              type="button"
              onClick={() => setReminderPlan(orgReminderPlan)}
              disabled={orgReminderPlan.podCount === 0}
              data-testid="survey-remind-all"
              className="flex items-center gap-2 px-4 py-2 bg-indigo-600 text-white rounded-lg hover:bg-indigo-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed whitespace-nowrap"
            >
              <Send className="w-4 h-4" />
              Remind {orgReminderPlan.podCount} pending pod{orgReminderPlan.podCount === 1 ? "" : "s"}
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
              {visibleGroups.map((group) =>
                group.type === "other" ? (
                  <Fragment key={group.key}>
                    <OtherHeaderRow
                      name={group.name}
                      expanded={isGroupExpanded(group.key)}
                      onToggle={() => toggleGroup(group.key)}
                    />
                    {isGroupExpanded(group.key) &&
                      group.visibleTeams.map((team) => (
                        <TeamRow
                          key={team.teamId}
                          team={team}
                          indentClass="pl-12"
                          onRemind={() => openTeamReminder(team)}
                          showPercentBadge={showPercentBadge}
                        />
                      ))}
                  </Fragment>
                ) : (
                  <PersonGroupRows
                    key={group.key}
                    group={group}
                    depth={0}
                    isExpanded={isGroupExpanded}
                    onToggle={toggleGroup}
                    onRemindLeader={openPersonReminder}
                    onRemindTeam={openTeamReminder}
                    showFullyCompletedBadge={isFullyCompletedFilter}
                    hideStatusColumn={hideParentStatusColumn}
                    hierarchyAggregateIndex={hierarchyAggregateIndex}
                    showPercentBadge={showPercentBadge}
                  />
                ),
              )}
            </tbody>
          </table>
        </div>

        <p className="px-5 py-4 text-xs text-gray-500 italic border-t">
          Reminders go to each team&apos;s leadership owner about teams that aren&apos;t fully complete;
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
