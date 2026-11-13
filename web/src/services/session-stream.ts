import { daemon } from './daemon';

// How many pages of this window show each session: the outlet keeps the page it leaves a moment beside the next,
// and both can show the same session.
const shown = new Map<string, number>();

/** A page shows a session from now on: its stream comes to this window's socket once the page opens it. */
export function sessionShown(sessionId: string): void {
  shown.set(sessionId, (shown.get(sessionId) ?? 0) + 1);
}

/**
 * A page that showed a session left. The daemon is told to stop sending the session's stream only once no page of
 * the window shows it: told by a page another one had just replaced, it would cut the stream of the one that stays.
 */
export function sessionLeft(sessionId: string): void {
  const still = (shown.get(sessionId) ?? 1) - 1;
  if (still > 0) {
    shown.set(sessionId, still);
    return;
  }
  shown.delete(sessionId);
  daemon.send({ sessionId, type: 'closeSession' });
}
