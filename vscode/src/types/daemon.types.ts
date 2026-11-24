/**
 * The daemon to talk to, as the host tells the bundle: the address it answers at, and the access token it asks for.
 * Both are the same parameters the app's own link carries (`daemon` and `token` in the address's query).
 */
export interface DaemonTarget {
  daemon: string;
  token: string;
}

/** Whether the daemon answers on its port: the status bar's two states. */
export type DaemonProbe = { kind: 'connected' } | { kind: 'unreachable'; problem: string };
