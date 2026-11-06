import { Bus } from '@jadis/core';

import { ENDPOINTS } from '../../../shared/protocol/endpoints';
import { StatusSchema } from '../../../shared/protocol/messages.schema';
import { authorizationHeaders } from './access';
import { pairingFailed } from './pairing';

import type { DaemonState } from '../types/daemon.types';

const UNAUTHORIZED = 401;
// llm-server can stop or change models at any time: the panels ask again now and then.
const POLL_MS = 15_000;

export const daemonStatusBus = new Bus<{ changed: DaemonState }>();

const state: { current: DaemonState } = { current: { kind: 'unknown' } };

export const daemonState = (): DaemonState => state.current;

async function fetchState(): Promise<DaemonState> {
  const response = await fetch(ENDPOINTS.status, { headers: authorizationHeaders() }).catch(() => undefined);
  if (!response) {
    return { kind: 'offline' };
  }
  if (response.status === UNAUTHORIZED) {
    return { kind: 'locked', pairingFailed: pairingFailed() };
  }
  const parsed = StatusSchema.safeParse(await response.json().catch(() => undefined));
  return parsed.success ? { kind: 'ready', status: parsed.data } : { kind: 'offline' };
}

export async function refreshStatus(): Promise<void> {
  state.current = await fetchState();
  daemonStatusBus.emit('changed', state.current);
}

/** Asks for the status now, then every little while, for as long as the page is open. */
export function watchStatus(): void {
  void refreshStatus();
  setInterval(() => void refreshStatus(), POLL_MS);
}
