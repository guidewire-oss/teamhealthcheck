"use client";

import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from "recharts";

interface TrendPoint {
  label: string;
  completion: number;
  [key: string]: unknown; // satisfies Recharts' generic chart-data-input shape
}

interface CompletionTrendChartProps {
  timeSeries: TrendPoint[];
}

function TrendTooltip({
  active,
  payload,
  label,
}: {
  active?: boolean;
  payload?: Array<{ value?: number }>;
  label?: string;
}) {
  if (!active || !payload || !payload.length) return null;

  return (
    <div className="bg-gray-900 text-white text-xs rounded-lg px-3 py-2 shadow-lg">
      <div className="font-semibold">{label}</div>
      <div className="text-gray-300 mt-0.5">{payload[0].value}% complete</div>
    </div>
  );
}

export default function CompletionTrendChart({ timeSeries }: CompletionTrendChartProps) {
  return (
    <ResponsiveContainer width="100%" height={220}>
      <LineChart data={timeSeries} margin={{ top: 8, right: 16, left: 0, bottom: 4 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
        <XAxis dataKey="label" tick={{ fontSize: 11, fill: "#6b7280" }} />
        <YAxis domain={[0, 100]} tickFormatter={(v) => `${v}%`} tick={{ fontSize: 11, fill: "#9ca3af" }} width={40} />
        <Tooltip content={<TrendTooltip />} />
        <Line
          type="monotone"
          dataKey="completion"
          stroke="#4f46e5"
          strokeWidth={2}
          dot={{ r: 3, fill: "#4f46e5" }}
          isAnimationActive
          animationDuration={700}
        />
      </LineChart>
    </ResponsiveContainer>
  );
}
