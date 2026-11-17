import { registerPlugin } from '@capacitor/core';

import { NoticeTracker } from '../lib/notice-tracker';
import { daemonBus } from './daemon';
import { inAndroidApp } from './native-app';
import { navigate, sessionPath } from './navigation';

import type { Notice } from '../lib/notice-tracker';

// The Android app asks its own permission once, and keeps telling after that: what the person picks in Android's
// settings is what the app finds again, so nothing of it is kept here.
const PERMISSION = 'notifications';
// The app shows again: Android hands over the session a notification tapped while it slept.
const RESUME = 'resume';
const VISIBLE = 'visible';

/** The Android app's own plugin (mobile/android/app/src/main/java/com/bioleyl/swcode/NotifierPlugin.java). */
const Notifier = registerPlugin<{
  requestPermissions(): Promise<Partial<Record<typeof PERMISSION, string>>>;
  show(notice: { body: string; sessionId: string; title: string }): Promise<void>;
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
 * same notices the desktop app's tray shows. A tap on one opens the session it came from.
 */
export function followNotices(signal: AbortSignal): void {
  if (!inAndroidApp()) {
    return;
  }
  void Notifier.requestPermissions().catch(() => undefined);
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

/** The session the last tapped notification named, once the app is back to the front. */
async function openTapped(): Promise<void> {
  const taken = await Notifier.takeSession().catch(() => undefined);
  const sessionId = taken?.sessionId;
  if (sessionId) {
    navigate(sessionPath(sessionId));
  }
}
