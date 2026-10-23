import { CalendarDays, Hash, Tag, User } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'
import { Modal } from '@/components/Dialog'
import { Avatar, Button, Input, Textarea } from '@/components/ui'
import { taskKey } from '@/lib/format'
import { useSession } from '@/lib/session'
import { useSprints } from '@/lib/sprints'
import { type TaskPatch, useCreateTask } from '@/lib/tasks'
import { quickAdd, useTaskParam } from '@/lib/ui'
import { AssigneeMenu, ChipButton, LabelsMenu, PriorityIcon, PriorityMenu, SprintMenu, StatusIcon, StatusMenu } from './TaskBits'
import { priorities, statuses } from '@/lib/tasks'
import { uuid } from '@/lib/uuid'

const POINTS = [1, 2, 3, 5, 8]

export function QuickAddTask() {
  const { open, defaults } = quickAdd.use()
  const { user, memberById, labelById } = useSession()
  const { active, planned } = useSprints()
  const create = useCreateTask()
  const { open: openTask } = useTaskParam()
  const titleRef = useRef<HTMLInputElement>(null)
  const [form, setForm] = useState<TaskPatch>({})
  const [another, setAnother] = useState(false)

  useEffect(() => {
    if (open) setForm({ status: 'todo', priority: 2, labels: [], assigneeId: user.id, sprintId: active?.id ?? null, ...defaults })
  }, [open, defaults, user.id, active?.id])

  const set = (p: TaskPatch) => setForm((f) => ({ ...f, ...p }))
  const close = () => quickAdd.set({ open: false, defaults: {} })

  const submit = (e?: React.FormEvent) => {
    e?.preventDefault()
    const title = form.title?.trim()
    if (!title) return titleRef.current?.focus()
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
            <Button variant="primary" onClick={() => submit()}>
              Create task
            </Button>
          </div>
        </div>
      }
    >
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
        <div className="flex flex-wrap gap-1.5">
          <StatusMenu value={form.status ?? 'todo'} onChange={(status) => set({ status })}>
            <ChipButton>
              <StatusIcon status={form.status ?? 'todo'} />
              {statuses.find((s) => s.value === form.status)?.label}
            </ChipButton>
          </StatusMenu>
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
          <SprintMenu value={form.sprintId ?? null} onChange={(sprintId) => set({ sprintId })}>
            <ChipButton>{sprint ? `Sprint ${sprint.number}${sprint.status === 'active' ? ' (current)' : ' (next)'}` : 'Backlog'}</ChipButton>
          </SprintMenu>
          <LabelsMenu value={form.labels ?? []} onChange={(labels) => set({ labels })}>
            <ChipButton>
              <Tag className="size-3.5 text-subtle" />
              {form.labels?.length ? form.labels.map((id) => labelById.get(id)?.name).join(', ') : 'Labels'}
            </ChipButton>
          </LabelsMenu>
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
