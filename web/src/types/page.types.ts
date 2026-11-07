import type { ServerMessage } from '../../../shared/protocol/messages.schema';

export type ServerMessageMap = { [Message in ServerMessage as Message['type']]: Message };

/**
 * One handler per kind of message from the daemon, for what a page shows itself. A kind the page leaves out is one it
 * has no business with: it reaches the app through a service that lists, searches or asks. A new kind the page must
 * show is added here, and its handler fails to compile until it is written.
 */
export type ServerHandlers = { [Type in keyof ServerMessageMap]?: (message: ServerMessageMap[Type]) => void };
