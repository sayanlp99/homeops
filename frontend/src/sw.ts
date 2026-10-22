/// <reference lib="webworker" />
import { clientsClaim } from 'workbox-core'
import { cleanupOutdatedCaches, createHandlerBoundToURL, precacheAndRoute } from 'workbox-precaching'
import { NavigationRoute, registerRoute } from 'workbox-routing'
import { CacheFirst } from 'workbox-strategies'
import { ExpirationPlugin } from 'workbox-expiration'

declare const self: ServiceWorkerGlobalScope & { __WB_MANIFEST: Array<{ url: string; revision: string | null }> }

// App shell: every built asset is precached, so the app opens instantly and offline.
cleanupOutdatedCaches()
precacheAndRoute(self.__WB_MANIFEST)
registerRoute(new NavigationRoute(createHandlerBoundToURL('/index.html'), { denylist: [/^\/api\//] }))

// Recipe photos never change for a given ID: cache them so the library works offline.
registerRoute(
  ({ url }) => url.pathname.startsWith('/api/files/'),
  new CacheFirst({ cacheName: 'photos', plugins: [new ExpirationPlugin({ maxEntries: 400, maxAgeSeconds: 180 * 24 * 3600 })] }),
)

self.addEventListener('message', (e) => {
  if (e.data?.type === 'SKIP_WAITING') self.skipWaiting()
})
clientsClaim()

interface PushPayload {
  id: string
  title: string
  body: string
  url: string
  tag: string
  badge: number
}

// Every push must show a notification: iOS revokes push permission for apps that
// receive pushes silently.
self.addEventListener('push', (event) => {
  let p: PushPayload
  try {
    p = event.data!.json() as PushPayload
  } catch {
    p = { id: String(Date.now()), title: 'HomeOps', body: event.data?.text() ?? '', url: '/', tag: 'generic', badge: 0 }
  }
  const nav = self.navigator as WorkerNavigator & { setAppBadge?: (n?: number) => Promise<void> }
  event.waitUntil(
    Promise.all([
      self.registration.showNotification(p.title, {
        body: p.body,
        tag: p.id,
        icon: '/pwa-192x192.png',
        badge: '/pwa-64x64.png',
        data: { url: p.url },
      }),
      p.badge > 0 ? nav.setAppBadge?.(p.badge).catch(() => {}) : Promise.resolve(),
      // Let open tabs refresh their inbox immediately.
      self.clients.matchAll({ type: 'window' }).then((cs) => cs.forEach((c) => c.postMessage({ type: 'push', id: p.id }))),
    ]),
  )
})

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  const url = (event.notification.data?.url as string) || '/'
  event.waitUntil(
    (async () => {
      const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true })
      const existing = windows.find((w) => new URL(w.url).origin === self.location.origin)
      if (existing) {
        await existing.focus()
        existing.postMessage({ type: 'navigate', url })
        return
      }
      await self.clients.openWindow(url)
    })(),
  )
})

// Browsers occasionally rotate subscriptions; re-register without waiting for the app to open.
self.addEventListener('pushsubscriptionchange', (event) => {
  const e = event as Event & { oldSubscription?: PushSubscription; waitUntil(p: Promise<unknown>): void }
  e.waitUntil(
    (async () => {
      const key = e.oldSubscription?.options.applicationServerKey
      if (!key) return
      const sub = await self.registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: key })
      await fetch('/api/push/subscribe', {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json', 'X-Requested-With': 'homeops' },
        body: JSON.stringify(sub.toJSON()),
      })
    })(),
  )
})
