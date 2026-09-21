import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

// The component talks to the backend only through these functions, so mocking
// the module keeps the test on the component's own behaviour.
const getSettings = vi.fn();
const sync = vi.fn();
const clearCache = vi.fn();
// The threshold card nested in this component fetches on mount; stub it so
// these specs stay about the sync flow and never hit the network.
// The unlocked-default threshold response, shared by the threshold-read and
// hold-dismiss stubs below so this suite's baseline lives in one place.
const UNLOCKED_THRESHOLD = {
  maxDeletePercent: 20,
  source: 'default',
  defaultPercent: 20,
  minPercent: 1,
  maxPercent: 100,
  locked: false,
};
const getThreshold = vi.fn().mockResolvedValue(UNLOCKED_THRESHOLD);
const dismissHold = vi.fn().mockResolvedValue(UNLOCKED_THRESHOLD);

// getMassDeletionHold is deliberately NOT mocked: the component's decision to
// show the hold banner depends on how that helper reads a real error body, so
// the spec exercises the real one.
vi.mock('@/lib/api/admin', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/admin')>();
  return {
    ...actual,
    getOrganizationProviderSettings: (...a: any[]) => getSettings(...a),
    syncOrganizationProvider: (...a: any[]) => sync(...a),
    clearAdminCache: (...a: any[]) => clearCache(...a),
    getOrgSyncDeletionThreshold: (...a: any[]) => getThreshold(...a),
    dismissMassDeletionHold: (...a: any[]) => dismissHold(...a),
  };
});

import DataProviderConfig from '../DataProviderConfig';
import {
  readSyncState,
  writeSyncState,
  setActiveSyncPromise,
  setActiveRequestId,
  SYNC_STALE_TIMEOUT_MS,
} from '@/lib/admin-sync-state';

const READY = {
  provider: 'data-provider',
  baseUrlConfigured: true,
  tokenConfigured: true,
  readyToSync: true,
};

const SYNC_RESULT = {
  status: 'completed',
  teamsSynced: 24,
  usersSynced: 312,
  membershipsSynced: 407,
  membershipsRemoved: 0,
  healthChecksDisabled: 0,
  healthChecksEnabled: 0,
  usersDeleted: 0,
  teamsDeleted: 0,
  actionItemsDeleted: 0,
  usersSkipped: 0,
  skippedUsers: [],
  managerLinksCleared: 0,
  teamLeadsCleared: 0,
  membershipsDiscarded: 0,
  startedAt: '2026-08-31T05:00:00Z',
  completedAt: '2026-08-31T05:00:04Z',
};

// The backend's held-sync body, shaped exactly as the API returns it: a
// standard error envelope plus the aggregate counts. 6 of 20 users (30%) and
// 3 of 10 teams (30%) over a 20% threshold, with an informational membership
// cascade that is NOT a reason for the hold.
const HOLD_REPORT = {
  threshold: 20,
  users: { kind: 'deleted', existing: 20, deleting: 6, percent: 30, threshold: 20, incoming: 14, exceedsThreshold: true, contributesToHold: true },
  teams: { kind: 'deleted', existing: 10, deleting: 3, percent: 30, threshold: 20, incoming: 7, exceedsThreshold: true, contributesToHold: true },
  memberships: { kind: 'cascaded', existing: 50, deleting: 8, percent: 16, threshold: 20, incoming: 42, exceedsThreshold: false, contributesToHold: false },
};

const holdError = (report: any = HOLD_REPORT) =>
  Object.assign(new Error('Synchronization held for review'), {
    name: 'APIRequestError',
    statusCode: 409,
    apiError: {
      error: 'Synchronization held for review',
      message: 'This sync would remove an unusually large share of users or teams.',
      code: 'mass_deletion_hold',
      applied: false,
      massDeletion: report,
    },
  });

// renderReady mounts the component and waits for the initial settings load,
// so specs start from a settled screen rather than the loading state.
const renderReady = async (settings = READY) => {
  getSettings.mockResolvedValue(settings);
  render(<DataProviderConfig />);
  await screen.findByTestId('sync-now-btn');
};

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  // The active-sync handle is module-level (deliberately, so an SPA remount
  // can reattach to the real in-flight request) -- reset it between tests so
  // one spec's in-flight promise can't leak into the next.
  setActiveSyncPromise(null);
  setActiveRequestId(null);
});

