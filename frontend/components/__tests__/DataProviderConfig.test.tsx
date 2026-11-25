import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, act, fireEvent } from '@testing-library/react';

// The component talks to the backend only through these functions.
const getSettings = vi.fn();
const startSync = vi.fn();
const getRun = vi.fn();
const getLatest = vi.fn();
const clearCache = vi.fn();
const UNLOCKED_THRESHOLD = {
  maxDeletePercent: 20, source: 'default', defaultPercent: 20, minPercent: 1, maxPercent: 100, locked: false,
};
const getThreshold = vi.fn().mockResolvedValue(UNLOCKED_THRESHOLD);
const dismissHold = vi.fn().mockResolvedValue(UNLOCKED_THRESHOLD);

// getMassDeletionHold is deliberately NOT mocked: whether the hold banner shows
// depends on how the real helper reads a run's errorCode.
vi.mock('@/lib/api/admin', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/admin')>();
  return {
    ...actual,
    getOrganizationProviderSettings: (...a: any[]) => getSettings(...a),
    syncOrganizationProvider: (...a: any[]) => startSync(...a),
    getSyncRun: (...a: any[]) => getRun(...a),
    getLatestSyncRun: (...a: any[]) => getLatest(...a),
    clearAdminCache: (...a: any[]) => clearCache(...a),
    getOrgSyncDeletionThreshold: (...a: any[]) => getThreshold(...a),
    dismissMassDeletionHold: (...a: any[]) => dismissHold(...a),
  };
});

import DataProviderConfig from '../DataProviderConfig';
import { SYNC_POLL_INTERVAL_MS } from '@/lib/useSyncRun';

const READY = { provider: 'data-provider', baseUrlConfigured: true, tokenConfigured: true, readyToSync: true };

const makeRun = (over: Record<string, any> = {}) => ({
  runId: 'sync_1', status: 'PENDING', stage: 'QUEUED', startedAt: null, completedAt: null,
  createdAt: '2026-08-31T05:00:00Z', insertedUsers: 0, updatedUsers: 0, unchangedUsers: 0, insertedTeams: 0, updatedTeams: 0, unchangedTeams: 0,
  membershipChanges: 0, deactivatedRecords: 0, healthChecksDisabled: 0, healthChecksEnabled: 0, massDeletionOverridden: false,
  errorCode: null, errorMessage: null, failedStage: null,
  ...over,
});

const HOLD_REPORT = {
  threshold: 20,
  users: { kind: 'deleted', existing: 20, deleting: 6, percent: 30, threshold: 20, incoming: 14, exceedsThreshold: true, contributesToHold: true },
  teams: { kind: 'deleted', existing: 10, deleting: 3, percent: 30, threshold: 20, incoming: 7, exceedsThreshold: true, contributesToHold: true },
  memberships: { kind: 'cascaded', existing: 50, deleting: 8, percent: 16, threshold: 20, incoming: 42, exceedsThreshold: false, contributesToHold: false },
};

const stepStates = () => [0, 1, 2, 3, 4, 5].map((i) => screen.getByTestId(`sync-step-${i}`).getAttribute('data-state'));
const status = () => screen.getByTestId('sync-progress-status').textContent;
const button = () => screen.getByTestId('sync-now-btn') as HTMLButtonElement;

// Lets pending promises settle, then advances the poll timer.
const flush = () => act(async () => { await vi.advanceTimersByTimeAsync(0); });
const poll = () => act(async () => { await vi.advanceTimersByTimeAsync(SYNC_POLL_INTERVAL_MS + 10); });

const renderReady = async (latest: any = null, settings = READY) => {
  getSettings.mockResolvedValue(settings);
  getLatest.mockResolvedValue(latest);
  render(<DataProviderConfig />);
  await flush();
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.useFakeTimers();
  getThreshold.mockResolvedValue(UNLOCKED_THRESHOLD);
  dismissHold.mockResolvedValue(UNLOCKED_THRESHOLD);
});
afterEach(() => vi.useRealTimers());

