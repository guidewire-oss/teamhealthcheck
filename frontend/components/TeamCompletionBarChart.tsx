"use client";

import { buildInProgressBars } from "@/lib/team-completion-filter";
import { STATUS_BADGE_CLASS } from "@/lib/status-colors";
import type { SurveyCompletionData } from "./SurveyCompletionDashboard";

interface TeamCompletionBarChartProps {
  teamStats: SurveyCompletionData["teamStats"];
}

// The exact wording requested for this specific badge -- "Completed" (not
// lib/status-colors.ts's "Complete") and "In Progress" (capital P, matching
// the filter tab's own label). Colors still come from the shared
// STATUS_BADGE_CLASS palette, so this is a wording-only override scoped to
// this one badge; every other status label elsewhere in the app is
// unaffected.
const DISPLAY_STATUS_LABEL: Record<"complete" | "in_progress", string> = {
  complete: "Completed",
  in_progress: "In Progress",
};

/**
 * Plain HTML rows (not a Recharts chart) so the percent text and status
 * badge are both always visible and reliably testable -- a Recharts
 * LabelList/Tooltip can't guarantee that (SVG label content isn't a
 * reliable jsdom assertion target, and a Tooltip only shows on hover).
 *
 * This list only ever shows genuinely in-progress teams/pods -- Completed
 * teams (including a raw in_progress team whose percentage rounds up to
 * 100%), Not Started, and Opted Out are all excluded by buildInProgressBars
 * -- so every badge below always reads "In Progress" and is never green.
 * Search and ascending-percent sorting still run first, on whatever the
 * caller already narrowed down -- see lib/team-completion-filter.ts.
 */
export default function TeamCompletionBarChart({ teamStats }: TeamCompletionBarChartProps) {
  const bars = buildInProgressBars(teamStats);

  return (
    <ul className="space-y-2.5" data-testid="team-completion-list">
      {bars.map((bar) => (
        <li
          key={bar.teamId}
          className="flex items-center gap-3"
          data-testid="team-completion-row"
          title={`${bar.completed} of ${bar.total} completed`}
        >
          <span className="w-20 flex-shrink-0 text-xs text-gray-700 truncate">{bar.teamName}</span>
          <div className="flex-1 h-2.5 bg-gray-100 rounded-full overflow-hidden">
            <div
              className="h-full rounded-full transition-all"
              style={{ width: `${Math.min(100, bar.percent)}%`, backgroundColor: bar.color }}
              data-testid="team-completion-progress-bar"
            />
          </div>
          <span
            className="text-xs font-semibold text-gray-700 flex-shrink-0 whitespace-nowrap"
            data-testid="team-completion-percent"
          >
            {bar.percent}% completed
          </span>
          <span
            className={`px-2 py-0.5 rounded-full text-xs font-semibold flex-shrink-0 whitespace-nowrap ${STATUS_BADGE_CLASS[bar.status]}`}
            data-testid="team-completion-status-badge"
          >
            {DISPLAY_STATUS_LABEL[bar.status]}
          </span>
        </li>
      ))}
    </ul>
  );
}
