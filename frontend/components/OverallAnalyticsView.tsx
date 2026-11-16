"use client";

import { useState } from "react";
import { ArrowLeft, Maximize2 } from "lucide-react";
import CompletionBreakdownChart from "./CompletionBreakdownChart";
import CompletionByTeamPanel from "./CompletionByTeamPanel";
import CompletionByTeamFullscreenModal from "./CompletionByTeamFullscreenModal";
import CompletionTrendChart from "./CompletionTrendChart";
import type { SurveyCompletionData } from "./SurveyCompletionDashboard";

interface OverallAnalyticsViewProps {
  data: SurveyCompletionData;
  assessmentPeriod: string;
  onBack: () => void;
}

export default function OverallAnalyticsView({ data, assessmentPeriod, onBack }: OverallAnalyticsViewProps) {
  const [isTeamBarsFullscreen, setIsTeamBarsFullscreen] = useState(false);

  return (
    <div data-testid="overall-analytics-view">
      <button
        type="button"
        onClick={onBack}
        data-testid="survey-back-to-teams"
        className="flex items-center gap-1.5 text-sm font-semibold text-indigo-600 hover:text-indigo-700 mb-4 focus:outline-none focus:ring-2 focus:ring-indigo-500 rounded"
      >
        <ArrowLeft className="w-4 h-4" />
        Back to teams list
      </button>

      <div className="mb-5">
        <h3 className="text-lg font-semibold text-gray-900">Overall completion analytics</h3>
        <p className="text-sm text-gray-500 mt-1">
          A closer look at the {data.overallCompletion}% overall completion figure — status mix, per-team
          progress, and the trend so far in {assessmentPeriod}.
        </p>
      </div>

      {/*
        Both chart cards below share the identical fixed height class
        (h-[22rem]). A grid row's default "stretch" behavior only equalizes
        heights AFTER each item's own intrinsic size is computed — it can't
        cap growth, so with many teams the "Completion by team" list's own
        intrinsic height would otherwise grow the whole row. An explicit
        shared height, with the team card's list living in its own
        overflow-y-auto region, is what actually keeps the two cards the
        same size regardless of team count.
      */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <div
          className="bg-white rounded-xl border border-gray-200 shadow-sm p-5 flex flex-col h-[22rem]"
          data-testid="chart-card-breakdown"
        >
          <div className="flex-shrink-0">
            <h4 className="text-sm font-semibold text-gray-900 mb-1">Status breakdown</h4>
            <p className="text-xs text-gray-500 mb-2">Share of teams in each completion state</p>
          </div>
          <div className="flex-1 min-h-0 overflow-y-auto">
            <CompletionBreakdownChart
              fullyComplete={data.fullyComplete}
              inProgress={data.inProgress}
              notStarted={data.notStarted}
              optedOut={data.optedOut}
            />
          </div>
        </div>

        <div
          className="bg-white rounded-xl border border-gray-200 shadow-sm p-5 flex flex-col h-[22rem]"
          data-testid="chart-card-team-bars"
        >
          {/* Header stays fixed — only the panel's own list below scrolls. */}
          <div className="flex-shrink-0 flex items-start justify-between gap-2 mb-1">
            <div>
              <h4 className="text-sm font-semibold text-gray-900">Completion by team</h4>
              <p className="text-xs text-gray-500">Filter by status, search, or expand for the full view</p>
            </div>
            <button
              type="button"
              onClick={() => setIsTeamBarsFullscreen(true)}
              data-testid="team-bars-expand-button"
              aria-label="Expand Completion by team"
              className="text-gray-400 hover:text-gray-600 focus:outline-none focus:ring-2 focus:ring-indigo-500 rounded flex-shrink-0"
            >
              <Maximize2 className="w-4 h-4" />
            </button>
          </div>

          <div className="flex-1 min-h-0">
            <CompletionByTeamPanel teamStats={data.teamStats} />
          </div>
        </div>

        <div
          className="bg-white rounded-xl border border-gray-200 shadow-sm p-5 lg:col-span-2"
          data-testid="chart-card-trend"
        >
          <h4 className="text-sm font-semibold text-gray-900 mb-1">Completion trend — {assessmentPeriod}</h4>
          <p className="text-xs text-gray-500 mb-2">Overall completion percentage by week</p>
          <CompletionTrendChart timeSeries={data.timeSeries} />
        </div>
      </div>

      {isTeamBarsFullscreen && (
        <CompletionByTeamFullscreenModal
          teamStats={data.teamStats}
          onClose={() => setIsTeamBarsFullscreen(false)}
        />
      )}
    </div>
  );
}
