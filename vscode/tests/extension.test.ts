import { beforeEach, describe, expect, it, vi } from 'vitest';

import { activate } from '../src/extension';

import {
  ADD_TESTS_COMMAND,
  ADD_TESTS_PROMPT,
  ASK_COMMAND,
  ASK_PROMPT,
  EXPLAIN_COMMAND,
  EXPLAIN_PROMPT,
  FIX_PROBLEMS_COMMAND,
  FIX_PROBLEMS_PROMPT,
  REFACTOR_COMMAND,
  REFACTOR_PROMPT,
  VIEW_ID,
} from '../src/swcode.constants';

import type { ExtensionContext, WebviewView } from 'vscode';

/** What VS Code is given and what the extension is handed back: the mock records the commands and the panel. */
const host = vi.hoisted<{
  commands: Map<string, (text?: string, includeProblems?: boolean) => void>;
  providers: Map<string, { resolveWebviewView: (view: WebviewView) => void }>;
}>(() => ({ commands: new Map(), providers: new Map() }));

vi.mock('vscode', () => ({
  // The theme's names, in the order the editor counts them, so the bundle's page is drawn with the right colours.
  ColorThemeKind: { Dark: 2, HighContrast: 3, HighContrastLight: 4, Light: 1 },
  commands: {
    registerCommand: (id: string, handler: (text?: string, includeProblems?: boolean) => void) =>
      host.commands.set(id, handler),
  },
  Disposable: {
    from: (items: { dispose: () => void }[]) => ({ dispose: () => items.forEach((item) => item.dispose()) }),
  },
  languages: { getDiagnostics: () => [], onDidChangeDiagnostics: vi.fn() },
  StatusBarAlignment: { Left: 1 },
  window: {
    activeColorTheme: { kind: 2 },
    activeTextEditor: undefined,
    createStatusBarItem: () => ({}),
    onDidChangeActiveColorTheme: vi.fn(),
    onDidChangeTextEditorSelection: vi.fn(),
    registerWebviewViewProvider: (id: string, provider: { resolveWebviewView: (view: WebviewView) => void }) =>
      host.providers.set(id, provider),
  },
}));

/** The panel, as VS Code hands it: the mock keeps the fields the extension reads and writes. */
const view = {
  onDidChangeVisibility: vi.fn(),
  show: vi.fn(),
  visible: false,
  webview: { onDidReceiveMessage: vi.fn(), options: {}, postMessage: vi.fn() },
};

/** Activates the extension and resolves the panel, so the commands have a webview to post to. */
function activated(): void {
  activate({ subscriptions: [] } as unknown as ExtensionContext);
  host.providers.get(VIEW_ID)?.resolveWebviewView(view as unknown as WebviewView);
}

beforeEach(() => {
  host.commands.clear();
  host.providers.clear();
  vi.clearAllMocks();
  activated();
});

describe('the commands the palette and the editor’s menu offer', () => {
  it('registers the four refactor commands beside “Ask swcode”', () => {
    expect([...host.commands.keys()]).toEqual([
      ASK_COMMAND,
      REFACTOR_COMMAND,
      EXPLAIN_COMMAND,
      FIX_PROBLEMS_COMMAND,
      ADD_TESTS_COMMAND,
    ]);
  });

  it('sends its own prompt, and shows the panel', () => {
    host.commands.get(REFACTOR_COMMAND)?.();
    expect(view.webview.postMessage).toHaveBeenLastCalledWith({ text: REFACTOR_PROMPT, type: 'prompt' });
    expect(view.show).toHaveBeenCalledWith(true);

    host.commands.get(EXPLAIN_COMMAND)?.();
    expect(view.webview.postMessage).toHaveBeenLastCalledWith({ text: EXPLAIN_PROMPT, type: 'prompt' });

    host.commands.get(ADD_TESTS_COMMAND)?.();
    expect(view.webview.postMessage).toHaveBeenLastCalledWith({ text: ADD_TESTS_PROMPT, type: 'prompt' });

    host.commands.get(ASK_COMMAND)?.();
    expect(view.webview.postMessage).toHaveBeenLastCalledWith({ text: ASK_PROMPT, type: 'prompt' });
  });

  it('asks for the problems with “Fix problems”, and leaves the choice to the person for the others', () => {
    host.commands.get(FIX_PROBLEMS_COMMAND)?.();
    expect(view.webview.postMessage).toHaveBeenLastCalledWith({
      includeProblems: true,
      text: FIX_PROBLEMS_PROMPT,
      type: 'prompt',
    });
  });
});