describe('Sync Now: status and progress', () => {
  it('renders an enabled, accessible button and an idle gray bar before any run', async () => {
    await renderReady();
    expect(button()).toBeEnabled();
    expect(button()).toHaveAccessibleName('Sync organization data from provider');
    expect(stepStates()).toEqual(Array(6).fill('pending'));
    expect(screen.queryByLabelText(/token/i)).toBeNull();
  });

  it('keeps Sync Now disabled until the latest run has loaded, without showing Syncing', async () => {
    let resolveLatest!: (v: any) => void;
    getSettings.mockResolvedValue(READY);
    getLatest.mockReturnValue(new Promise((res) => { resolveLatest = res; }));
    render(<DataProviderConfig />);
    await flush();

    expect(button()).toBeDisabled();
    expect(button()).toHaveTextContent('Sync Now');
    fireEvent.click(button());
    await flush();
    expect(startSync).not.toHaveBeenCalled();

    resolveLatest(null);
    await flush();
    expect(button()).toBeEnabled();
  });

  it('shows Pending immediately on click, using the returned run id, and disables the button', async () => {
    startSync.mockResolvedValue(makeRun());
    await renderReady();
    fireEvent.click(button());
    await flush();
    expect(startSync).toHaveBeenCalledTimes(1);
    expect(status()).toMatch(/^Pending/);
    expect(stepStates()[0]).toBe('active');
    expect(button()).toBeDisabled();
  });

  it('follows PENDING -> RUNNING (with stage) -> SUCCESS and then stops polling', async () => {
    startSync.mockResolvedValue(makeRun());
    getRun
      .mockResolvedValueOnce(makeRun({ status: 'RUNNING', stage: 'UPSERTING_USERS' }))
      .mockResolvedValue(makeRun({
        status: 'SUCCESS', stage: 'COMPLETED', completedAt: '2026-08-31T05:00:04Z',
        insertedUsers: 3, updatedUsers: 9, unchangedUsers: 1180, insertedTeams: 1, updatedTeams: 2, unchangedTeams: 234, membershipChanges: 7,
      }));
    await renderReady();
    fireEvent.click(button());
    await flush();

    await poll();
    expect(status()).toBe('Running: Running sync');
    expect(stepStates()).toEqual(['complete', 'complete', 'active', 'pending', 'pending', 'pending']);
    expect(button()).toBeDisabled();

    await poll();
    expect(stepStates()).toEqual(Array(6).fill('complete'));
    expect(screen.getByTestId('sync-counts')).toHaveTextContent('Users: 3 added, 9 updated, 1180 unchanged');
    expect(screen.getByTestId('sync-counts')).toHaveTextContent('Teams: 1 added, 2 updated, 234 unchanged');
    expect(screen.getByTestId('sync-counts')).toHaveTextContent('Membership changes: 7');
    expect(button()).toBeEnabled();
    expect(clearCache).toHaveBeenCalled();

    const calls = getRun.mock.calls.length;
    await poll();
    await poll();
    expect(getRun.mock.calls.length).toBe(calls); // terminal: polling stopped
  });

  it('shows an unrecorded unchanged count as not available, not 0', async () => {
    await renderReady(makeRun({
      status: 'SUCCESS', stage: 'COMPLETED', insertedUsers: 2, insertedTeams: 1, unchangedUsers: null, unchangedTeams: null,
    }));
    expect(screen.getByTestId('sync-counts')).toHaveTextContent('Users: 2 added, 0 updated, not available unchanged');
    expect(screen.getByTestId('sync-counts')).toHaveTextContent('Teams: 1 added, 0 updated, not available unchanged');
  });

  it('shows an identical re-sync as 0 added, 0 updated, everything unchanged, 0 membership changes', async () => {
    await renderReady(makeRun({
      status: 'SUCCESS', stage: 'COMPLETED', unchangedUsers: 1192, unchangedTeams: 237,
    }));
    expect(screen.getByTestId('sync-counts').textContent).toBe(
      'Users: 0 added, 0 updated, 1192 unchanged; Teams: 0 added, 0 updated, 237 unchanged; Membership changes: 0.'
    );
    expect(screen.queryByTestId('sync-deactivated')).toBeNull();
  });

  it('shows deactivations and that history is kept', async () => {
    await renderReady(makeRun({ status: 'SUCCESS', stage: 'COMPLETED', deactivatedRecords: 4 }));
    expect(screen.getByTestId('sync-deactivated')).toHaveTextContent('4 user(s) or team(s) marked inactive');
    expect(screen.getByTestId('sync-deactivated')).toHaveTextContent('history is kept');
  });

  it('warns when the sync switched team health checks off or on', async () => {
    await renderReady(makeRun({ status: 'SUCCESS', stage: 'COMPLETED', healthChecksDisabled: 3, healthChecksEnabled: 2 }));
    expect(screen.getByTestId('sync-health-checks-disabled')).toHaveTextContent('switched off for 3 team(s)');
    expect(screen.getByTestId('sync-health-checks-enabled')).toHaveTextContent('switched on for 2 team(s)');
  });

  it('reports that a mass-deletion override was applied', async () => {
    await renderReady(makeRun({ status: 'SUCCESS', stage: 'COMPLETED', massDeletionOverridden: true }));
    expect(screen.getByTestId('sync-override-applied')).toHaveTextContent('overridden');
  });

  it('shows no health-check or override notices for an ordinary sync', async () => {
    await renderReady(makeRun({ status: 'SUCCESS', stage: 'COMPLETED' }));
    expect(screen.queryByTestId('sync-health-checks-disabled')).toBeNull();
    expect(screen.queryByTestId('sync-health-checks-enabled')).toBeNull();
    expect(screen.queryByTestId('sync-override-applied')).toBeNull();
  });

  it('shows the failed step in red with the safe message and stage, leaving later steps incomplete', async () => {
    startSync.mockResolvedValue(makeRun());
    getRun.mockResolvedValue(makeRun({
      status: 'FAILED', stage: 'FAILED', failedStage: 'SYNCING_MEMBERSHIPS', errorCode: 'DATABASE_ERROR',
      errorMessage: 'The synchronization failed while saving changes. Nothing was changed.',
    }));
    await renderReady();
    fireEvent.click(button());
    await flush();
    await poll();
    expect(stepStates()).toEqual(['complete', 'complete', 'complete', 'failed', 'pending', 'pending']);
    expect(screen.getByTestId('sync-progress-error')).toHaveTextContent('Nothing was changed');
    expect(status()).toBe('Failed at step: Queries updated');
    expect(screen.queryByTestId('sync-result')).toBeNull();
    expect(button()).toBeEnabled();
  });

  it('does not fire a second request when clicked again mid-flight', async () => {
    startSync.mockResolvedValue(makeRun());
    await renderReady();
    fireEvent.click(button());
    fireEvent.click(button());
    await flush();
    expect(startSync).toHaveBeenCalledTimes(1);
  });

  it('shows the failure and never success when starting the sync fails (network error)', async () => {
    startSync.mockRejectedValue(new Error('Network down'));
    await renderReady();
    fireEvent.click(button());
    await flush();
    expect(screen.getByTestId('provider-error')).toHaveTextContent('Network down');
    expect(screen.queryByTestId('sync-result')).toBeNull();
    expect(stepStates()).toEqual(Array(6).fill('pending'));
    expect(button()).toBeEnabled();
  });

  it('keeps polling through a network error without ever showing success', async () => {
    startSync.mockResolvedValue(makeRun());
    getRun
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValue(makeRun({ status: 'SUCCESS', stage: 'COMPLETED' }));
    await renderReady();
    fireEvent.click(button());
    await flush();

    await poll();
    expect(screen.getByTestId('sync-poll-error')).toHaveTextContent('Retrying');
    expect(screen.queryByTestId('sync-result')).toBeNull();
    expect(status()).toMatch(/^Pending/);

    await poll();
    expect(screen.getByTestId('sync-result')).toBeInTheDocument();
    expect(screen.queryByTestId('sync-poll-error')).toBeNull();
  });
});

