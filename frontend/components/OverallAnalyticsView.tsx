"use client";

import { ArrowLeft } from "lucide-react";
import CompletionBreakdownChart from "./CompletionBreakdownChart";
import TeamCompletionBarChart from "./TeamCompletionBarChart";
import CompletionTrendChart from "./CompletionTrendChart";
import type { SurveyCompletionData } from "./SurveyCompletionDashboard";

interface OverallAnalyticsViewProps {
  data: SurveyCompletionData;
  onBack: () => void;
}

export default function OverallAnalyticsView({ data, onBack }: OverallAnalyticsViewProps) {
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
          progress, and the trend so far this half.
        </p>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <div className="bg-white rounded-xl border border-gray-200 shadow-sm p-5" data-testid="chart-card-breakdown">
          <h4 className="text-sm font-semibold text-gray-900 mb-1">Status breakdown</h4>
          <p className="text-xs text-gray-500 mb-2">Share of teams in each completion state</p>
          <CompletionBreakdownChart
            fullyComplete={data.fullyComplete}
            inProgress={data.inProgress}
            notStarted={data.notStarted}
            optedOut={data.optedOut}
          />
        </div>

        <div className="bg-white rounded-xl border border-gray-200 shadow-sm p-5" data-testid="chart-card-team-bars">
          <h4 className="text-sm font-semibold text-gray-900 mb-1">Completion by team</h4>
          <p className="text-xs text-gray-500 mb-2">Individual survey completion, opted-in teams</p>
          <TeamCompletionBarChart teamStats={data.teamStats} />
        </div>

        <div
          className="bg-white rounded-xl border border-gray-200 shadow-sm p-5 lg:col-span-2"
          data-testid="chart-card-trend"
        >
          <h4 className="text-sm font-semibold text-gray-900 mb-1">Completion trend this half</h4>
          <p className="text-xs text-gray-500 mb-2">Overall completion percentage by week</p>
          <CompletionTrendChart timeSeries={data.timeSeries} />
        </div>
      </div>
    </div>
  );
}
