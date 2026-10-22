import { MutationCache, QueryCache, QueryClient } from '@tanstack/react-query'
import { createAsyncStoragePersister } from '@tanstack/query-async-storage-persister'
import { get as idbGet, set as idbSet, del as idbDel } from 'idb-keyval'
import { ApiError } from './api'
import { registerMealMutations } from './meals'
import { registerMoneyMutations } from './money'
import { registerTaskMutations } from './tasks'

// A 401 anywhere means the session ended (expired or signed out elsewhere): refresh
// bootstrap so the app shows the sign-in screen.
const onError = (err: unknown) => {
  if (err instanceof ApiError && err.status === 401) queryClient.invalidateQueries({ queryKey: ['bootstrap'] })
}

export const queryClient: QueryClient = new QueryClient({
  queryCache: new QueryCache({ onError }),
  mutationCache: new MutationCache({ onError }),
  defaultOptions: {
    queries: {
      // Show cached data instantly; realtime events invalidate what actually changed.
      staleTime: 30_000,
      gcTime: 7 * 24 * 60 * 60 * 1000,
      refetchOnWindowFocus: true,
      retry: (count, err) => !(err instanceof ApiError && err.status >= 400 && err.status < 500) && count < 2,
    },
    mutations: {
      retry: (count, err) => err instanceof ApiError && err.status === 0 && count < 3,
    },
  },
})

registerTaskMutations(queryClient)
registerMealMutations(queryClient)
registerMoneyMutations(queryClient)

// The cache (and any writes queued while offline) lives in IndexedDB so the app opens
// instantly with the last known data, even without a connection.
export const persister = createAsyncStoragePersister({
  storage: {
    getItem: (k) => idbGet(k),
    setItem: (k, v) => idbSet(k, v),
    removeItem: (k) => idbDel(k),
  },
  key: 'homeops-cache',
  throttleTime: 500,
})

export const CACHE_BUSTER = 'v1'

/** Wipe local data, e.g. on sign-out, so the next user starts clean. */
export async function clearLocalData() {
  queryClient.clear()
  await idbDel('homeops-cache')
}
