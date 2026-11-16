import type { DaemonState } from '../types/daemon.types';

const LOCAL_HOSTS = new Set(['localhost', '127.0.0.1', '[::1]']);

/** What a code shown at an address only this computer reaches means. */
export const LOCAL_ADDRESS_NOTE =
  'This address only works on this computer: swcode listens here alone. Give it an address on your tailnet (Settings → Phone), or have it listen on every interface (Settings → Network).';

/** The address another machine reaches: the daemon's public one, else its address on the network, else the page's. */
export const reachableBaseOf = (state: DaemonState, pageOrigin: string): string => {
  const status = state.kind === 'ready' ? state.status : undefined;
  return status?.publicUrl ?? status?.lanUrls?.[0] ?? pageOrigin;
};

/** Whether an address only works on this computer: the daemon listens there alone. */
export const isLocalBase = (base: string): boolean => LOCAL_HOSTS.has(new URL(base).hostname);

/** The number the phone shows is six digits, as `swcode pair` takes it. */
export const PAIR_NUMBER_HINT = 'The number the phone shows is six digits.';

/** What the Phone topic says once the number waits here for the phone to take it. */
export const pairingWaitingText = (number: string, lifetimeMs: number): string =>
  `Pairing number ${number} waits ${Math.round(lifetimeMs / 60_000)} minutes: the phone takes it, and is paired.`;
