import type { SessionKind } from '../../../shared/protocol/session.schema';

/** Goes to a page of the app: the router follows the hash. */
export const navigate = (path: string): void => {
  location.hash = `#${path}`;
};

/** Goes to a page in place of the one in the address, which the history forgets: an address that only leads on. */
export const redirect = (path: string): void => {
  location.replace(`#${path}`);
};

export const hrefOf = (path: string): string => `#${path}`;

export const sessionPath = (id: string): string => `/sessions/${encodeURIComponent(id)}`;

/** Every session, those that need the person first: a phone's tab. */
export const SESSIONS_PATH = '/sessions';

/** A project: its sessions, and a new one in it. */
export const projectPath = (id: string): string => `/projects/${encodeURIComponent(id)}`;

export const HOME_PATH = '/';

export const NEW_SESSION_PATH = '/new';

/** The form for a new chat. */
export const NEW_CHAT_PATH = '/chat';

const NEW_PATHS: Readonly<Record<SessionKind, string>> = { chat: NEW_CHAT_PATH, code: NEW_SESSION_PATH };

/** Where a new session of a kind starts: the form for a chat, or for a coding session. */
export const newPathOf = (kind: SessionKind): string => NEW_PATHS[kind];

export const SETTINGS_PATH = '/settings';

/** A topic of the settings page: the models, the SSH hosts, the phones… */
export const settingsPath = (topic: string): string => `${SETTINGS_PATH}/${encodeURIComponent(topic)}`;

/** The form for a new session, its project chosen. */
export const newSessionPath = (projectId: string): string =>
  `${NEW_SESSION_PATH}/${encodeURIComponent(projectId)}`;
