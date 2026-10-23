import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Copy, Mail, Trash2, UserMinus } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { Menu, MenuItem, MenuLabel } from '@/components/Menu'
import { Avatar, Badge, Button, Card, CardHeader, Field, IconButton, Input, Select } from '@/components/ui'
import { api, del, get, patch, post } from '@/lib/api'
import { timeAgo } from '@/lib/format'
import { clearLocalData } from '@/lib/queryClient'
import { bootstrapKey, hasRole, roleHelp, roleLabels, useSession } from '@/lib/session'
import type { InviteResult, Member, PendingInvite, Role } from '@/lib/types'

const ROLES: Role[] = ['owner', 'admin', 'member', 'viewer']
const rank: Record<Role, number> = { viewer: 1, member: 2, admin: 3, owner: 4 }

export function MembersSettings() {
  const { members, user } = useSession()
  const canManage = hasRole(user, 'admin')
  return (
    <>
      <Card>
        <CardHeader title="Members" subtitle={`${members.length} ${members.length === 1 ? 'person' : 'people'} in this household`} />
        <ul>
          {members.map((m) => (
            <MemberRow key={m.id} m={m} />
          ))}
        </ul>
      </Card>
      {canManage && <InviteCard />}
      <LeaveCard />
    </>
  )
}

function MemberRow({ m }: { m: Member }) {
  const { user } = useSession()
  const qc = useQueryClient()
  const me = m.id === user.id
  // Admins manage everyone up to their own level; only owners touch owners.
  const canEdit = hasRole(user, 'admin') && !me && rank[m.role] <= rank[user.role as Role]
  const setRole = useMutation({
    mutationFn: (role: Role) => patch(`/members/${m.id}`, { role }),
    onSuccess: () => qc.invalidateQueries({ queryKey: bootstrapKey }),
    onError: (e: Error) => toast.error(e.message),
  })
  const remove = useMutation({
    mutationFn: () => del(`/members/${m.id}`),
    onSuccess: () => {
      toast.success(`${m.displayName} removed`)
      qc.invalidateQueries({ queryKey: bootstrapKey })
    },
    onError: (e: Error) => toast.error(e.message),
  })
  return (
    <li className="flex items-center gap-3 border-b border-border px-4 py-2.5 last:border-b-0">
      <Avatar name={m.displayName} color={m.color} size={28} />
      <div className="min-w-0 flex-1">
        <p className="truncate text-[13px] font-medium">
          {m.displayName} {me && <span className="font-normal text-subtle">(you)</span>}
        </p>
        <p className="truncate text-[12px] text-subtle">
          @{m.username}
          {m.email && ` · ${m.email}`}
        </p>
      </div>
      {canEdit ? (
        <Menu
          align="end"
          width="w-72"
          trigger={
            <button className="flex h-7 items-center rounded-md border border-border px-2 text-[12px] font-medium hover:bg-surface-2">
              {roleLabels[m.role]} ▾
            </button>
          }
        >
          <MenuLabel>Role</MenuLabel>
          {ROLES.filter((r) => rank[r] <= rank[user.role as Role]).map((r) => (
            <MenuItem key={r} checked={m.role === r} onSelect={() => setRole.mutate(r)}>
              <span className="block">{roleLabels[r]}</span>
              <span className="block text-[11px] text-subtle">{roleHelp[r]}</span>
            </MenuItem>
          ))}
          <MenuItem icon={<UserMinus className="size-4" />} danger onSelect={() => confirm(`Remove ${m.displayName} from this household? Their open tasks go back to unassigned.`) && remove.mutate()}>
            Remove from household
          </MenuItem>
        </Menu>
      ) : (
        <Badge tone={m.role === 'owner' ? 'accent' : 'neutral'}>{roleLabels[m.role]}</Badge>
      )}
    </li>
  )
}

