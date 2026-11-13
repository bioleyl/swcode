import { Bus } from '@jadis/core';

import type { InstallPromptEvent, InstallState } from '../types/install.types';

const WORKER_PATH = '/sw.js';
const BEFORE_INSTALL = 'beforeinstallprompt';
const INSTALLED = 'appinstalled';
const STANDALONE_QUERY = '(display-mode: standalone)';

// The browser's offer to install, kept until the person asks for it: a page may show it once only.
const held: { offer: InstallPromptEvent | undefined } = { offer: undefined };

/** The install state changed: an offer came, or the app was installed. */
export const installBus = new Bus<{ changed: undefined }>();

const isOffer = (event: Event): event is InstallPromptEvent => 'prompt' in event;

/**
 * Whether swcode is installed on this device, can be with a press, can be from the browser's own menu, or cannot:
 * a browser installs an app only from a page it trusts, HTTPS or this computer itself.
 */
export function installState(): InstallState {
  if (window.matchMedia(STANDALONE_QUERY).matches) {
    return 'installed';
  }
  if (!window.isSecureContext) {
    return 'insecure';
  }
  return held.offer ? 'offered' : 'manual';
}

/** Shows the browser's own install dialog, once it offered one. */
export async function installApp(): Promise<void> {
  const offer = held.offer;
  held.offer = undefined;
  await offer?.prompt();
  installBus.emit('changed');
}

/**
 * Makes the page an app a phone installs: its service worker keeps the app's own files, so it opens at once and
 * says so itself when the daemon cannot be reached; the browser's offer to install is kept for the settings to
 * show. Only where a browser allows it, and never while developing: the worker would keep the dev server's files.
 */
export function startApp(signal: AbortSignal): void {
  window.addEventListener(
    BEFORE_INSTALL,
    (event) => {
      if (!isOffer(event)) {
        return;
      }
      event.preventDefault();
      held.offer = event;
      installBus.emit('changed');
    },
    { signal }
  );
  window.addEventListener(INSTALLED, () => installBus.emit('changed'), { signal });
  if (!import.meta.env.PROD || !window.isSecureContext || !('serviceWorker' in navigator)) {
    return;
  }
  // A worker that cannot be registered leaves the app as a page: it works the same, without its own copy.
  void navigator.serviceWorker.register(WORKER_PATH).catch(() => undefined);
}
