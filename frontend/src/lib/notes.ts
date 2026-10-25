import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { del, get, patch, post } from './api'
import { uuid } from './uuid'

export type NoteColor = 'yellow' | 'pink' | 'blue' | 'green' | 'orange'

export const NOTE_COLORS: { value: NoteColor; label: string }[] = [
  { value: 'yellow', label: 'Yellow' },
  { value: 'pink', label: 'Pink' },
  { value: 'blue', label: 'Blue' },
  { value: 'green', label: 'Green' },
  { value: 'orange', label: 'Orange' },
]

export interface NoteItem {
  id: string
  text: string
  done: boolean
}

export interface FridgeNote {
  id: string
  authorId: string | null
  text: string
  color: NoteColor
  pinned: boolean
  /** Shown through this day (YYYY-MM-DD), then taken down. */
  expiresOn: string | null
  /** A checklist; `text` is then its (optional) title. */
  items: NoteItem[]
  createdAt: string
  updatedAt: string
}

export type NotePatch = Partial<Pick<FridgeNote, 'text' | 'color' | 'pinned' | 'expiresOn' | 'items'>>

// Notes cached on the device (offline support) may predate newer fields.
const normalize = (list: FridgeNote[]) => list.map((n) => ({ ...n, expiresOn: n.expiresOn ?? null, items: n.items ?? [] }))

export const useNotes = () => useQuery({ queryKey: ['notes'], queryFn: () => get<FridgeNote[]>('/notes'), select: normalize })

/** Pinned first, then newest — the order the server sends. */
function sorted(list: FridgeNote[]) {
  return [...list].sort((a, b) => Number(b.pinned) - Number(a.pinned) || b.createdAt.localeCompare(a.createdAt))
}

export function useNoteActions() {
  const qc = useQueryClient()
  const optimistic = (change: (list: FridgeNote[]) => FridgeNote[]) => {
    const before = qc.getQueryData<FridgeNote[]>(['notes'])
    qc.setQueryData<FridgeNote[]>(['notes'], (l = []) => sorted(change(l)))
    return { before }
  }
  const settle = {
    onError: (e: Error, _v: unknown, ctx?: { before?: FridgeNote[] }) => {
      if (ctx?.before) qc.setQueryData(['notes'], ctx.before)
      toast.error(e.message)
    },
    onSettled: () => qc.invalidateQueries({ queryKey: ['notes'] }),
  }
  const create = useMutation({
    mutationFn: (n: { id: string; text: string; color: NoteColor; pinned: boolean; expiresOn: string | null; items: NoteItem[]; authorId: string }) => post<FridgeNote>('/notes', n),
    onMutate: async (n) => {
      await qc.cancelQueries({ queryKey: ['notes'] })
      const now = new Date().toISOString()
      return optimistic((l) => [{ ...n, createdAt: now, updatedAt: now }, ...l])
    },
    ...settle,
  })
  const update = useMutation({
    mutationFn: ({ id, changes }: { id: string; changes: NotePatch }) => patch<FridgeNote>(`/notes/${id}`, changes),
    onMutate: async ({ id, changes }) => {
      await qc.cancelQueries({ queryKey: ['notes'] })
      return optimistic((l) => l.map((n) => (n.id === id ? { ...n, ...changes } : n)))
    },
    ...settle,
  })
  const tick = useMutation({
    mutationFn: ({ id, item, done }: { id: string; item: string; done: boolean }) => patch<FridgeNote>(`/notes/${id}/items/${item}`, { done }),
    onMutate: async ({ id, item, done }) => {
      await qc.cancelQueries({ queryKey: ['notes'] })
      return optimistic((l) => l.map((n) => (n.id === id ? { ...n, items: (n.items ?? []).map((it) => (it.id === item ? { ...it, done } : it)) } : n)))
    },
    ...settle,
  })
  const remove = useMutation({
    mutationFn: (id: string) => del(`/notes/${id}`),
    onMutate: async (id) => {
      await qc.cancelQueries({ queryKey: ['notes'] })
      return optimistic((l) => l.filter((n) => n.id !== id))
    },
    ...settle,
  })
  return {
    add: (authorId: string, n: { text: string; color: NoteColor; pinned: boolean; expiresOn: string | null; items: NoteItem[] }) => create.mutate({ ...n, id: uuid(), authorId }),
    update: (id: string, changes: NotePatch) => update.mutate({ id, changes }),
    tick: (id: string, item: string, done: boolean) => tick.mutate({ id, item, done }),
    remove: (id: string) => remove.mutate(id),
  }
}
