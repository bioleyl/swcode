import { describe, expect, it } from 'vitest';

import { canDeleteSession, deleteHintOf, withWorkflowRunStates } from '../src/lib/session-status';

import type { SessionSummary } from '../../shared/protocol/session.schema';

describe('deleting a session', () => {
  it('is possible once its turn is over and its subagents have reported', () => {
    expect(canDeleteSession({ status: 'idle', subagents: 0 })).toBe(true);
    expect(deleteHintOf({ status: 'idle', subagents: 0 })).toBe('Delete this session');
  });

  it('waits while its turn runs, or waits for an answer, as the daemon would refuse', () => {
    expect(canDeleteSession({ status: 'working', subagents: 0 })).toBe(false);
    expect(canDeleteSession({ status: 'waiting', subagents: 0 })).toBe(false);
    expect(deleteHintOf({ status: 'working', subagents: 0 })).toBe('Stop this session before deleting it');
  });

  it('waits while its subagents still work, though its own turn ended', () => {
    expect(canDeleteSession({ status: 'idle', subagents: 1 })).toBe(false);
  });
});

describe('withWorkflowRunStates', () => {
  const listed = (id: string, fields: Partial<SessionSummary> = {}): SessionSummary => ({
    concise: false,
    host: { kind: 'local' },
    id,
    jobs: [],
    kind: 'code',
    mode: 'auto',
    pins: [],
    queued: [],
    queuedNow: false,
    status: 'idle',
    subagents: 0,
    thinking: 'full',
    title: id,
    updatedAt: '2026-10-23T12:00:00.000Z',
    workspace: '/w/app',
    ...fields,
  });

  it('shows a run row as its steps make it: their most pressing state, subagents and last activity', () => {
    const run = listed('run', { updatedAt: '2026-10-23T12:00:00.000Z', workflowRunId: 'r1' });
    const working = listed('work', {
      subagents: 2,
      updatedAt: '2026-10-23T12:05:00.000Z',
      workflowRunId: 'r1',
      workflowStepId: 'work',
    });
    const waiting = listed('review', {
      status: 'waiting',
      updatedAt: '2026-10-23T12:04:00.000Z',
      workflowRunId: 'r1',
      workflowStepId: 'review',
    });
    const other = listed('other');

    const [shown, shownWork, shownWaiting, shownOther] = withWorkflowRunStates([run, working, waiting, other]);

    expect(shown).toMatchObject({
      status: 'waiting',
      subagents: 2,
      updatedAt: '2026-10-23T12:05:00.000Z',
    });
    expect(shownWork).toBe(working);
    expect(shownWaiting).toBe(waiting);
    expect(shownOther).toBe(other);
  });

  it('counts the subagents of a step as the run working', () => {
    const run = listed('run', { workflowRunId: 'r1' });
    const step = listed('work', { subagents: 1, workflowRunId: 'r1', workflowStepId: 'work' });

    expect(withWorkflowRunStates([run, step])[0]?.status).toBe('working');
  });

  it('leaves a run row alone until one of its steps exists', () => {
    const run = listed('run', { workflowRunId: 'r1' });

    expect(withWorkflowRunStates([run])[0]).toBe(run);
  });
});
