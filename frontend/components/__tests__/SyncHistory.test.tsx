import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, within, fireEvent } from '@testing-library/react';

const listRuns = vi.fn();
vi.mock('@/lib/api/admin', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/admin')>();
  return { ...actual, listSyncRunHistory: (...a: any[]) => listRuns(...a) };
});

import SyncHistory from '../SyncHistory';
import { formatDuration, initiatorLabel, shortId, statusLabel, stageLabel } from '@/lib/sync-history';

const run = (over: Record<string, any> = {}) => ({
  runId: 'run-aaaaaaaa-bbbb-cccc-dddd-000000000001', snapshotType: 'COMPLETE', trigger: 'MANUAL',
  status: 'SUCCESS', stage: 'COMPLETED', initiatedBy: 'admin',
  createdAt: '2026-11-01T10:00:00Z', startedAt: '2026-11-01T10:00:01Z', completedAt: '2026-11-01T10:00:31Z',
  sourceGeneratedAt: '2026-11-01T09:00:00Z', durationSeconds: 30,
  failedStage: null, errorCode: null, errorMessage: null, ...over,
});
const page = (runs: any[], over: Record<string, any> = {}) => ({
  runs,
  pagination: { page: 1, pageSize: 20, totalItems: runs.length, totalPages: runs.length ? 1 : 0, hasNextPage: false, hasPreviousPage: false, ...over },
});

const failed = run({
  runId: 'run-failed-0000-0000-0000-000000000002', status: 'FAILED', stage: 'FAILED', trigger: 'SCHEDULED',
  initiatedBy: 'scheduler', sourceGeneratedAt: null, failedStage: 'FETCHING', errorCode: 'PROVIDER_FETCH_FAILED',
  errorMessage: 'Provider call failed with status 503', durationSeconds: 2,
});
const running = run({
  runId: 'run-live-00000-0000-0000-000000000003', status: 'RUNNING', stage: 'UPSERTING_USERS', completedAt: null,
  sourceGeneratedAt: null, durationSeconds: null,
});

// Route calls by filter so the table and the three summary queries each get the right data.
const serve = (all: any[]) =>
  listRuns.mockImplementation(async (p: any = {}) => {
    let rs = all.filter((r) => (!p.status || r.status === p.status) && (!p.trigger || r.trigger === p.trigger));
    const size = p.pageSize ?? 20;
    const totalItems = rs.length;
    rs = rs.slice(((p.page ?? 1) - 1) * size, (p.page ?? 1) * size);
    return page(rs, { page: p.page ?? 1, pageSize: size, totalItems, totalPages: Math.ceil(totalItems / size), hasNextPage: (p.page ?? 1) * size < totalItems, hasPreviousPage: (p.page ?? 1) > 1 });
  });

beforeEach(() => vi.clearAllMocks());

