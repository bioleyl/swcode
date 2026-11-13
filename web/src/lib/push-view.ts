import type { PushState } from '../types/push.types';

/** What the settings say of notifications on this device, by where they stand. */
export const PUSH_LINES: Readonly<Record<PushState, string>> = {
  denied:
    'Notifications are blocked for this page: allow them for this site in the browser’s settings, then come back.',
  insecure:
    'A browser notifies only from an HTTPS address, and this page came without it. Give swcode one (a tailnet does, above), then open swcode from that address.',
  needsInstall:
    'On an iPhone or an iPad, add swcode to the Home Screen first (above), then turn notifications on from the app it opens.',
  off: 'Be told on this device when a session waits for you or ends its turn, with swcode closed.',
  on: 'This device is told when a session waits for you or ends its turn, with swcode closed.',
  unsupported: 'This browser shows no notifications for a web app.',
};

/** The button that changes where notifications stand, where a press can: none otherwise. */
export const PUSH_BUTTONS: Readonly<Record<PushState, string | undefined>> = {
  denied: undefined,
  insecure: undefined,
  needsInstall: undefined,
  off: 'Notify me on this device',
  on: 'Stop notifying this device',
  unsupported: undefined,
};

const APPLE_PHONE = /iPhone|iPad|iPod/;
const APPLE_COMPUTER = /Macintosh/;

/**
 * Whether the device is an iPhone or an iPad, which notify only for an app on their Home Screen. An iPad says it is
 * a Mac, but a Mac has no touch screen.
 */
export const isAppleMobile = (userAgent: string, touchPoints: number): boolean =>
  APPLE_PHONE.test(userAgent) || (APPLE_COMPUTER.test(userAgent) && touchPoints > 1);

const BASE64_GROUP = 4;

/** The daemon's push key as a browser takes it to subscribe: its bytes, from base64url. */
export const keyBytes = (key: string): Uint8Array<ArrayBuffer> => {
  const padded = key.replaceAll('-', '+').replaceAll('_', '/');
  const text = atob(padded.padEnd(Math.ceil(padded.length / BASE64_GROUP) * BASE64_GROUP, '='));
  const bytes = new Uint8Array(new ArrayBuffer(text.length));
  for (let index = 0; index < text.length; index += 1) {
    bytes[index] = text.charCodeAt(index);
  }
  return bytes;
};
