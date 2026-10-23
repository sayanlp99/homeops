import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Monitor, Moon, Sun, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Avatar, Button, Card, CardHeader, Field, IconButton, Input, Segmented, Select, Switch } from '@/components/ui'
import { del, patch, post } from '@/lib/api'
import { weekdays } from '@/lib/format'
import { bootstrapKey, hasRole, useSession } from '@/lib/session'
import { useTheme, type Theme } from '@/lib/theme'
import type { Household, Label } from '@/lib/types'

const COLORS = ['#2563eb', '#db2777', '#059669', '#d97706', '#7c3aed', '#0891b2', '#dc2626', '#4b5563']

function useSave<T>(fn: (v: T) => Promise<unknown>, msg = 'Saved') {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      toast.success(msg)
      qc.invalidateQueries({ queryKey: bootstrapKey })
      qc.invalidateQueries({ queryKey: ['sprints'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })
}

export function ProfileSettings() {
  const { user } = useSession()
  const [name, setName] = useState(user.displayName)
  const save = useSave((p: { displayName?: string; color?: string }) => patch('/me', p))
  return (
    <Card>
      <CardHeader title="Profile" subtitle={`Signed in as @${user.username}`} />
      <div className="space-y-4 p-4">
        <div className="flex items-end gap-3">
          <Avatar name={name || user.displayName} color={user.color} size={40} />
          <Field label="Display name" className="flex-1">
            <Input value={name} onChange={(e) => setName(e.target.value)} maxLength={40} />
          </Field>
          <Button disabled={!name.trim() || name === user.displayName} onClick={() => save.mutate({ displayName: name })}>
            Save
          </Button>
        </div>
        <EmailField />
        <Field label="Your colour" hint="Used for your avatar and charts.">
          <div className="flex flex-wrap gap-2">
            {COLORS.map((c) => (
              <button
                key={c}
                aria-label={c}
                onClick={() => save.mutate({ color: c })}
                className={`size-7 rounded-full ring-offset-2 ring-offset-surface ${user.color === c ? 'ring-2 ring-text' : ''}`}
                style={{ background: c }}
              />
            ))}
          </div>
        </Field>
      </div>
    </Card>
  )
}

function EmailField() {
  const { user } = useSession()
  const [email, setEmail] = useState(user.email)
  const [password, setPassword] = useState('')
  const changed = email.trim().toLowerCase() !== user.email
  const save = useSave((p: { email: string; password: string }) => patch('/me', p), 'Email updated')
  return (
    <form
      className="space-y-2"
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate({ email, password }, { onSuccess: () => setPassword('') })
      }}
    >
      <Field label="Email" hint="Used for invites and password resets.">
        <Input type="email" autoCapitalize="none" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="name@example.com" required />
      </Field>
      {changed && (
        <div className="flex items-end gap-2">
          <Field label="Current password" className="flex-1">
            <Input type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
          </Field>
          <Button type="submit" loading={save.isPending}>
            Save email
          </Button>
        </div>
      )}
    </form>
  )
}

