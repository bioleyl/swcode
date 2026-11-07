import { describe, expect, it } from 'vitest';

import { daemonLine, problemOf } from '../src/lib/daemon-lines';

describe('what the daemon said when it refused', () => {
  it('takes the words the daemon sent with its refusal', () => {
    expect(problemOf(500, '{"message":"llm-server is not reachable"}')).toBe('llm-server is not reachable');
  });

  it('falls back to the code when the body says nothing the app can show', () => {
    expect(problemOf(502, '<html>Bad Gateway</html>')).toBe('HTTP 502');
    expect(problemOf(503, '')).toBe('HTTP 503');
  });

  it('does not take a word that is not the message', () => {
    expect(problemOf(400, '{"other":"nope"}')).toBe('HTTP 400');
  });
});

describe('what each state of the daemon shows', () => {
  it('keeps a daemon that is up but unhappy apart from one nobody reaches', () => {
    expect(daemonLine({ kind: 'offline' }).text).toBe('The daemon does not answer');
    expect(daemonLine({ kind: 'refused', problem: 'llm-server is not reachable', status: 500 })).toEqual({
      detail: 'The daemon is up but refused the request: look at what it logs.',
      text: 'llm-server is not reachable',
      tone: 'problem',
    });
    expect(daemonLine({ kind: 'badAnswer' })).toEqual({
      detail: 'The daemon answered, but not with its status: look at what it logs.',
      text: 'The daemon answered badly',
      tone: 'problem',
    });
  });
});
