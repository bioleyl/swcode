import type { Theme } from '../services/theme';

/**
 * What a host that ships the bundle (the VS Code extension's webview) tells it through the address's query.
 *
 * `daemon` is where the daemon answers (`http://127.0.0.1:4500`); empty for the origin that served the page.
 * `token` is the access token the daemon asks for; empty for none. `theme` is the host's own theme, and is left
 * out when the host names none, which leaves the system's.
 */
export interface HostConfig {
  daemon: string;
  token: string;
  theme?: Theme;
}
