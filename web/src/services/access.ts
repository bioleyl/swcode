import { THIS_COMPUTER } from '../../../shared/protocol/desktop.schema';
import { BEARER_PREFIX, ENDPOINTS, TOKEN_PARAM } from '../../../shared/protocol/endpoints';
import { keepComputers, shownComputer, storedComputers } from './computer-store';

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

// The daemon the page talks to: the one that served it, or another computer's this browser paired with, at its own
// address and with the token that pairing gave.
const shown = shownComputer();
const session = { base: shown?.address ?? '', token: shown?.token ?? stored() };

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

/**
 * Uses a token of the computer that served the page from now on, and keeps it for the next visits. The page then
 * shows that computer: the link or the code that brought the token is its own.
 */
export function keepToken(token: string): void {
  session.base = '';
  session.token = token;
  store(token);
  keepComputers({ ...storedComputers(), current: THIS_COMPUTER });
}

/** The token of the computer that served the page, whichever one the page shows. */
export const homeToken = (): string | undefined => stored();

/** Where the daemon the page talks to answers: the address its own links start with. */
export const daemonOrigin = (): string => session.base || location.origin;

/** An address of the daemon the page talks to, from a path of its API. */
export const daemonUrl = (path: string): string => `${session.base}${path}`;

export const authorizationHeaders = (): Record<string, string> =>
  session.token ? { authorization: `${BEARER_PREFIX}${session.token}` } : {};

/** The socket's address: a browser cannot set headers on a WebSocket, so the token goes in the query. */
export function socketUrl(): string {
  const url = new URL(daemonUrl(ENDPOINTS.socket), location.href);
  url.protocol = url.protocol === SECURE_PAGE ? SECURE_SOCKET : PLAIN_SOCKET;
  url.searchParams.set(TOKEN_PARAM, session.token ?? '');
  return url.toString();
}

/** A daemon address an element loads by itself (a picture): it cannot send headers, so the token goes in the query. */
export function authorizedUrl(path: string): string {
  const url = new URL(daemonUrl(path), location.href);
  url.searchParams.set(TOKEN_PARAM, session.token ?? '');
  return url.toString();
}
