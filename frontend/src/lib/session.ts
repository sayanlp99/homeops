import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'
import { get } from './api'
import type { Bootstrap, Household, Label, Member, MyHousehold, Role, User } from './types'

export const bootstrapKey = ['bootstrap'] as const

export function useBootstrap() {
  return useQuery({ queryKey: bootstrapKey, queryFn: () => get<Bootstrap>('/bootstrap'), staleTime: 60_000 })
}

export interface Session {
  user: User
  household: Household
  members: Member[]
  labels: Label[]
  households: MyHousehold[]
  mailEnabled: boolean
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
      households: data.households ?? [],
      mailEnabled: data.mailEnabled ?? false,
      vapidPublicKey: data.vapidPublicKey ?? '',
      memberById: new Map(members.map((m) => [m.id, m])),
      labelById: new Map(labels.map((l) => [l.id, l])),
      partner: members.find((m) => m.id !== data.user!.id),
    }
  }, [data])
}

const rank: Record<Role, number> = { viewer: 1, member: 2, admin: 3, owner: 4 }

/** Whether the signed-in person has at least this role in the current household. */
export function hasRole(user: User, role: Role) {
  return user.role !== '' && rank[user.role] >= rank[role]
}

export const roleLabels: Record<Role, string> = { owner: 'Owner', admin: 'Admin', member: 'Member', viewer: 'Viewer' }
export const roleHelp: Record<Role, string> = {
  owner: 'Everything, including managing admins and deleting the household',
  admin: 'Household settings, invites and members',
  member: 'Tasks, meals, grocery and budget',
  viewer: 'Can look but not change anything',
}
