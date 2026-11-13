// swcode's service worker: the app opens from the device's own copy, at once, and where the daemon cannot be reached
// it still opens and says so itself. Nothing the daemon says is ever kept here: /api is left to the network.
const CACHE = 'swcode-shell-v2';
const PAGE = '/';
const ASSETS = '/assets/';
const API = '/api/';
// How long the page is waited for before the copy kept from the last visit shows instead.
const PAGE_WAIT_MS = 3000;

self.addEventListener('install', () => self.skipWaiting());

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((names) => Promise.all(names.filter((name) => name !== CACHE).map((name) => caches.delete(name))))
      .then(() => self.clients.claim())
  );
});

const keep = async (key, response) => {
  if (response.ok) {
    const cache = await caches.open(CACHE);
    await cache.put(key, response.clone());
  }
  return response;
};

// A file named after its content never changes: the copy kept is the file.
const assetOf = async (request) => (await caches.match(request)) ?? keep(request, await fetch(request));

// Any other file, the page first of all: the network's when it answers in time, so a new build shows at once;
// the copy kept otherwise.
const freshOf = async (key, request) => {
  const kept = await caches.match(key);
  const fetched = fetch(request).then((response) => keep(key, response));
  if (!kept) {
    return fetched;
  }
  const late = new Promise((resolve) => setTimeout(() => resolve(kept), PAGE_WAIT_MS));
  return Promise.race([fetched.catch(() => kept), late]);
};

// A push from the daemon: a session waits for its person, or ended its turn. What it says was encrypted for this
// browser alone, and is shown as it is.
const APP_NAME = 'swcode';
const ICON = '/icon-192.png';
const SESSION_PAGE = '/#/sessions/';
// What the page is told when a notification of one of its sessions is pressed: it goes there itself.
const OPEN_SESSION = 'openSession';

const noticeOf = (event) => {
  try {
    return event.data ? event.data.json() : {};
  } catch {
    return {};
  }
};

self.addEventListener('push', (event) => {
  const notice = noticeOf(event);
  const sessionId = typeof notice.sessionId === 'string' ? notice.sessionId : '';
  event.waitUntil(
    self.registration.showNotification(typeof notice.title === 'string' && notice.title ? notice.title : APP_NAME, {
      body: typeof notice.body === 'string' ? notice.body : '',
      data: { sessionId },
      icon: ICON,
      // One notification a session: its latest word takes the place of the one before.
      tag: sessionId || APP_NAME,
    })
  );
});

// Pressing a notification shows its session: in the window that is open, which goes there itself, else in a new one.
const showSession = async (sessionId) => {
  const windows = await self.clients.matchAll({ includeUncontrolled: true, type: 'window' });
  const open = windows[0];
  if (!open) {
    return self.clients.openWindow(sessionId ? SESSION_PAGE + encodeURIComponent(sessionId) : PAGE);
  }
  if (sessionId) {
    open.postMessage({ sessionId, type: OPEN_SESSION });
  }
  return open.focus();
};

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const sessionId = event.notification.data?.sessionId;
  event.waitUntil(showSession(typeof sessionId === 'string' ? sessionId : ''));
});

self.addEventListener('fetch', (event) => {
  const request = event.request;
  const url = new URL(request.url);
  if (request.method !== 'GET' || url.origin !== self.location.origin || url.pathname.startsWith(API)) {
    return;
  }
  if (url.pathname.startsWith(ASSETS)) {
    event.respondWith(assetOf(request));
    return;
  }
  // Every address of the app is the same page: one copy serves them all.
  event.respondWith(freshOf(request.mode === 'navigate' ? PAGE : request, request));
});
