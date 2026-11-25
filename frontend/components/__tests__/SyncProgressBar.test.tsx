import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import SyncProgressBar from '../SyncProgressBar';

const states = () => [0, 1, 2, 3, 4, 5].map((i) => screen.getByTestId(`sync-step-${i}`).getAttribute('data-state'));
const segs = () => screen.getAllByTestId('progress-segment').map((s) => s.getAttribute('data-state'));

describe('SyncProgressBar', () => {
  it('renders the six steps in order, all gray before any run', () => {
    render(<SyncProgressBar run={null} />);
    ['Sync started', 'Data loaded', 'Running sync', 'Queries updated', 'Checking values', 'Success'].forEach((l) =>
      expect(screen.getByText(l, { exact: false })).toBeInTheDocument()
    );
    expect(states()).toEqual(Array(6).fill('pending'));
    expect(segs()).toEqual(Array(5).fill('gray'));
    expect(screen.queryAllByTestId('step-icon-complete')).toHaveLength(0);
  });

  it('shows a loading indicator on the current step and fills earlier segments green', () => {
    render(<SyncProgressBar run={{ status: 'RUNNING', stage: 'UPSERTING_USERS' }} />);
    expect(states()).toEqual(['complete', 'complete', 'active', 'pending', 'pending', 'pending']);
    expect(screen.getAllByTestId('step-icon-complete')).toHaveLength(2);
    expect(screen.getAllByTestId('step-icon-active')).toHaveLength(1);
    expect(segs()).toEqual(['green', 'green', 'progress', 'gray', 'gray']);
  });

  it('keeps the whole line green with a check on every step on success', () => {
    render(<SyncProgressBar run={{ status: 'SUCCESS', stage: 'COMPLETED' }} />);
    expect(screen.getAllByTestId('step-icon-complete')).toHaveLength(6);
    expect(segs()).toEqual(Array(5).fill('green'));
    expect(screen.queryByTestId('step-icon-active')).toBeNull();
  });

  it('shows the failed step in red with the error message; later steps stay incomplete', () => {
    render(
      <SyncProgressBar
        run={{ status: 'FAILED', stage: 'FAILED', failedStage: 'SYNCING_MEMBERSHIPS' }}
        errorMessage="The synchronization failed while saving changes."
      />
    );
    expect(states()).toEqual(['complete', 'complete', 'complete', 'failed', 'pending', 'pending']);
    expect(screen.getAllByTestId('step-icon-failed')).toHaveLength(1);
    expect(screen.getByRole('alert')).toHaveTextContent('failed while saving changes');
    expect(segs()).toEqual(['green', 'green', 'green', 'gray', 'gray']);
  });

  it('announces state in words, not colour alone', () => {
    render(<SyncProgressBar run={{ status: 'RUNNING', stage: 'FETCHING' }} />);
    expect(screen.getByTestId('sync-progress-status')).toHaveTextContent('Running: Data loaded');
    expect(screen.getByTestId('sync-step-1')).toHaveTextContent('in progress');
    expect(screen.getByTestId('sync-step-0')).toHaveTextContent('complete');
    expect(screen.getByTestId('sync-step-2')).toHaveTextContent('not started');
    expect(screen.getByTestId('sync-step-1')).toHaveAttribute('aria-current', 'step');
  });

  it('does not show an error line for a non-failed run', () => {
    render(<SyncProgressBar run={{ status: 'RUNNING', stage: 'FETCHING' }} errorMessage="x" />);
    expect(screen.queryByTestId('sync-progress-error')).toBeNull();
  });
});
