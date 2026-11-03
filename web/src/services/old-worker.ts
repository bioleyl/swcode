// The app's files were kept by a service worker once: what it left in a browser that had it goes away.
const OLD_CACHES = ['swcode-app-v1'];

/** Only over HTTPS or on localhost could a page have had one, and only there do the caches exist. */
export function dropServiceWorker(): void {
  if (!window.isSecureContext || !('serviceWorker' in navigator)) {
    return;
  }
  void navigator.serviceWorker
    .getRegistrations()
    .then((registrations) => Promise.all(registrations.map((registration) => registration.unregister())))
    .catch(() => undefined);
  void Promise.all(OLD_CACHES.map((name) => caches.delete(name))).catch(() => undefined);
}
