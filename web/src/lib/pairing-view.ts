import type { Tailnet } from '../../../shared/protocol/pairing.schema';
import type { DaemonState } from '../types/daemon.types';

const LOCAL_HOSTS = new Set(['localhost', '127.0.0.1', '[::1]']);
const MS_PER_SECOND = 1000;
// A new code comes this long before the shown one expires, so the page never shows a dead one.
const RENEW_AHEAD_MS = 2000;

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

/** The seconds the shown code still lasts; 0 when the page should ask for a new one. */
export const secondsUntilRenewal = (expiresAt: string, now: number): number =>
  Math.max(0, Math.ceil((Date.parse(expiresAt) - now - RENEW_AHEAD_MS) / MS_PER_SECOND));

/** How long the shown code lasts, in words. */
export const renewalLine = (seconds: number): string => `A new code in ${seconds} s`;

/**
 * What the tailnet does for a phone, in a line: swcode's HTTPS address there once Tailscale serves it, the address
 * it would have, or why this computer has none.
 */
export function tailnetLine(tailnet: Tailnet): string {
  if (!tailnet.available) {
    return tailnet.problem ?? '';
  }
  return tailnet.served
    ? `swcode is at ${tailnet.address} on your tailnet, over HTTPS: the QR code leads there, and a phone on the tailnet installs swcode from it and reaches it from anywhere.`
    : `Tailscale can give swcode an HTTPS address on your tailnet, ${tailnet.address}: a phone on the tailnet installs swcode from it, and reaches it from anywhere.`;
}
