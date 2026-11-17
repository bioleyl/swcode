import { registerPlugin } from '@capacitor/core';

import { NoticeTracker } from '../lib/notice-tracker';
import { daemonBus } from './daemon';
import { inAndroidApp } from './native-app';
import { navigate, sessionPath } from './navigation';

import type { Notice } from '../lib/notice-tracker';
import type { PushOutcome } from '../types/push.types';

// The app shows again: Android hands over the session a notification tapped while it slept.
const RESUME = 'resume';
const VISIBLE = 'visible';
const NO_SERVICE = 'Android did not take the app to its front: the phone is not kept awake.';

/** The Android app's own plugin (mobile/android/app/src/main/java/com/bioleyl/swcode/NotifierPlugin.java). */
const Notifier = registerPlugin<{
  requestPermissions(): Promise<unknown>;
  start(): Promise<unknown>;
  status(): Promise<{ running?: boolean }>;
  stop(): Promise<unknown>;
  show(notice: { body: string; sessionId: string; title: string }): Promise<unknown>;
  takeSession(): Promise<{ sessionId?: string }>;
}>('Notifier');

/** Tells this device, through Android itself, what a session just said. */
export function notify(notice: Notice): void {
  if (!inAndroidApp()) {
    return;
  }
  void Notifier.show({ body: notice.text, sessionId: notice.sessionId, title: notice.title }).catch(
    () => undefined
  );
}

/**
 * Follows the daemon's socket and notifies this phone through Android, for the app that has no web push service: the
 * same notices the desktop app's tray shows. A tap on one opens the session it came from, and the app goes to
 * Android's front so the phone keeps being told while it sleeps.
 */
export function followNotices(signal: AbortSignal): void {
  if (!inAndroidApp()) {
    return;
  }
  void startNotices();
  const tracker = new NoticeTracker();
  daemonBus.register(
    'message',
    (message) => {
      const notice = tracker.take(message);
      if (notice) {
        notify(notice);
      }
    },
    signal
  );
  const opened = () => {
    if (document.visibilityState === VISIBLE) {
      void openTapped();
    }
  };
  document.addEventListener('visibilitychange', opened, { signal });
  window.addEventListener(RESUME, opened, { signal });
}

/** Whether Android keeps the app at its front, which is what a sleeping phone is told by. */
export async function noticesRunning(): Promise<boolean> {
  const status = await Notifier.status().catch(() => undefined);
  return status?.running === true;
}

/** Goes to Android's front, and asks its notification permission with it. */
export async function startNotices(): Promise<PushOutcome> {
  const started = await Notifier.start().then(
    () => true,
    () => false
  );
  return started ? { ok: true } : { ok: false, problem: NO_SERVICE };
}

/** Lets Android freeze the app again: fewer notices, a phone that sleeps deeper. */
export async function stopNotices(): Promise<PushOutcome> {
  const stopped = await Notifier.stop().then(
    () => true,
    () => false
  );
  return stopped ? { ok: true } : { ok: false, problem: NO_SERVICE };
}

/** The session the last tapped notification named, once the app is back to the front. */
async function openTapped(): Promise<void> {
  const taken = await Notifier.takeSession().catch(() => undefined);
  const sessionId = taken?.sessionId;
  if (sessionId) {
    navigate(sessionPath(sessionId));
  }
}
