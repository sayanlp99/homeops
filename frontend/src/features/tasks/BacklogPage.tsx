import clsx from 'clsx'
import { ArrowRightLeft, ChevronDown, Flag, Play, Plus, Target } from 'lucide-react'
import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { Modal } from '@/components/Dialog'
import { Menu, MenuItem, MenuLabel } from '@/components/Menu'
import { Avatar, Badge, Button, Card, IconButton, PageHeader, Spinner } from '@/components/ui'
import { sprintRange } from '@/lib/format'
import { useSession } from '@/lib/session'
import { useCompleteSprint, useSprints, useStartSprint, useUpdateSprintGoal } from '@/lib/sprints'
import { byRank, useCreateTask, useTasks, useUpdateTask } from '@/lib/tasks'
import type { Sprint, Task } from '@/lib/types'
import { TaskRow } from './TaskList'
import { uuid } from '@/lib/uuid'

/**
 * Sprint planning: review the current sprint, pull work from the backlog into next week,
 * check the split between you two, then start the sprint.
 */
export function BacklogPage() {
  const { data: tasks, isLoading } = useTasks()
  const { active, planned, isLoading: sl } = useSprints()
  const [completing, setCompleting] = useState(false)
  const start = useStartSprint()

  const groups = useMemo(() => {
    const ts = (tasks ?? []).slice().sort(byRank)
    return {
      active: active ? ts.filter((t) => t.sprintId === active.id) : [],
      planned: planned ? ts.filter((t) => t.sprintId === planned.id) : [],
      backlog: ts.filter((t) => !t.sprintId && t.status !== 'done'),
    }
  }, [tasks, active, planned])

  if (isLoading || sl) return <Spinner className="mx-auto mt-16" />

  return (
    <div>
      <PageHeader title="Planning" subtitle="Review this week, pull work into next week, then start the sprint." />
      <div className="space-y-4">
        {active && (
          <SprintSection
            sprint={active}
            tasks={groups.active}
            badge={<Badge tone="success">Active</Badge>}
            action={
              <Button size="sm" icon={<Flag className="size-3.5" />} onClick={() => setCompleting(true)}>
                Complete sprint
              </Button>
            }
          />
        )}
        {planned && (
          <SprintSection
            sprint={planned}
            tasks={groups.planned}
            badge={<Badge tone="accent">Next</Badge>}
            defaultOpen
            showSplit
            action={
              <Button
                size="sm"
                variant={active ? 'secondary' : 'primary'}
                icon={<Play className="size-3.5" />}
                disabled={!!active}
                title={active ? 'Complete the current sprint first' : undefined}
                loading={start.isPending}
                onClick={() => start.mutate(planned.id)}
              >
                Start sprint
              </Button>
            }
          />
        )}
        <Section title="Backlog" count={groups.backlog.length} points={sumPoints(groups.backlog)} defaultOpen>
          <InlineAdd sprintId={null} />
          {groups.backlog.map((t) => (
            <TaskRow key={t.id} task={t} actions={<MoveMenu task={t} />} />
          ))}
          {groups.backlog.length === 0 && <p className="px-3 py-6 text-center text-[13px] text-subtle">Backlog is empty. Ideas and someday-tasks go here.</p>}
        </Section>
      </div>
      {active && <CompleteSprintDialog open={completing} onOpenChange={setCompleting} sprint={active} tasks={groups.active} nextNumber={planned?.number} />}
    </div>
  )
}

const sumPoints = (ts: Task[]) => ts.reduce((s, t) => s + (t.points ?? 0), 0)

