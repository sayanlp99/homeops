import { PersistQueryClientProvider } from '@tanstack/react-query-persist-client'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { Toaster, toast } from 'sonner'
import { registerSW } from 'virtual:pwa-register'
import { App } from './App'
import { CACHE_BUSTER, persister, queryClient } from './lib/queryClient'
import { useTheme } from './lib/theme'
import './index.css'

// A new version waits until the user agrees, so nobody loses a half-typed form.
const updateSW = registerSW({
  onNeedRefresh() {
    toast('A new version of HomeOps is ready', {
      duration: Infinity,
      action: { label: 'Reload', onClick: () => updateSW(true) },
    })
  },
  onRegisteredSW(_url, reg) {
    // Check for updates hourly and whenever the app comes back to the foreground.
    if (!reg) return
    setInterval(() => reg.update().catch(() => {}), 60 * 60 * 1000)
    document.addEventListener('visibilitychange', () => document.visibilityState === 'visible' && reg.update().catch(() => {}))
  },
})

function Root() {
  const [theme] = useTheme()
  return (
    <PersistQueryClientProvider
      client={queryClient}
      persistOptions={{ persister, buster: CACHE_BUSTER, maxAge: 7 * 24 * 60 * 60 * 1000 }}
      onSuccess={() => {
        // Cached data renders instantly; replay queued offline writes, then refresh in the
        // background in case anything changed while the app was closed.
        queryClient.resumePausedMutations().finally(() => queryClient.invalidateQueries())
      }}
    >
      <BrowserRouter>
        <App />
      </BrowserRouter>
      <Toaster position="top-center" theme={theme} closeButton toastOptions={{ className: 'text-[13px]' }} offset={{ top: 'calc(env(safe-area-inset-top) + 56px)' }} />
    </PersistQueryClientProvider>
  )
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <Root />
  </StrictMode>,
)
