import { cloneElement, type ReactElement } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { render, waitFor } from '@testing-library/react';
import CompletionBreakdownChart from '@/components/CompletionBreakdownChart';

// jsdom has no layout, so the real ResponsiveContainer measures 0x0 and
// Recharts renders neither the pie nor the legend. Replace only the container
// with a fixed-size one that hands its child explicit dimensions; the chart,
// pie, and legend under test are still the real Recharts components.
vi.mock('recharts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('recharts')>();
  return {
    ...actual,
    ResponsiveContainer: ({ children }: { children: ReactElement }) => (
      <div style={{ width: 800, height: 400 }}>{cloneElement(children, { width: 800, height: 400 })}</div>
    ),
  };
});

// Recharts 3 mounts the legend one tick after the chart, so legend assertions
// wait for it instead of reading the DOM synchronously.
async function legendItems(container: HTMLElement): Promise<string[]> {
  await waitFor(() => expect(container.querySelectorAll('.recharts-legend-item').length).toBeGreaterThan(0));
  return Array.from(container.querySelectorAll('.recharts-legend-item')).map((li) => li.textContent ?? '');
}

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
    expect(container.querySelector('.recharts-wrapper')).toHaveAttribute('width', '800');
  });

  it('displays all four existing statuses in the legend, with their existing labels', async () => {
    const { container } = render(
      <CompletionBreakdownChart fullyComplete={3} inProgress={2} notStarted={1} optedOut={1} />,
    );
    const items = await legendItems(container);

    expect(items).toHaveLength(4);
    expect(items[0]).toContain('Fully complete');
    expect(items[1]).toContain('In progress');
    expect(items[2]).toContain('Not started');
    expect(items[3]).toContain('Opted out');
  });

  it("shows each status's share of the total in its legend entry", async () => {
    const { container } = render(
      <CompletionBreakdownChart fullyComplete={3} inProgress={2} notStarted={1} optedOut={1} />,
    );
    const items = await legendItems(container);

    expect(items[0]).toContain('43%'); // 3 of 7
    expect(items[1]).toContain('29%'); // 2 of 7
    expect(items[2]).toContain('14%'); // 1 of 7
  });

  it('still displays a status in the legend even when its own count is zero', async () => {
    const { container } = render(
      <CompletionBreakdownChart fullyComplete={5} inProgress={0} notStarted={0} optedOut={0} />,
    );
    const items = (await legendItems(container)).join(' | ');

    expect(items).toContain('In progress');
    expect(items).toContain('Not started');
    expect(items).toContain('Opted out');
  });
});