export function HouseholdSettings() {
  const { household, user } = useSession()
  const isAdmin = hasRole(user, 'admin')
  const [h, setH] = useState<Household>(household)
  useEffect(() => setH(household), [household])
  const save = useSave((p: Partial<Household>) => patch('/household', p), 'Household settings saved')
  const dirty = JSON.stringify(h) !== JSON.stringify(household)

  return (
    <>
      <Card>
        <CardHeader title="Household" subtitle={isAdmin ? undefined : 'Only owners and admins can change these.'} />
        <fieldset disabled={!isAdmin} className="grid gap-4 p-4 sm:grid-cols-2">
          <Field label="Name" className="sm:col-span-2">
            <Input value={h.name} onChange={(e) => setH({ ...h, name: e.target.value })} />
          </Field>
          <Field label="Sprints start on" hint="Each sprint runs one week from this day.">
            <Select value={h.sprintStartDow} onChange={(e) => setH({ ...h, sprintStartDow: Number(e.target.value) })}>
              {weekdays.map((d, i) => (
                <option key={d} value={i}>
                  {d}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Time zone">
            <Input value={h.timezone} onChange={(e) => setH({ ...h, timezone: e.target.value })} />
          </Field>
          <Field label="Planning & retro day">
            <Select value={h.planningDow} onChange={(e) => setH({ ...h, planningDow: Number(e.target.value) })}>
              {weekdays.map((d, i) => (
                <option key={d} value={i}>
                  {d}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Planning time" hint="You both get a reminder 30 minutes before.">
            <Input type="time" value={h.planningTime} onChange={(e) => setH({ ...h, planningTime: e.target.value })} />
          </Field>
          <div className="sm:col-span-2">
            <span className="mb-1.5 block text-[12px] font-medium text-muted">Meal times</span>
            <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
              {(['breakfast', 'lunch', 'snacks', 'dinner'] as const).map((slot) => (
                <label key={slot} className="block">
                  <span className="block text-[11px] text-subtle capitalize">{slot}</span>
                  <Input type="time" value={h.mealTimes?.[slot] ?? ''} onChange={(e) => setH({ ...h, mealTimes: { ...h.mealTimes, [slot]: e.target.value } })} />
                </label>
              ))}
            </div>
            <span className="mt-1 block text-[12px] text-subtle">Used for prep reminders, e.g. "soak rajma" 10 hours before lunch.</span>
          </div>
          <label className="flex items-center justify-between gap-4 sm:col-span-2">
            <span>
              <span className="block text-[13px] font-medium">Auto-roll sprints</span>
              <span className="block text-[12px] text-subtle">When a sprint's week ends, complete it, carry unfinished tasks over and start the next one automatically.</span>
            </span>
            <Switch label="Auto-roll" checked={h.autoRoll} onChange={(autoRoll) => setH({ ...h, autoRoll })} disabled={!isAdmin} />
          </label>
          {isAdmin && (
            <div className="sm:col-span-2">
              <Button variant="primary" disabled={!dirty} loading={save.isPending} onClick={() => save.mutate(h)}>
                Save changes
              </Button>
            </div>
          )}
        </fieldset>
      </Card>
    </>
  )
}

export function LabelSettings() {
  const { labels } = useSession()
  const [name, setName] = useState('')
  const [color, setColor] = useState(COLORS[0]!)
  const add = useSave((l: Omit<Label, 'id'>) => post('/labels', l), 'Label added')
  const update = useSave((l: Label) => patch(`/labels/${l.id}`, l), 'Label updated')
  const remove = useSave((id: string) => del(`/labels/${id}`), 'Label deleted')
  return (
    <Card>
      <CardHeader title="Labels" subtitle="Categorise tasks, e.g. Cleaning, Bills, Repairs." />
      <ul>
        {labels.map((l) => (
          <li key={l.id} className="flex items-center gap-3 border-b border-border px-4 py-2">
            <input type="color" value={l.color} onChange={(e) => update.mutate({ ...l, color: e.target.value })} className="size-6 cursor-pointer rounded border-0 bg-transparent p-0" aria-label="Colour" />
            <input
              defaultValue={l.name}
              onBlur={(e) => e.target.value.trim() && e.target.value !== l.name && update.mutate({ ...l, name: e.target.value })}
              className="min-w-0 flex-1 rounded bg-transparent px-1 py-1 outline-none hover:bg-surface-2 focus:bg-surface-2"
              maxLength={30}
            />
            <IconButton label="Delete label" onClick={() => confirm(`Delete label “${l.name}”? It will be removed from all tasks.`) && remove.mutate(l.id)}>
              <Trash2 className="size-3.5" />
            </IconButton>
          </li>
        ))}
      </ul>
      <form
        className="flex items-center gap-2 p-4"
        onSubmit={(e) => {
          e.preventDefault()
          if (name.trim()) add.mutate({ name, color }, { onSuccess: () => setName('') })
        }}
      >
        <input type="color" value={color} onChange={(e) => setColor(e.target.value)} className="size-8 cursor-pointer rounded border-0 bg-transparent p-0" aria-label="Colour" />
        <Input placeholder="New label" value={name} onChange={(e) => setName(e.target.value)} maxLength={30} />
        <Button type="submit">Add</Button>
      </form>
    </Card>
  )
}

export function AppearanceSettings() {
  const [theme, setTheme] = useTheme()
  return (
    <Card>
      <CardHeader title="Appearance" subtitle="Saved per device." />
      <div className="p-4">
        <Segmented<Theme>
          value={theme}
          onChange={setTheme}
          options={[
            { value: 'light', label: <span className="flex items-center gap-1.5"><Sun className="size-3.5" />Light</span> },
            { value: 'dark', label: <span className="flex items-center gap-1.5"><Moon className="size-3.5" />Dark</span> },
            { value: 'system', label: <span className="flex items-center gap-1.5"><Monitor className="size-3.5" />System</span> },
          ]}
        />
      </div>
    </Card>
  )
}
