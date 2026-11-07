import { Bus } from '@jadis/core';

import { seenAtOf } from '../lib/session-lights';
import { daemon } from './daemon';
import { allSessions, sessionListBus } from './session-list';

import type { SessionSummary } from '../../../shared/protocol/session.schema';

// The list lives as long as the page: its bus listener never needs to stop.
const forever = new AbortController().signal;

/**
 * What the person saw of each session changed: the daemon keeps it for every device and lists it with the sessions,
 * so it comes with each list.
 */
export const seenBus = new Bus<{ changed: undefined }>();

/** The last activity of each session the person saw, on any device. */
export const seenSessions = (): Readonly<Record<string, string>> =>
  Object.fromEntries(allSessions().map((session) => [session.id, seenAtOf(session)]));

/** The person sees the session as it is now: its icon empties on every device, until it works again. */
export function markSeen(session: SessionSummary): void {
  if (seenAtOf(session) === session.updatedAt) {
    return;
  }
  daemon.send({ seenAt: session.updatedAt, sessionId: session.id, type: 'markSeen' });
}

sessionListBus.register('changed', () => seenBus.emit('changed'), forever);
