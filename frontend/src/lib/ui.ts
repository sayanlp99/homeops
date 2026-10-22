import { useCallback, useSyncExternalStore } from 'react'
import { useSearchParams } from 'react-router-dom'
import type { TaskPatch } from './tasks'
import type { Task } from './types'

/** Tiny global store for app-wide UI state (quick-add dialog, command palette). */
function store<T>(initial: T) {
  let value = initial
  const subs = new Set<() => void>()
  return {
    get: () => value,
    set: (v: T) => {
      value = v
      subs.forEach((s) => s())
    },
    use: () =>
      useSyncExternalStore(
        (cb) => {
          subs.add(cb)
          return () => subs.delete(cb)
        },
        () => value,
      ),
  }
}

export const quickAdd = store<{ open: boolean; defaults: TaskPatch }>({ open: false, defaults: {} })
export const openQuickAdd = (defaults: TaskPatch = {}) => quickAdd.set({ open: true, defaults })

export const palette = store(false)

/** The task drawer is driven by ?task=<number|id> so it works from any page and from links. */
export function useTaskParam() {
  const [params, setParams] = useSearchParams()
  const value = params.get('task')
  const open = useCallback(
    (t: Pick<Task, 'id' | 'number'>) =>
      setParams((p) => {
        p.set('task', t.number > 0 ? String(t.number) : t.id)
        return p
      }),
    [setParams],
  )
  const close = useCallback(
    () =>
      setParams((p) => {
        p.delete('task')
        return p
      }),
    [setParams],
  )
  return { value, open, close }
}

export function findTask(tasks: Task[] | undefined, param: string | null) {
  if (!param || !tasks) return undefined
  return /^\d+$/.test(param) ? tasks.find((t) => t.number === Number(param)) : tasks.find((t) => t.id === param)
}

export const quickExpense = store<{ open: boolean; defaults: { categoryId?: string | null; amount?: number; payee?: string } }>({ open: false, defaults: {} })
export const openQuickExpense = (defaults: { categoryId?: string | null; amount?: number; payee?: string } = {}) => quickExpense.set({ open: true, defaults })
