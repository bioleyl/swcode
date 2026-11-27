import type { DaemonState } from '../types/daemon.types';

const LOCAL_HOSTS = new Set(['localhost', '127.0.0.1', '[::1]']);

/** What the pairing panel says in place of an address no other device can open. */
export const LOCAL_ONLY_HEADING = 'Other devices cannot reach this swcode';
export const LOCAL_ONLY_NOTE =
  'It listens on this computer only, so a phone or another computer can neither open it nor pair with it: a number typed here would wait for nothing. Have it listen on every interface (Settings → Network, its Network tab), or give it an address on your tailnet (below).';

/** The address another machine reaches: the daemon's public one, else its address on the network, else the page's. */
export const reachableBaseOf = (state: DaemonState, pageOrigin: string): string => {
  const status = state.kind === 'ready' ? state.status : undefined;
  return status?.publicUrl ?? status?.lanUrls?.[0] ?? pageOrigin;
};

/** Whether an address only works on this computer: the daemon listens there alone. */
export const isLocalBase = (base: string): boolean => LOCAL_HOSTS.has(new URL(base).hostname);

/**
 * Whether no other device can reach the daemon, as far as it says: it answered with no address of its own to give
 * (none it was given, none on the network), and the page itself came from this computer. Not while its status is
 * still on its way, when nothing is known yet.
 */
export function listensHereAlone(state: DaemonState, pageOrigin: string): boolean {
  if (state.kind !== 'ready') {
    return false;
  }
  const { lanUrls, publicUrl } = state.status;
  return publicUrl === undefined && (lanUrls ?? []).length === 0 && isLocalBase(pageOrigin);
}

/** What a device says of an address nothing answers at, in place of a number that would wait for nothing. */
export const nothingAnswersAt = (origin: string): string =>
  `Nothing answers at ${origin}. Check the address, that swcode runs there, and that it listens on the network (its Settings → Network): one that listens on its computer alone cannot be reached from another device.`;

/** What a device says under a number that waits: why it may wait for nothing, since the device cannot tell. */
export const NO_ANSWER_HINT =
  'That computer answers. The number is taken once you run the command there, on the computer itself.';

/** The number the phone shows is six digits, as `swcode pair` takes it. */
export const PAIR_NUMBER_HINT = 'The number the phone shows is six digits.';

/** What the Phone topic says once the number waits here for the phone to take it. */
export const pairingWaitingText = (number: string, lifetimeMs: number): string =>
  `Pairing number ${number} waits ${Math.round(lifetimeMs / 60_000)} minutes: the phone takes it, and is paired.`;