afterEach(() => {
  vi.useRealTimers();
  localStorage.clear();
  setActiveSyncPromise(null);
  setActiveRequestId(null);
});

describe('DataProviderConfig — Sync Now button', () => {
  it('renders an enabled, accessible button once the provider is ready', async () => {
    await renderReady();

    const btn = screen.getByRole('button', { name: 'Sync organization data from provider' });
    expect(btn).toBeEnabled();
    expect(btn).toHaveTextContent('Sync Now');
    expect(screen.queryByTestId('provider-not-configured')).not.toBeInTheDocument();
  });

  it('never renders a token entry field', async () => {
    await renderReady();
    expect(screen.queryByTestId('provider-token-input')).not.toBeInTheDocument();
    expect(screen.queryByTestId('save-provider-token-btn')).not.toBeInTheDocument();
  });

  it('calls the sync API exactly once when clicked', async () => {
    const user = userEvent.setup();
    sync.mockResolvedValue(SYNC_RESULT);
    await renderReady();

    await user.click(screen.getByTestId('sync-now-btn'));

    await waitFor(() => expect(sync).toHaveBeenCalledTimes(1));
  });

  it('disables the button and shows a running label while the sync is in flight', async () => {
    const user = userEvent.setup();
    let release: (v: unknown) => void = () => {};
    sync.mockImplementation(() => new Promise((resolve) => { release = resolve; }));
    await renderReady();

    await user.click(screen.getByTestId('sync-now-btn'));

    const btn = screen.getByTestId('sync-now-btn');
    await waitFor(() => expect(btn).toBeDisabled());
    expect(btn).toHaveTextContent('Syncing...');
    expect(btn).toHaveAttribute('aria-busy', 'true');

    release(SYNC_RESULT);
    await waitFor(() => expect(btn).toHaveTextContent('Sync Now'));
  });

  it('does not fire a second request when clicked again mid-flight', async () => {
    const user = userEvent.setup();
    let release: (v: unknown) => void = () => {};
    sync.mockImplementation(() => new Promise((resolve) => { release = resolve; }));
    await renderReady();

    const btn = screen.getByTestId('sync-now-btn');
    await user.click(btn);
    await waitFor(() => expect(sync).toHaveBeenCalledTimes(1));

    // A disabled button swallows pointer events, so drive the handler directly
    // to prove the component's own re-entry guard holds too.
    btn.click();
    await user.click(btn, { pointerEventsCheck: 0 });

    expect(sync).toHaveBeenCalledTimes(1);

    release(SYNC_RESULT);
    await waitFor(() => expect(btn).toBeEnabled());
  });

  it('shows the synchronized counts on success and refreshes cached admin data', async () => {
    const user = userEvent.setup();
    sync.mockResolvedValue(SYNC_RESULT);
    await renderReady();

    await user.click(screen.getByTestId('sync-now-btn'));

    const result = await screen.findByTestId('sync-result');
    expect(result).toHaveTextContent('312 users');
    expect(result).toHaveTextContent('24 teams');
    expect(result).toHaveTextContent('407 team memberships');
    expect(clearCache).toHaveBeenCalledTimes(1);
  });

  it('shows deletion counts when the provider no longer reports a user or team', async () => {
    const user = userEvent.setup();
    sync.mockResolvedValue({ ...SYNC_RESULT, usersDeleted: 3, teamsDeleted: 1 });
    await renderReady();

    await user.click(screen.getByTestId('sync-now-btn'));

    expect(await screen.findByTestId('sync-deleted')).toHaveTextContent('3 user(s) and 1 team(s) removed');
  });

  it('shows an action-items-deleted notice when a deletion cascades into action items', async () => {
    const user = userEvent.setup();
    sync.mockResolvedValue({ ...SYNC_RESULT, actionItemsDeleted: 3 });
    await renderReady();

    await user.click(screen.getByTestId('sync-now-btn'));

    expect(await screen.findByTestId('sync-action-items-deleted')).toHaveTextContent(
      '3 action item(s) removed'
    );
  });

  it('reports teams whose health checks were switched off, without ever mentioning skipped users', async () => {
    const user = userEvent.setup();
    sync.mockResolvedValue({
      ...SYNC_RESULT,
      usersSkipped: 3,
      skippedUsers: [{ userId: 'u1', username: 'exec', reason: 'unknown_hierarchy_level' }],
      healthChecksDisabled: 24,
    });
    await renderReady();

    await user.click(screen.getByTestId('sync-now-btn'));

    expect(screen.getByTestId('sync-health-check-warning')).toHaveTextContent(
      'Health checks switched off for 24 team(s)'
    );
    // Skipped-user counts (including anyone the provider excludes by hierarchy level)
    // are intentionally never surfaced in this UI.
    expect(screen.queryByTestId('sync-skipped')).not.toBeInTheDocument();
  });

  it('shows a safe message on failure and leaves the button usable', async () => {
    const user = userEvent.setup();
    sync.mockRejectedValue(new Error('a synchronization is already in progress'));
    await renderReady();

    await user.click(screen.getByTestId('sync-now-btn'));

    const err = await screen.findByTestId('provider-error');
    expect(err).toHaveTextContent('a synchronization is already in progress');
    expect(screen.queryByTestId('sync-result')).not.toBeInTheDocument();
    expect(screen.getByTestId('sync-now-btn')).toBeEnabled();
  });

  it('shows a distinct mass-deletion-hold message and leaves the button usable', async () => {
    const user = userEvent.setup();
    sync.mockRejectedValue(holdError());
    await renderReady();

    await user.click(screen.getByTestId('sync-now-btn'));

    const holdBanner = await screen.findByTestId('sync-mass-deletion-hold');
    expect(holdBanner).toHaveTextContent('held for review');
    expect(screen.getByTestId('sync-now-btn')).toBeEnabled();
  });

  it('disables the button and explains why when the provider is not ready', async () => {
    await renderReady({
      ...READY,
      baseUrlConfigured: false,
      tokenConfigured: false,
      readyToSync: false,
    });

    expect(screen.getByTestId('sync-now-btn')).toBeDisabled();
    const banner = screen.getByTestId('provider-not-configured');
    expect(banner).toHaveTextContent('DATA_PROVIDER_BASE_URL');
    expect(banner).toHaveTextContent('DATA_PROVIDER_API_TOKEN');
  });
});