function InviteCard() {
  const { user, mailEnabled } = useSession()
  const qc = useQueryClient()
  const [email, setEmail] = useState('')
  const [role, setRole] = useState<Role>('member')
  const [last, setLast] = useState<(InviteResult & { email: string }) | null>(null)
  const { data: invites = [] } = useQuery({ queryKey: ['invites'], queryFn: () => get<PendingInvite[]>('/invites') })
  const send = useMutation({
    mutationFn: () => post<InviteResult>('/invites', { email, role }),
    onSuccess: (res) => {
      setLast({ ...res, email })
      if (res.emailSent) toast.success(`Invite emailed to ${email}`)
      setEmail('')
      qc.invalidateQueries({ queryKey: ['invites'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })
  const revoke = useMutation({
    mutationFn: (code: string) => del(`/invites/${code}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['invites'] }),
  })
  const copy = (url: string) => {
    navigator.clipboard?.writeText(url)
    toast.success('Invite link copied')
  }
  return (
    <Card>
      <CardHeader title="Invite people" subtitle="They get an email with a link to join. Links work once and expire in 7 days." />
      <form
        className="grid gap-3 p-4 sm:grid-cols-[1fr_160px_auto] sm:items-end"
        onSubmit={(e) => {
          e.preventDefault()
          send.mutate()
        }}
      >
        <Field label="Email">
          <Input type="email" autoCapitalize="none" placeholder="name@example.com" value={email} onChange={(e) => setEmail(e.target.value)} required />
        </Field>
        <Field label="Role">
          <Select value={role} onChange={(e) => setRole(e.target.value as Role)}>
            {ROLES.filter((r) => rank[r] <= rank[user.role as Role]).map((r) => (
              <option key={r} value={r}>
                {roleLabels[r]}
              </option>
            ))}
          </Select>
        </Field>
        <Button type="submit" variant="primary" icon={<Mail className="size-3.5" />} loading={send.isPending}>
          Send invite
        </Button>
        <p className="text-[12px] text-subtle sm:col-span-3">{roleHelp[role]}.</p>
      </form>
      {last && !last.emailSent && (
        <div className="space-y-2 border-t border-border p-4">
          <p className="text-[13px] text-warning">
            {mailEnabled ? "The email couldn't be sent" : 'Email isn’t set up on this server yet'}. Send {last.email} this link yourself:
          </p>
          <div className="flex gap-2">
            <Input readOnly value={last.url} onFocus={(e) => e.target.select()} className="font-mono text-[12px]" />
            <IconButton label="Copy" onClick={() => copy(last.url)}>
              <Copy className="size-4" />
            </IconButton>
          </div>
        </div>
      )}
      {invites.length > 0 && (
        <div className="border-t border-border">
          <div className="px-4 pt-3 pb-1 text-[11px] font-medium tracking-wide text-subtle uppercase">Pending</div>
          <ul>
            {invites.map((i) => (
              <li key={i.code} className="flex items-center gap-3 px-4 py-2">
                <div className="min-w-0 flex-1">
                  <p className="truncate text-[13px]">{i.email}</p>
                  <p className="text-[12px] text-subtle">
                    {roleLabels[i.role]} · sent {timeAgo(i.createdAt)}
                    {i.invitedBy && ` by ${i.invitedBy}`}
                  </p>
                </div>
                <IconButton label="Copy invite link" onClick={() => copy(i.url)}>
                  <Copy className="size-3.5" />
                </IconButton>
                <IconButton label="Revoke invite" onClick={() => revoke.mutate(i.code)}>
                  <Trash2 className="size-3.5" />
                </IconButton>
              </li>
            ))}
          </ul>
        </div>
      )}
    </Card>
  )
}

function LeaveCard() {
  const { user, household, members } = useSession()
  const [confirmName, setConfirmName] = useState('')
  const owners = members.filter((m) => m.role === 'owner').length
  const lastOwner = user.role === 'owner' && owners <= 1
  const done = async () => {
    await clearLocalData()
    location.assign('/')
  }
  const leave = useMutation({ mutationFn: () => del(`/members/${user.id}`), onSuccess: done, onError: (e: Error) => toast.error(e.message) })
  const remove = useMutation({
    mutationFn: () => api('DELETE', '/household', { confirmName }),
    onSuccess: done,
    onError: (e: Error) => toast.error(e.message),
  })
  return (
    <Card>
      <CardHeader title="Leave or delete" />
      <div className="space-y-4 p-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-[13px] text-muted">
            {lastOwner ? 'You’re the only owner. Make someone else an owner before leaving.' : `Leave ${household.name}. You can be invited back later.`}
          </p>
          <Button variant="ghost" disabled={lastOwner} loading={leave.isPending} onClick={() => confirm(`Leave ${household.name}?`) && leave.mutate()}>
            Leave household
          </Button>
        </div>
        {user.role === 'owner' && (
          <div className="space-y-2 rounded-md border border-danger/30 bg-danger-soft p-3">
            <p className="text-[13px] text-danger">
              Deleting removes all of {household.name}'s tasks, meals, recipes and budget data for everyone. This can't be undone.
            </p>
            <div className="flex flex-wrap gap-2">
              <Input placeholder={`Type “${household.name}” to confirm`} value={confirmName} onChange={(e) => setConfirmName(e.target.value)} className="max-w-xs" />
              <Button variant="danger" disabled={confirmName.trim() !== household.name} loading={remove.isPending} onClick={() => remove.mutate()}>
                Delete household
              </Button>
            </div>
          </div>
        )}
      </div>
    </Card>
  )
}
