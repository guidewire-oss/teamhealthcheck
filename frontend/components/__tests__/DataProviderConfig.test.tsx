import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

// The component talks to the backend only through these functions, so mocking
// the module keeps the test on the component's own behaviour.
const getSettings = vi.fn();
const sync = vi.fn();
const clearCache = vi.fn();

vi.mock('@/lib/api/admin', () => ({
  getOrganizationProviderSettings: (...a: any[]) => getSettings(...a),
  syncOrganizationProvider: (...a: any[]) => sync(...a),
  clearAdminCache: (...a: any[]) => clearCache(...a),
}));

import DataProviderConfig from '../DataProviderConfig';
import {
  readSyncState,
  writeSyncState,
  setActiveSyncPromise,
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
});

afterEach(() => {
  vi.useRealTimers();
  localStorage.clear();
  setActiveSyncPromise(null);
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
    sync.mockRejectedValue(new Error('Sync held for review: this sync would remove an unusually large share of users or teams.'));
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

    const { unmount } = render(<DataProviderConfig />);
    getSettings.mockResolvedValue(READY);
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

  it('leaves an idle, enabled button on mount when no sync is active (unchanged existing behavior)', async () => {
    await renderReady();
    expect(screen.getByTestId('sync-now-btn')).toBeEnabled();
    expect(readSyncState()).toBeNull();
  });
});