describe('DataProviderConfig — persisted sync state (reload, tab switch, navigation)', () => {
  it('persists an in-progress record to localStorage before/at the same time the request starts', async () => {
    const user = userEvent.setup();
    let release: (v: unknown) => void = () => {};
    sync.mockImplementation(() => new Promise((resolve) => { release = resolve; }));
    await renderReady();

    await user.click(screen.getByTestId('sync-now-btn'));

    await waitFor(() => {
      const state = readSyncState();
      expect(state?.status).toBe('in_progress');
    });

    release(SYNC_RESULT);
    await waitFor(() => expect(screen.getByTestId('sync-now-btn')).toBeEnabled());
  });

  it('clears the persisted state and re-enables the button on success', async () => {
    const user = userEvent.setup();
    sync.mockResolvedValue(SYNC_RESULT);
    await renderReady();

    await user.click(screen.getByTestId('sync-now-btn'));
    await screen.findByTestId('sync-result');

    expect(readSyncState()).toBeNull();
    expect(screen.getByTestId('sync-now-btn')).toBeEnabled();
  });

  it('clears the persisted state and re-enables the button on failure', async () => {
    const user = userEvent.setup();
    sync.mockRejectedValue(new Error('Synchronization failed'));
    await renderReady();

    await user.click(screen.getByTestId('sync-now-btn'));
    await screen.findByTestId('provider-error');

    expect(readSyncState()).toBeNull();
    expect(screen.getByTestId('sync-now-btn')).toBeEnabled();
  });

  it('restores a disabled, syncing button on remount (refresh) when a fresh in-progress state is persisted', async () => {
    writeSyncState('in_progress', Date.now());
    // No live promise survives a real reload, and the sync API must not be
    // called again just because a valid in-progress record exists.
    getSettings.mockResolvedValue(READY);

    render(<DataProviderConfig />);
    await screen.findByTestId('sync-now-btn');

    expect(screen.getByTestId('sync-now-btn')).toBeDisabled();
    expect(screen.getByTestId('sync-now-btn')).toHaveTextContent('Syncing...');
    expect(sync).not.toHaveBeenCalled();
  });

  it('discards a stale in-progress state on remount and re-enables the button without calling the API', async () => {
    writeSyncState('in_progress', Date.now() - (SYNC_STALE_TIMEOUT_MS + 1000));
    getSettings.mockResolvedValue(READY);

    render(<DataProviderConfig />);
    await screen.findByTestId('sync-now-btn');

    expect(screen.getByTestId('sync-now-btn')).toBeEnabled();
    expect(sync).not.toHaveBeenCalled();
    expect(readSyncState()).toBeNull();
  });

  it('self-clears a valid-but-unresolved in-progress state once the stale timeout elapses', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    writeSyncState('in_progress', Date.now());
    getSettings.mockResolvedValue(READY);

    render(<DataProviderConfig />);
    await waitFor(() => expect(screen.getByTestId('sync-now-btn')).toBeDisabled());

    await act(async () => {
      await vi.advanceTimersByTimeAsync(SYNC_STALE_TIMEOUT_MS + 100);
    });

    expect(screen.getByTestId('sync-now-btn')).toBeEnabled();
    expect(readSyncState()).toBeNull();
  });

  it('reattaches to the real in-flight request on an SPA remount (tab switch/navigation) instead of starting a duplicate', async () => {
    const user = userEvent.setup();
    let release: (v: unknown) => void = () => {};
    sync.mockImplementation(() => new Promise((resolve) => { release = resolve; }));

    getSettings.mockResolvedValue(READY);
    const { unmount } = render(<DataProviderConfig />);
    await screen.findByTestId('sync-now-btn');
    await user.click(screen.getByTestId('sync-now-btn'));
    await waitFor(() => expect(sync).toHaveBeenCalledTimes(1));

    // Simulate switching to the Hierarchy tab and back: the component
    // unmounts while the request is still pending, then remounts.
    unmount();
    render(<DataProviderConfig />);
    await screen.findByTestId('sync-now-btn');

    expect(screen.getByTestId('sync-now-btn')).toBeDisabled();
    expect(sync).toHaveBeenCalledTimes(1); // still just the one original call

    release(SYNC_RESULT);
    await waitFor(() => expect(screen.getByTestId('sync-now-btn')).toBeEnabled());
    expect(await screen.findByTestId('sync-result')).toBeInTheDocument();
    expect(readSyncState()).toBeNull();
  });

  it('does not treat an unmount/remount as a failure while the original request is still pending', async () => {
    let release: (v: unknown) => void = () => {};
    sync.mockImplementation(() => new Promise((resolve) => { release = resolve; }));
    getSettings.mockResolvedValue(READY);

    const { unmount } = render(<DataProviderConfig />);
    await screen.findByTestId('sync-now-btn');
    await userEvent.setup().click(screen.getByTestId('sync-now-btn'));
    await waitFor(() => expect(sync).toHaveBeenCalledTimes(1));

    unmount();
    render(<DataProviderConfig />);
    await screen.findByTestId('sync-now-btn');

    expect(screen.queryByTestId('provider-error')).not.toBeInTheDocument();

    release(SYNC_RESULT);
    await waitFor(() => expect(screen.getByTestId('sync-now-btn')).toBeEnabled());
  });

  it('surfaces the completion banner on remount when the sync finished entirely while unmounted', async () => {
    let release: (v: unknown) => void = () => {};
    sync.mockImplementation(() => new Promise((resolve) => { release = resolve; }));
    getSettings.mockResolvedValue(READY);

    const { unmount } = render(<DataProviderConfig />);
    await screen.findByTestId('sync-now-btn');
    await userEvent.setup().click(screen.getByTestId('sync-now-btn'));
    await waitFor(() => expect(sync).toHaveBeenCalledTimes(1));

    unmount();

    // The sync completes while nothing is mounted to observe it, then the
    // admin returns to the tab afterward -- not while it is still pending.
    await act(async () => {
      release(SYNC_RESULT);
      await Promise.resolve();
      await Promise.resolve();
    });

    render(<DataProviderConfig />);
    await screen.findByTestId('sync-now-btn');

    expect(await screen.findByTestId('sync-result')).toBeInTheDocument();
  });

  it('leaves an idle, enabled button on mount when no sync is active (unchanged existing behavior)', async () => {
    await renderReady();
    expect(screen.getByTestId('sync-now-btn')).toBeEnabled();
    expect(readSyncState()).toBeNull();
  });

  // The browser only fires `storage` in tabs OTHER than the one that made the
  // change, so a manually dispatched StorageEvent here is exactly what a
  // second tab -- mounted before this one started (or finished) a sync --
  // would receive. Without reacting to it, that tab would keep showing an
  // enabled button and could clobber this tab's in-progress record by
  // clicking Sync Now itself.
  it('disables the button in real time when another tab starts a sync, without a remount', async () => {
    await renderReady();
    expect(screen.getByTestId('sync-now-btn')).toBeEnabled();

    const otherTabRequestId = writeSyncState('in_progress');
    act(() => {
      window.dispatchEvent(
        new StorageEvent('storage', {
          key: 'adminSyncState',
          newValue: JSON.stringify(readSyncState()),
        })
      );
    });

    await waitFor(() => expect(screen.getByTestId('sync-now-btn')).toBeDisabled());
    expect(sync).not.toHaveBeenCalled();

    // The other tab finishes and clears its own record.
    const { clearSyncState } = await import('@/lib/admin-sync-state');
    clearSyncState(otherTabRequestId);
    act(() => {
      window.dispatchEvent(new StorageEvent('storage', { key: 'adminSyncState', newValue: null }));
    });

    await waitFor(() => expect(screen.getByTestId('sync-now-btn')).toBeEnabled());
  });

  it("does not let this tab's stale-timeout clear erase a newer record another tab has since written", async () => {
    // This tab discovers an in-progress record left over from a reload and
    // starts timing it out.
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const staleOwnerRequestId = writeSyncState('in_progress', Date.now());
    getSettings.mockResolvedValue(READY);

    render(<DataProviderConfig />);
    await waitFor(() => expect(screen.getByTestId('sync-now-btn')).toBeDisabled());

    // Before the timeout fires, a different tab starts its own, genuinely
    // fresh sync -- taking over the shared slot with a new requestId.
    const { clearSyncState } = await import('@/lib/admin-sync-state');
    const freshRequestId = writeSyncState('in_progress', Date.now());
    expect(freshRequestId).not.toBe(staleOwnerRequestId);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(SYNC_STALE_TIMEOUT_MS + 100);
    });

    // The old record's timeout fired, but clearSyncState was scoped to the
    // requestId it originally read, so it must not have erased the newer one.
    const state = readSyncState();
    expect(state).not.toBeNull();
    expect(state?.requestId).toBe(freshRequestId);
    clearSyncState(freshRequestId);
  });
});

