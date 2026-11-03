/** Goes to a page of the app: the router follows the hash. */
export const navigate = (path: string): void => {
  location.hash = `#${path}`;
};

export const hrefOf = (path: string): string => `#${path}`;

export const sessionPath = (id: string): string => `/sessions/${encodeURIComponent(id)}`;

/** Every session, those that need the person first: a phone's tab. */
export const SESSIONS_PATH = '/sessions';

/** A project: its sessions, and a new one in it. */
export const projectPath = (id: string): string => `/projects/${encodeURIComponent(id)}`;

export const HOME_PATH = '/';

export const NEW_SESSION_PATH = '/new';

export const SETTINGS_PATH = '/settings';

/** A topic of the settings page: the LLM, the skills, the plugins… */
export const settingsPath = (topic: string): string => `${SETTINGS_PATH}/${encodeURIComponent(topic)}`;

/** The form for a new session, its project chosen. */
export const newSessionPath = (projectId: string): string =>
  `${NEW_SESSION_PATH}/${encodeURIComponent(projectId)}`;
