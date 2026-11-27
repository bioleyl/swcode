import { z } from 'zod';

import { THIS_COMPUTER } from '../../../shared/protocol/desktop.schema';
import { ENDPOINTS } from '../../../shared/protocol/endpoints';
import { PushKeySchema, PushSubscriptionSchema } from '../../../shared/protocol/pairing.schema';
import { isAppleMobile, keyBytes } from '../lib/push-view';
import { jsonOf, request } from './api';
import { shownComputer } from './computer-store';
import { askNotifications, inDesktopApp } from './desktop';
import { readStored, writeStored } from './device-store';
import { inAndroidApp } from './native-app';
import { noticesRunning, startNotices, stopNotices } from './native-notifications';
import { navigate, sessionPath } from './navigation';

import type { PushOutcome, PushState } from '../types/push.types';

const STANDALONE_QUERY = '(display-mode: standalone)';
const GRANTED: NotificationPermission = 'granted';
const DENIED: NotificationPermission = 'denied';
// What the service worker tells the page when a notification of one of its sessions is pressed (`sw.js`).
const OPEN_SESSION = 'openSession';

const NOT_ALLOWED = 'Notifications were not allowed: nothing changed.';
const NO_KEY = 'The daemon did not give its push key.';
const NOT_SUBSCRIBED = 'The browser could not subscribe to its push service.';
const NOT_KEPT = 'The daemon did not keep the subscription: nothing changed.';

// Which computer holds this browser's push subscription. A browser has one, made with one computer's key: the
// others cannot write to it, so for them notifications are off, whatever the browser holds.
const HOLDER_KEY = 'swcode.push-computer';
const HOLDER = z.string();

const shownId = (): string => shownComputer()?.id ?? THIS_COMPUTER;

/** Whether the computer shown is the one the subscription was made with; one made before this was kept counts. */
function heldHere(): boolean {
  const holder = readStored(HOLDER_KEY, HOLDER, '');
  return holder === '' || holder === shownId();
}

const canPush = (): boolean => 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window;

/** Where notifications stand on this device. */
export async function pushState(): Promise<PushState> {
  // The desktop app's window is not a browser: its tray notifies through Windows, and keeps the answer between starts.
  if (inDesktopApp()) {
    return (await askNotifications()) ? 'on' : 'off';
  }
  // The phone app is installed already, and its WebView has no push service: Android itself does the notifying.
  if (inAndroidApp()) {
    return (await noticesRunning()) ? 'on' : 'off';
  }
  if (!window.isSecureContext) {
    return 'insecure';
  }
  const installed = window.matchMedia(STANDALONE_QUERY).matches;
  if (!installed && isAppleMobile(navigator.userAgent, navigator.maxTouchPoints)) {
    return 'needsInstall';
  }
  if (!canPush()) {
    return 'unsupported';
  }
  if (Notification.permission === DENIED) {
    return 'denied';
  }
  const registration = await navigator.serviceWorker.getRegistration();
  // No service worker: a page of the dev server, which registers none.
  if (!registration) {
    return 'unsupported';
  }
  return (await registration.pushManager.getSubscription()) && heldHere() ? 'on' : 'off';
}

/** Subscribes this browser to its push service with the daemon's key, and has the daemon keep the subscription. */
export async function turnPushOn(): Promise<PushOutcome> {
  if (inDesktopApp()) {
    await askNotifications(true);
    return { ok: true };
  }
  if (inAndroidApp()) {
    return startNotices();
  }
  if ((await Notification.requestPermission()) !== GRANTED) {
    return { ok: false, problem: NOT_ALLOWED };
  }
  const key = PushKeySchema.safeParse(await jsonOf(await request('GET', ENDPOINTS.push)));
  if (!key.success) {
    return { ok: false, problem: NO_KEY };
  }
  const registration = await navigator.serviceWorker.ready;
  // A subscription made with another computer's key is let go first: the browser refuses a second key, and that
  // computer drops it when its next push is refused.
  if (!heldHere()) {
    const other = await registration.pushManager.getSubscription();
    await other?.unsubscribe().catch(() => false);
  }
  const subscription = await registration.pushManager
    .subscribe({ applicationServerKey: keyBytes(key.data.key), userVisibleOnly: true })
    .catch(() => undefined);
  if (!subscription) {
    return { ok: false, problem: NOT_SUBSCRIBED };
  }
  const told = PushSubscriptionSchema.safeParse(subscription.toJSON());
  const kept = told.success ? await request('POST', ENDPOINTS.push, told.data) : undefined;
  if (!kept?.ok) {
    // A subscription the daemon does not know would never be written to: the browser lets it go.
    await subscription.unsubscribe().catch(() => false);
    return { ok: false, problem: NOT_KEPT };
  }
  writeStored(HOLDER_KEY, shownId());
  return { ok: true };
}

/** Has the daemon forget this browser, then the browser let its subscription go. */
export async function turnPushOff(): Promise<PushOutcome> {
  if (inDesktopApp()) {
    await askNotifications(false);
    return { ok: true };
  }
  if (inAndroidApp()) {
    return stopNotices();
  }
  const registration = await navigator.serviceWorker.getRegistration();
  const subscription = await registration?.pushManager.getSubscription();
  if (!subscription) {
    return { ok: true };
  }
  await request('DELETE', ENDPOINTS.push, { endpoint: subscription.endpoint });
  await subscription.unsubscribe().catch(() => false);
  return { ok: true };
}

const sessionOf = (data: unknown): string | undefined => {
  if (typeof data !== 'object' || data === null || !('type' in data) || data.type !== OPEN_SESSION) {
    return undefined;
  }
  return 'sessionId' in data && typeof data.sessionId === 'string' ? data.sessionId : undefined;
};

/** Shows the session of a notification pressed while the app is open: the service worker says which. */
export function followPushes(signal: AbortSignal): void {
  if (!('serviceWorker' in navigator)) {
    return;
  }
  navigator.serviceWorker.addEventListener(
    'message',
    (event) => {
      const sessionId = sessionOf(event.data);
      if (sessionId !== undefined) {
        navigate(sessionPath(sessionId));
      }
    },
    { signal }
  );
}
