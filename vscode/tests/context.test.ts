import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ask, currentContext, startContext } from '../src/context';

import type { Diagnostic, TextEditor, Uri, Webview, WebviewView } from 'vscode';

/** The editor and the language server, as VS Code answers: the mock reads and writes these fields. */
const host = vi.hoisted<{
  activeTextEditor: TextEditor | undefined;
  diagnostics: Diagnostic[];
  onDidChangeDiagnostics: ReturnType<typeof vi.fn>;
  onDidChangeTextEditorSelection: ReturnType<typeof vi.fn>;
  onDidChangeVisibility: ReturnType<typeof vi.fn>;
}>(() => ({
  activeTextEditor: undefined,
  diagnostics: [],
  onDidChangeDiagnostics: vi.fn(),
  onDidChangeTextEditorSelection: vi.fn(),
  onDidChangeVisibility: vi.fn(),
}));

vi.mock('vscode', () => ({
  languages: {
    getDiagnostics: () => host.diagnostics,
    onDidChangeDiagnostics: host.onDidChangeDiagnostics,
  },
  window: host,
}));

function editor(path: string, language: string, text: string): TextEditor {
  return {
    document: {
      getText: () => text,
      languageId: language,
      uri: { fsPath: path } as Uri,
    },
    selection: {
      end: { character: 8, line: 2 },
      start: { character: 0, line: 0 },
    },
  } as unknown as TextEditor;
}

const PROBLEM = {
  code: 'no-unused-vars',
  message: "'a' is never read.",
  range: { start: { character: 6, line: 0 }, end: { character: 7, line: 0 } },
  severity: 1,
  source: 'eslint',
} as unknown as Diagnostic;

/** The panel, as VS Code hands it: the mock keeps the fields the extension reads and writes. */
const view = {
  onDidChangeVisibility: vi.fn(),
  show: vi.fn(),
  visible: false,
  webview: { postMessage: vi.fn() },
};

/** The listener VS Code was given when the extension registered for an event. */
const registered = (registration: ReturnType<typeof vi.fn>): ((value: never) => void) =>
  registration.mock.calls[0]?.[0] as (value: never) => void;

beforeEach(() => {
  vi.useFakeTimers();
  vi.clearAllMocks();
  host.diagnostics = [PROBLEM];
  host.activeTextEditor = editor('/home/dev/app/main.ts', 'typescript', 'const a = 1;');
});

describe('the editor’s context', () => {
  it('reads the file, the selection and the problems, 1-based and named', () => {
    expect(currentContext()).toEqual({
      diagnostics: [
        {
          code: 'no-unused-vars',
          column: 7,
          line: 1,
          message: "'a' is never read.",
          severity: 'warning',
          source: 'eslint',
        },
      ],
      language: 'typescript',
      path: '/home/dev/app/main.ts',
      selection: { endColumn: 9, endLine: 3, startColumn: 1, startLine: 1, text: 'const a = 1;' },
    });
  });

  it('gives nothing when no file is open', () => {
    host.activeTextEditor = undefined;
    expect(currentContext()).toBeUndefined();
  });

  it('keeps a problem without a source or a code', () => {
    host.diagnostics = [
      {
        message: 'missing semicolon',
        range: { start: { character: 0, line: 3 } },
        severity: 0,
      } as unknown as Diagnostic,
    ];
    expect(currentContext()?.diagnostics).toEqual([
      { code: '', column: 1, line: 4, message: 'missing semicolon', severity: 'error', source: '' },
    ]);
  });
});

describe('the context pushed to the panel', () => {
  it('pushes it when the panel opens, and once the selection settled', () => {
    startContext(view as unknown as WebviewView);
    expect(view.webview.postMessage).toHaveBeenLastCalledWith({ context: currentContext(), type: 'context' });
    registered(host.onDidChangeTextEditorSelection)({ textEditor: host.activeTextEditor } as never);
    registered(host.onDidChangeTextEditorSelection)({ textEditor: host.activeTextEditor } as never);
    vi.advanceTimersByTime(150);
    expect(view.webview.postMessage).toHaveBeenCalledTimes(2);
  });

  it('ignores a selection of an editor that is not the active one', () => {
    startContext(view as unknown as WebviewView);
    registered(host.onDidChangeTextEditorSelection)({ textEditor: {} } as never);
    vi.advanceTimersByTime(150);
    expect(view.webview.postMessage).toHaveBeenCalledTimes(1);
  });

  it('pushes the problems as the language server answers, and the panel is shown again', () => {
    startContext(view as unknown as WebviewView);
    registered(host.onDidChangeDiagnostics)(undefined as never);
    vi.advanceTimersByTime(150);
    view.visible = true;
    registered(view.onDidChangeVisibility)(undefined as never);
    expect(view.webview.postMessage).toHaveBeenCalledTimes(3);
  });

  it('sends a prompt the editor asked with', () => {
    ask(view.webview as unknown as Webview, 'why is this red?');
    expect(view.webview.postMessage).toHaveBeenLastCalledWith({ text: 'why is this red?', type: 'prompt' });
  });

  it('sends the problems along when the command asks for them', () => {
    ask(view.webview as unknown as Webview, 'fix them', true);
    expect(view.webview.postMessage).toHaveBeenLastCalledWith({
      includeProblems: true,
      text: 'fix them',
      type: 'prompt',
    });
  });
});
