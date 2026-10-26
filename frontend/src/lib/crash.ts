import { del as idbDel } from "idb-keyval";

const sent = new Set<string>();

/** Send an error to the server log, once per message, so blank screens on a phone can be diagnosed. */
export function reportError(err: unknown) {
  const e = err instanceof Error ? err : new Error(String(err));
  if (sent.has(e.message) || sent.size > 20) return;
  sent.add(e.message);
  fetch("/api/client-errors", {
    method: "POST",
    keepalive: true,
    headers: {
      "Content-Type": "application/json",
      "X-Requested-With": "homeops",
    },
    body: JSON.stringify({
      message: e.message,
      stack: e.stack ?? "",
      url: location.pathname,
      build: __BUILD_ID__,
    }),
  }).catch(() => {});
}

/** Last resort: drop saved data and the installed copy of the app, then load it fresh. */
export async function resetApp() {
  try {
    await idbDel("homeops-cache");
    for (const r of (await navigator.serviceWorker?.getRegistrations()) ?? [])
      await r.unregister();
    for (const k of (await caches?.keys()) ?? []) await caches.delete(k);
  } finally {
    location.replace("/");
  }
}

export function reportUncaught() {
  window.addEventListener("error", (e) => reportError(e.error ?? e.message));
  window.addEventListener("unhandledrejection", (e) => reportError(e.reason));
}
