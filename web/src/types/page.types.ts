import type { ServerMessage } from '../../../shared/protocol/messages.schema';

export type ServerMessageMap = { [Message in ServerMessage as Message['type']]: Message };

/** One handler per kind of message from the daemon: a new kind does not compile until a page decides. */
export type ServerHandlers = { [Type in keyof ServerMessageMap]: (message: ServerMessageMap[Type]) => void };
