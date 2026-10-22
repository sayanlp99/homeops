import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import { Pencil, Plus, Repeat, Shuffle, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Modal } from '@/components/Dialog'
import { Avatar, Badge, Button, Card, EmptyState, Field, IconButton, Input, PageHeader, Select, Spinner, Switch, Textarea } from '@/components/ui'
import { del, get, patch, post } from '@/lib/api'
import { weekdaysShort } from '@/lib/format'
import { useSession } from '@/lib/session'
import { priorities } from '@/lib/tasks'
import type { Recurring } from '@/lib/types'
import { LabelChip, PriorityIcon } from '../tasks/TaskBits'

const blank: Omit<Recurring, 'id' | 'anchorDate'> = {
  title: '', description: '', priority: 2, points: null, labels: [], assigneeId: null, rotate: false,
  freq: 'weekly', interval: 1, weekdays: [6], monthDay: 1, active: true,
}

function describe(r: Pick<Recurring, 'freq' | 'interval' | 'weekdays' | 'monthDay'>) {
  const every = (unit: string) => (r.interval === 1 ? `Every ${unit}` : `Every ${r.interval} ${unit}s`)
  switch (r.freq) {
    case 'daily':
      return every('day')
    case 'weekly':
      return `${every('week')} on ${[...r.weekdays].sort().map((d) => weekdaysShort[d]).join(', ')}`
    case 'monthly':
      return `${every('month')} on day ${r.monthDay}`
  }
}

export function RecurringPage() {
  const qc = useQueryClient()
  const { data, isLoading } = useQuery({ queryKey: ['recurring'], queryFn: () => get<Recurring[]>('/recurring') })
  const { memberById, labelById } = useSession()
  const [editing, setEditing] = useState<Partial<Recurring> | null>(null)

  const remove = useMutation({
    mutationFn: (id: string) => del(`/recurring/${id}`),
    onSuccess: () => {
      toast.success('Recurring chore deleted')
      qc.invalidateQueries({ queryKey: ['recurring'] })
      qc.invalidateQueries({ queryKey: ['tasks'] })
    },
  })

  return (
    <div>
      <PageHeader
        title="Recurring chores"
        subtitle="Templates that drop tasks into each sprint automatically — on the right day, for the right person."
        actions={
          <Button variant="primary" icon={<Plus className="size-4" />} onClick={() => setEditing({ ...blank })}>
            New chore
          </Button>
        }
      />
      <Card>
        {isLoading ? (
          <Spinner className="mx-auto my-10" />
        ) : !data?.length ? (
          <EmptyState
            icon={<Repeat className="size-5" />}
            title="No recurring chores yet"
            description="E.g. “Clean bathroom” every Saturday, “Pay electricity bill” on the 5th, or “Take out trash” every other day, rotating between you."
            action={
              <Button variant="primary" onClick={() => setEditing({ ...blank })}>
                Add your first chore
              </Button>
            }
          />
        ) : (
          <ul>
            {data.map((r) => {
              const a = r.assigneeId ? memberById.get(r.assigneeId) : undefined
              return (
                <li key={r.id} className={clsx('flex items-center gap-3 border-b border-border px-4 py-2.5 last:border-b-0', !r.active && 'opacity-55')}>
                  <PriorityIcon priority={r.priority} />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="truncate font-medium">{r.title}</span>
                      {!r.active && <Badge>Paused</Badge>}
                    </div>
                    <div className="mt-0.5 flex flex-wrap items-center gap-1.5 text-[12px] text-muted">
                      <Repeat className="size-3" /> {describe(r)}
                      {r.points != null && <span>· {r.points} pts</span>}
                      {r.labels.map((id) => labelById.get(id)).filter(Boolean).map((l) => <LabelChip key={l!.id} label={l!} />)}
                    </div>
                  </div>
                  {r.rotate ? (
                    <Badge tone="accent">
                      <Shuffle className="size-3" /> Rotates
                    </Badge>
                  ) : (
                    <Avatar name={a?.displayName} color={a?.color} />
                  )}
                  <IconButton label="Edit" onClick={() => setEditing(r)}>
                    <Pencil className="size-3.5" />
                  </IconButton>
                  <IconButton label="Delete" onClick={() => confirm(`Delete “${r.title}”? Upcoming unstarted tasks from it are removed too.`) && remove.mutate(r.id)}>
                    <Trash2 className="size-3.5" />
                  </IconButton>
                </li>
              )
            })}
          </ul>
        )}
      </Card>
      <RecurringEditor value={editing} onClose={() => setEditing(null)} />
    </div>
  )
}

