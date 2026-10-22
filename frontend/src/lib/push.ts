import { post } from './api'
import type { PushResult } from './types'

export const isIOS = () =>
  /iPad|iPhone|iPod/.test(navigator.userAgent) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1)

export const isStandalone = () =>
  window.matchMedia('(display-mode: standalone)').matches || (navigator as { standalone?: boolean }).standalone === true

export type PushSupport = 'supported' | 'needs-install' | 'unsupported'

export function pushSupport(): PushSupport {
  // iOS only exposes Web Push to apps added to the Home Screen.
  if (isIOS() && !isStandalone()) return 'needs-install'
  if (!('serviceWorker' in navigator) || !('PushManager' in window) || !('Notification' in window)) return 'unsupported'
  return 'supported'
}

function urlBase64ToUint8Array(base64: string) {
  const padding = '='.repeat((4 - (base64.length % 4)) % 4)
  const raw = atob((base64 + padding).replace(/-/g, '+').replace(/_/g, '/'))
  return Uint8Array.from(raw, (c) => c.charCodeAt(0))
}

function sameKey(a: ArrayBuffer | null | undefined, b: Uint8Array) {
  if (!a) return false
  const x = new Uint8Array(a)
  return x.length === b.length && x.every((v, i) => v === b[i])
}

async function subscribe(vapidKey: string) {
  const reg = await navigator.serviceWorker.ready
  const key = urlBase64ToUint8Array(vapidKey)
  let sub = await reg.pushManager.getSubscription()
  // If the server's keys changed, the old subscription can't receive our pushes.
  if (sub && !sameKey(sub.options.applicationServerKey, key)) {
    await sub.unsubscribe()
    sub = null
  }
  sub ??= await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: key })
  await post('/push/subscribe', sub.toJSON())
  return sub
}

/** Ask for permission (must run from a click) and register this device. */
export async function enablePush(vapidKey: string) {
  const perm = await Notification.requestPermission()
  if (perm !== 'granted') throw new Error('Notifications were blocked. Allow them in your browser or system settings.')
  await subscribe(vapidKey)
}

export async function disablePush() {
  const reg = await navigator.serviceWorker.ready
  const sub = await reg.pushManager.getSubscription()
  if (sub) {
    await post('/push/unsubscribe', { endpoint: sub.endpoint })
    await sub.unsubscribe()
  }
}

/**
 * Called on every app start: if permission is granted, make sure this device has a live
 * subscription registered to the current user. Browsers occasionally rotate or drop
 * subscriptions, and this is what keeps delivery reliable.
 */
export async function syncPush(vapidKey: string) {
  if (pushSupport() !== 'supported' || Notification.permission !== 'granted' || !vapidKey) return
  try {
    await subscribe(vapidKey)
  } catch (e) {
    console.warn('push sync failed', e)
  }
}

export async function currentSubscription() {
  if (pushSupport() !== 'supported') return null
  const reg = await navigator.serviceWorker.getRegistration()
  return (await reg?.pushManager.getSubscription()) ?? null
}

export const sendTestPush = () => post<PushResult[]>('/push/test')
