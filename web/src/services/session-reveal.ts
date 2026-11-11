import { Bus } from '@jadis/core';

/** A search's match in a session: the event to bring into view, and whose conversation it is in. */
export const sessionRevealBus = new Bus<{ asked: { eventId: string; sessionId: string } }>();

// The event a session's page has yet to show: kept until it draws it, since the page it is for may not be open yet.
let pending: { eventId: string; sessionId: string } | undefined;

/** Asks a session's page to bring the event a search found into view: the one open, or the one that opens next. */
export function revealInSession(sessionId: string, eventId: string): void {
  pending = { eventId, sessionId };
  sessionRevealBus.emit('asked', pending);
}

/** The event this session's page must show, once: nothing for another session, or once a page took it. */
export function takeReveal(sessionId: string): string | undefined {
  if (pending?.sessionId !== sessionId) {
    return undefined;
  }
  const { eventId } = pending;
  pending = undefined;
  return eventId;
}
