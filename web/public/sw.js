// Minimal app-shell service worker. Never caches /api/ responses — financial
// data must always come from the network. Only static assets and the HTML
// shell are cached, so the app can install and reopen offline.
const CACHE_NAME = 'myfi-static-v2'

self.addEventListener('install', (event) => {
  self.skipWaiting()
})

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((names) =>
      Promise.all(names.filter((n) => n !== CACHE_NAME).map((n) => caches.delete(n))),
    ).then(() => self.clients.claim()),
  )
})

self.addEventListener('fetch', (event) => {
  const { request } = event
  if (request.method !== 'GET') return

  const url = new URL(request.url)
  if (url.origin !== self.location.origin) return
  if (url.pathname.startsWith('/api/')) return

  if (request.mode === 'navigate') {
    event.respondWith(
      fetch(request)
        .then((res) => {
          caches.open(CACHE_NAME).then((cache) => cache.put(request, res.clone()))
          return res
        })
        .catch(() => caches.match(request).then((res) => res || caches.match('/'))),
    )
    return
  }

  // Build assets have a content hash in their name and never change, so a
  // cached copy is final; only fetch them when missing.
  if (url.pathname.startsWith('/assets/')) {
    event.respondWith(
      caches.match(request).then(
        (cached) =>
          cached ||
          fetch(request).then((res) => {
            if (res.ok) caches.open(CACHE_NAME).then((cache) => cache.put(request, res.clone()))
            return res
          }),
      ),
    )
    return
  }

  event.respondWith(
    caches.match(request).then((cached) => {
      const network = fetch(request)
        .then((res) => {
          if (res.ok) caches.open(CACHE_NAME).then((cache) => cache.put(request, res.clone()))
          return res
        })
        .catch(() => cached)
      return cached || network
    }),
  )
})