describe('SyncHistory', () => {
  it('shows loading, then populated rows with text status badges', async () => {
    serve([running, failed, run()]);
    render(<SyncHistory />);
    expect(screen.getByTestId('sync-history-loading')).toBeInTheDocument();
    const rows = await screen.findAllByTestId('sync-history-row');
    expect(rows).toHaveLength(3);
    const badges = within(screen.getByTestId('sync-history-table')).getAllByTestId('sync-status-badge');
    expect(badges.map((b) => b.textContent)).toEqual(['In progress', 'Failed', 'Succeeded']);
    // In-progress runs say so rather than showing a finish time or a duration.
    expect(within(rows[0]).getByText('In progress', { selector: 'td' })).toBeInTheDocument();
    expect(rows[0].textContent).toContain('—');
  });

  it('requests newest-first page 1 with the default page size and no filters', async () => {
    serve([run()]);
    render(<SyncHistory />);
    await screen.findByTestId('sync-history-table');
    expect(listRuns).toHaveBeenCalledWith({ page: 1, pageSize: 20, status: undefined, trigger: undefined });
  });

  it('shows an empty state when no run has ever been recorded, and summary empty states', async () => {
    serve([]);
    render(<SyncHistory />);
    expect(await screen.findByTestId('sync-history-empty')).toHaveTextContent('No sync runs have been recorded yet.');
    await waitFor(() => expect(screen.getByTestId('sync-summary-success')).toHaveTextContent('No successful sync yet'));
    expect(screen.getByTestId('sync-summary-failed')).toHaveTextContent('None');
    expect(screen.getByTestId('sync-summary-latest')).toHaveTextContent('No sync has run yet');
  });

  it('derives summary cards from the newest run, success and failure', async () => {
    serve([running, failed, run()]);
    render(<SyncHistory />);
    await waitFor(() => expect(screen.getByTestId('sync-summary-latest')).toHaveTextContent('In progress'));
    expect(screen.getByTestId('sync-summary-success')).toHaveTextContent('Manual · Complete snapshot');
    expect(screen.getByTestId('sync-summary-failed')).toHaveTextContent('Scheduled · Complete snapshot');
    expect(listRuns).toHaveBeenCalledWith({ pageSize: 1, status: 'FAILED' });
  });

  it('says "None" for failures when there are none', async () => {
    serve([run()]);
    render(<SyncHistory />);
    await waitFor(() => expect(screen.getByTestId('sync-summary-failed')).toHaveTextContent('None'));
  });

  it('filters by status and sync type server-side and resets to page 1', async () => {
    serve([running, failed, run()]);
    render(<SyncHistory />);
    await screen.findAllByTestId('sync-history-row');

    fireEvent.change(screen.getByTestId('sync-filter-status'), { target: { value: 'FAILED' } });
    await waitFor(() => expect(screen.getAllByTestId('sync-history-row')).toHaveLength(1));
    expect(listRuns).toHaveBeenCalledWith({ page: 1, pageSize: 20, status: 'FAILED', trigger: undefined });

    fireEvent.change(screen.getByTestId('sync-filter-trigger'), { target: { value: 'MANUAL' } });
    expect(await screen.findByTestId('sync-history-empty')).toHaveTextContent('No sync runs match these filters.');
    expect(listRuns).toHaveBeenCalledWith({ page: 1, pageSize: 20, status: 'FAILED', trigger: 'MANUAL' });
  });

  it('pages forward and back', async () => {
    const many = Array.from({ length: 25 }, (_, i) => run({ runId: `run-${String(i).padStart(2, '0')}-xxxxxxxx`, createdAt: `2026-11-01T10:${String(59 - i).padStart(2, '0')}:00Z` }));
    serve(many);
    render(<SyncHistory />);
    await screen.findAllByTestId('sync-history-row');
    expect(screen.getByTestId('sync-history-page-info')).toHaveTextContent('Page 1 of 2 · 25 runs');
    expect(screen.getByTestId('sync-history-prev')).toBeDisabled();

    fireEvent.click(screen.getByTestId('sync-history-next'));
    await waitFor(() => expect(screen.getByTestId('sync-history-page-info')).toHaveTextContent('Page 2 of 2'));
    expect(screen.getAllByTestId('sync-history-row')).toHaveLength(5);
    expect(screen.getByTestId('sync-history-next')).toBeDisabled();

    fireEvent.click(screen.getByTestId('sync-history-prev'));
    await waitFor(() => expect(screen.getByTestId('sync-history-page-info')).toHaveTextContent('Page 1 of 2'));
  });

  it('opens details for a failed run showing the sanitized failure, and closes with Escape', async () => {
    serve([failed]);
    render(<SyncHistory />);
    const row = await screen.findByTestId('sync-history-row');
    fireEvent.click(within(row).getByRole('button', { name: /view details/i }));
    const dialog = screen.getByRole('dialog');
    expect(dialog).toHaveAttribute('aria-modal', 'true');
    expect(within(dialog).getByText('run-failed-0000-0000-0000-000000000002')).toBeInTheDocument(); // full id
    expect(within(dialog).getByText('Scheduler (automatic)')).toBeInTheDocument();
    const failure = within(dialog).getByTestId('sync-run-failure');
    expect(failure).toHaveTextContent('Fetching from provider');
    expect(failure).toHaveTextContent('Could not fetch from provider');
    expect(failure).toHaveTextContent('Provider call failed with status 503');
    expect(within(dialog).getByText('Provider snapshot generated at').nextSibling).toHaveTextContent('—');

    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('shows no failure section and the source-generated time for a successful run', async () => {
    serve([run()]);
    render(<SyncHistory />);
    fireEvent.click(await screen.findByTestId('sync-history-row'));
    const dialog = screen.getByRole('dialog');
    expect(within(dialog).queryByTestId('sync-run-failure')).not.toBeInTheDocument();
    expect(within(dialog).getByText('Queued at')).toBeInTheDocument();
    expect(within(dialog).getByText('Provider snapshot generated at')).toBeInTheDocument();
    expect(within(dialog).getByText('30 s')).toBeInTheDocument();
  });

  it('shows an in-progress run as such in details, with no duration', async () => {
    serve([running]);
    render(<SyncHistory />);
    fireEvent.click(await screen.findByTestId('sync-history-row'));
    const dialog = screen.getByRole('dialog');
    expect(within(dialog).getByText('Finished at').nextSibling).toHaveTextContent('In progress');
    expect(within(dialog).getByText('Duration').nextSibling).toHaveTextContent('—');
    expect(within(dialog).queryByTestId('sync-run-failure')).not.toBeInTheDocument();
  });

  it('shows an error with Retry that reloads and recovers', async () => {
    serve([run()]);
    // The table request is issued first; the three summary requests succeed.
    listRuns.mockRejectedValueOnce(new Error('Failed to load sync history'));
    render(<SyncHistory />);
    expect(await screen.findByTestId('sync-history-error')).toHaveTextContent('Failed to load sync history');
    fireEvent.click(screen.getByTestId('sync-history-retry'));
    expect(await screen.findByTestId('sync-history-table')).toBeInTheDocument();
    expect(screen.queryByTestId('sync-history-error')).not.toBeInTheDocument();
  });

  it('has no write controls', async () => {
    serve([run()]);
    render(<SyncHistory />);
    await screen.findByTestId('sync-history-table');
    expect(screen.queryByRole('button', { name: /retry sync|sync now|run sync|trigger/i })).not.toBeInTheDocument();
  });
});

describe('sync-history helpers', () => {
  it('maps known values and falls back to the raw value for unknown ones', () => {
    expect(statusLabel('PENDING')).toBe('Queued');
    expect(statusLabel('SOMETHING_NEW')).toBe('SOMETHING_NEW');
    expect(stageLabel(null)).toBe('—');
  });
  it('formats duration without inventing zero', () => {
    expect(formatDuration(null)).toBe('—');
    expect(formatDuration(0)).toBe('< 1 s');
    expect(formatDuration(3725)).toBe('1 h 2 min 5 s');
  });
  it('shortens ids and labels initiators', () => {
    expect(shortId('abcdefghijkl')).toBe('abcdefgh…');
    expect(initiatorLabel('scheduler')).toBe('Scheduler (automatic)');
    expect(initiatorLabel('')).toBe('—');
  });
});
