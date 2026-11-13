import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, within, fireEvent } from '@testing-library/react';

const listRuns = vi.fn();
vi.mock('@/lib/api/admin', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/admin')>();
  return { ...actual, listSyncRunHistory: (...a: any[]) => listRuns(...a) };
});

import SyncHistory from '../SyncHistory';
import { formatCount, formatDuration, initiatorLabel, shortId, statusLabel, stageLabel } from '@/lib/sync-history';

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

const counts = (over: Record<string, any> = {}) => ({
  insertedUsers: 0, updatedUsers: 0, unchangedUsers: 1193, insertedTeams: 0, updatedTeams: 0, unchangedTeams: 237, membershipChanges: 0, ...over,
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

describe('SyncHistory added counts', () => {
  const rowFor = (id: string) => screen.getAllByTestId('sync-history-row').find((r) => r.textContent?.includes(id.slice(0, 8)))!;

  it('shows each run its own added counts in the table and details, and keeps them across filter, page and detail navigation', async () => {
    const a = run({ runId: 'aaaaaaaa-run-a', ...counts({ insertedUsers: 2, insertedTeams: 1, updatedUsers: 5, membershipChanges: 4, unchangedUsers: 10, unchangedTeams: 3 }) });
    const b = run({ runId: 'bbbbbbbb-run-b', trigger: 'SCHEDULED', ...counts() });
    serve([a, b]);
    render(<SyncHistory />);
    await screen.findByTestId('sync-history-table');
    expect(screen.getByRole('columnheader', { name: 'Users added' })).toBeInTheDocument();
    expect(screen.getByRole('columnheader', { name: 'Teams added' })).toBeInTheDocument();
    const [ra, rb] = [rowFor('aaaaaaaa'), rowFor('bbbbbbbb')];
    expect(within(ra).getByTestId('sync-users-added')).toHaveTextContent(/^2$/);
    expect(within(ra).getByTestId('sync-teams-added')).toHaveTextContent(/^1$/);
    // A genuine zero is shown as 0, not hidden.
    expect(within(rb).getByTestId('sync-users-added')).toHaveTextContent(/^0$/);
    expect(within(rb).getByTestId('sync-teams-added')).toHaveTextContent(/^0$/);

    fireEvent.click(within(ra).getByRole('button', { name: /view details/i }));
    let d = screen.getByRole('dialog');
    expect(within(d).getByTestId('detail-users-added')).toHaveTextContent(/^2$/);
    expect(within(d).getByTestId('detail-teams-added')).toHaveTextContent(/^1$/);
    expect(within(d).getByTestId('detail-users-updated')).toHaveTextContent(/^5$/); // updates are not additions
    expect(within(d).getByTestId('detail-users-unchanged')).toHaveTextContent(/^10$/);
    expect(within(d).getByTestId('detail-teams-unchanged')).toHaveTextContent(/^3$/);
    expect(within(d).getByTestId('detail-membership-changes')).toHaveTextContent(/^4$/);
    fireEvent.keyDown(document, { key: 'Escape' });

    // Filter to B, then open its details: still B's own zeros.
    fireEvent.change(screen.getByTestId('sync-filter-trigger'), { target: { value: 'SCHEDULED' } });
    await waitFor(() => expect(screen.getAllByTestId('sync-history-row')).toHaveLength(1));
    fireEvent.click(screen.getByRole('button', { name: /view details/i }));
    d = screen.getByRole('dialog');
    expect(within(d).getByTestId('detail-users-added')).toHaveTextContent(/^0$/);
    expect(within(d).getByTestId('detail-membership-changes')).toHaveTextContent(/^0$/);
  });

  it('shows a legacy/unknown count as unavailable, never as 0', async () => {
    serve([run({ ...counts({ unchangedUsers: null, unchangedTeams: null }) }), run({ runId: 'legacy-00-run', insertedUsers: null, insertedTeams: null })]);
    render(<SyncHistory />);
    await screen.findByTestId('sync-history-table');
    const legacy = rowFor('legacy-00');
    expect(within(legacy).getByTestId('sync-users-added')).toHaveTextContent('—');
    expect(within(legacy).getByTestId('sync-teams-added')).toHaveTextContent('—');
    fireEvent.click(within(legacy).getByRole('button', { name: /view details/i }));
    expect(within(screen.getByRole('dialog')).getByTestId('detail-users-added')).toHaveTextContent('Not available');
    fireEvent.keyDown(document, { key: 'Escape' });
    // Added is known (0) but unchanged was never recorded.
    fireEvent.click(within(screen.getAllByTestId('sync-history-row')[0]).getByRole('button', { name: /view details/i }));
    const d = screen.getByRole('dialog');
    expect(within(d).getByTestId('detail-users-added')).toHaveTextContent(/^0$/);
    expect(within(d).getByTestId('detail-users-unchanged')).toHaveTextContent('Not available');
  });

  it('shows no counts for running and failed runs and explains why', async () => {
    serve([running, failed]);
    render(<SyncHistory />);
    await screen.findByTestId('sync-history-table');
    for (const r of screen.getAllByTestId('sync-history-row')) {
      expect(within(r).getByTestId('sync-users-added')).toHaveTextContent('—');
      expect(within(r).getByTestId('sync-teams-added')).toHaveTextContent('—');
    }
    fireEvent.click(within(rowFor('run-live-')).getByRole('button', { name: /view details/i }));
    expect(within(screen.getByRole('dialog')).getByTestId('sync-run-counts')).toHaveTextContent('This run has not finished.');
    fireEvent.keyDown(document, { key: 'Escape' });
    fireEvent.click(within(rowFor('run-fail')).getByRole('button', { name: /view details/i }));
    expect(within(screen.getByRole('dialog')).getByTestId('sync-run-counts')).toHaveTextContent('did not apply any changes');
  });
});

describe('sync-history helpers', () => {
  it('formatCount keeps 0 and only treats null/undefined as unavailable', () => {
    expect(formatCount(0)).toBe('0');
    expect(formatCount(7)).toBe('7');
    expect(formatCount(null)).toBe('—');
    expect(formatCount(undefined, 'Not available')).toBe('Not available');
  });

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
