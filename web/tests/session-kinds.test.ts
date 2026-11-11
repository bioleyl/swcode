import { describe, expect, it } from 'vitest';

import { sessionHost, sessionPlace, sessionPlaceName, sessionsOf } from '../src/lib/session-kinds';

import type { SessionKind, SessionSummary } from '../../shared/protocol/session.schema';

const session = (id: string, kind: SessionKind, workspace: string): SessionSummary => ({
  concise: false,
  host: { endpoint: 'me@box', kind: 'ssh' },
  id,
  jobs: [],
  kind,
  mode: 'auto',
  pins: [],
  queued: [],
  queuedNow: false,
  status: 'idle',
  subagents: 0,
  thinking: 'full',
  title: id,
  updatedAt: '2026-11-06T09:00:00.000Z',
  workspace,
});

describe('session kinds', () => {
  const chat = session('c1', 'chat', '');
  const code = session('s1', 'code', '/home/me/shop');

  it('keep the chats apart from the coding sessions, in the order given', () => {
    const later = session('c2', 'chat', '');
    expect(sessionsOf('chat', [later, code, chat]).map((each) => each.id)).toEqual(['c2', 'c1']);
    expect(sessionsOf('code', [later, code, chat]).map((each) => each.id)).toEqual(['s1']);
  });

  it('say a chat works nowhere, and a coding session in its folder on its host', () => {
    expect(sessionPlace(chat)).toBe('Chat');
    expect(sessionHost(chat)).toBe('Chat');
    expect(sessionPlaceName(chat)).toBe('Chat');
    expect(sessionPlaceName(code)).toBe('shop');
    expect(sessionPlace(code)).toContain('/home/me/shop');
    expect(sessionHost(code)).toContain('box');
  });
});