function Section({ title, count, points, children, defaultOpen = false, badge, action, extra }: { title: ReactNode; count: number; points: number; children: ReactNode; defaultOpen?: boolean; badge?: ReactNode; action?: ReactNode; extra?: ReactNode }) {
  const [open, setOpen] = useState(defaultOpen)
  return (
    <Card className="overflow-hidden">
      <div className="flex min-h-12 flex-wrap items-center gap-2 border-b border-border bg-surface-2/50 px-3 py-2">
        <button onClick={() => setOpen(!open)} className="flex min-w-0 items-center gap-2 text-left">
          <ChevronDown className={clsx('size-4 shrink-0 text-subtle transition-transform', !open && '-rotate-90')} />
          <span className="truncate text-[13px] font-semibold">{title}</span>
        </button>
        {badge}
        <span className="text-[12px] text-subtle tabular">
          {count} {count === 1 ? 'task' : 'tasks'}
          {points > 0 && ` · ${points} pts`}
        </span>
        <div className="ml-auto flex items-center gap-2">{action}</div>
        {extra && <div className="w-full">{extra}</div>}
      </div>
      {open && <div>{children}</div>}
    </Card>
  )
}

function SprintSection({ sprint, tasks, badge, action, defaultOpen, showSplit }: { sprint: Sprint; tasks: Task[]; badge: ReactNode; action: ReactNode; defaultOpen?: boolean; showSplit?: boolean }) {
  return (
    <Section
      title={`Sprint ${sprint.number} · ${sprintRange(sprint)}`}
      count={tasks.length}
      points={sumPoints(tasks)}
      badge={badge}
      action={action}
      defaultOpen={defaultOpen}
      extra={
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2 pt-1 pl-6">
          <GoalInput sprint={sprint} />
          {showSplit && <WorkSplit tasks={tasks} />}
        </div>
      }
    >
      <InlineAdd sprintId={sprint.id} />
      {tasks.map((t) => (
        <TaskRow key={t.id} task={t} actions={<MoveMenu task={t} />} />
      ))}
      {tasks.length === 0 && <p className="px-3 py-6 text-center text-[13px] text-subtle">No tasks yet. Move some in from the backlog.</p>}
    </Section>
  )
}

function GoalInput({ sprint }: { sprint: Sprint }) {
  const [goal, setGoal] = useState(sprint.goal)
  const save = useUpdateSprintGoal()
  useEffect(() => setGoal(sprint.goal), [sprint.goal])
  return (
    <label className="flex min-w-0 flex-1 items-center gap-1.5 sm:max-w-md">
      <Target className="size-3.5 shrink-0 text-subtle" />
      <input
        value={goal}
        onChange={(e) => setGoal(e.target.value)}
        onBlur={() => goal !== sprint.goal && save.mutate({ id: sprint.id, goal })}
        onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
        placeholder="Sprint goal, e.g. Deep-clean the kitchen"
        className="min-w-0 flex-1 rounded bg-transparent px-1 py-0.5 text-[12px] outline-none placeholder:text-subtle hover:bg-surface-3 focus:bg-surface"
        maxLength={300}
      />
    </label>
  )
}

/** Points per person, so the week is planned fairly. */
function WorkSplit({ tasks }: { tasks: Task[] }) {
  const { members } = useSession()
  const total = sumPoints(tasks) || 1
  return (
    <div className="flex items-center gap-3">
      {members.map((m) => {
        const pts = sumPoints(tasks.filter((t) => t.assigneeId === m.id))
        return (
          <span key={m.id} className="flex items-center gap-1.5 text-[12px] text-muted tabular" title={`${Math.round((pts / total) * 100)}% of points`}>
            <Avatar name={m.displayName} color={m.color} size={16} />
            {pts} pts
          </span>
        )
      })}
      <span className="flex items-center gap-1.5 text-[12px] text-subtle tabular">
        <Avatar size={16} />
        {sumPoints(tasks.filter((t) => !t.assigneeId))} pts
      </span>
    </div>
  )
}

function MoveMenu({ task }: { task: Task }) {
  const { active, planned } = useSprints()
  const update = useUpdateTask()
  const move = (sprintId: string | null) =>
    update(task.id, { sprintId, ...(sprintId === null ? { status: task.status === 'done' ? 'done' : 'backlog' } : task.status === 'backlog' ? { status: 'todo' } : {}) })
  return (
    <Menu
      align="end"
      trigger={
        <IconButton label="Move to…" className="size-7 lg:size-7">
          <ArrowRightLeft className="size-3.5" />
        </IconButton>
      }
    >
      <MenuLabel>Move to</MenuLabel>
      {active && (
        <MenuItem checked={task.sprintId === active.id} onSelect={() => move(active.id)}>
          Current sprint ({active.number})
        </MenuItem>
      )}
      {planned && (
        <MenuItem checked={task.sprintId === planned.id} onSelect={() => move(planned.id)}>
          Next sprint ({planned.number})
        </MenuItem>
      )}
      <MenuItem checked={!task.sprintId} onSelect={() => move(null)}>
        Backlog
      </MenuItem>
    </Menu>
  )
}

