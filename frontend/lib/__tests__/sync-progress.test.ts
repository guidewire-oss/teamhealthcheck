import { describe, it, expect } from 'vitest';
import { stepStates, segmentStates, describeRun, isActive, SYNC_STEPS } from '../sync-progress';

const run = (status: any, stage: string, failedStage?: string) => ({ status, stage, failedStage });

describe('stepStates', () => {
  it('has the six required steps in order', () => {
    expect([...SYNC_STEPS]).toEqual([
      'Sync started', 'Data loaded', 'Running sync', 'Queries updated', 'Checking values', 'Success',
    ]);
  });

  it('is entirely pending when there is no run', () => {
    expect(stepStates(null)).toEqual(Array(6).fill('pending'));
  });

  it.each([
    ['QUEUED', 0], ['ACQUIRING_LOCK', 0], ['FETCHING', 1], ['VALIDATING', 1],
    ['UPSERTING_USERS', 2], ['UPSERTING_TEAMS', 2],
    ['SYNCING_MEMBERSHIPS', 3], ['RECONCILING_REMOVALS', 3], ['COMMITTING', 4],
  ])('marks the step for stage %s as active with earlier steps complete', (stage, idx) => {
    const s = stepStates(run('RUNNING', stage));
    s.forEach((state, i) => expect(state).toBe(i < idx ? 'complete' : i === idx ? 'active' : 'pending'));
  });

  it('keeps every step complete on success', () => {
    expect(stepStates(run('SUCCESS', 'COMPLETED'))).toEqual(Array(6).fill('complete'));
  });

  it('turns the failed step red, keeps earlier ones green and later ones incomplete', () => {
    expect(stepStates(run('FAILED', 'FAILED', 'UPSERTING_TEAMS'))).toEqual(
      ['complete', 'complete', 'failed', 'pending', 'pending', 'pending']
    );
  });

  it('attributes a failure with no recorded stage to the first step', () => {
    expect(stepStates(run('FAILED', 'FAILED'))[0]).toBe('failed');
  });

  it('fails at the lock step when the lock was never obtained', () => {
    expect(stepStates(run('FAILED', 'FAILED', 'ACQUIRING_LOCK'))).toEqual(
      ['failed', 'pending', 'pending', 'pending', 'pending', 'pending']
    );
  });
});

describe('segmentStates (left-to-right fill)', () => {
  it('is all gray with no run', () => {
    expect(segmentStates(null)).toEqual(Array(5).fill('gray'));
  });
  it('fills green progressively and animates the segment after the active step', () => {
    expect(segmentStates(run('RUNNING', 'UPSERTING_USERS'))).toEqual(['green', 'green', 'progress', 'gray', 'gray']);
  });
  it('is fully green on success', () => {
    expect(segmentStates(run('SUCCESS', 'COMPLETED'))).toEqual(Array(5).fill('green'));
  });
  it('stops green at the failure and leaves the rest gray', () => {
    expect(segmentStates(run('FAILED', 'FAILED', 'SYNCING_MEMBERSHIPS'))).toEqual(
      ['green', 'green', 'green', 'gray', 'gray']
    );
  });
});

describe('describeRun / isActive', () => {
  it('describes each status distinctly in words', () => {
    expect(describeRun(run('PENDING', 'QUEUED'))).toMatch(/^Pending/);
    expect(describeRun(run('RUNNING', 'FETCHING'))).toBe('Running: Data loaded');
    expect(describeRun(run('SUCCESS', 'COMPLETED'))).toMatch(/^Success/);
    expect(describeRun(run('FAILED', 'FAILED', 'VALIDATING'))).toBe('Failed at step: Data loaded');
  });
  it('treats only PENDING and RUNNING as active', () => {
    expect(isActive(run('PENDING', 'QUEUED'))).toBe(true);
    expect(isActive(run('RUNNING', 'FETCHING'))).toBe(true);
    expect(isActive(run('SUCCESS', 'COMPLETED'))).toBe(false);
    expect(isActive(run('FAILED', 'FAILED'))).toBe(false);
    expect(isActive(null)).toBe(false);
  });
});

import { mergeRun, isTerminal } from '../sync-progress';

describe('mergeRun (stale-response guard)', () => {
  const r = (runId: string, status: any) => ({ runId, status, stage: 'X' });

  it('moves a run forward', () => {
    expect(mergeRun(r('a', 'PENDING'), r('a', 'RUNNING'))?.status).toBe('RUNNING');
    expect(mergeRun(r('a', 'RUNNING'), r('a', 'SUCCESS'))?.status).toBe('SUCCESS');
    expect(mergeRun(r('a', 'RUNNING'), r('a', 'FAILED'))?.status).toBe('FAILED');
  });

  it.each(['PENDING', 'RUNNING'])('never lets a late %s answer change SUCCESS', (late) => {
    const done = r('a', 'SUCCESS');
    expect(mergeRun(done, r('a', late))).toBe(done);
  });

  it.each(['PENDING', 'RUNNING'])('never lets a late %s answer change FAILED', (late) => {
    const done = r('a', 'FAILED');
    expect(mergeRun(done, r('a', late))).toBe(done);
  });

  it('never lets one terminal state overwrite the other', () => {
    const done = r('a', 'SUCCESS');
    expect(mergeRun(done, r('a', 'FAILED'))).toBe(done);
  });

  it('never moves a live run backwards', () => {
    const running = r('a', 'RUNNING');
    expect(mergeRun(running, r('a', 'PENDING'))).toBe(running);
  });

  it('ignores an answer about a different run', () => {
    const current = r('b', 'RUNNING');
    expect(mergeRun(current, r('a', 'SUCCESS'))).toBe(current);
  });

  it('knows which statuses are terminal', () => {
    expect(isTerminal(r('a', 'SUCCESS'))).toBe(true);
    expect(isTerminal(r('a', 'FAILED'))).toBe(true);
    expect(isTerminal(r('a', 'RUNNING'))).toBe(false);
    expect(isTerminal(null)).toBe(false);
  });
});
