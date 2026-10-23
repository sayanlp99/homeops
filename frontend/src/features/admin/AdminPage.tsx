import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Copy, Home, Mail, Plus, ShieldCheck, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { Modal } from '@/components/Dialog'
import { Avatar, Badge, Button, Card, CardHeader, EmptyState, Field, IconButton, Input, PageHeader, Select, Spinner } from '@/components/ui'
import { api, get, patch, post } from '@/lib/api'
import { formatDateTime, timeAgo } from '@/lib/format'
import { switchHousehold } from '@/lib/households'
import { bootstrapKey, roleLabels } from '@/lib/session'
import type { InviteResult, Role, User } from '@/lib/types'

interface AdminHousehold {
  id: string
  name: string
  timezone: string
  createdAt: string
  members: number
  owners: string[]
  pendingInvites: number
  isMember: boolean
}

interface AdminUser {
  id: string
  username: string
  displayName: string
  email: string
  color: string
  isPlatformAdmin: boolean
  createdAt: string
  lastSeenAt: string | null
  households: { id: string; name: string; role: Role }[]
}

/** Platform administration: households and people across this HomeOps installation. */
export function AdminPage({ user }: { user: User }) {
  return (
    <div className="space-y-4">
      <PageHeader title="Administration" subtitle="Households and people on this HomeOps. You only see a household's data if you're a member of it." />
      <HouseholdsCard user={user} />
      <UsersCard user={user} />
    </div>
  )
}