function InlineAdd({ sprintId }: { sprintId: string | null }) {
  const [title, setTitle] = useState('')
  const create = useCreateTask()
  const { user } = useSession()
  const submit = () => {
    const t = title.trim()
    if (!t) return
    create.mutate({ id: uuid(), title: t, sprintId, status: sprintId ? 'todo' : 'backlog', assigneeId: sprintId ? user.id : null })
    setTitle('')
  }
  return (
    <div className="flex h-11 items-center gap-2.5 border-b border-border px-3 lg:h-9">
      <Plus className="size-3.5 shrink-0 text-subtle" />
      <input
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        onKeyDown={(e) => e.key === 'Enter' && submit()}
        placeholder="Add a task and press Enter"
        className="min-w-0 flex-1 bg-transparent outline-none placeholder:text-subtle"
        maxLength={200}
      />
    </div>
  )
}

function CompleteSprintDialog({ open, onOpenChange, sprint, tasks, nextNumber }: { open: boolean; onOpenChange: (o: boolean) => void; sprint: Sprint; tasks: Task[]; nextNumber?: number }) {
  const complete = useCompleteSprint()
  const [moveTo, setMoveTo] = useState<'next' | 'backlog'>('next')
  const done = tasks.filter((t) => t.status === 'done')
  const open_ = tasks.length - done.length
  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title={`Complete Sprint ${sprint.number}`}
      description={sprintRange(sprint)}
      footer={
        <>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button variant="primary" loading={complete.isPending} onClick={() => complete.mutate({ id: sprint.id, moveTo }, { onSuccess: () => onOpenChange(false) })}>
            Complete sprint
          </Button>
        </>
      }
    >
      <div className="grid grid-cols-3 gap-3">
        <Card className="p-3">
          <div className="text-[11px] text-subtle uppercase">Done</div>
          <div className="text-xl font-semibold text-success tabular">{done.length}</div>
        </Card>
        <Card className="p-3">
          <div className="text-[11px] text-subtle uppercase">Open</div>
          <div className="text-xl font-semibold tabular">{open_}</div>
        </Card>
        <Card className="p-3">
          <div className="text-[11px] text-subtle uppercase">Points</div>
          <div className="text-xl font-semibold tabular">
            {sumPoints(done)}
            <span className="text-[13px] font-normal text-subtle">/{sumPoints(tasks)}</span>
          </div>
        </Card>
      </div>
      {open_ > 0 && (
        <fieldset className="mt-4 space-y-2">
          <legend className="mb-2 text-[13px] font-medium">Move {open_} unfinished {open_ === 1 ? 'task' : 'tasks'} to</legend>
          {(
            [
              ['next', `Next sprint${nextNumber ? ` (Sprint ${nextNumber})` : ''}`],
              ['backlog', 'Backlog'],
            ] as const
          ).map(([v, label]) => (
            <label key={v} className={clsx('flex cursor-pointer items-center gap-2 rounded-md border px-3 py-2 text-[13px]', moveTo === v ? 'border-accent bg-accent-soft' : 'border-border')}>
              <input type="radio" name="moveTo" checked={moveTo === v} onChange={() => setMoveTo(v)} className="accent-[var(--accent)]" />
              {label}
            </label>
          ))}
        </fieldset>
      )}
      <p className="mt-4 text-[12px] text-subtle">
        Tip: hold your retro first —{' '}
        <a href={`/retro?sprint=${sprint.id}`} className="text-accent hover:underline">
          open Sprint {sprint.number}'s retro
        </a>
        . Its action items become tasks in the next sprint.
      </p>
    </Modal>
  )
}
