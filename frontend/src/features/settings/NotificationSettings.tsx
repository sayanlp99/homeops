import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { BellOff, BellRing, CheckCircle2, Share, SquarePlus, XCircle } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { Badge, Button, Card, CardHeader, Input, Switch } from '@/components/ui'
import { get, patch, post } from '@/lib/api'
import { timeAgo } from '@/lib/format'
import { currentSubscription, disablePush, enablePush, isIOS, pushSupport, sendTestPush } from '@/lib/push'
import { bootstrapKey, useSession } from '@/lib/session'
import type { NotifyKind, PushResult, User } from '@/lib/types'

const kinds: { kind: NotifyKind; label: string; hint: string; later?: boolean }[] = [
  { kind: 'task_assigned', label: 'Assigned to you', hint: 'When your partner gives you a task' },
  { kind: 'task_due', label: 'Morning digest', hint: "8:00 AM — what's due today" },
  { kind: 'task_overdue', label: 'Evening nudge', hint: "7:00 PM — what's still open or overdue" },
  { kind: 'ceremony', label: 'Sprint planning & retro', hint: '30 minutes before, and when it starts' },
  { kind: 'kudos', label: 'Kudos', hint: 'When your partner thanks you for a task' },
  { kind: 'meals', label: 'Meals & pantry', hint: "7:00 AM today's menu, advance-prep reminders, and items expiring soon" },
  { kind: 'budget', label: 'Budget & bills', hint: 'A category reaches 80% / 100% of its budget, and bill due reminders' },
]

export function NotificationSettings() {
  return (
    <>
      <DeviceCard />
      <RegisteredDevices />
      <PreferencesCard />
    </>
  )
}

interface PushDevice {
  endpoint: string
  device: string
  browser: string
  service: string
  createdAt: string
  lastSuccessAt: string | null
  failures: number
  lastError: string
  lastFailureAt: string | null
}

/** Every device registered for your account; each one receives every notification. */
function RegisteredDevices() {
  const qc = useQueryClient()
  const { data: devices = [] } = useQuery({ queryKey: ['push-devices'], queryFn: () => get<PushDevice[]>('/push/devices') })
  const { data: mine } = useQuery({ queryKey: ['push-endpoint'], queryFn: async () => (await currentSubscription())?.endpoint ?? null })
  const remove = useMutation({
    mutationFn: (endpoint: string) => post('/push/unsubscribe', { endpoint }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['push-devices'] }),
  })
  return (
    <Card>
      <CardHeader title="Your devices" subtitle="Every device listed here gets your notifications. Enable each phone or computer from that device." />
      {devices.length === 0 ? (
        <p className="px-4 py-5 text-[13px] text-subtle">No devices yet. Enable notifications above on each device you use.</p>
      ) : (
        <ul className="divide-y divide-border">
          {devices.map((d) => (
            <li key={d.endpoint} className="flex items-start gap-3 px-4 py-2.5">
              {d.failures > 0 ? <XCircle className="mt-0.5 size-4 shrink-0 text-danger" /> : <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-success" />}
              <div className="min-w-0 flex-1">
                <p className="flex flex-wrap items-center gap-2 text-[13px] font-medium">
                  {d.browser} on {d.device}
                  {d.endpoint === mine && <Badge tone="accent">This device</Badge>}
                </p>
                <p className="text-[12px] text-subtle">
                  {d.lastSuccessAt ? `Last delivered ${timeAgo(d.lastSuccessAt)}` : 'Not delivered yet — send a test'} · via {d.service}
                </p>
                {d.failures > 0 && (
                  <p className="text-[12px] text-danger">
                    {d.failures} failed {d.failures === 1 ? 'delivery' : 'deliveries'}
                    {d.lastError && `: ${d.lastError}`}
                  </p>
                )}
              </div>
              <Button size="sm" variant="ghost" onClick={() => remove.mutate(d.endpoint)}>
                Remove
              </Button>
            </li>
          ))}
        </ul>
      )}
    </Card>
  )
}

