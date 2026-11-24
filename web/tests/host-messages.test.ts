// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest';

import { hostMessageFrom, requestOpenInHost } from '../src/services/host-messages';

const CONTEXT = {
  diagnostics: [{ code: 'x', column: 3, line: 2, message: 'bad', severity: 'error', source: 'eslint' }],
  language: 'go',
  path: '/home/dev/app/main.go',
  selection: { endColumn: 1, endLine: 4, startColumn: 1, startLine: 2, text: 'x := 1' },
};

describe('the host’s messages', () => {
  it('takes the context the extension posts', () => {
    expect(hostMessageFrom({ context: CONTEXT, type: 'context' })).toEqual({
      context: CONTEXT,
      kind: 'context',
    });
  });

  it('takes the prompt the editor asks with', () => {
    expect(hostMessageFrom({ text: 'why this?', type: 'prompt' })).toEqual({ kind: 'prompt', text: 'why this?' });
  });

  it('takes the problems a command asks for with its prompt', () => {
    expect(hostMessageFrom({ includeProblems: true, text: 'fix them', type: 'prompt' })).toEqual({
      includeProblems: true,
      kind: 'prompt',
      text: 'fix them',
    });
    expect(hostMessageFrom({ includeProblems: false, text: 'refactor', type: 'prompt' })).toEqual({
      includeProblems: false,
      kind: 'prompt',
      text: 'refactor',
    });
  });

  it('keeps what a partial context carries and fills the rest', () => {
    expect(hostMessageFrom({ context: { path: '/a.ts' }, type: 'context' })).toEqual({
      context: {
        diagnostics: [],
        language: '',
        path: '/a.ts',
        selection: { endColumn: 1, endLine: 1, startColumn: 1, startLine: 1, text: '' },
      },
      kind: 'context',
    });
  });

  it('ignores the messages it does not know', () => {
    expect(hostMessageFrom({ type: 'other' })).toBeUndefined();
    expect(hostMessageFrom('a string')).toBeUndefined();
    expect(hostMessageFrom(undefined)).toBeUndefined();
  });
});

describe('the open request the bundle posts to the host', () => {
  it('names the file and the range of lines to show', () => {
    const posted = vi.spyOn(window.parent, 'postMessage');
    requestOpenInHost('/home/dev/app/src/main.ts', 12, 14);
    expect(posted).toHaveBeenCalledWith(
      { endLine: 14, path: '/home/dev/app/src/main.ts', startLine: 12, type: 'open' },
      '*'
    );
  });

  it('asks for the whole file when it names no lines', () => {
    const posted = vi.spyOn(window.parent, 'postMessage');
    requestOpenInHost('/home/dev/app/src/main.ts');
    expect(posted).toHaveBeenCalledWith({ path: '/home/dev/app/src/main.ts', type: 'open' }, '*');
  });
});
