import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'
import { get } from './api'
import type { Bootstrap, Household, Label, Member, User } from './types'

export const bootstrapKey = ['bootstrap'] as const

export function useBootstrap() {
  return useQuery({ queryKey: bootstrapKey, queryFn: () => get<Bootstrap>('/bootstrap'), staleTime: 60_000 })
}

export interface Session {
  user: User
  household: Household
  members: Member[]
  labels: Label[]
  vapidPublicKey: string
  memberById: Map<string, Member>
  labelById: Map<string, Label>
  partner: Member | undefined
}

/** For use inside the signed-in app shell, where bootstrap is guaranteed loaded. */
export function useSession(): Session {
  const { data } = useBootstrap()
  return useMemo(() => {
    if (!data?.user) throw new Error('useSession used outside the signed-in app')
    const members = data.members ?? []
    const labels = data.labels ?? []
    return {
      user: data.user,
      household: data.household!,
      members,
      labels,
      vapidPublicKey: data.vapidPublicKey ?? '',
      memberById: new Map(members.map((m) => [m.id, m])),
      labelById: new Map(labels.map((l) => [l.id, l])),
      partner: members.find((m) => m.id !== data.user!.id),
    }
  }, [data])
}
