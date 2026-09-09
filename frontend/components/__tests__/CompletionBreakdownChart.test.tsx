import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';
import CompletionBreakdownChart from '@/components/CompletionBreakdownChart';

describe('CompletionBreakdownChart', () => {
  it('renders as a Recharts pie chart', () => {
    const { container } = render(
      <CompletionBreakdownChart fullyComplete={3} inProgress={2} notStarted={1} optedOut={1} />,
    );
    // Recharts tags the pie chart's own wrapper group with this class
    // regardless of innerRadius -- it's the reliable, synchronous signal
    // that this is a Pie chart (as opposed to e.g. a Bar/Line chart).
    // Individual <path> sector elements are subject to Recharts' mount
    // entrance animation and are not a stable jsdom assertion target (see
    // TeamCompletionBarChart's own tests for the same constraint), so this
    // test deliberately does not inspect sector geometry or count.
    expect(container.querySelector('.recharts-pie')).not.toBeNull();
  });

  it('displays all four existing statuses in the legend, with their existing labels', () => {
    const { container } = render(
      <CompletionBreakdownChart fullyComplete={3} inProgress={2} notStarted={1} optedOut={1} />,
    );
    const legendText = container.querySelector('.recharts-default-legend')?.textContent ?? '';
    expect(legendText).toContain('Fully complete');
    expect(legendText).toContain('In progress');
    expect(legendText).toContain('Not started');
    expect(legendText).toContain('Opted out');
  });

  it('still displays a status in the legend even when its own count is zero', () => {
    const { container } = render(
      <CompletionBreakdownChart fullyComplete={5} inProgress={0} notStarted={0} optedOut={0} />,
    );
    const legendText = container.querySelector('.recharts-default-legend')?.textContent ?? '';
    expect(legendText).toContain('In progress');
    expect(legendText).toContain('Not started');
    expect(legendText).toContain('Opted out');
  });
});
