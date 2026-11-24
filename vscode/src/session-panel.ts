import { window } from 'vscode';
import type { Disposable, WebviewView, WebviewViewProvider } from 'vscode';

import { ask, startContext } from './context';
import { openInEditor } from './open-file';
import { themeOf } from './lib/theme';
import { frameHtml } from './lib/webview-html';

import type { DaemonTarget } from './types/daemon.types';

/**
 * The side bar's session panel: the shared bundle (ticket 1's `vscode.html`) in a webview. Its socket drives the
 * list, opening a session, streaming, sending, stopping and answering; the bundle posts back what the person asks
 * of the editor (ticket 4 opens a file at its changed range).
 */
export class SessionPanel implements WebviewViewProvider {
  #view: WebviewView | undefined;

  constructor(
    private readonly target: DaemonTarget,
    private readonly subscriptions: Disposable[],
  ) {}

  resolveWebviewView(view: WebviewView): void {
    this.#view = view;
    view.webview.options = { enableScripts: true };
    this.#draw(view);
    // The bundle posts back what the person asks of the editor, so the webview keeps a message channel open.
    this.subscriptions.push(startContext(view), view.webview.onDidReceiveMessage(openInEditor));
  }

  /** Puts the bundle's page back in the frame, with the theme the IDE wears now. */
  refresh(): void {
    if (this.#view) {
      this.#draw(this.#view);
    }
  }

  /** A command's prompt: the panel shows, and the prompt goes with the editor's context and the problems it asks for. */
  ask(text: string, includeProblems?: boolean): void {
    if (this.#view) {
      ask(this.#view.webview, text, includeProblems);
      this.#view.show(true);
    }
  }

  #draw(view: WebviewView): void {
    view.webview.html = frameHtml(this.target, themeOf(window.activeColorTheme.kind));
  }
}