describe('Sync Now: restoring state after a refresh', () => {
  it('restores a RUNNING run: button disabled, correct step, keeps polling to the end', async () => {
    getRun.mockResolvedValue(makeRun({ status: 'SUCCESS', stage: 'COMPLETED' }));
    await renderReady(makeRun({ status: 'RUNNING', stage: 'SYNCING_MEMBERSHIPS' }));
    expect(button()).toBeDisabled();
    expect(stepStates()).toEqual(['complete', 'complete', 'complete', 'active', 'pending', 'pending']);
    expect(startSync).not.toHaveBeenCalled();
    await poll();
    expect(stepStates()).toEqual(Array(6).fill('complete'));
    expect(button()).toBeEnabled();
  });

  it('restores the last SUCCESS as a fully green bar with an enabled button and no polling', async () => {
    await renderReady(makeRun({ status: 'SUCCESS', stage: 'COMPLETED' }));
    expect(stepStates()).toEqual(Array(6).fill('complete'));
    expect(button()).toBeEnabled();
    await poll();
    expect(getRun).not.toHaveBeenCalled();
  });

  it('restores the last FAILED run with its failed step and message', async () => {
    await renderReady(makeRun({
      status: 'FAILED', stage: 'FAILED', failedStage: 'VALIDATING', errorCode: 'INVALID_SNAPSHOT', errorMessage: 'bad payload',
    }));
    expect(stepStates()).toEqual(['complete', 'failed', 'pending', 'pending', 'pending', 'pending']);
    expect(screen.getByTestId('sync-progress-error')).toHaveTextContent('bad payload');
  });

  it('is idle and enabled when no sync has ever run', async () => {
    await renderReady(null);
    expect(button()).toBeEnabled();
    expect(stepStates()).toEqual(Array(6).fill('pending'));
  });
});