describe('DataProviderConfig — mass-deletion hold and Sync Anyway override', () => {
  it('shows the calculated users, teams, memberships and threshold in a red warning', async () => {
    const user = userEvent.setup();
    sync.mockRejectedValue(holdError());
    await renderReady();

    await user.click(screen.getByTestId('sync-now-btn'));

    const banner = await screen.findByTestId('sync-mass-deletion-hold');
    expect(banner).toHaveAttribute('role', 'alert');
    expect(banner.className).toMatch(/border-red/);

    expect(screen.getByTestId('sync-hold-users')).toHaveTextContent('6 deleted / 20 existing = 30.0%');
    expect(screen.getByTestId('sync-hold-teams')).toHaveTextContent('3 deleted / 10 existing = 30.0%');
    // Labelled by what the backend says it is: a cascade, not a deletion.
    expect(screen.getByTestId('sync-hold-memberships')).toHaveTextContent('8 cascaded / 50 existing = 16.0%');
    expect(screen.getByTestId('sync-hold-threshold')).toHaveTextContent('20.0%');
    // The diagnostic that explains a surprising count: what the provider sent.
    expect(screen.getByTestId('sync-hold-users')).toHaveTextContent('provider sent 14');
    expect(screen.getByTestId('sync-hold-teams')).toHaveTextContent('provider sent 7');
    expect(screen.getByTestId('sync-hold-incoming-hint')).toHaveTextContent("provider's snapshot is missing those records");
    expect(screen.getByTestId('sync-hold-not-applied')).toHaveTextContent('has not been applied');
    expect(screen.queryByTestId('sync-result')).not.toBeInTheDocument();
  });

  it('omits the membership line when the backend did not measure one', async () => {
    const user = userEvent.setup();
    const { memberships, ...withoutMemberships } = HOLD_REPORT;
    sync.mockRejectedValue(holdError(withoutMemberships));
    await renderReady();

    await user.click(screen.getByTestId('sync-now-btn'));

    await screen.findByTestId('sync-hold-users');
    expect(screen.queryByTestId('sync-hold-memberships')).not.toBeInTheDocument();
  });

  it('never sends an override on its own', async () => {
    const user = userEvent.setup();
    sync.mockRejectedValue(holdError());
    await renderReady();

    await user.click(screen.getByTestId('sync-now-btn'));
    await screen.findByTestId('sync-mass-deletion-hold');

    expect(sync).toHaveBeenCalledTimes(1);
    expect(sync).toHaveBeenCalledWith({ overrideMassDeletion: false });
    // Sync Anyway is offered, but nothing is retried until it is used.
    expect(screen.getByTestId('sync-anyway-btn')).toBeInTheDocument();
    expect(screen.queryByTestId('sync-anyway-confirm-btn')).not.toBeInTheDocument();
  });

  it('asks for confirmation before sending the override', async () => {
    const user = userEvent.setup();
    sync.mockRejectedValue(holdError());
    await renderReady();

    await user.click(screen.getByTestId('sync-now-btn'));
    await user.click(await screen.findByTestId('sync-anyway-btn'));

    const confirm = screen.getByTestId('sync-anyway-confirm');
    expect(confirm).toHaveTextContent('6 user(s)');
    expect(confirm).toHaveTextContent('3 team(s)');
    // Still only the original request: revealing the confirmation sends nothing.
    expect(sync).toHaveBeenCalledTimes(1);
  });

  it('abandons the override when the admin cancels', async () => {
    const user = userEvent.setup();
    sync.mockRejectedValue(holdError());
    await renderReady();

    await user.click(screen.getByTestId('sync-now-btn'));
    await user.click(await screen.findByTestId('sync-anyway-btn'));
    await user.click(screen.getByTestId('sync-anyway-cancel-btn'));

    expect(screen.queryByTestId('sync-anyway-confirm')).not.toBeInTheDocument();
    expect(screen.getByTestId('sync-mass-deletion-hold')).toBeInTheDocument();
    expect(sync).toHaveBeenCalledTimes(1);
  });

  it('sends the one-time override and shows the result once confirmed', async () => {
    const user = userEvent.setup();
    sync.mockRejectedValueOnce(holdError());
    sync.mockResolvedValueOnce({
      ...SYNC_RESULT,
      usersDeleted: 6,
      teamsDeleted: 3,
      massDeletionOverridden: true,
      massDeletion: HOLD_REPORT,
    });
    await renderReady();

    await user.click(screen.getByTestId('sync-now-btn'));
    await user.click(await screen.findByTestId('sync-anyway-btn'));
    await user.click(screen.getByTestId('sync-anyway-confirm-btn'));

    const result = await screen.findByTestId('sync-result');
    expect(result).toHaveTextContent('Sync complete');
    expect(screen.getByTestId('sync-override-applied')).toBeInTheDocument();
    expect(screen.getByTestId('sync-deleted')).toHaveTextContent('6 user(s) and 3 team(s) removed');

    expect(sync).toHaveBeenNthCalledWith(2, {
      overrideMassDeletion: true,
      confirmedMassDeletion: { usersExisting: 20, usersDeleting: 6, teamsExisting: 10, teamsDeleting: 3 },
    });
    // The hold is resolved, so the warning and its buttons are gone.
    expect(screen.queryByTestId('sync-mass-deletion-hold')).not.toBeInTheDocument();
    expect(clearCache).toHaveBeenCalled();
  });

  it('shows the error when the overridden sync itself fails validation', async () => {
    const user = userEvent.setup();
    sync.mockRejectedValueOnce(holdError());
    sync.mockRejectedValueOnce(new Error('Provider returned data that failed contract validation'));
    await renderReady();

    await user.click(screen.getByTestId('sync-now-btn'));
    await user.click(await screen.findByTestId('sync-anyway-btn'));
    await user.click(screen.getByTestId('sync-anyway-confirm-btn'));

    const err = await screen.findByTestId('provider-error');
    expect(err).toHaveTextContent('failed contract validation');
    expect(screen.queryByTestId('sync-mass-deletion-hold')).not.toBeInTheDocument();
    expect(screen.queryByTestId('sync-result')).not.toBeInTheDocument();
  });

  it('leaves a safe sync completely unchanged', async () => {
    const user = userEvent.setup();
    sync.mockResolvedValue(SYNC_RESULT);
    await renderReady();

    await user.click(screen.getByTestId('sync-now-btn'));

    await screen.findByTestId('sync-result');
    expect(sync).toHaveBeenCalledWith({ overrideMassDeletion: false });
    expect(screen.queryByTestId('sync-mass-deletion-hold')).not.toBeInTheDocument();
    expect(screen.queryByTestId('sync-override-applied')).not.toBeInTheDocument();
  });


  it('makes an under-reported provider payload visible as the cause of the hold', async () => {
    const user = userEvent.setup();
    // The real-world shape: every user came back, almost no teams did. Users
    // read as healthy while teams look catastrophic -- which is a payload
    // problem, not a mass departure, and the banner has to say so.
    sync.mockRejectedValue(
      holdError({
        threshold: 20,
        users: { kind: 'deleted', existing: 240, deleting: 0, percent: 0, threshold: 20, incoming: 240, exceedsThreshold: false, contributesToHold: true },
        teams: { kind: 'deleted', existing: 243, deleting: 237, percent: 97.5, threshold: 20, incoming: 6, exceedsThreshold: true, contributesToHold: true },
      })
    );
    await renderReady();

    await user.click(screen.getByTestId('sync-now-btn'));

    await screen.findByTestId('sync-mass-deletion-hold');
    expect(screen.getByTestId('sync-hold-users')).toHaveTextContent('0 deleted / 240 existing = 0.0%');
    expect(screen.getByTestId('sync-hold-users')).toHaveTextContent('provider sent 240');
    expect(screen.getByTestId('sync-hold-teams')).toHaveTextContent('237 deleted / 243 existing = 97.5%');
    expect(screen.getByTestId('sync-hold-teams')).toHaveTextContent('provider sent 6');
  });

  it('locks the deletion threshold while a hold is unresolved', async () => {
    const user = userEvent.setup();
    sync.mockRejectedValue(holdError());
    await renderReady();

    await user.click(screen.getByTestId('sync-now-btn'));
    await screen.findByTestId('sync-mass-deletion-hold');

    // The hold banner says so, and the threshold card's controls are disabled.
    expect(screen.getByTestId('sync-hold-threshold-locked')).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByTestId('threshold-input')).toBeDisabled()
    );
    expect(screen.getByTestId('threshold-save-btn')).toBeDisabled();
  });

  it('dismisses a hold without applying it, which unlocks the threshold', async () => {
    const user = userEvent.setup();
    sync.mockRejectedValue(holdError());
    await renderReady();

    await user.click(screen.getByTestId('sync-now-btn'));
    await screen.findByTestId('sync-mass-deletion-hold');

    const syncCallsBefore = sync.mock.calls.length;
    await user.click(screen.getByTestId('sync-hold-dismiss-btn'));

    await waitFor(() =>
      expect(screen.queryByTestId('sync-mass-deletion-hold')).not.toBeInTheDocument()
    );
    expect(dismissHold).toHaveBeenCalledTimes(1);
    // Dismissing resolves the review; it never applies the sync.
    expect(sync).toHaveBeenCalledTimes(syncCallsBefore);
    await waitFor(() => expect(screen.getByTestId('threshold-input')).not.toBeDisabled());
  });
});
