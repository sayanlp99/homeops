import { useQueryClient } from '@tanstack/react-query'
import { CalendarDays, Hash, Repeat, Sparkles, Tag, User, X } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'
import { Modal } from '@/components/Dialog'
import { Avatar, Button, IconButton, Input, Textarea } from '@/components/ui'
import { post } from '@/lib/api'
import { taskKey } from '@/lib/format'
import { useSession } from '@/lib/session'
import { useSprints } from '@/lib/sprints'
import { type TaskPatch, useCreateTask } from '@/lib/tasks'
import { quickAdd, useTaskParam } from '@/lib/ui'
import { AssigneeMenu, ChipButton, LabelsMenu, PriorityIcon, PriorityMenu, SprintMenu, StatusIcon, StatusMenu } from './TaskBits'
import { priorities, statuses } from '@/lib/tasks'
import { uuid } from '@/lib/uuid'

const POINTS = [1, 2, 3, 5, 8]
const DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

type RepeatRule = { freq: 'daily' | 'weekly' | 'monthly'; interval: number; weekdays: number[]; monthDay: number | null }
type AITask = Required<Pick<TaskPatch, 'title' | 'description' | 'assigneeId' | 'dueDate' | 'priority' | 'labels'>> & { repeat: RepeatRule | null }

const ordinal = (n: number) => n + (n % 10 === 1 && n !== 11 ? 'st' : n % 10 === 2 && n !== 12 ? 'nd' : n % 10 === 3 && n !== 13 ? 'rd' : 'th')

function repeatLabel(r: RepeatRule) {
  const every = (one: string, many: string) => (r.interval === 1 ? `Every ${one}` : `Every ${r.interval} ${many}`)
  if (r.freq === 'daily') return every('day', 'days')
  if (r.freq === 'weekly') return `${every('week', 'weeks')} on ${[...r.weekdays].sort((a, b) => ((a + 6) % 7) - ((b + 6) % 7)).map((d) => DAYS[d]).join(', ')}`
  return `${every('month', 'months')} on the ${ordinal(r.monthDay ?? 1)}`
}

