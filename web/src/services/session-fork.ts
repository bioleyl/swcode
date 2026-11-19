import { daemon, daemonBus } from './daemon';

/** How long a fork waits for the daemon's answer before the person is told it did not come. */
const FORK_WAIT_MS = 5_000;

/**
 * Branches a session at a decision: the daemon makes a new session with the source's first `keep` events and starts
 * a turn with `text`. Answers the new session's id, or undefined when the daemon refused or said nothing.
 */
export function forkSession(sourceId: string, keep: number, text: string): Promise<string | undefined> {
  return new Promise((resolve) => {
    const controller = new AbortController();
    const timer = setTimeout(() => {
      controller.abort();
      resolve(undefined);
    }, FORK_WAIT_MS);
    daemonBus.register(
      'message',
      (message) => {
        if (message.type !== 'sessionCreated' || message.session.forkOf !== sourceId) {
          return;
        }
        clearTimeout(timer);
        controller.abort();
        resolve(message.session.id);
      },
      controller.signal
    );
    daemon.send({ keep, sessionId: sourceId, text, type: 'forkSession' });
  });
}
