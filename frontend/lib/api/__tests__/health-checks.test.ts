import { describe, it, expect, vi, beforeEach } from 'vitest';
import { checkSurveyEligibility } from '../health-checks';

const apiRequest = vi.fn();
vi.mock('../client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../client')>()),
  apiRequest: (...args: unknown[]) => apiRequest(...args),
}));

describe('checkSurveyEligibility', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    apiRequest.mockResolvedValue({ ok: true, json: async () => ({ eligible: true }) });
  });

  it('asks about the user for an individual survey', async () => {
    await checkSurveyEligibility({ surveyType: 'individual', assessmentPeriod: '2026 H1', userId: 'user-1' });

    const url = new URL(apiRequest.mock.calls[0][0] as string, 'http://localhost');
    expect(url.pathname).toBe('/api/v1/health-checks/eligibility');
    expect(url.searchParams.get('surveyType')).toBe('individual');
    expect(url.searchParams.get('assessmentPeriod')).toBe('2026 H1');
    expect(url.searchParams.get('userId')).toBe('user-1');
    expect(url.searchParams.has('teamId')).toBe(false);
  });

  it('asks about the team for a post-workshop survey', async () => {
    await checkSurveyEligibility({ surveyType: 'post_workshop', assessmentPeriod: '2026 H1', teamId: 'team-1' });

    const url = new URL(apiRequest.mock.calls[0][0] as string, 'http://localhost');
    expect(url.searchParams.get('teamId')).toBe('team-1');
    expect(url.searchParams.has('userId')).toBe(false);
  });

  // The parameter type already rules these out; the runtime check protects callers that are not
  // type-checked, since the server answers a missing scope with a 400.
  it('refuses to send an individual check without a userId', async () => {
    await expect(
      checkSurveyEligibility({ surveyType: 'individual', assessmentPeriod: '2026 H1' } as never),
    ).rejects.toThrow(/userId is required/);
    expect(apiRequest).not.toHaveBeenCalled();
  });

  it('refuses to send a post-workshop check without a teamId', async () => {
    await expect(
      checkSurveyEligibility({ surveyType: 'post_workshop', assessmentPeriod: '2026 H1' } as never),
    ).rejects.toThrow(/teamId is required/);
    expect(apiRequest).not.toHaveBeenCalled();
  });
});