export function QuickAddTask() {
  const { open, defaults } = quickAdd.use()
  const { user, memberById, labelById, aiEnabled } = useSession()
  const qc = useQueryClient()
  const { active, planned } = useSprints()
  const create = useCreateTask()
  const { open: openTask } = useTaskParam()
  const titleRef = useRef<HTMLInputElement>(null)
  const [form, setForm] = useState<TaskPatch>({})
  const [another, setAnother] = useState(false)
  const [sentence, setSentence] = useState('')
  const [reading, setReading] = useState(false)
  const [repeat, setRepeat] = useState<RepeatRule | null>(null)
  const [savingChore, setSavingChore] = useState(false)

  useEffect(() => {
    if (open) {
      setForm({ status: 'todo', priority: 2, labels: [], assigneeId: user.id, sprintId: active?.id ?? null, ...defaults })
      setSentence('')
      setRepeat(null)
    }
  }, [open, defaults, user.id, active?.id])

  const fillFromSentence = async () => {
    if (!sentence.trim()) return
    setReading(true)
    try {
      const { repeat, ...t } = await post<AITask>('/ai/task', { text: sentence })
      set(t)
      setRepeat(repeat)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Couldn't read that")
    } finally {
      setReading(false)
    }
  }

  // A repeating sentence becomes a recurring chore, which makes its own tasks on schedule.
  const createChore = async (title: string) => {
    setSavingChore(true)
    try {
      await post('/recurring', { title, description: form.description ?? '', priority: form.priority ?? 2, points: form.points ?? null, labels: form.labels ?? [], assigneeId: form.assigneeId ?? null, ...repeat })
      qc.invalidateQueries({ queryKey: ['recurring'] })
      qc.invalidateQueries({ queryKey: ['tasks'] })
      toast.success(`Recurring chore added: ${repeatLabel(repeat!).toLowerCase()}`)
      close()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Couldn't add the chore")
    } finally {
      setSavingChore(false)
    }
  }

  const set = (p: TaskPatch) => setForm((f) => ({ ...f, ...p }))
  const close = () => quickAdd.set({ open: false, defaults: {} })

  const submit = (e?: React.FormEvent) => {
    e?.preventDefault()
    const title = form.title?.trim()
    if (!title) return titleRef.current?.focus()
    if (repeat) return createChore(title)
    const id = uuid()
    const status = form.sprintId ? (form.status === 'backlog' ? 'todo' : form.status) : 'backlog'
    create.mutate(
      { ...form, id, title, status },
      { onSuccess: (t) => toast.success(`${taskKey(t.number)} created`, { action: { label: 'Open', onClick: () => openTask(t) } }) },
    )
    if (!navigator.onLine) toast.message('Saved offline — will sync when you reconnect')
    if (another) {
      set({ title: '', description: '' })
      titleRef.current?.focus()
    } else {
      close()
    }
  }

  const assignee = form.assigneeId ? memberById.get(form.assigneeId) : undefined
  const sprint = [active, planned].find((s) => s && s.id === form.sprintId)

  return (
    <Modal
      open={open}
      onOpenChange={(o) => !o && close()}
      title="New task"
      width="sm:max-w-xl"
      footer={
        <div className="flex w-full items-center justify-between gap-3">
          <label className="flex items-center gap-2 text-[12px] text-muted">
            <input type="checkbox" checked={another} onChange={(e) => setAnother(e.target.checked)} className="accent-[var(--accent)]" />
            Create another
          </label>
          <div className="flex gap-2">
            <Button variant="ghost" onClick={close}>
              Cancel
            </Button>
            <Button variant="primary" loading={savingChore} onClick={() => submit()}>
              {repeat ? 'Create chore' : 'Create task'}
            </Button>
          </div>
        </div>
      }
    >
      {aiEnabled && (
        <form
          className="mb-3 flex gap-2"
          onSubmit={(e) => {
            e.preventDefault()
            fillFromSentence()
          }}
        >
          <Input
            placeholder="Or say it: remind Sangita to pay the maid every 5th"
            value={sentence}
            onChange={(e) => setSentence(e.target.value)}
            maxLength={1000}
            enterKeyHint="go"
            aria-label="Describe the task in a sentence"
          />
          <Button type="submit" loading={reading} disabled={!sentence.trim()} icon={<Sparkles className="size-4" />}>
            Fill in
          </Button>
        </form>
      )}
      <form onSubmit={submit} className="space-y-3">
        <Input
          ref={titleRef}
          autoFocus
          placeholder="What needs doing? e.g. Clean the fridge"
          value={form.title ?? ''}
          onChange={(e) => set({ title: e.target.value })}
          className="h-11 text-[15px] font-medium lg:h-10"
          maxLength={200}
        />
        <Textarea
          placeholder="Add details (optional)"
          value={form.description ?? ''}
          onChange={(e) => set({ description: e.target.value })}
          className="min-h-20"
          onKeyDown={(e) => {
            if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) submit()
          }}
        />
        {repeat && (
          <div className="flex items-center gap-2 rounded-md border border-accent/40 bg-accent-soft px-2.5 py-1.5 text-[12px] text-accent">
            <Repeat className="size-3.5 shrink-0" />
            <span className="min-w-0 flex-1">
              {repeatLabel(repeat)} · saved as a recurring chore
            </span>
            <IconButton type="button" label="Make it a one-off task" onClick={() => setRepeat(null)} className="size-6 text-accent">
              <X className="size-3.5" />
            </IconButton>
          </div>
        )}
        <div className="flex flex-wrap gap-1.5">
          {!repeat && (
            <StatusMenu value={form.status ?? 'todo'} onChange={(status) => set({ status })}>
              <ChipButton>
                <StatusIcon status={form.status ?? 'todo'} />
                {statuses.find((s) => s.value === form.status)?.label}
              </ChipButton>
            </StatusMenu>
          )}
          <PriorityMenu value={form.priority ?? 2} onChange={(priority) => set({ priority })}>
            <ChipButton>
              <PriorityIcon priority={form.priority ?? 2} />
              {priorities[form.priority ?? 2]!.label}
            </ChipButton>
          </PriorityMenu>
          <AssigneeMenu value={form.assigneeId ?? null} onChange={(assigneeId) => set({ assigneeId })}>
            <ChipButton>
              {assignee ? <Avatar name={assignee.displayName} color={assignee.color} size={16} /> : <User className="size-3.5 text-subtle" />}
              {assignee?.displayName ?? 'Unassigned'}
            </ChipButton>
          </AssigneeMenu>
          {!repeat && (
            <SprintMenu value={form.sprintId ?? null} onChange={(sprintId) => set({ sprintId })}>
              <ChipButton>{sprint ? `Sprint ${sprint.number}${sprint.status === 'active' ? ' (current)' : ' (next)'}` : 'Backlog'}</ChipButton>
            </SprintMenu>
          )}
          <LabelsMenu value={form.labels ?? []} onChange={(labels) => set({ labels })}>
            <ChipButton>
              <Tag className="size-3.5 text-subtle" />
              {form.labels?.length ? form.labels.map((id) => labelById.get(id)?.name).join(', ') : 'Labels'}
            </ChipButton>
          </LabelsMenu>
          {!repeat && (
            <label className="inline-flex h-7 items-center gap-1.5 rounded-md border border-border bg-surface px-2 text-[12px]">
              <CalendarDays className="size-3.5 text-subtle" />
              <input
                type="date"
                value={form.dueDate ?? ''}
                onChange={(e) => set({ dueDate: e.target.value || null })}
                className="w-[7.5rem] bg-transparent text-[12px] outline-none"
                aria-label="Due date"
              />
            </label>
          )}
        </div>
        <div className="flex items-center gap-2">
          <span className="flex items-center gap-1 text-[12px] text-subtle">
            <Hash className="size-3.5" /> Points
          </span>
          {POINTS.map((p) => (
            <button
              key={p}
              type="button"
              onClick={() => set({ points: form.points === p ? null : p })}
              className={`h-7 min-w-8 rounded-md border px-2 text-[12px] font-semibold tabular ${form.points === p ? 'border-accent bg-accent-soft text-accent' : 'border-border text-muted hover:bg-surface-2'}`}
            >
              {p}
            </button>
          ))}
        </div>
      </form>
    </Modal>
  )
}
