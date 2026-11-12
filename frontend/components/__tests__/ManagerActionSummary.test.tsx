import { describe, it, expect, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ManagerActionSummary from '../ManagerActionSummary';
import type { TeamActionSummary } from '@/lib/api/action-items';

const row = (over: Partial<TeamActionSummary> = {}): TeamActionSummary => ({
  teamId: 't1',
  teamName: 'Phoenix',
  openCount: 4,
  statusCounts: { open: 2, onHold: 1, inProgress: 1, completed: 3 },
  ...over,
});

const renderIt = (props: Partial<React.ComponentProps<typeof ManagerActionSummary>> = {}) =>
  render(<ManagerActionSummary summary={[row()]} loading={false} error={false} onSelectTeam={vi.fn()} {...props} />);

describe('ManagerActionSummary', () => {
  it('shows each status count for a team', () => {
    renderIt();
    const cells = within(screen.getByTestId('manager-action-row')).getAllByRole('cell');
    expect(cells.map((c) => c.textContent)).toEqual(['Phoenix', '2', '1', '1', '3']);
  });

  it('renders genuine zero counts as 0, not as unavailable', () => {
    renderIt({ summary: [row({ statusCounts: { open: 0, onHold: 0, inProgress: 0, completed: 0 } })] });
    const cells = within(screen.getByTestId('manager-action-row')).getAllByRole('cell');
    expect(cells.slice(1).map((c) => c.textContent)).toEqual(['0', '0', '0', '0']);
  });

  it('renders a dash, not zero, when a team has no counts', () => {
    renderIt({ summary: [row({ statusCounts: undefined as any })] });
    const cells = within(screen.getByTestId('manager-action-row')).getAllByRole('cell');
    expect(cells.slice(1).map((c) => c.textContent)).toEqual(['—', '—', '—', '—']);
  });

  it('shows an error, not the empty state, when loading failed', () => {
    renderIt({ summary: [], error: true });
    expect(screen.getByTestId('manager-actions-error')).toBeInTheDocument();
    expect(screen.queryByText(/no action items found/i)).not.toBeInTheDocument();
  });

  it('shows the empty state when there are no teams and no error', () => {
    renderIt({ summary: [] });
    expect(screen.getByText(/no action items found/i)).toBeInTheDocument();
    expect(screen.queryByTestId('manager-actions-error')).not.toBeInTheDocument();
  });

  it('shows a spinner and no table while loading', () => {
    renderIt({ loading: true });
    expect(screen.queryByTestId('manager-action-row')).not.toBeInTheDocument();
    expect(screen.queryByTestId('manager-actions-error')).not.toBeInTheDocument();
  });

  it('reports the clicked team', async () => {
    const onSelectTeam = vi.fn();
    renderIt({ onSelectTeam });
    await userEvent.setup().click(screen.getByTestId('manager-action-row'));
    expect(onSelectTeam).toHaveBeenCalledWith('t1', 'Phoenix');
  });
});
