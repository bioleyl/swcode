import { commands, window } from 'vscode';

import type { ExtensionContext } from 'vscode';

import { readTarget } from './daemon';
import { SessionPanel } from './session-panel';
import { createStatusBar } from './status-bar';
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
} from './swcode.constants';

/** Puts swcode's session panel in the side bar, on the daemon of this computer. */
export function activate(context: ExtensionContext): void {
  const target = readTarget();
  const panel = new SessionPanel(target, context.subscriptions);
  context.subscriptions.push(
    window.registerWebviewViewProvider(VIEW_ID, panel),
    window.onDidChangeActiveColorTheme(() => panel.refresh()),
    createStatusBar(target),
    commands.registerCommand(ASK_COMMAND, () => panel.ask(ASK_PROMPT)),
    commands.registerCommand(REFACTOR_COMMAND, () => panel.ask(REFACTOR_PROMPT)),
    commands.registerCommand(EXPLAIN_COMMAND, () => panel.ask(EXPLAIN_PROMPT)),
    commands.registerCommand(FIX_PROBLEMS_COMMAND, () => panel.ask(FIX_PROBLEMS_PROMPT, true)),
    commands.registerCommand(ADD_TESTS_COMMAND, () => panel.ask(ADD_TESTS_PROMPT)),
  );
}
