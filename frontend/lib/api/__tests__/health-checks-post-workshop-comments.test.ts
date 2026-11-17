import { describe, it, expect, vi, beforeEach } from 'vitest';
import { getManagerFinalPostWorkshopComments } from '../health-checks';

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

describe('getManagerFinalPostWorkshopComments', () => {
  it('requests the manager-wide endpoint without a period filter', async () => {
    const { authenticatedFetch } = await import('@/lib/auth');
    const payload = {
      managerId: 'mgr1',
      comments: { team1: [{ teamId: 'team1', sessionId: 's1', dimensionId: 'mission', comment: 'Great', date: '2024-07-15' }] },
    };
    (authenticatedFetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(payload),
    });

    const result = await getManagerFinalPostWorkshopComments('mgr1');

    expect(authenticatedFetch).toHaveBeenCalledWith(
      '/api/v1/managers/mgr1/dashboard/final-post-workshop-comments',
      expect.any(Object)
    );
    expect(result.comments.team1).toHaveLength(1);
  });

  it('appends the assessment period query param when provided', async () => {
    const { authenticatedFetch } = await import('@/lib/auth');
    (authenticatedFetch as ReturnType<typeof vi.fn>).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ managerId: 'mgr1', comments: {} }),
    });

    await getManagerFinalPostWorkshopComments('mgr1', '2024 - 1st Half');

    expect(authenticatedFetch).toHaveBeenCalledWith(
      '/api/v1/managers/mgr1/dashboard/final-post-workshop-comments?assessmentPeriod=2024%20-%201st%20Half',
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

    await expect(getManagerFinalPostWorkshopComments('mgr1')).rejects.toThrow();
  });
});
