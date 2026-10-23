import { post } from './api'
import { clearLocalData, queryClient } from './queryClient'

/**
 * Move this session into another household. Cached data belongs to the old one, so
 * it's wiped and the app reloads from scratch.
 */
export async function switchHousehold(id: string, to = '/') {
  if (queryClient.getMutationCache().getAll().some((m) => m.state.isPaused)) {
    throw new Error("Some changes haven't synced yet. Reconnect and try again.")
  }
  await post('/me/household', { householdId: id })
  await clearLocalData()
  location.assign(to)
}