function RecurringEditor({ value, onClose }: { value: Partial<Recurring> | null; onClose: () => void }) {
  const qc = useQueryClient()
  const { members, labels } = useSession()
  const [f, setF] = useState<Partial<Recurring>>({})
  useEffect(() => {
    if (value) setF(value)
  }, [value])
  const set = (p: Partial<Recurring>) => setF((x) => ({ ...x, ...p }))

  const save = useMutation({
    mutationFn: (r: Partial<Recurring>) => (r.id ? patch<Recurring>(`/recurring/${r.id}`, r) : post<Recurring>('/recurring', r)),
    onSuccess: () => {
      toast.success('Chore saved — tasks added to the current and next sprint')
      qc.invalidateQueries({ queryKey: ['recurring'] })
      qc.invalidateQueries({ queryKey: ['tasks'] })
      onClose()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const assign = f.rotate ? 'rotate' : f.assigneeId ?? ''
  return (
    <Modal
      open={!!value}
      onOpenChange={(o) => !o && onClose()}
      title={f.id ? 'Edit recurring chore' : 'New recurring chore'}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={save.isPending} onClick={() => save.mutate(f)}>
            Save
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label="Title">
          <Input autoFocus value={f.title ?? ''} onChange={(e) => set({ title: e.target.value })} placeholder="Clean bathroom" maxLength={200} />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Repeats">
            <Select value={f.freq} onChange={(e) => set({ freq: e.target.value as Recurring['freq'] })}>
              <option value="daily">Daily</option>
              <option value="weekly">Weekly</option>
              <option value="monthly">Monthly</option>
            </Select>
          </Field>
          <Field label={`Every N ${f.freq === 'daily' ? 'days' : f.freq === 'weekly' ? 'weeks' : 'months'}`}>
            <Input type="number" min={1} max={52} value={f.interval ?? 1} onChange={(e) => set({ interval: Math.max(1, Number(e.target.value) || 1) })} />
          </Field>
        </div>
        {f.freq === 'weekly' && (
          <Field label="On">
            <div className="flex flex-wrap gap-1">
              {weekdaysShort.map((d, i) => {
                const on = f.weekdays?.includes(i)
                return (
                  <button
                    key={d}
                    type="button"
                    onClick={() => set({ weekdays: on ? f.weekdays!.filter((x) => x !== i) : [...(f.weekdays ?? []), i] })}
                    className={clsx('h-8 w-11 rounded-md border text-[12px] font-medium', on ? 'border-accent bg-accent-soft text-accent' : 'border-border text-muted hover:bg-surface-2')}
                  >
                    {d}
                  </button>
                )
              })}
            </div>
          </Field>
        )}
        {f.freq === 'monthly' && (
          <Field label="Day of month" hint="Days past the month's end (e.g. 31) fall on the last day.">
            <Input type="number" min={1} max={31} value={f.monthDay ?? 1} onChange={(e) => set({ monthDay: Math.min(31, Math.max(1, Number(e.target.value) || 1)) })} />
          </Field>
        )}
        <div className="grid grid-cols-2 gap-3">
          <Field label="Assign to">
            <Select value={assign} onChange={(e) => (e.target.value === 'rotate' ? set({ rotate: true, assigneeId: null }) : set({ rotate: false, assigneeId: e.target.value || null }))}>
              <option value="">Unassigned</option>
              {members.map((m) => (
                <option key={m.id} value={m.id}>
                  {m.displayName}
                </option>
              ))}
              {members.length > 1 && <option value="rotate">Take turns (rotate)</option>}
            </Select>
          </Field>
          <Field label="Priority">
            <Select value={f.priority ?? 2} onChange={(e) => set({ priority: Number(e.target.value) })}>
              {priorities.map((p) => (
                <option key={p.value} value={p.value}>
                  {p.label}
                </option>
              ))}
            </Select>
          </Field>
        </div>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Points">
            <Select value={f.points ?? ''} onChange={(e) => set({ points: e.target.value ? Number(e.target.value) : null })}>
              <option value="">None</option>
              {[1, 2, 3, 5, 8].map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Label">
            <Select value={f.labels?.[0] ?? ''} onChange={(e) => set({ labels: e.target.value ? [e.target.value] : [] })}>
              <option value="">None</option>
              {labels.map((l) => (
                <option key={l.id} value={l.id}>
                  {l.name}
                </option>
              ))}
            </Select>
          </Field>
        </div>
        <Field label="Notes">
          <Textarea value={f.description ?? ''} onChange={(e) => set({ description: e.target.value })} className="min-h-16" />
        </Field>
        <label className="flex items-center justify-between gap-3">
          <span className="text-[13px]">Active</span>
          <Switch checked={f.active ?? true} onChange={(active) => set({ active })} label="Active" />
        </label>
      </div>
    </Modal>
  )
}
