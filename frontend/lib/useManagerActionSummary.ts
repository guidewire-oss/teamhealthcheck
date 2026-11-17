import { useEffect, useState } from 'react';
import { listManagerTeamsActionSummary, TeamActionSummary } from '@/lib/api/action-items';

interface ManagerActionSummaryResult {
  summary: TeamActionSummary[];
  loading: boolean;
  error: boolean;
}

// `managerId` records which manager the settled result belongs to; null means
// nothing has settled yet.
interface SettledState {
  managerId: string | null;
  summary: TeamActionSummary[];
  error: boolean;
}

const IDLE: SettledState = { managerId: null, summary: [], error: false };

/**
 * Loads the per-team action item counts for a manager while `enabled` is true.
 *
 * - `loading` is derived (enabled, and no result settled for this manager), so
 *   the very first render already reports loading and never flashes an empty
 *   state before the request starts.
 * - Only the latest request may update state: when the manager changes, the
 *   tab is left, or the component unmounts, an in-flight request is ignored, so
 *   a slow or failed earlier request can never overwrite a newer result.
 * - Leaving the tab (enabled=false) discards the result, so coming back always
 *   reloads instead of showing stale counts.
 */
export function useManagerActionSummary(managerId: string | undefined, enabled: boolean): ManagerActionSummaryResult {
  const [settled, setSettled] = useState<SettledState>(IDLE);

  useEffect(() => {
    if (!managerId || !enabled) {
      setSettled((prev) => (prev.managerId === null ? prev : IDLE));
      return;
    }

    let stale = false;
    listManagerTeamsActionSummary(managerId)
      .then((summary) => {
        if (!stale) setSettled({ managerId, summary, error: false });
      })
      .catch(() => {
        if (!stale) setSettled({ managerId, summary: [], error: true });
      });

    return () => {
      stale = true;
    };
  }, [managerId, enabled]);

  const loading = enabled && !!managerId && settled.managerId !== managerId;
  return { summary: settled.summary, loading, error: settled.error };
}
