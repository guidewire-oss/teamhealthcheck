import { describe, it, expect, vi, beforeEach } from 'vitest';
import { getManagerMemberOverview } from '../health-checks';

vi.mock('@/lib/api/client', async () => {
  const actual = await vi.importActual<typeof import('../client')>('../client');
  return { ...actual, API_BASE_URL: '' };
});

vi.mock('@/lib/auth', () => ({
  authenticatedFetch: vi.fn(),
}));

beforeEach(() => {
  vi.restoreAllMocks();
});

describe('getManagerMemberOverview', () => {
  it('requests the manager-wide endpoint without any filters', async () => {
    const { authenticatedFetch } = await import('@/lib/auth');
    const payload = {
      managerId: 'mgr1',
      teams: [
        {
          teamId: 'team1',
          teamName: 'Team One',
          overallHealth: 2.4,
          submissionCount: 3,
          dimensions: [{ dimensionId: 'mission', avgScore: 2.4, responseCount: 3 }],
        },
      ],
    };
    (authenticatedFetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(payload),
    });

    const result = await getManagerMemberOverview('mgr1');

    expect(authenticatedFetch).toHaveBeenCalledWith(
      '/api/v1/managers/mgr1/dashboard/member-overview',
      expect.any(Object)
    );
    expect(result.teams).toHaveLength(1);
    expect(result.teams[0].teamId).toBe('team1');
  });

  it('appends the teamId query param when provided', async () => {
    const { authenticatedFetch } = await import('@/lib/auth');
    (authenticatedFetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ managerId: 'mgr1', teams: [] }),
    });

    await getManagerMemberOverview('mgr1', 'team1');

    expect(authenticatedFetch).toHaveBeenCalledWith(
      '/api/v1/managers/mgr1/dashboard/member-overview?teamId=team1',
      expect.any(Object)
    );
  });

  it('appends both teamId and assessmentPeriod query params when provided', async () => {
    const { authenticatedFetch } = await import('@/lib/auth');
    (authenticatedFetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ managerId: 'mgr1', teams: [] }),
    });

    await getManagerMemberOverview('mgr1', 'team1', '2024 - 1st Half');

    expect(authenticatedFetch).toHaveBeenCalledWith(
      '/api/v1/managers/mgr1/dashboard/member-overview?teamId=team1&assessmentPeriod=2024+-+1st+Half',
      expect.any(Object)
    );
  });

  it('throws when the response is not ok', async () => {
    const { authenticatedFetch } = await import('@/lib/auth');
    (authenticatedFetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
      json: () => Promise.resolve({}),
    });

    await expect(getManagerMemberOverview('mgr1')).rejects.toThrow();
  });
});
