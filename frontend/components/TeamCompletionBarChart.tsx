"use client";

import type { ReactNode } from "react";
import { BarChart, Bar, Cell, LabelList, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from "recharts";
import type { SurveyStatus, SurveyCompletionData } from "./SurveyCompletionDashboard";

interface TeamCompletionBarChartProps {
  teamStats: SurveyCompletionData["teamStats"];
}

// Same palette used for the status pills/badges in the "All teams" table.
const STATUS_CHART_COLOR: Record<SurveyStatus, string> = {
  complete: "#10B981",
  in_progress: "#F59E0B",
  not_started: "#EF4444",
  opted_out: "#9CA3AF",
};

interface BarDatum {
  teamName: string;
  percent: number;
  completed: number;
  total: number;
  color: string;
  [key: string]: unknown; // satisfies Recharts' generic chart-data-input shape
}

function TeamBarTooltip({ active, payload }: { active?: boolean; payload?: Array<{ payload: BarDatum }> }) {
  if (!active || !payload || !payload.length) return null;
  const d = payload[0].payload;

  return (
    <div className="bg-gray-900 text-white text-xs rounded-lg px-3 py-2 shadow-lg">
      <div className="font-semibold">{d.teamName}</div>
      <div className="text-gray-300 mt-0.5">
        {d.completed} of {d.total} completed ({d.percent}%)
      </div>
    </div>
  );
}

export default function TeamCompletionBarChart({ teamStats }: TeamCompletionBarChartProps) {
  // Opted-out teams have no meaningful completion percentage — omit them
  // rather than showing a misleading 0% bar.
  const bars: BarDatum[] = teamStats
    .filter((team) => team.status !== "opted_out" && team.total > 0)
    .map((team) => ({
      teamName: team.teamName,
      percent: Math.round((team.completed / team.total) * 100),
      completed: team.completed,
      total: team.total,
      color: STATUS_CHART_COLOR[team.status],
    }))
    .sort((a, b) => b.percent - a.percent);

  return (
    <ResponsiveContainer width="100%" height={Math.max(180, bars.length * 34)}>
      <BarChart data={bars} layout="vertical" margin={{ top: 4, right: 40, left: 8, bottom: 4 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" horizontal={false} />
        <XAxis
          type="number"
          domain={[0, 100]}
          tickFormatter={(v) => `${v}%`}
          tick={{ fontSize: 11, fill: "#9ca3af" }}
        />
        <YAxis type="category" dataKey="teamName" width={70} tick={{ fontSize: 12, fill: "#374151" }} />
        <Tooltip content={<TeamBarTooltip />} cursor={{ fill: "#f3f4f6" }} />
        <Bar dataKey="percent" radius={[0, 4, 4, 0]} isAnimationActive animationDuration={600}>
          {bars.map((bar) => (
            <Cell key={bar.teamName} fill={bar.color} />
          ))}
          <LabelList
            dataKey="percent"
            position="right"
            formatter={(label: ReactNode) => `${label}%`}
            style={{ fontSize: 11, fill: "#374151", fontWeight: 600 }}
          />
        </Bar>
      </BarChart>
    </ResponsiveContainer>
  );
}
