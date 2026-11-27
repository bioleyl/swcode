import type { ServerMessage } from '../../../shared/protocol/messages.schema';

/** Where a session's workspace stands in git, as the daemon said it. */
export type GitState = Omit<Extract<ServerMessage, { type: 'gitState' }>, 'sessionId' | 'type'>;

/** What the session's header says of it: a few words beside the folder, and a sentence when they are pointed at. */
export interface GitLine {
  text: string;
  hint: string;
}
