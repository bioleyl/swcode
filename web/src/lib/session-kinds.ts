import { hostLabel } from '../../../shared/hosts/host-key';
import { folderName } from './format';
import { placeText } from './hosts';

import type { SessionKind, SessionSummary } from '../../../shared/protocol/session.schema';

const CHAT_PLACE = 'Chat';

/** Where a session works, in full: a chat works nowhere. */
const PLACES: Readonly<Record<SessionKind, (session: SessionSummary) => string>> = {
  chat: () => CHAT_PLACE,
  code: (session) => placeText(session.host, session.workspace),
};

/** Where a session works, in a word: its host, or nowhere for a chat. */
const HOSTS: Readonly<Record<SessionKind, (session: SessionSummary) => string>> = {
  chat: () => CHAT_PLACE,
  code: (session) => hostLabel(session.host),
};

/** Where a session works, by name: its project's folder, or nowhere for a chat. */
const NAMES: Readonly<Record<SessionKind, (session: SessionSummary) => string>> = {
  chat: () => CHAT_PLACE,
  code: (session) => folderName(session.workspace),
};

/** The sessions of a kind, in the order given (the daemon lists them newest first). */
export const sessionsOf = (kind: SessionKind, sessions: readonly SessionSummary[]): SessionSummary[] =>
  sessions.filter((session) => session.kind === kind);

/** Where a session works, as the palette and its page say it: its folder and host, or that it is a chat. */
export const sessionPlace = (session: SessionSummary): string => PLACES[session.kind](session);

/** Where a session works, as a list row says it: its host, or that it is a chat. */
export const sessionHost = (session: SessionSummary): string => HOSTS[session.kind](session);

/** Where a session works, as a line too short for its path says it: its project's folder, or that it is a chat. */
export const sessionPlaceName = (session: SessionSummary): string => NAMES[session.kind](session);
