import { describe, expect, it } from 'vitest';

import { currentPlan, unfinishedSteps } from '../plan/plan-state.js';

import type { PlanStep, SessionEvent } from '../protocol/session.schema.js';

const AT = '2026-10-24T12:00:00.000Z';

const turn = (id: string): SessionEvent => ({ at: AT, id, kind: 'turnStarted' });

const planned = (callId: string, steps: PlanStep[]): SessionEvent => ({
  at: AT,
  callId,
  display: { kind: 'plan', steps },
  kind: 'toolFinished',
  ok: true,
  output: '',
});

const FIRST: PlanStep[] = [
  { content: 'Profile', status: 'in_progress' },
  { content: 'Fix', status: 'pending' },
];
const SECOND: PlanStep[] = [
  { content: 'Profile', status: 'completed' },
  { content: 'Fix', status: 'in_progress' },
];

describe('currentPlan', () => {
  it('is the latest plan of the last turn', () => {
    expect(currentPlan([turn('t1'), planned('c1', FIRST), planned('c2', SECOND)])).toEqual(SECOND);
  });

  it('is none before the agent writes one, nor in a new turn without one', () => {
    expect(currentPlan([turn('t1')])).toEqual([]);
    expect(currentPlan([turn('t1'), planned('c1', FIRST), turn('t2')])).toEqual([]);
  });
});

describe('unfinishedSteps', () => {
  it('keeps the steps pending or in progress', () => {
    expect(unfinishedSteps(SECOND)).toEqual([{ content: 'Fix', status: 'in_progress' }]);
  });
});
