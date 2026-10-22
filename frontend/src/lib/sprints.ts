import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { get, patch, post } from './api'
import type { Sprint } from './types'

export function useSprints() {
  const q = useQuery({ queryKey: ['sprints'], queryFn: () => get<Sprint[]>('/sprints') })
  const sprints = q.data ?? []
  return {
    ...q,
    sprints,
    active: sprints.find((s) => s.status === 'active'),
    planned: sprints.filter((s) => s.status === 'planned').sort((a, b) => a.startsOn.localeCompare(b.startsOn))[0],
    completed: sprints.filter((s) => s.status === 'completed'),
  }
}

function useSprintAction<V>(fn: (v: V) => Promise<unknown>, success: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      toast.success(success)
      qc.invalidateQueries({ queryKey: ['sprints'] })
      qc.invalidateQueries({ queryKey: ['tasks'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })
}

export const useStartSprint = () => useSprintAction((id: string) => post(`/sprints/${id}/start`), 'Sprint started')

export const useCompleteSprint = () =>
  useSprintAction(
    ({ id, moveTo }: { id: string; moveTo: 'next' | 'backlog' }) => post(`/sprints/${id}/complete`, { moveTo }),
    'Sprint completed',
  )

export const useUpdateSprintGoal = () =>
  useSprintAction(({ id, goal }: { id: string; goal: string }) => patch(`/sprints/${id}`, { goal }), 'Sprint goal saved')
