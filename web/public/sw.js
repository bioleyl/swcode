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
