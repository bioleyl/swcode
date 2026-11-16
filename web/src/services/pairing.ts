import { ENDPOINTS } from '../../../shared/protocol/endpoints';
import { DevicesSchema, PairAnswerSchema } from '../../../shared/protocol/pairing.schema';
import { problemOf } from '../lib/daemon-lines';
import { jsonOf, jsonOfText, request } from './api';

import type { Device } from '../../../shared/protocol/pairing.schema';
import type { Answer } from '../types/answer.types';
import type { PairAsk } from '../types/pairing.types';

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

const NOT_WAITING = 403;
const TOO_MANY = 429;
const JSON_TYPE = { 'content-type': 'application/json' };
// How often an app asks whether the number waits yet: often enough to pair at once, rarely enough for the computer's
// limit on attempts.
const POLL_MS = 3000;
// A refused try for lack of attempts: the wait is longer than a poll, or it would never get through.
const RATE_LIMITED_MS = 30_000;
const PAIRING_STOPPED = 'Pairing stopped before that computer made the number wait.';

/** What a device calls itself when it pairs. */
export const deviceName = (userAgent: string): string =>
  DEVICE_NAMES.find(([pattern]) => pattern.test(userAgent))?.[1] ?? SOME_BROWSER;

/** One ask at another swcode: the token of the number it waits for, or why it gave none, and whether to ask again. */
export async function askToPair(origin: string, number: string, name: string): Promise<PairAsk> {
  const response = await fetch(`${origin}${ENDPOINTS.pairing}`, {
    body: JSON.stringify({ name, number }),
    headers: JSON_TYPE,
    method: 'POST',
  }).catch(() => undefined);
  // The computer may be starting, or its address wrong: the person can still make the number wait.
  if (!response) {
    return { ok: false, problem: '', retry: true };
  }
  const body = await response.text().catch(() => '');
  const paired = PairAnswerSchema.safeParse(jsonOfText(body));
  if (response.ok && paired.success) {
    return { ok: true, token: paired.data.token };
  }
  if (response.status === NOT_WAITING) {
    return { ok: false, problem: '', retry: true };
  }
  if (response.status === TOO_MANY) {
    return { ok: false, problem: '', retry: true, waitMs: RATE_LIMITED_MS };
  }
  return { ok: false, problem: problemOf(response.status, body), retry: false };
}

/**
 * Asks until that computer waits for the number, which a person makes wait with `swcode pair` there: its token. Only
 * the person can stop the wait, by the signal.
 */
export async function waitForPairing(
  origin: string,
  number: string,
  name: string,
  signal: AbortSignal
): Promise<Answer<string>> {
  while (!signal.aborted) {
    const ask = await askToPair(origin, number, name);
    if (ask.ok) {
      return { answer: ask.token, ok: true };
    }
    if (!ask.retry) {
      return { ok: false, problem: ask.problem };
    }
    await wait(ask.waitMs ?? POLL_MS, signal);
  }
  return { ok: false, problem: PAIRING_STOPPED };
}

const wait = (ms: number, signal: AbortSignal): Promise<void> =>
  new Promise((resolve) => {
    const done = (): void => {
      clearTimeout(timer);
      signal.removeEventListener('abort', done);
      resolve();
    };
    const timer = setTimeout(done, ms);
    signal.addEventListener('abort', done);
  });

export async function pairedDevices(): Promise<Device[]> {
  const parsed = DevicesSchema.safeParse(await jsonOf(await request('GET', ENDPOINTS.devices)));
  return parsed.success ? parsed.data.devices : [];
}

/** Forgets a paired phone: its token stops working at once. */
export async function forgetDevice(id: string): Promise<void> {
  await request('DELETE', `${ENDPOINTS.devices}/${encodeURIComponent(id)}`);
}