function DeviceCard() {
  const { vapidPublicKey } = useSession()
  const support = pushSupport()
  const sub = useQuery({ queryKey: ['push-subscription'], queryFn: async () => !!(await currentSubscription()), enabled: support === 'supported' })
  const qc = useQueryClient()
  const [results, setResults] = useState<PushResult[] | null>(null)
  const permission = typeof Notification !== 'undefined' ? Notification.permission : 'default'
  const enabled = permission === 'granted' && sub.data

  const enable = useMutation({
    mutationFn: () => enablePush(vapidPublicKey),
    onSuccess: () => {
      toast.success('Notifications enabled on this device')
      qc.invalidateQueries({ queryKey: ['push-subscription'] })
      qc.invalidateQueries({ queryKey: ['push-devices'] })
      qc.invalidateQueries({ queryKey: ['push-endpoint'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })
  const disable = useMutation({
    mutationFn: disablePush,
    onSuccess: () => {
      toast.success('Notifications turned off on this device')
      qc.invalidateQueries({ queryKey: ['push-subscription'] })
      qc.invalidateQueries({ queryKey: ['push-devices'] })
    },
  })
  const test = useMutation({
    mutationFn: sendTestPush,
    onSuccess: (r) => {
      setResults(r)
      qc.invalidateQueries({ queryKey: ['push-devices'] })
      if (r.length === 0) toast.error('No devices are registered for push yet')
    },
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <Card>
      <CardHeader
        title="This device"
        subtitle="Each phone and computer is enabled separately."
        actions={enabled ? <Badge tone="success">On</Badge> : <Badge>{permission === 'denied' ? 'Blocked' : 'Off'}</Badge>}
      />
      <div className="space-y-4 p-4">
        {support === 'needs-install' && (
          <div className="space-y-2 rounded-md bg-accent-soft p-3 text-[13px]">
            <p className="font-medium">On iPhone, notifications need HomeOps on your Home Screen:</p>
            <ol className="list-decimal space-y-1 pl-5 text-muted">
              <li>
                Tap <Share className="inline size-3.5 align-[-2px]" /> <b>Share</b> in Safari's toolbar
              </li>
              <li>
                Choose <SquarePlus className="inline size-3.5 align-[-2px]" /> <b>Add to Home Screen</b>
              </li>
              <li>Open HomeOps from the Home Screen icon and come back here</li>
            </ol>
            <p className="text-[12px] text-subtle">Requires iOS 16.4 or later.</p>
          </div>
        )}
        {support === 'unsupported' && <p className="text-[13px] text-muted">This browser doesn't support web push notifications.</p>}
        {support === 'supported' && permission === 'denied' && (
          <p className="text-[13px] text-muted">
            Notifications are blocked for this site. {isIOS() ? 'Open iOS Settings → Notifications → HomeOps and allow them.' : 'Click the lock icon in the address bar → Site settings → allow Notifications, then reload.'}
          </p>
        )}
        {support === 'supported' && permission !== 'denied' && (
          <div className="flex flex-wrap gap-2">
            {enabled ? (
              <Button icon={<BellOff className="size-4" />} loading={disable.isPending} onClick={() => disable.mutate()}>
                Turn off on this device
              </Button>
            ) : (
              <Button variant="primary" icon={<BellRing className="size-4" />} loading={enable.isPending} onClick={() => enable.mutate()}>
                Enable notifications
              </Button>
            )}
          </div>
        )}
        <div className="border-t border-border pt-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div>
              <p className="text-[13px] font-medium">Test delivery</p>
              <p className="text-[12px] text-subtle">Sends a test to all your registered devices, ignoring quiet hours.</p>
            </div>
            <Button size="sm" loading={test.isPending} onClick={() => test.mutate()}>
              Send test
            </Button>
          </div>
          {results && results.length > 0 && (
            <ul className="mt-3 space-y-1.5">
              {results.map((r, i) => (
                <li key={i} className="flex items-center gap-2 text-[13px]">
                  {r.ok ? <CheckCircle2 className="size-4 text-success" /> : <XCircle className="size-4 text-danger" />}
                  <span>{r.device}</span>
                  <span className="text-[12px] text-subtle">{r.ok ? 'delivered to push service' : r.error}</span>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </Card>
  )
}

function PreferencesCard() {
  const { user } = useSession()
  const qc = useQueryClient()
  const save = useMutation({
    mutationFn: (p: Partial<Pick<User, 'notifyPrefs'>> & { quietStart?: string; quietEnd?: string }) => patch<User>('/me', p),
    onSuccess: () => qc.invalidateQueries({ queryKey: bootstrapKey }),
    onError: (e: Error) => toast.error(e.message),
  })
  const quiet = !!(user.quietStart && user.quietEnd)
  const [qs, setQs] = useState(user.quietStart ?? '22:00')
  const [qe, setQe] = useState(user.quietEnd ?? '07:00')

  return (
    <Card>
      <CardHeader title="What to notify me about" />
      <ul>
        {kinds.map((k) => (
          <li key={k.kind} className="flex items-center justify-between gap-4 border-b border-border px-4 py-3">
            <div>
              <p className="text-[13px] font-medium">{k.label}</p>
              <p className="text-[12px] text-subtle">{k.hint}</p>
            </div>
            <Switch
              label={k.label}
              disabled={k.later}
              checked={!k.later && user.notifyPrefs[k.kind] !== false}
              onChange={(v) => save.mutate({ notifyPrefs: { [k.kind]: v } })}
            />
          </li>
        ))}
        <li className="px-4 py-3">
          <div className="flex items-center justify-between gap-4">
            <div>
              <p className="text-[13px] font-medium">Quiet hours</p>
              <p className="text-[12px] text-subtle">Notifications wait in your inbox and are sent once quiet hours end.</p>
            </div>
            <Switch label="Quiet hours" checked={quiet} onChange={(v) => save.mutate(v ? { quietStart: qs, quietEnd: qe } : { quietStart: '', quietEnd: '' })} />
          </div>
          {quiet && (
            <div className="mt-3 flex items-center gap-2 text-[13px]">
              <Input type="time" value={qs} onChange={(e) => setQs(e.target.value)} onBlur={() => qs && save.mutate({ quietStart: qs })} className="w-32" />
              <span className="text-subtle">to</span>
              <Input type="time" value={qe} onChange={(e) => setQe(e.target.value)} onBlur={() => qe && save.mutate({ quietEnd: qe })} className="w-32" />
            </div>
          )}
        </li>
      </ul>
    </Card>
  )
}
