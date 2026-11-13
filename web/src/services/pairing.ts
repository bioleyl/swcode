import { ENDPOINTS, PAIR_PARAM } from '../../../shared/protocol/endpoints';
import { DevicesSchema, PairAnswerSchema, PairingCodeSchema } from '../../../shared/protocol/pairing.schema';
import { problemOf } from '../lib/daemon-lines';
import { keepToken } from './access';
import { jsonOf, jsonOfText, request, textOf } from './api';

import type { Device, PairingCode } from '../../../shared/protocol/pairing.schema';
import type { Answer } from '../types/answer.types';

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

/** The daemon gives no code at all, as when it is not running: the phone topic says so in its place. */
export const NO_CODE = 'The daemon gives no pairing code: is it running?';

/** A new code for the QR code; when the daemon does not answer or refuses one, what it said. */
export async function newPairingCode(): Promise<Answer<PairingCode>> {
  const response = await request('GET', ENDPOINTS.pairingCode);
  if (!response) {
    return { ok: false, problem: NO_CODE };
  }
  const body = await textOf(response);
  const parsed = PairingCodeSchema.safeParse(jsonOfText(body));
  if (!response.ok || !parsed.success) {
    return { ok: false, problem: problemOf(response.status, body) };
  }
  return { answer: parsed.data, ok: true };
}

export async function pairedDevices(): Promise<Device[]> {
  const parsed = DevicesSchema.safeParse(await jsonOf(await request('GET', ENDPOINTS.devices)));
  return parsed.success ? parsed.data.devices : [];
}

/** Forgets a paired phone: its token stops working at once. */
export async function forgetDevice(id: string): Promise<void> {
  await request('DELETE', `${ENDPOINTS.devices}/${encodeURIComponent(id)}`);
}

/** The link the QR code holds: the address a phone reaches, with the code. */
export function pairingLink(base: string, code: string): string {
  const url = new URL(base);
  url.searchParams.set(PAIR_PARAM, code);
  return url.toString();
}
