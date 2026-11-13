import { ENDPOINTS } from '../../../shared/protocol/endpoints';
import { PushKeySchema, PushSubscriptionSchema } from '../../../shared/protocol/pairing.schema';
import { isAppleMobile, keyBytes } from '../lib/push-view';
import { jsonOf, request } from './api';
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

const canPush = (): boolean => 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window;

/** Where notifications stand on this device. */
export async function pushState(): Promise<PushState> {
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
  return (await registration.pushManager.getSubscription()) ? 'on' : 'off';
}

/** Subscribes this browser to its push service with the daemon's key, and has the daemon keep the subscription. */
export async function turnPushOn(): Promise<PushOutcome> {
  if ((await Notification.requestPermission()) !== GRANTED) {
    return { ok: false, problem: NOT_ALLOWED };
  }
  const key = PushKeySchema.safeParse(await jsonOf(await request('GET', ENDPOINTS.push)));
  if (!key.success) {
    return { ok: false, problem: NO_KEY };
  }
  const registration = await navigator.serviceWorker.ready;
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
  return { ok: true };
}

/** Has the daemon forget this browser, then the browser let its subscription go. */
export async function turnPushOff(): Promise<PushOutcome> {
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
