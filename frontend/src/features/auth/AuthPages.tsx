import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Fingerprint } from 'lucide-react'
import { useState, type FormEvent, type ReactNode } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { Button, Card, Field, Input, Spinner } from '@/components/ui'
import { ApiError, get, post } from '@/lib/api'
import { clearLocalData } from '@/lib/queryClient'
import { bootstrapKey, roleLabels } from '@/lib/session'
import type { Role } from '@/lib/types'
import { loginWithPasskey, passkeysSupported } from '@/lib/webauthn'

export function AuthLayout({ title, subtitle, children }: { title: string; subtitle?: ReactNode; children: ReactNode }) {
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

function useAuthSubmit(to = '/') {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  /** fresh: wipe any cached data from a previous account and reload into the new session. */
  const run = async (fn: () => Promise<unknown>, fresh = false) => {
    setError('')
    setBusy(true)
    try {
      await fn()
      if (fresh) {
        await clearLocalData()
        location.assign(to)
        return
      }
      await qc.invalidateQueries({ queryKey: bootstrapKey })
      navigate(to, { replace: true })
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
  return (
    <AuthLayout title="Sign in" subtitle="Welcome back.">
      <LoginForm />
    </AuthLayout>
  )
}

function LoginForm({ to = '/', defaultUser = '' }: { to?: string; defaultUser?: string }) {
  const [username, setUsername] = useState(defaultUser)
  const [password, setPassword] = useState('')
  const { error, busy, run } = useAuthSubmit(to)
  const submit = (e: FormEvent) => {
    e.preventDefault()
    run(() => post('/auth/login', { username, password }))
  }
  return (
    <>
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
        <Field label="Username or email">
          <Input autoComplete="username webauthn" autoCapitalize="none" value={username} onChange={(e) => setUsername(e.target.value)} required />
        </Field>
        <Field label="Password">
          <Input type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
        </Field>
        {error && <p className="text-[13px] text-danger">{error}</p>}
        <Button type="submit" className="h-10 w-full" loading={busy}>
          Sign in
        </Button>
        <p className="text-center text-[13px]">
          <Link to="/forgot" className="text-accent hover:underline">
            Forgot password?
          </Link>
        </p>
      </form>
    </>
  )
}

export function ForgotPage() {
  const [email, setEmail] = useState('')
  const [sent, setSent] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await post('/auth/password/forgot', { email })
      setSent(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong')
    } finally {
      setBusy(false)
    }
  }
  if (sent)
    return (
      <AuthLayout title="Check your email" subtitle={`If an account uses ${email}, a reset link is on its way. It expires in 1 hour.`}>
        <p className="text-[13px] text-muted">Nothing after a few minutes? Check spam, or ask your household admin.</p>
        <Link to="/" className="mt-4 inline-block text-[13px] text-accent hover:underline">
          Back to sign in
        </Link>
      </AuthLayout>
    )
  return (
    <AuthLayout title="Reset your password" subtitle="We'll email you a link to choose a new one.">
      <form onSubmit={submit} className="space-y-3">
        <Field label="Email">
          <Input type="email" autoComplete="email" autoCapitalize="none" value={email} onChange={(e) => setEmail(e.target.value)} required />
        </Field>
        {error && <p className="text-[13px] text-danger">{error}</p>}
        <Button type="submit" variant="primary" className="h-10 w-full" loading={busy}>
          Send reset link
        </Button>
        <p className="text-center text-[13px]">
          <Link to="/" className="text-accent hover:underline">
            Back to sign in
          </Link>
        </p>
      </form>
    </AuthLayout>
  )
}

export function ResetPage() {
  const { token = '' } = useParams()
  const info = useQuery({ queryKey: ['reset', token], queryFn: () => get<{ username: string }>(`/auth/password/reset/${token}`), retry: false })
  const [password, setPassword] = useState('')
  const { error, busy, run } = useAuthSubmit()
  if (info.isLoading)
    return (
      <AuthLayout title="Checking link…">
        <Spinner className="mx-auto" />
      </AuthLayout>
    )
  if (info.error)
    return (
      <AuthLayout title="Link not valid" subtitle={info.error.message}>
        <Link to="/forgot" className="text-[13px] text-accent hover:underline">
          Request a new link
        </Link>
      </AuthLayout>
    )
  return (
    <AuthLayout title="Choose a new password" subtitle={`For @${info.data!.username}. You'll be signed out everywhere else.`}>
      <form
        onSubmit={(e) => {
          e.preventDefault()
          run(() => post('/auth/password/reset', { token, password }), true)
        }}
        className="space-y-3"
      >
        <input type="text" autoComplete="username" value={info.data!.username} readOnly hidden />
        <Field label="New password" hint="At least 8 characters">
          <Input type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} required minLength={8} autoFocus />
        </Field>
        {error && <p className="text-[13px] text-danger">{error}</p>}
        <Button type="submit" variant="primary" className="h-10 w-full" loading={busy}>
          Save password and sign in
        </Button>
      </form>
    </AuthLayout>
  )
}

export function SetupPage() {
  const [f, setF] = useState({ householdName: '', displayName: '', email: '', username: '', password: '' })
  const { error, busy, run } = useAuthSubmit()
  const set = (k: keyof typeof f) => (e: React.ChangeEvent<HTMLInputElement>) => setF({ ...f, [k]: e.target.value })
  const submit = (e: FormEvent) => {
    e.preventDefault()
    run(() => post('/setup', { ...f, timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'Asia/Kolkata' }))
  }
  return (
    <AuthLayout title="Set up HomeOps" subtitle="You'll administer this HomeOps and own the first household. Invite people by email right after.">
      <form onSubmit={submit} className="space-y-3">
        <Field label="Household name">
          <Input placeholder="Our Home" value={f.householdName} onChange={set('householdName')} />
        </Field>
        <Field label="Your name">
          <Input autoComplete="name" value={f.displayName} onChange={set('displayName')} required maxLength={40} />
        </Field>
        <Field label="Email" hint="For password resets and invites">
          <Input type="email" autoComplete="email" autoCapitalize="none" value={f.email} onChange={set('email')} required />
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

interface InviteInfo {
  householdName: string
  invitedBy: string
  email: string
  role: Role
  accountExists: boolean
}

function useInvite(code: string) {
  return useQuery({ queryKey: ['invite', code], queryFn: () => get<InviteInfo>(`/auth/invite/${code}`), retry: false })
}

function InviteError({ error }: { error: Error }) {
  return (
    <AuthLayout title="Invite not valid" subtitle={error instanceof ApiError ? error.message : 'Ask for a new invite link.'}>
      <a href="/" className="text-[13px] text-accent hover:underline">
        Go to HomeOps
      </a>
    </AuthLayout>
  )
}

/** Signed out: create an account from the invite, or sign in if one exists for that email. */
export function JoinPage() {
  const { code = '' } = useParams()
  const invite = useInvite(code)
  const [f, setF] = useState({ displayName: '', username: '', password: '' })
  const { error, busy, run } = useAuthSubmit()
  const set = (k: keyof typeof f) => (e: React.ChangeEvent<HTMLInputElement>) => setF({ ...f, [k]: e.target.value })

  if (invite.isLoading)
    return (
      <AuthLayout title="Joining…">
        <Spinner className="mx-auto" />
      </AuthLayout>
    )
  if (invite.error) return <InviteError error={invite.error} />
  const inv = invite.data!
  const subtitle = `${inv.invitedBy} invited ${inv.email} as ${roleLabels[inv.role].toLowerCase()}.`

  if (inv.accountExists)
    return (
      <AuthLayout title={`Join ${inv.householdName}`} subtitle={`${subtitle} You already have an account — sign in to accept.`}>
        <LoginForm to={`/join/${code}`} defaultUser={inv.email} />
      </AuthLayout>
    )

  return (
    <AuthLayout title={`Join ${inv.householdName}`} subtitle={subtitle}>
      <form
        onSubmit={(e) => {
          e.preventDefault()
          run(() => post('/auth/invite/accept', { ...f, code }), true)
        }}
        className="space-y-3"
      >
        <Field label="Email">
          <Input value={inv.email} readOnly disabled />
        </Field>
        <Field label="Your name">
          <Input autoComplete="name" value={f.displayName} onChange={set('displayName')} required maxLength={40} />
        </Field>
        <Field label="Username" hint="You can sign in with this or your email">
          <Input autoComplete="username" autoCapitalize="none" value={f.username} onChange={set('username')} required minLength={2} maxLength={32} />
        </Field>
        <Field label="Password" hint="At least 8 characters">
          <Input type="password" autoComplete="new-password" value={f.password} onChange={set('password')} required minLength={8} />
        </Field>
        {error && <p className="text-[13px] text-danger">{error}</p>}
        <Button type="submit" variant="primary" className="h-10 w-full" loading={busy}>
          Create account and join
        </Button>
      </form>
    </AuthLayout>
  )
}

/** Signed in: accept an invite into another household. */
export function JoinSignedInPage({ email }: { email: string }) {
  const { code = '' } = useParams()
  const invite = useInvite(code)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  if (invite.isLoading)
    return (
      <AuthLayout title="Joining…">
        <Spinner className="mx-auto" />
      </AuthLayout>
    )
  if (invite.error) return <InviteError error={invite.error} />
  const inv = invite.data!
  const mismatch = inv.email.toLowerCase() !== email.toLowerCase()
  const accept = async () => {
    setBusy(true)
    setError('')
    try {
      await post(`/invites/${code}/join`)
      await clearLocalData()
      location.assign('/')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong')
      setBusy(false)
    }
  }
  return (
    <AuthLayout title={`Join ${inv.householdName}`} subtitle={`${inv.invitedBy} invited ${inv.email} as ${roleLabels[inv.role].toLowerCase()}.`}>
      {mismatch ? (
        <p className="text-[13px] text-danger">
          You're signed in as {email || 'an account without an email'}. Sign out and sign in as {inv.email} to accept this invite.
        </p>
      ) : (
        <Button variant="primary" className="h-10 w-full" loading={busy} onClick={accept}>
          Join {inv.householdName}
        </Button>
      )}
      {error && <p className="mt-3 text-[13px] text-danger">{error}</p>}
      <a href="/" className="mt-4 inline-block text-[13px] text-accent hover:underline">
        Not now
      </a>
    </AuthLayout>
  )
}
