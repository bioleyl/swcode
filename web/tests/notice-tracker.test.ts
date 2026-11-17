import { describe, expect, it } from 'vitest';

import { NoticeTracker } from '../src/lib/notice-tracker';

import type { ServerMessage } from '../../shared/protocol/messages.schema';

const sessions = (title: string, subagents = 0): ServerMessage => ({
  sessions: [{ id: 's-1', subagents, title } as never],
  type: 'sessions',
});

const event = (kind: string, fields: Record<string, unknown>): ServerMessage =>
  ({
    event: { at: '2026-11-17T10:00:00Z', id: 'e-1', kind, ...fields },
    sessionId: 's-1',
    type: 'sessionEvent',
  }) as never;

describe('what the phone app is told about a session', () => {
  it('names the session by its title, and says what it asks first', () => {
    const tracker = new NoticeTracker();
    tracker.take(sessions('Fix the parser'));
    const notice = tracker.take(event('permissionRequested', { callId: 'call-0', requestId: 'r-1' }));
    expect(notice?.title).toBe('Fix the parser');
    expect(notice?.kind).toBe('needsYou');
  });

  it('names the tool call an approval is for', () => {
    const tracker = new NoticeTracker();
    tracker.take(sessions('Fix the parser'));
    tracker.take(event('toolCall', { callId: 'call-0', summary: 'read src/parser.ts' }));
    const notice = tracker.take(event('permissionRequested', { callId: 'call-0', requestId: 'r-1' }));
    expect(notice?.text).toBe('Asks you first: read src/parser.ts');
  });

  it('says the question asked, the password wanted, and the terminal waiting', () => {
    const tracker = new NoticeTracker();
    tracker.take(sessions('Fix the parser'));
    expect(
      tracker.take(
        event('questionAsked', {
          callId: 'c',
          questions: [{ header: 'h', multiSelect: false, options: [], question: 'Which file?' }],
          requestId: 'r',
        })
      )?.text
    ).toBe('Which file?');
    expect(
      tracker.take(event('passwordRequested', { callId: 'c', prompt: '[sudo] password', requestId: 'r' }))?.text
    ).toBe('[sudo] password');
    expect(
      tracker.take(event('terminalWaiting', { callId: 'c', terminalId: 't', title: 'tests' }))?.text
    ).toContain('terminal “tests”');
  });

  it('tells a turn over with its last answer, a failed one with its problem, and a stopped one with nothing', () => {
    const tracker = new NoticeTracker();
    tracker.take(sessions('Fix the parser'));
    tracker.take(event('assistantMessage', { text: 'The parser reads\nboth forms.', toolCalls: [] }));
    expect(tracker.take(event('turnFinished', { outcome: 'done' }))).toEqual({
      kind: 'finished',
      sessionId: 's-1',
      text: 'The parser reads both forms.',
      title: 'Fix the parser',
    });
    expect(
      tracker.take(event('turnFinished', { message: 'llm-server went quiet', outcome: 'failed' }))?.text
    ).toBe('llm-server went quiet');
    expect(tracker.take(event('turnFinished', { outcome: 'failed' }))?.text).toBe('Its turn failed.');
    expect(tracker.take(event('turnFinished', { outcome: 'stopped' }))).toBeUndefined();
  });

  it('keeps quiet while subagents work, and tells once they are gone', () => {
    const tracker = new NoticeTracker();
    tracker.take(sessions('Fix the parser', 2));
    expect(tracker.take(event('turnFinished', { outcome: 'done' }))).toBeUndefined();
    tracker.take(sessions('Fix the parser', 0));
    expect(tracker.take(event('turnFinished', { outcome: 'limit' }))?.kind).toBe('finished');
  });

  it('cuts a long text to what a notification shows, on one line', () => {
    const tracker = new NoticeTracker();
    tracker.take(sessions(''));
    const notice = tracker.take(
      event('passwordRequested', { callId: 'c', prompt: `${'a'.repeat(300)}  b`, requestId: 'r' })
    );
    expect(notice?.title).toBe('A session');
    expect(notice?.text).toHaveLength(160);
    expect(notice?.text.endsWith('…')).toBe(true);
  });
});
