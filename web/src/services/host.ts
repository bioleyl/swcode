import { TOKEN_PARAM } from '../../../shared/protocol/endpoints';

import type { HostConfig } from '../types/host.types';
import type { Theme } from './theme';

const DAEMON_PARAM = 'daemon';
const THEME_PARAM = 'theme';
const THEMES: readonly Theme[] = ['light', 'dark'];

const isTheme = (value: string | null): value is Theme => THEMES.some((theme) => theme === value);

// A page rendered outside a browser (a server render, a test) has no address to read a query from.
const pageSearch = (): string => (typeof location === 'undefined' ? '' : location.search);

/**
 * The host's own settings, read from the address's query: a host that ships the bundle (the VS Code extension, the
 * desktop app) says there which daemon to talk to and with which token, as the link the daemon prints does with
 * `?token=`. The app takes its token the same way, so both entries read the same parameters.
 *
 * `search` is the address's query, the page's own when nothing is given.
 */
export function hostConfig(search: string = pageSearch()): HostConfig {
  const params = new URLSearchParams(search);
  const theme = params.get(THEME_PARAM);
  return {
    daemon: params.get(DAEMON_PARAM) ?? '',
    token: params.get(TOKEN_PARAM) ?? '',
    ...(isTheme(theme) ? { theme } : {}),
  };
}
