import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Fingerprint, Laptop, Smartphone, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { Badge, Button, Card, CardHeader, Field, IconButton, Input } from '@/components/ui'
import { del, get, post } from '@/lib/api'
import { formatDateTime, timeAgo } from '@/lib/format'
import { passkeysSupported, registerPasskey } from '@/lib/webauthn'

interface Passkey {
  id: string
  name: string
  createdAt: string
  lastUsedAt: string | null
}
interface Session {
  id: string
  userAgent: string
  createdAt: string
  lastSeenAt: string
  current: boolean
}

export function SecuritySettings() {
  return (
    <>
      <PasskeysCard />
      <PasswordCard />
      <SessionsCard />
    </>
  )
}

function PasskeysCard() {
  const qc = useQueryClient()
  const { data = [] } = useQuery({ queryKey: ['passkeys'], queryFn: () => get<Passkey[]>('/me/passkeys') })
  const add = useMutation({
    mutationFn: () => registerPasskey(),
    onSuccess: () => {
      toast.success('Passkey added — you can now sign in with Face ID / fingerprint')
      qc.invalidateQueries({ queryKey: ['passkeys'] })
    },
    onError: (e: Error) => !(e instanceof DOMException && e.name === 'NotAllowedError') && toast.error(e.message),
  })
  const remove = useMutation({
    mutationFn: (id: string) => del(`/me/passkeys/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['passkeys'] }),
  })
  return (
    <Card>
      <CardHeader
        title="Passkeys"
        subtitle="Sign in with Face ID, Touch ID, fingerprint or Windows Hello — no password needed."
        actions={
          passkeysSupported() && (
            <Button size="sm" variant="primary" icon={<Fingerprint className="size-3.5" />} loading={add.isPending} onClick={() => add.mutate()}>
              Add passkey
            </Button>
          )
        }
      />
      {data.length === 0 ? (
        <p className="px-4 py-5 text-[13px] text-subtle">No passkeys yet. Add one on each phone for one-tap sign-in.</p>
      ) : (
        <ul>
          {data.map((p) => (
            <li key={p.id} className="flex items-center gap-3 border-b border-border px-4 py-2.5 last:border-b-0">
              <Fingerprint className="size-4 text-subtle" />
              <div className="min-w-0 flex-1">
                <p className="text-[13px] font-medium">{p.name}</p>
                <p className="text-[12px] text-subtle">
                  Added {formatDateTime(p.createdAt)}
                  {p.lastUsedAt && ` · last used ${timeAgo(p.lastUsedAt)}`}
                </p>
              </div>
              <IconButton label="Remove passkey" onClick={() => confirm(`Remove passkey “${p.name}”?`) && remove.mutate(p.id)}>
                <Trash2 className="size-3.5" />
              </IconButton>
            </li>
          ))}
        </ul>
      )}
    </Card>
  )
}

function PasswordCard() {
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const change = useMutation({
    mutationFn: () => post('/me/password', { current, new: next }),
    onSuccess: () => {
      toast.success('Password changed. Other devices were signed out.')
      setCurrent('')
      setNext('')
    },
    onError: (e: Error) => toast.error(e.message),
  })
  return (
    <Card>
      <CardHeader title="Password" />
      <form
        className="grid gap-3 p-4 sm:grid-cols-[1fr_1fr_auto] sm:items-end"
        onSubmit={(e) => {
          e.preventDefault()
          change.mutate()
        }}
      >
        <Field label="Current password">
          <Input type="password" autoComplete="current-password" value={current} onChange={(e) => setCurrent(e.target.value)} required />
        </Field>
        <Field label="New password">
          <Input type="password" autoComplete="new-password" minLength={8} value={next} onChange={(e) => setNext(e.target.value)} required />
        </Field>
        <Button type="submit" loading={change.isPending}>
          Change
        </Button>
      </form>
    </Card>
  )
}

function SessionsCard() {
  const qc = useQueryClient()
  const { data = [] } = useQuery({ queryKey: ['sessions'], queryFn: () => get<Session[]>('/me/sessions') })
  const revoke = useMutation({
    mutationFn: (id: string) => del(`/me/sessions/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['sessions'] }),
  })
  return (
    <Card>
      <CardHeader title="Signed-in devices" subtitle="Sessions last 90 days and renew while you use the app." />
      <ul>
        {data.map((s) => {
          const mobile = /iPhone|Android|iPad/.test(s.userAgent)
          return (
            <li key={s.id} className="flex items-center gap-3 border-b border-border px-4 py-2.5 last:border-b-0">
              {mobile ? <Smartphone className="size-4 text-subtle" /> : <Laptop className="size-4 text-subtle" />}
              <div className="min-w-0 flex-1">
                <p className="flex items-center gap-2 truncate text-[13px] font-medium">
                  {describeUA(s.userAgent)}
                  {s.current && <Badge tone="success">This device</Badge>}
                </p>
                <p className="text-[12px] text-subtle">Active {timeAgo(s.lastSeenAt)} · signed in {formatDateTime(s.createdAt)}</p>
              </div>
              {!s.current && (
                <Button size="sm" variant="ghost" onClick={() => revoke.mutate(s.id)}>
                  Sign out
                </Button>
              )}
            </li>
          )
        })}
      </ul>
    </Card>
  )
}

function describeUA(ua: string) {
  const os = /iPhone/.test(ua) ? 'iPhone' : /iPad/.test(ua) ? 'iPad' : /Android/.test(ua) ? 'Android' : /Mac OS X/.test(ua) ? 'Mac' : /Windows/.test(ua) ? 'Windows' : /Linux/.test(ua) ? 'Linux' : 'Device'
  const browser = /Edg\//.test(ua) ? 'Edge' : /Firefox\//.test(ua) ? 'Firefox' : /Chrome\//.test(ua) ? 'Chrome' : /Safari\//.test(ua) ? 'Safari' : 'Browser'
  return `${browser} on ${os}`
}
