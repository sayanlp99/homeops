import { QueryClient, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { del, get, patch, post } from './api'
import type { Task } from './types'

export const tasksKey = ['tasks'] as const

export function useTasks() {
  return useQuery({ queryKey: tasksKey, queryFn: () => get<Task[]>('/tasks') })
}

export function useSprintTasks(sprintId: string | null) {
  const qc = useQueryClient()
  return useQuery({
    queryKey: ['tasks', 'sprint', sprintId],
    queryFn: () => get<Task[]>(`/tasks?sprint=${sprintId}`),
    enabled: !!sprintId,
    // Recent sprints are usually already in the working set.
    placeholderData: () => qc.getQueryData<Task[]>(tasksKey)?.filter((t) => t.sprintId === sprintId),
  })
}

export type TaskPatch = Partial<
  Pick<Task, 'title' | 'description' | 'status' | 'priority' | 'points' | 'labels' | 'assigneeId' | 'sprintId' | 'dueDate' | 'rank'>
>
export type NewTask = TaskPatch & { id: string; title: string }

const setTasks = (qc: QueryClient, fn: (ts: Task[]) => Task[]) =>
  qc.setQueryData<Task[]>(tasksKey, (old) => (old ? fn(old) : old))

function settle(qc: QueryClient) {
  // Refetch once the last in-flight task write finishes, so the UI doesn't flicker
  // between optimistic and server states during quick successive edits.
  if (qc.isMutating({ predicate: (m) => String(m.options.mutationKey?.[0]).startsWith('task.') }) <= 1) {
    qc.invalidateQueries({ queryKey: tasksKey })
    qc.invalidateQueries({ queryKey: ['sprints'] })
  }
}

/**
 * Mutation defaults are registered on the client (not in components) so writes queued
 * while offline can resume after the app is closed and reopened.
 */
export function registerTaskMutations(qc: QueryClient) {
  qc.setMutationDefaults(['task.create'], {
    mutationFn: (t: NewTask) => post<Task>('/tasks', t),
    onMutate: async (t: NewTask) => {
      await qc.cancelQueries({ queryKey: tasksKey })
      const now = new Date().toISOString()
      const draft: Task = {
        number: 0, description: '', status: 'todo', priority: 2, points: null, labels: [], assigneeId: null,
        reporterId: null, sprintId: null, dueDate: null, rank: Number.MAX_SAFE_INTEGER, recurringId: null,
        completedAt: null, completedBy: null, createdAt: now, updatedAt: now, ...t,
      }
      setTasks(qc, (ts) => [...ts, draft])
    },
    onSuccess: (saved: Task) => setTasks(qc, (ts) => ts.map((x) => (x.id === saved.id ? saved : x))),
    onError: (err: Error, t: NewTask) => {
      setTasks(qc, (ts) => ts.filter((x) => x.id !== t.id))
      toast.error(`Couldn't create task: ${err.message}`)
    },
    onSettled: () => settle(qc),
  })

  type UpdateVars = { id: string; changes: TaskPatch }
  qc.setMutationDefaults<Task, Error, UpdateVars, { prev?: Task }>(['task.update'], {
    mutationFn: ({ id, changes }) => patch<Task>(`/tasks/${id}`, changes),
    onMutate: async ({ id, changes }) => {
      await qc.cancelQueries({ queryKey: tasksKey })
      const prev = qc.getQueryData<Task[]>(tasksKey)?.find((t) => t.id === id)
      setTasks(qc, (ts) =>
        ts.map((t) => {
          if (t.id !== id) return t
          const next = { ...t, ...changes }
          if (changes.status === 'done' && t.status !== 'done') next.completedAt = new Date().toISOString()
          if (changes.status && changes.status !== 'done') next.completedAt = null
          return next
        }),
      )
      return { prev }
    },
    onSuccess: (saved) => setTasks(qc, (ts) => ts.map((x) => (x.id === saved.id ? saved : x))),
    onError: (err, _v, ctx) => {
      if (ctx?.prev) setTasks(qc, (ts) => ts.map((x) => (x.id === ctx.prev!.id ? ctx.prev! : x)))
      toast.error(`Couldn't save: ${err.message}`)
    },
    onSettled: () => settle(qc),
  })

  qc.setMutationDefaults(['task.delete'], {
    mutationFn: (id: string) => del(`/tasks/${id}`),
    onMutate: async (id: string) => {
      await qc.cancelQueries({ queryKey: tasksKey })
      const prev = qc.getQueryData<Task[]>(tasksKey)
      setTasks(qc, (ts) => ts.filter((t) => t.id !== id))
      return { prev }
    },
    onError: (err: Error, _id: string, ctx?: { prev?: Task[] }) => {
      if (ctx?.prev) qc.setQueryData(tasksKey, ctx.prev)
      toast.error(`Couldn't delete: ${err.message}`)
    },
    onSettled: () => settle(qc),
  })
}

export function useCreateTask() {
  return useMutation<Task, Error, NewTask>({ mutationKey: ['task.create'] })
}

export function useUpdateTask() {
  const m = useMutation<Task, Error, { id: string; changes: TaskPatch }>({ mutationKey: ['task.update'] })
  return (id: string, changes: TaskPatch) => m.mutate({ id, changes })
}

export function useDeleteTask() {
  const m = useMutation<void, Error, string>({ mutationKey: ['task.delete'] })
  return (id: string) => m.mutate(id)
}

/** Rank between two neighbours, for drag-and-drop reordering. */
export function rankBetween(before?: number, after?: number) {
  if (before === undefined && after === undefined) return 1024
  if (before === undefined) return after! - 1024
  if (after === undefined) return before + 1024
  return (before + after) / 2
}

export const byRank = (a: Task, b: Task) => a.rank - b.rank || a.number - b.number

export const priorities = [
  { value: 0, label: 'No priority' },
  { value: 1, label: 'Low' },
  { value: 2, label: 'Medium' },
  { value: 3, label: 'High' },
  { value: 4, label: 'Urgent' },
] as const

export const statuses = [
  { value: 'backlog', label: 'Backlog' },
  { value: 'todo', label: 'To Do' },
  { value: 'in_progress', label: 'In Progress' },
  { value: 'done', label: 'Done' },
] as const
