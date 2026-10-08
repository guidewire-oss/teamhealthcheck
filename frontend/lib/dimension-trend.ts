export type DimensionTrendValue = 'improving' | 'stable' | 'declining';

export interface DimensionTrendDisplay {
  trend: DimensionTrendValue;
  label: 'Improving' | 'Stable' | 'Declining';
  colorClass: string;
  icon: 'up' | 'down' | 'stable';
}

const SUPPORTED_TRENDS: DimensionTrendValue[] = ['improving', 'stable', 'declining'];

/**
 * Maps the raw `trend` value from the final post-workshop `health_check_responses`
 * record to display info for the Team Cards dimension card. Returns null when the
 * trend is missing or not one of the supported values, so callers know to hide
 * the indicator entirely.
 */
export function getDimensionTrendDisplay(
  trend: string | null | undefined
): DimensionTrendDisplay | null {
  if (!trend || !SUPPORTED_TRENDS.includes(trend as DimensionTrendValue)) {
    return null;
  }

  const value = trend as DimensionTrendValue;

  switch (value) {
    case 'improving':
      return { trend: value, label: 'Improving', colorClass: 'text-green-600', icon: 'up' };
    case 'declining':
      return { trend: value, label: 'Declining', colorClass: 'text-red-600', icon: 'down' };
    case 'stable':
      return { trend: value, label: 'Stable', colorClass: 'text-gray-400', icon: 'stable' };
  }
}
