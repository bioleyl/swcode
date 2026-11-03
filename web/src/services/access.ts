import { BEARER_PREFIX, ENDPOINTS, TOKEN_PARAM } from '../../../shared/protocol/endpoints';

const STORAGE_KEY = 'swcode.token';
const SECURE_PAGE = 'https:';
const SECURE_SOCKET = 'wss:';
const PLAIN_SOCKET = 'ws:';

const stored = (): string | undefined => {
  try {
    return localStorage.getItem(STORAGE_KEY) ?? undefined;
  } catch {
    return undefined;
  }
};

const store = (token: string): void => {
  try {
    localStorage.setItem(STORAGE_KEY, token);
  } catch {
    // Without storage, the token lives as long as the page.
  }
};

const session = { token: stored() };

/** Takes the token from the link the daemon printed, keeps it, and takes it out of the address bar. */
export function adoptToken(): void {
  const url = new URL(location.href);
  const token = url.searchParams.get(TOKEN_PARAM);
  if (token === null) {
    return;
  }
  keepToken(token);
  url.searchParams.delete(TOKEN_PARAM);
  history.replaceState(history.state, '', url);
}

/** Uses a token from now on, and keeps it for the next visits. */
export function keepToken(token: string): void {
  session.token = token;
  store(token);
}

export const authorizationHeaders = (): Record<string, string> =>
  session.token ? { authorization: `${BEARER_PREFIX}${session.token}` } : {};

/** The socket's address: a browser cannot set headers on a WebSocket, so the token goes in the query. */
export function socketUrl(): string {
  const url = new URL(ENDPOINTS.socket, location.href);
  url.protocol = location.protocol === SECURE_PAGE ? SECURE_SOCKET : PLAIN_SOCKET;
  url.searchParams.set(TOKEN_PARAM, session.token ?? '');
  return url.toString();
}

/** A daemon address an element loads by itself (a picture): it cannot send headers, so the token goes in the query. */
export function authorizedUrl(path: string): string {
  const url = new URL(path, location.href);
  url.searchParams.set(TOKEN_PARAM, session.token ?? '');
  return url.toString();
}
