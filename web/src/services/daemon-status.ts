import { Bus } from '@jadis/core';

import { ENDPOINTS } from '../../../shared/protocol/endpoints';
import { StatusSchema } from '../../../shared/protocol/messages.schema';
import { problemOf } from '../lib/daemon-lines';
import { jsonOfText, request, textOf } from './api';
import { pairingFailed } from './pairing';

import type { DaemonState } from '../types/daemon.types';

const UNAUTHORIZED = 401;
// llm-server can stop or change models at any time: the panels ask again now and then.
const POLL_MS = 15_000;

export const daemonStatusBus = new Bus<{ changed: DaemonState }>();

const state: { current: DaemonState } = { current: { kind: 'unknown' } };

export const daemonState = (): DaemonState => state.current;

async function fetchState(): Promise<DaemonState> {
  const response = await request('GET', ENDPOINTS.status);
  if (!response) {
    return { kind: 'offline' };
  }
  const body = await textOf(response);
  if (!response.ok) {
    return response.status === UNAUTHORIZED
      ? { kind: 'locked', pairingFailed: pairingFailed() }
      : { kind: 'refused', problem: problemOf(response.status, body), status: response.status };
  }
  const parsed = StatusSchema.safeParse(jsonOfText(body));
  return parsed.success ? { kind: 'ready', status: parsed.data } : { kind: 'badAnswer' };
}

export async function refreshStatus(): Promise<void> {
  state.current = await fetchState();
  daemonStatusBus.emit('changed', state.current);
}

/** Asks for the status now, then every little while, until the signal is cut: the page's own `killSignal`. */
export function watchStatus(signal: AbortSignal): void {
  void refreshStatus();
  const timer = setInterval(() => void refreshStatus(), POLL_MS);
  signal.addEventListener('abort', () => clearInterval(timer), { once: true });
}
