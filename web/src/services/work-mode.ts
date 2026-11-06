import { Bus } from '@jadis/core';

import { SessionKindSchema } from '../../../shared/protocol/session.schema';
import { readStored, writeStored } from './device-store';

import type { SessionKind } from '../../../shared/protocol/session.schema';

const STORAGE_KEY = 'swcode.mode';
const FIRST_MODE: SessionKind = 'code';

/**
 * Which sessions the person works with, chosen at the navigation's foot and kept on this device: the chats, or the
 * projects and their coding sessions. Opening a session of the other kind switches to it.
 */
export const workModeBus = new Bus<{ changed: SessionKind }>();

const state = { mode: readStored(STORAGE_KEY, SessionKindSchema, FIRST_MODE) };

export const workMode = (): SessionKind => state.mode;

export function setWorkMode(mode: SessionKind): void {
  if (mode === state.mode) {
    return;
  }
  state.mode = mode;
  writeStored(STORAGE_KEY, mode);
  workModeBus.emit('changed', mode);
}