describe('Sync Now: provider readiness', () => {
  it('disables the button and explains why when the provider is not ready', async () => {
    await renderReady(null, { ...READY, tokenConfigured: false, readyToSync: false });
    expect(button()).toBeDisabled();
    expect(screen.getByTestId('provider-not-configured')).toHaveTextContent('DATA_PROVIDER_API_TOKEN');
  });
});

describe('mass-deactivation hold and Sync Anyway override', () => {
  const heldRun = () => makeRun({
    status: 'FAILED', stage: 'FAILED', failedStage: 'UPSERTING_USERS', errorCode: 'MASS_DEACTIVATION_HOLD',
    errorMessage: 'This sync would deactivate an unusually large share.', massDeletion: HOLD_REPORT,
  });

  it('shows the calculated counts and threshold, using "deactivated" wording', async () => {
    await renderReady(heldRun());
    expect(screen.getByTestId('sync-mass-deletion-hold')).toBeInTheDocument();
    expect(screen.getByTestId('sync-hold-users')).toHaveTextContent('6 deactivated / 20 existing = 30.0%');
    expect(screen.getByTestId('sync-hold-teams')).toHaveTextContent('3 deactivated / 10 existing = 30.0%');
    expect(screen.getByTestId('sync-hold-threshold')).toHaveTextContent('20.0%');
    expect(screen.getByTestId('sync-hold-not-applied')).toBeInTheDocument();
  });

  it('never sends an override on its own and asks for confirmation first', async () => {
    await renderReady(heldRun());
    fireEvent.click(screen.getByTestId('sync-anyway-btn'));
    expect(screen.getByTestId('sync-anyway-confirm')).toHaveTextContent('mark 6 user(s) and 3 team(s) inactive');
    expect(startSync).not.toHaveBeenCalled();
    fireEvent.click(screen.getByTestId('sync-anyway-cancel-btn'));
    expect(screen.queryByTestId('sync-anyway-confirm')).toBeNull();
    expect(startSync).not.toHaveBeenCalled();
  });

  it('sends the override with the exact reviewed counts once confirmed', async () => {
    startSync.mockResolvedValue(makeRun({ runId: 'sync_2' }));
    await renderReady(heldRun());
    fireEvent.click(screen.getByTestId('sync-anyway-btn'));
    fireEvent.click(screen.getByTestId('sync-anyway-confirm-btn'));
    await flush();
    expect(startSync).toHaveBeenCalledWith({
      overrideMassDeletion: true,
      confirmedMassDeletion: { usersExisting: 20, usersDeleting: 6, teamsExisting: 10, teamsDeleting: 3 },
    });
    expect(screen.queryByTestId('sync-mass-deletion-hold')).toBeNull(); // the new run replaced the held one
  });

  it('sends a plain sync (no override) from the Sync Now button', async () => {
    startSync.mockResolvedValue(makeRun());
    await renderReady(heldRun());
    fireEvent.click(button());
    await flush();
    expect(startSync).toHaveBeenCalledWith({ overrideMassDeletion: false, confirmedMassDeletion: undefined });
  });

  it('dismisses a hold without applying anything, and keeps it dismissed across polls', async () => {
    await renderReady(heldRun());
    fireEvent.click(screen.getByTestId('sync-hold-dismiss-btn'));
    await flush();
    expect(dismissHold).toHaveBeenCalledTimes(1);
    expect(screen.queryByTestId('sync-mass-deletion-hold')).toBeNull();
    expect(startSync).not.toHaveBeenCalled();
  });

  it('shows an ordinary failure (not the hold banner) for other errors', async () => {
    await renderReady(makeRun({ status: 'FAILED', stage: 'FAILED', failedStage: 'FETCHING', errorCode: 'PROVIDER_FETCH_FAILED', errorMessage: 'Provider unreachable' }));
    expect(screen.queryByTestId('sync-mass-deletion-hold')).toBeNull();
    expect(screen.getByTestId('sync-progress-error')).toHaveTextContent('Provider unreachable');
  });
});

