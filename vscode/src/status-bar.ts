import { Disposable, StatusBarAlignment, window } from 'vscode';
import type { StatusBarItem } from 'vscode';

import { addressOf, probe } from './daemon';
import {
  STATUS_BAR_PRIORITY,
  STATUS_CONNECTED,
  STATUS_NOT_RUNNING,
  STATUS_POLL_MS,
  VIEW_CONTAINER_ID,
} from './swcode.constants';

import type { DaemonProbe, DaemonTarget } from './types/daemon.types';

// The status bar's words, per state of the daemon's socket.
// biome-ignore assist/source/useSortedKeys: the order is the order of the states
const STATE_TEXTS: Record<DaemonProbe['kind'], string> = {
  connected: STATUS_CONNECTED,
  unreachable: STATUS_NOT_RUNNING,
};

const PROBE_FAILURE = 'The daemon did not answer';

/**
 * The status bar's item, showing the daemon's state: it asks the port now, then every little while, and stops
 * doing so when it is disposed.
 */
export function createStatusBar(target: DaemonTarget): Disposable {
  const item = window.createStatusBarItem(StatusBarAlignment.Left, STATUS_BAR_PRIORITY);
  item.name = VIEW_CONTAINER_ID;
  const ask = () => void probe(target).then((state) => applyState(item, target, state));
  ask();
  const timer = setInterval(ask, STATUS_POLL_MS);
  return Disposable.from(item, { dispose: () => clearInterval(timer) });
}

function applyState(item: StatusBarItem, target: DaemonTarget, state: DaemonProbe): void {
  item.text = STATE_TEXTS[state.kind];
  const where = state.kind === 'unreachable' ? PROBE_FAILURE : VIEW_CONTAINER_ID;
  item.tooltip = `${where} — ${addressOf(target)}`;
}