function HouseholdsCard({ user }: { user: User }) {
  const qc = useQueryClient()
  const { data, isLoading } = useQuery({ queryKey: ['admin', 'households'], queryFn: () => get<AdminHousehold[]>('/admin/households') })
  const [creating, setCreating] = useState(false)
  const [inviteTo, setInviteTo] = useState<AdminHousehold | null>(null)
  const remove = useMutation({
    mutationFn: (h: AdminHousehold) => api('DELETE', `/admin/households/${h.id}`, { confirmName: h.name }),
    onSuccess: () => {
      toast.success('Household deleted')
      qc.invalidateQueries({ queryKey: ['admin'] })
      qc.invalidateQueries({ queryKey: bootstrapKey })
    },
    onError: (e: Error) => toast.error(e.message),
  })
  return (
    <Card>
      <CardHeader
        title="Households"
        actions={
          <Button size="sm" variant="primary" icon={<Plus className="size-3.5" />} onClick={() => setCreating(true)}>
            New household
          </Button>
        }
      />
      {isLoading ? (
        <div className="p-6">
          <Spinner className="mx-auto" />
        </div>
      ) : !data?.length ? (
        <EmptyState icon={<Home className="size-5" />} title="No households yet" />
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-[13px]">
            <thead>
              <tr className="border-b border-border text-left text-[11px] tracking-wide text-subtle uppercase">
                <th className="px-4 py-2 font-medium">Name</th>
                <th className="px-2 py-2 font-medium">Owners</th>
                <th className="px-2 py-2 text-right font-medium">People</th>
                <th className="hidden px-2 py-2 font-medium sm:table-cell">Created</th>
                <th className="px-4 py-2" />
              </tr>
            </thead>
            <tbody>
              {data.map((h) => (
                <tr key={h.id} className="border-b border-border last:border-b-0">
                  <td className="px-4 py-2.5">
                    <div className="font-medium">{h.name}</div>
                    {h.pendingInvites > 0 && <div className="text-[12px] text-subtle">{h.pendingInvites} pending invite{h.pendingInvites > 1 ? 's' : ''}</div>}
                  </td>
                  <td className="px-2 py-2.5 text-muted">{h.owners.join(', ') || <span className="text-warning">Awaiting owner</span>}</td>
                  <td className="px-2 py-2.5 text-right tabular">{h.members}</td>
                  <td className="hidden px-2 py-2.5 text-muted sm:table-cell">{formatDateTime(h.createdAt)}</td>
                  <td className="px-4 py-2.5">
                    <div className="flex justify-end gap-1">
                      {h.isMember && h.id !== user.householdId && (
                        <Button size="sm" variant="ghost" onClick={() => switchHousehold(h.id).catch((e: Error) => toast.error(e.message))}>
                          Open
                        </Button>
                      )}
                      <IconButton label={`Invite someone to ${h.name}`} onClick={() => setInviteTo(h)}>
                        <Mail className="size-3.5" />
                      </IconButton>
                      <IconButton
                        label={`Delete ${h.name}`}
                        onClick={() => prompt(`This deletes all of ${h.name}'s data for everyone. Type the household name to confirm:`)?.trim() === h.name && remove.mutate(h)}
                      >
                        <Trash2 className="size-3.5" />
                      </IconButton>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <CreateHouseholdDialog open={creating} onClose={() => setCreating(false)} />
      <AdminInviteDialog household={inviteTo} onClose={() => setInviteTo(null)} />
    </Card>
  )
}

function InviteLink({ result }: { result: InviteResult }) {
  if (result.emailSent) return <p className="text-[13px] text-success">Invite emailed.</p>
  return (
    <div className="space-y-2">
      <p className="text-[13px] text-warning">The email wasn't sent (is SMTP set up?). Share this link instead:</p>
      <div className="flex gap-2">
        <Input readOnly value={result.url} onFocus={(e) => e.target.select()} className="font-mono text-[12px]" />
        <IconButton
          label="Copy"
          onClick={() => {
            navigator.clipboard?.writeText(result.url)
            toast.success('Copied')
          }}
        >
          <Copy className="size-4" />
        </IconButton>
      </div>
    </div>
  )
}

function CreateHouseholdDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const qc = useQueryClient()
  const [name, setName] = useState('')
  const [ownerEmail, setOwnerEmail] = useState('')
  const [result, setResult] = useState<{ owner: string; invite?: InviteResult } | null>(null)
  const create = useMutation({
    mutationFn: () => post<{ id: string; owner: string; invite?: InviteResult }>('/admin/households', { name, ownerEmail, timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'Asia/Kolkata' }),
    onSuccess: (r) => {
      setResult(r)
      qc.invalidateQueries({ queryKey: ['admin'] })
      qc.invalidateQueries({ queryKey: bootstrapKey })
      if (r.owner !== 'invited') toast.success(r.owner === 'you' ? `${name} created — switch to it from the household menu` : `${name} created and its owner notified`)
    },
    onError: (e: Error) => toast.error(e.message),
  })
  const close = () => {
    setName('')
    setOwnerEmail('')
    setResult(null)
    onClose()
  }
  return (
    <Modal open={open} onOpenChange={(o) => !o && close()} title="New household">
      {result?.invite ? (
        <div className="space-y-3">
          <p className="text-[13px]">
            {name} is ready. {ownerEmail} becomes its owner when they accept the invite.
          </p>
          <InviteLink result={result.invite} />
          <Button onClick={close}>Done</Button>
        </div>
      ) : result ? (
        <Button onClick={close}>Done</Button>
      ) : (
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault()
            create.mutate()
          }}
        >
          <Field label="Name">
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. Parents' home" required maxLength={60} autoFocus />
          </Field>
          <Field label="Owner's email" hint="Leave empty to own it yourself. An existing account is added straight away; anyone else gets an invite.">
            <Input type="email" autoCapitalize="none" value={ownerEmail} onChange={(e) => setOwnerEmail(e.target.value)} placeholder="name@example.com" />
          </Field>
          <Button type="submit" variant="primary" loading={create.isPending}>
            Create household
          </Button>
        </form>
      )}
    </Modal>
  )
}

function AdminInviteDialog({ household, onClose }: { household: AdminHousehold | null; onClose: () => void }) {
  const qc = useQueryClient()
  const [email, setEmail] = useState('')
  const [role, setRole] = useState<Role>('member')
  const [result, setResult] = useState<InviteResult | null>(null)
  const send = useMutation({
    mutationFn: () => post<InviteResult>(`/admin/households/${household!.id}/invites`, { email, role }),
    onSuccess: (r) => {
      setResult(r)
      qc.invalidateQueries({ queryKey: ['admin'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })
  const close = () => {
    setEmail('')
    setResult(null)
    onClose()
  }
  return (
    <Modal open={!!household} onOpenChange={(o) => !o && close()} title={`Invite to ${household?.name ?? ''}`}>
      {result ? (
        <div className="space-y-3">
          <InviteLink result={result} />
          <Button onClick={close}>Done</Button>
        </div>
      ) : (
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault()
            send.mutate()
          }}
        >
          <Field label="Email">
            <Input type="email" autoCapitalize="none" value={email} onChange={(e) => setEmail(e.target.value)} required autoFocus />
          </Field>
          <Field label="Role">
            <Select value={role} onChange={(e) => setRole(e.target.value as Role)}>
              {(['owner', 'admin', 'member', 'viewer'] as Role[]).map((r) => (
                <option key={r} value={r}>
                  {roleLabels[r]}
                </option>
              ))}
            </Select>
          </Field>
          <Button type="submit" variant="primary" loading={send.isPending}>
            Send invite
          </Button>
        </form>
      )}
    </Modal>
  )
}

function UsersCard({ user }: { user: User }) {
  const qc = useQueryClient()
  const { data = [] } = useQuery({ queryKey: ['admin', 'users'], queryFn: () => get<AdminUser[]>('/admin/users') })
  const update = useMutation({
    mutationFn: (v: { id: string; email?: string; isPlatformAdmin?: boolean }) => patch(`/admin/users/${v.id}`, v),
    onSuccess: () => {
      toast.success('Saved')
      qc.invalidateQueries({ queryKey: ['admin', 'users'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })
  return (
    <Card>
      <CardHeader title="People" subtitle={`${data.length} account${data.length === 1 ? '' : 's'}`} />
      <ul>
        {data.map((u) => (
          <li key={u.id} className="flex flex-wrap items-center gap-3 border-b border-border px-4 py-2.5 last:border-b-0">
            <Avatar name={u.displayName} color={u.color} size={28} />
            <div className="min-w-0 flex-1">
              <p className="flex items-center gap-1.5 text-[13px] font-medium">
                {u.displayName} <span className="font-normal text-subtle">@{u.username}</span>
                {u.isPlatformAdmin && (
                  <Badge tone="accent">
                    <ShieldCheck className="mr-0.5 inline size-3" />
                    Administrator
                  </Badge>
                )}
              </p>
              <p className="truncate text-[12px] text-subtle">
                {u.email || <span className="text-warning">No email</span>} · {u.lastSeenAt ? `active ${timeAgo(u.lastSeenAt)}` : 'never signed in'}
              </p>
              <p className="truncate text-[12px] text-muted">{u.households.map((h) => `${h.name} (${roleLabels[h.role].toLowerCase()})`).join(', ') || 'No household'}</p>
            </div>
            <div className="flex gap-1">
              <Button
                size="sm"
                variant="ghost"
                onClick={() => {
                  const email = prompt(`Email for ${u.displayName}:`, u.email)
                  if (email && email !== u.email) update.mutate({ id: u.id, email })
                }}
              >
                Edit email
              </Button>
              {u.id !== user.id && (
                <Button size="sm" variant="ghost" onClick={() => update.mutate({ id: u.id, isPlatformAdmin: !u.isPlatformAdmin })}>
                  {u.isPlatformAdmin ? 'Remove admin' : 'Make admin'}
                </Button>
              )}
            </div>
          </li>
        ))}
      </ul>
    </Card>
  )
}
