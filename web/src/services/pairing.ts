import { ENDPOINTS, PAIR_PARAM } from '../../../shared/protocol/endpoints';
import { DevicesSchema, PairAnswerSchema, PairingCodeSchema } from '../../../shared/protocol/pairing.schema';
import { authorizationHeaders, keepToken } from './access';

import type { Device, PairingCode } from '../../../shared/protocol/pairing.schema';

// What a phone calls itself in the computer's list, from what its browser says it runs on.
const DEVICE_NAMES: readonly (readonly [RegExp, string])[] = [
  [/iPhone/, 'iPhone'],
  [/iPad/, 'iPad'],
  [/Android/, 'Android phone'],
  [/Windows/, 'Windows computer'],
  [/Macintosh/, 'Mac'],
  [/Linux/, 'Linux computer'],
];
const SOME_BROWSER = 'A browser';

const state = { failed: false };

/** What a device calls itself when it pairs. */
export const deviceName = (userAgent: string): string =>
  DEVICE_NAMES.find(([pattern]) => pattern.test(userAgent))?.[1] ?? SOME_BROWSER;

/** Whether this page came from a pairing code the daemon refused: expired, or used already. */
export const pairingFailed = (): boolean => state.failed;

/**
 * A page opened from the QR code trades its pairing code for a token of its own, kept for the next visits, and takes
 * the code out of the address bar: it serves once.
 */
export async function pairFromLink(): Promise<void> {
  const url = new URL(location.href);
  const code = url.searchParams.get(PAIR_PARAM);
  if (code === null) {
    return;
  }
  url.searchParams.delete(PAIR_PARAM);
  history.replaceState(history.state, '', url);
  const response = await fetch(ENDPOINTS.pairing, {
    body: JSON.stringify({ code, name: deviceName(navigator.userAgent) }),
    headers: { 'content-type': 'application/json' },
    method: 'POST',
  }).catch(() => undefined);
  const answer = PairAnswerSchema.safeParse(await response?.json().catch(() => undefined));
  if (!response?.ok || !answer.success) {
    state.failed = true;
    return;
  }
  keepToken(answer.data.token);
}

/** A new code for the QR code, or nothing when the daemon does not answer. */
export async function newPairingCode(): Promise<PairingCode | undefined> {
  const response = await fetch(ENDPOINTS.pairingCode, { headers: authorizationHeaders() }).catch(() => undefined);
  const parsed = PairingCodeSchema.safeParse(await response?.json().catch(() => undefined));
  return parsed.success ? parsed.data : undefined;
}

export async function pairedDevices(): Promise<Device[]> {
  const response = await fetch(ENDPOINTS.devices, { headers: authorizationHeaders() }).catch(() => undefined);
  const parsed = DevicesSchema.safeParse(await response?.json().catch(() => undefined));
  return parsed.success ? parsed.data.devices : [];
}

/** Forgets a paired phone: its token stops working at once. */
export async function forgetDevice(id: string): Promise<void> {
  await fetch(`${ENDPOINTS.devices}/${encodeURIComponent(id)}`, {
    headers: authorizationHeaders(),
    method: 'DELETE',
  }).catch(() => undefined);
}

/** The link the QR code holds: the address a phone reaches, with the code. */
export function pairingLink(base: string, code: string): string {
  const url = new URL(base);
  url.searchParams.set(PAIR_PARAM, code);
  return url.toString();
}
