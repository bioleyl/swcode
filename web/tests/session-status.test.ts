import { describe, expect, it } from 'vitest';

import { canDeleteSession, deleteHintOf } from '../src/lib/session-status';

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
