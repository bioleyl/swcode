import type { DaemonState } from '../types/daemon.types';

const LOCAL_HOSTS = new Set(['localhost', '127.0.0.1', '[::1]']);
const MS_PER_SECOND = 1000;
// A new code comes this long before the shown one expires, so the page never shows a dead one.
const RENEW_AHEAD_MS = 2000;

/** What a code shown at an address only this computer reaches means. */
export const LOCAL_ADDRESS_NOTE =
  'This address only works on this computer: the daemon listens here alone. Start it with SWCODE_HOST=0.0.0.0 for the network, or SWCODE_PUBLIC_URL set to an address the other device reaches.';

/** The address another machine reaches: the daemon's public one, else its address on the network, else the page's. */
export const reachableBaseOf = (state: DaemonState, pageOrigin: string): string => {
  const status = state.kind === 'ready' ? state.status : undefined;
  return status?.publicUrl ?? status?.lanUrls?.[0] ?? pageOrigin;
};

/** Whether an address only works on this computer: the daemon listens there alone. */
export const isLocalBase = (base: string): boolean => LOCAL_HOSTS.has(new URL(base).hostname);

/** The seconds the shown code still lasts; 0 when the page should ask for a new one. */
export const secondsUntilRenewal = (expiresAt: string, now: number): number =>
  Math.max(0, Math.ceil((Date.parse(expiresAt) - now - RENEW_AHEAD_MS) / MS_PER_SECOND));

/** How long the shown code lasts, in words. */
export const renewalLine = (seconds: number): string => `A new code in ${seconds} s`;
