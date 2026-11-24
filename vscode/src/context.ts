import { languages, window } from 'vscode';

import { CONTEXT_MESSAGE, CONTEXT_REFRESH_MS, PROMPT_MESSAGE } from './swcode.constants';

import type { Disposable, TextEditor, Webview, WebviewView } from 'vscode';

import type { IdeContext, IdeDiagnostic, IdeSelection } from '../../web/src/types/ide-context.types';

/** What the extension posts into the webview, and the bundle reads off the window's messages. */
export type HostOutgoing =
  | { type: typeof CONTEXT_MESSAGE; context: IdeContext }
  | { type: typeof PROMPT_MESSAGE; text: string; includeProblems?: boolean };

/** The editor's severity, in the words the prompt uses. */
const SEVERITY_NAMES: Record<number, string> = { 0: 'error', 1: 'warning', 2: 'info', 3: 'hint' };

const severityOf = (severity: number): string => SEVERITY_NAMES[severity] ?? 'info';

/** The file's path, in the form the agent reads it: a plain path, not a `file:` address. */
const pathOf = (editor: TextEditor): string => editor.document.uri.fsPath;

const selectionOf = (editor: TextEditor): IdeSelection => ({
  endColumn: editor.selection.end.character + 1,
  endLine: editor.selection.end.line + 1,
  startColumn: editor.selection.start.character + 1,
  startLine: editor.selection.start.line + 1,
  text: editor.document.getText(editor.selection),
});

const diagnosticsOf = (editor: TextEditor): IdeDiagnostic[] =>
  languages.getDiagnostics(editor.document.uri).map((diagnostic) => ({
    code: diagnostic.code?.toString() ?? '',
    column: diagnostic.range.start.character + 1,
    line: diagnostic.range.start.line + 1,
    message: diagnostic.message,
    severity: severityOf(diagnostic.severity),
    source: diagnostic.source ?? '',
  }));

/** What the editor is looking at: its file, the part selected, and the problems found in it. */
export function currentContext(): IdeContext | undefined {
  const editor = window.activeTextEditor;
  if (editor === undefined) {
    return undefined;
  }
  return {
    diagnostics: diagnosticsOf(editor),
    language: editor.document.languageId,
    path: pathOf(editor),
    selection: selectionOf(editor),
  };
}

function post(webview: Webview, message: HostOutgoing): void {
  void webview.postMessage(message);
}

/**
 * The IDE's context, kept in step with the editor and handed to the webview: the active file with its selection and
 * its problems, pushed as they change and when the panel is shown, and a prompt asked for from the editor.
 */
export function startContext(view: WebviewView): Disposable {
  const push = (): void => {
    const context = currentContext();
    if (context !== undefined) {
      post(view.webview, { context, type: CONTEXT_MESSAGE });
    }
  };
  let refresh: ReturnType<typeof setTimeout> | undefined;
  const debounced = (): void => {
    clearTimeout(refresh);
    refresh = setTimeout(push, CONTEXT_REFRESH_MS);
  };
  const selectionChanges = window.onDidChangeTextEditorSelection((event) => {
    if (event.textEditor === window.activeTextEditor) {
      debounced();
    }
  });
  const diagnosticChanges = languages.onDidChangeDiagnostics(debounced);
  const visibility = view.onDidChangeVisibility(() => {
    if (view.visible) {
      push();
    }
  });
  push();
  return {
    dispose: () => {
      clearTimeout(refresh);
      selectionChanges.dispose();
      diagnosticChanges.dispose();
      visibility.dispose();
    },
  };
}

/**
 * Sends a prompt to the panel, with the editor's context in front of it (the editor's commands).
 * `includeProblems` decides for this prompt; left out, the composer keeps the state of its own toggle.
 */
export function ask(webview: Webview, text: string, includeProblems?: boolean): void {
  post(webview, { includeProblems, text, type: PROMPT_MESSAGE });
}