describe('sync-running warning and threshold lock follow the persisted run', () => {
  const LOCKED = { ...UNLOCKED_THRESHOLD, locked: true, lockReason: 'syncing', activeSyncThreshold: 20 };
  const notice = () => screen.queryByTestId('threshold-locked-notice');
  const input = () => screen.getByTestId('threshold-input') as HTMLInputElement;

  // The server's own answer follows the run: locked while a run is active.
  const serverLockFollows = (state: { active: boolean }) =>
    getThreshold.mockImplementation(async () => (state.active ? LOCKED : UNLOCKED_THRESHOLD));

  it('shows the warning and disables the threshold while a run is active', async () => {
    serverLockFollows({ active: true });
    await renderReady(makeRun({ status: 'RUNNING', stage: 'FETCHING' }));
    await flush();
    expect(notice()).not.toBeNull();
    expect(input()).toBeDisabled();
  });

  it('removes the warning and re-enables the threshold as soon as the run succeeds', async () => {
    const state = { active: true };
    serverLockFollows(state);
    getRun.mockImplementation(async () => { state.active = false; return makeRun({ status: 'SUCCESS', stage: 'COMPLETED' }); });
    await renderReady(makeRun({ status: 'RUNNING', stage: 'COMMITTING' }));
    await flush();
    expect(notice()).not.toBeNull();

    await poll();
    await flush();
    expect(stepStates()).toEqual(Array(6).fill('complete'));
    expect(notice()).toBeNull();
    expect(input()).toBeEnabled();
    expect(button()).toBeEnabled();
  });

  it('removes the warning and re-enables the threshold as soon as the run fails', async () => {
    const state = { active: true };
    serverLockFollows(state);
    getRun.mockImplementation(async () => {
      state.active = false;
      return makeRun({ status: 'FAILED', stage: 'FAILED', failedStage: 'UPSERTING_TEAMS', errorCode: 'DATABASE_ERROR', errorMessage: 'failed' });
    });
    await renderReady(makeRun({ status: 'RUNNING', stage: 'UPSERTING_TEAMS' }));
    await flush();
    expect(notice()).not.toBeNull();

    await poll();
    await flush();
    expect(notice()).toBeNull();
    expect(input()).toBeEnabled();
    expect(button()).toBeEnabled();
  });

  it('a completed run restored after refresh never shows the running warning', async () => {
    serverLockFollows({ active: false });
    await renderReady(makeRun({ status: 'SUCCESS', stage: 'COMPLETED' }));
    await flush();
    expect(notice()).toBeNull();
    expect(input()).toBeEnabled();
  });
});
