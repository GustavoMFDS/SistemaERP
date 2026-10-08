/* App-shell only. Never cache API responses, credentials, fiscal documents or POSTs. */
const CACHE = 'sistemaemgo-app-shell-v1'
self.addEventListener('install', (event) => {
  event.waitUntil(self.skipWaiting())
})
self.addEventListener('activate', (event) => {
  event.waitUntil((async () => {
    const keys = await caches.keys()
    await Promise.all(keys.filter((key) => key.startsWith('sistemaemgo-app-shell-') && key !== CACHE).map((key) => caches.delete(key)))
    await self.clients.claim()
  })())
})
self.addEventListener('fetch', (event) => {
  const request = event.request
  if (request.method !== 'GET') return
  const url = new URL(request.url)
  if (url.origin !== self.location.origin) return
  if (url.pathname.startsWith('/api/') || url.pathname.startsWith('/health/') ||
      url.pathname.startsWith('/metrics') || url.pathname.endsWith('.xml') ||
      url.pathname.endsWith('.pdf')) return
  const isNavigation = request.mode === 'navigate'
  const staticAsset = url.pathname.startsWith('/assets/') || url.pathname === '/app-icon.svg'
  if (!isNavigation && !staticAsset) return
  event.respondWith((async () => {
    const cache = await caches.open(CACHE)
    try {
      const result = await fetch(request)
      if (result.ok && result.type === 'basic') {
        try {
          await cache.put(isNavigation ? '/' : request, result.clone())
        } catch {
          // A full or blocked cache must not turn a successful fetch into a failed request.
        }
      }
      return result
    } catch (error) {
      const cached = await cache.match(isNavigation ? '/' : request)
      if (cached) return cached
      throw error
    }
  })())
})
