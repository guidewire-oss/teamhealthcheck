"use client";

import { PieChart, Pie, Cell, Tooltip, Legend, ResponsiveContainer } from "recharts";
import type { SurveyStatus } from "./SurveyCompletionDashboard";

interface CompletionBreakdownChartProps {
  fullyComplete: number;
  inProgress: number;
  notStarted: number;
  optedOut: number;
}

// Same palette used for the status pills/badges in the "All teams" table.
const STATUS_CHART_COLOR: Record<SurveyStatus, string> = {
  complete: "#10B981",
  in_progress: "#F59E0B",
  not_started: "#EF4444",
  opted_out: "#9CA3AF",
};

const STATUS_CHART_LABEL: Record<SurveyStatus, string> = {
  complete: "Fully complete",
  in_progress: "In progress",
  not_started: "Not started",
  opted_out: "Opted out",
};

interface BreakdownSlice {
  status: SurveyStatus;
  label: string;
  value: number;
  total: number;
  [key: string]: unknown; // satisfies Recharts' generic chart-data-input shape
}

function BreakdownTooltip({
  active,
  payload,
}: {
  active?: boolean;
  payload?: Array<{ payload: BreakdownSlice }>;
}) {
  if (!active || !payload || !payload.length) return null;
  const slice = payload[0].payload;
  const percent = slice.total ? Math.round((slice.value / slice.total) * 100) : 0;

  return (
    <div className="bg-gray-900 text-white text-xs rounded-lg px-3 py-2 shadow-lg">
      <div className="font-semibold">{slice.label}</div>
      <div className="text-gray-300 mt-0.5">
        {slice.value} teams &middot; {percent}%
      </div>
    </div>
  );
}

export default function CompletionBreakdownChart({
  fullyComplete,
  inProgress,
  notStarted,
  optedOut,
}: CompletionBreakdownChartProps) {
  const total = fullyComplete + inProgress + notStarted + optedOut;
  const data: BreakdownSlice[] = [
    { status: "complete", label: STATUS_CHART_LABEL.complete, value: fullyComplete, total },
    { status: "in_progress", label: STATUS_CHART_LABEL.in_progress, value: inProgress, total },
    { status: "not_started", label: STATUS_CHART_LABEL.not_started, value: notStarted, total },
    { status: "opted_out", label: STATUS_CHART_LABEL.opted_out, value: optedOut, total },
  ];

  return (
    <ResponsiveContainer width="100%" height={260}>
      <PieChart>
        <Pie
          data={data}
          dataKey="value"
          nameKey="label"
          cx="50%"
          cy="50%"
          innerRadius={55}
          outerRadius={85}
          paddingAngle={2}
          isAnimationActive
          animationDuration={600}
        >
          {data.map((slice) => (
            <Cell key={slice.status} fill={STATUS_CHART_COLOR[slice.status]} />
          ))}
        </Pie>
        <Tooltip content={<BreakdownTooltip />} />
        <Legend
          verticalAlign="bottom"
          height={36}
          iconType="circle"
          formatter={(value: string) => <span className="text-xs text-gray-600">{value}</span>}
        />
      </PieChart>
    </ResponsiveContainer>
  );
}
