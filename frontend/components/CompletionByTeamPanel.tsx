"use client";

import { useState } from "react";
import { Search, X } from "lucide-react";
import TeamCompletionBarChart from "./TeamCompletionBarChart";
import { buildInProgressBars, filterTeamStats, filterTeamsByStatus } from "@/lib/team-completion-filter";
import type { SurveyCompletionData } from "./SurveyCompletionDashboard";

interface CompletionByTeamPanelProps {
  teamStats: SurveyCompletionData["teamStats"];
}

/**
 * The "Completion by team" content: search box and the scrollable bar
 * chart/empty-state region. Rendered identically by both the dashboard
 * card (OverallAnalyticsView) and the fullscreen overlay
 * (CompletionByTeamFullscreenModal) -- fills whatever height its parent
 * gives it via flex, so neither caller needs its own copy of this
 * search/sort wiring.
 *
 * This list only ever shows genuinely in-progress teams/pods -- Completed
 * (including the 100%-by-rounding edge case), Not Started, and Opted Out
 * are always excluded, with no toggle to show them; there used to be
 * "All"/"Fully Completed" tabs here, but showing every status
 * contradicted that requirement, so this view is in-progress-only now.
 *
 * Pipeline order (status filter, then search, then sort) matches the
 * requirement exactly: filterTeamsByStatus("in_progress") runs first,
 * filterTeamStats (search) runs on that result, and
 * TeamCompletionBarChart's own buildInProgressBars sorts ascending by
 * percentage last (and drops the 100%-by-rounding edge case) over
 * whatever survived both filters.
 */
export default function CompletionByTeamPanel({ teamStats }: CompletionByTeamPanelProps) {
  const [searchQuery, setSearchQuery] = useState("");

  const inProgressTeamStats = filterTeamsByStatus(teamStats, "in_progress");
  const visibleTeamStats = filterTeamStats(inProgressTeamStats, searchQuery);
  // Counts the SAME final bars TeamCompletionBarChart will render (not
  // just the raw in_progress teams) so the empty state still shows even
  // when every surviving team rounds up to 100% and gets excluded there.
  const visibleBarsCount = buildInProgressBars(visibleTeamStats).length;
  const searchActive = searchQuery.trim().length > 0;
  const noTeamsMatch = searchActive && visibleBarsCount === 0;
  const noTeamsForFilter = !searchActive && visibleBarsCount === 0;

  return (
    <div className="flex flex-col h-full">
      <div className="flex-shrink-0">
        <div className="relative mb-2">
          <Search className="w-4 h-4 text-gray-400 absolute left-3 top-1/2 -translate-y-1/2 pointer-events-none" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Search teams"
            data-testid="team-bars-search-input"
            className="w-full pl-9 pr-9 py-1.5 border border-gray-300 rounded-lg text-sm focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
          />
          {searchQuery && (
            <button
              type="button"
              onClick={() => setSearchQuery("")}
              data-testid="team-bars-search-clear"
              aria-label="Clear search"
              className="absolute right-2.5 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600"
            >
              <X className="w-4 h-4" />
            </button>
          )}
        </div>
      </div>

      {/* min-h-0 is required for overflow-y-auto to actually engage inside
          a flex column -- otherwise this area grows to fit its content
          instead of scrolling. */}
      <div className="flex-1 min-h-0 overflow-y-auto" data-testid="team-bars-scroll-area">
        {noTeamsMatch ? (
          <div
            className="h-full flex items-center justify-center text-sm text-gray-500 text-center px-4"
            data-testid="team-bars-empty-state"
          >
            No teams match your search.
          </div>
        ) : noTeamsForFilter ? (
          <div
            className="h-full flex items-center justify-center text-sm text-gray-500 text-center px-4"
            data-testid="team-bars-empty-state"
          >
            No teams currently in progress.
          </div>
        ) : (
          <TeamCompletionBarChart teamStats={visibleTeamStats} />
        )}
      </div>
    </div>
  );
}
