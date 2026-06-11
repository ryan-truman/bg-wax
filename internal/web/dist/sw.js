// Minimal service worker — required for Chrome PWA installability.
// No caching strategy: all requests pass through to the network so the
// Go server is always the source of truth.
self.addEventListener('fetch', event => {
  event.respondWith(fetch(event.request))
})
