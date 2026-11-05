import { Bus } from '@jadis/core';
import { z } from 'zod';

import { allSessions, sessionListBus } from './session-list';

import type { SessionSummary } from '../../../shared/protocol/session.schema';

const STORAGE_KEY = 'swcode.seen';
// When each session was seen, by its last activity then.
const SeenSchema = z.record(z.string(), z.string());

// The record lives as long as the page: its bus listener never needs to stop.
const forever = new AbortController().signal;

/** When the person last saw each session on this device; `changed` once one is seen. */
export const seenBus = new Bus<{ changed: undefined }>();

const stored = (): Record<string, string> | undefined => {
  try {
    const text = localStorage.getItem(STORAGE_KEY);
    if (text === null) {
      return undefined;
    }
    const parsed = SeenSchema.safeParse(JSON.parse(text));
    return parsed.success ? parsed.data : {};
  } catch {
    return undefined;
  }
};

const keep = (seen: Record<string, string>): void => {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(seen));
  } catch {
    // Without storage, the record lasts as long as the page.
  }
};

// Nothing kept yet on this device: the first list counts as seen, so that every session does not show done at once.
const state: { seen: Record<string, string> | undefined } = { seen: stored() };

const asSeen = (sessions: readonly SessionSummary[]): Record<string, string> =>
  Object.fromEntries(sessions.map((session) => [session.id, session.updatedAt]));

const record = (seen: Record<string, string>): void => {
  state.seen = seen;
  keep(seen);
  seenBus.emit('changed');
};

/** The last activity of each session when the person last saw it. */
export const seenSessions = (): Readonly<Record<string, string>> => state.seen ?? {};

/** The person sees the session as it is now: its icon empties until it works again. */
export function markSeen(session: SessionSummary): void {
  if (state.seen?.[session.id] === session.updatedAt) {
    return;
  }
  record({ ...state.seen, [session.id]: session.updatedAt });
}

/**
 * The sessions of the first list on this device count as seen; later lists drop the sessions that went. An empty
 * list drops nothing: a daemon starting may list none for a moment.
 */
function follow(sessions: readonly SessionSummary[]): void {
  const seen = state.seen;
  if (seen === undefined) {
    record(asSeen(sessions));
    return;
  }
  if (sessions.length === 0) {
    return;
  }
  const ids = new Set(sessions.map((session) => session.id));
  const kept = Object.entries(seen).filter(([id]) => ids.has(id));
  if (kept.length === Object.keys(seen).length) {
    return;
  }
  record(Object.fromEntries(kept));
}

sessionListBus.register('changed', () => follow(allSessions()), forever);
