import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Fingerprint } from 'lucide-react'
import { useState, type FormEvent, type ReactNode } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { Button, Card, Field, Input, Spinner } from '@/components/ui'
import { ApiError, get, post } from '@/lib/api'
import { bootstrapKey } from '@/lib/session'
import { loginWithPasskey, passkeysSupported } from '@/lib/webauthn'

function AuthLayout({ title, subtitle, children }: { title: string; subtitle?: ReactNode; children: ReactNode }) {
  return (
    <div className="flex min-h-dvh items-center justify-center bg-bg px-4 py-10 pt-safe">
      <div className="w-full max-w-sm">
        <div className="mb-6 flex items-center gap-2.5">
          <img src="/logo.svg" alt="" className="size-8" />
          <span className="text-[15px] font-semibold tracking-tight">HomeOps</span>
        </div>
        <Card className="p-5">
          <h1 className="text-[16px] font-semibold">{title}</h1>
          {subtitle && <p className="mt-1 text-[13px] text-muted">{subtitle}</p>}
          <div className="mt-5">{children}</div>
        </Card>
      </div>
    </div>
  )
}

function useAuthSubmit() {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const run = async (fn: () => Promise<unknown>) => {
    setError('')
    setBusy(true)
    try {
      await fn()
      await qc.invalidateQueries({ queryKey: bootstrapKey })
      navigate('/', { replace: true })
    } catch (e) {
      // A user dismissing the passkey prompt isn't an error worth shouting about.
      if (e instanceof DOMException && e.name === 'NotAllowedError') setError('')
      else setError(e instanceof Error ? e.message : 'Something went wrong')
    } finally {
      setBusy(false)
    }
  }
  return { error, busy, run }
}

export function LoginPage() {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const { error, busy, run } = useAuthSubmit()
  const submit = (e: FormEvent) => {
    e.preventDefault()
    run(() => post('/auth/login', { username, password }))
  }
  return (
    <AuthLayout title="Sign in" subtitle="Welcome back.">
      {passkeysSupported() && (
        <>
          <Button variant="primary" className="h-10 w-full" icon={<Fingerprint className="size-4" />} onClick={() => run(loginWithPasskey)} disabled={busy}>
            Sign in with passkey
          </Button>
          <div className="my-4 flex items-center gap-3 text-[11px] text-subtle uppercase">
            <span className="h-px flex-1 bg-border" /> or <span className="h-px flex-1 bg-border" />
          </div>
        </>
      )}
      <form onSubmit={submit} className="space-y-3">
        <Field label="Username">
          <Input autoComplete="username webauthn" autoCapitalize="none" value={username} onChange={(e) => setUsername(e.target.value)} required />
        </Field>
        <Field label="Password">
          <Input type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
        </Field>
        {error && <p className="text-[13px] text-danger">{error}</p>}
        <Button type="submit" className="h-10 w-full" loading={busy}>
          Sign in
        </Button>
      </form>
    </AuthLayout>
  )
}

export function SetupPage() {
  const [f, setF] = useState({ householdName: '', displayName: '', username: '', password: '' })
  const { error, busy, run } = useAuthSubmit()
  const set = (k: keyof typeof f) => (e: React.ChangeEvent<HTMLInputElement>) => setF({ ...f, [k]: e.target.value })
  const submit = (e: FormEvent) => {
    e.preventDefault()
    run(() => post('/setup', { ...f, timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'Asia/Kolkata' }))
  }
  return (
    <AuthLayout title="Set up your household" subtitle="You'll be the admin. You can invite your partner right after.">
      <form onSubmit={submit} className="space-y-3">
        <Field label="Household name">
          <Input placeholder="Our Home" value={f.householdName} onChange={set('householdName')} />
        </Field>
        <Field label="Your name">
          <Input autoComplete="name" value={f.displayName} onChange={set('displayName')} required maxLength={40} />
        </Field>
        <Field label="Username" hint="Lowercase letters, digits, dot, dash or underscore">
          <Input autoComplete="username" autoCapitalize="none" value={f.username} onChange={set('username')} required minLength={2} maxLength={32} />
        </Field>
        <Field label="Password" hint="At least 8 characters. You can add a passkey (Face ID / fingerprint) next.">
          <Input type="password" autoComplete="new-password" value={f.password} onChange={set('password')} required minLength={8} />
        </Field>
        {error && <p className="text-[13px] text-danger">{error}</p>}
        <Button type="submit" variant="primary" className="h-10 w-full" loading={busy}>
          Create household
        </Button>
      </form>
    </AuthLayout>
  )
}

export function JoinPage() {
  const { code = '' } = useParams()
  const invite = useQuery({ queryKey: ['invite', code], queryFn: () => get<{ householdName: string; invitedBy: string }>(`/auth/invite/${code}`), retry: false })
  const [f, setF] = useState({ displayName: '', username: '', password: '' })
  const { error, busy, run } = useAuthSubmit()
  const set = (k: keyof typeof f) => (e: React.ChangeEvent<HTMLInputElement>) => setF({ ...f, [k]: e.target.value })

  if (invite.isLoading)
    return (
      <AuthLayout title="Joining…">
        <Spinner className="mx-auto" />
      </AuthLayout>
    )
  if (invite.error)
    return (
      <AuthLayout title="Invite not valid" subtitle={invite.error instanceof ApiError ? invite.error.message : 'Ask for a new invite link.'}>
        <a href="/" className="text-[13px] text-accent hover:underline">
          Go to sign in
        </a>
      </AuthLayout>
    )

  return (
    <AuthLayout title={`Join ${invite.data!.householdName}`} subtitle={`${invite.data!.invitedBy} invited you to HomeOps.`}>
      <form
        onSubmit={(e) => {
          e.preventDefault()
          run(() => post('/auth/invite/accept', { ...f, code }))
        }}
        className="space-y-3"
      >
        <Field label="Your name">
          <Input autoComplete="name" value={f.displayName} onChange={set('displayName')} required maxLength={40} />
        </Field>
        <Field label="Username">
          <Input autoComplete="username" autoCapitalize="none" value={f.username} onChange={set('username')} required minLength={2} maxLength={32} />
        </Field>
        <Field label="Password" hint="At least 8 characters">
          <Input type="password" autoComplete="new-password" value={f.password} onChange={set('password')} required minLength={8} />
        </Field>
        {error && <p className="text-[13px] text-danger">{error}</p>}
        <Button type="submit" variant="primary" className="h-10 w-full" loading={busy}>
          Join household
        </Button>
      </form>
    </AuthLayout>
  )
}
