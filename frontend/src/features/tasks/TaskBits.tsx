import clsx from 'clsx'
import { CalendarDays, Repeat } from 'lucide-react'
import type { ReactNode } from 'react'
import { Menu, MenuItem, MenuLabel, MenuSeparator } from '@/components/Menu'
import { Avatar } from '@/components/ui'
import { dueState, relativeDay, sprintRange } from '@/lib/format'
import { useSession } from '@/lib/session'
import { useSprints } from '@/lib/sprints'
import { priorities, statuses } from '@/lib/tasks'
import type { Label, Task, TaskStatus } from '@/lib/types'

export function StatusIcon({ status, className }: { status: TaskStatus; className?: string }) {
  const cls = clsx('size-3.5 shrink-0', className)
  switch (status) {
    case 'backlog':
      return (
        <svg viewBox="0 0 14 14" className={clsx(cls, 'text-subtle')}>
          <circle cx="7" cy="7" r="5.5" fill="none" stroke="currentColor" strokeWidth="1.5" strokeDasharray="2.2 2" />
        </svg>
      )
    case 'todo':
      return (
        <svg viewBox="0 0 14 14" className={clsx(cls, 'text-muted')}>
          <circle cx="7" cy="7" r="5.5" fill="none" stroke="currentColor" strokeWidth="1.5" />
        </svg>
      )
    case 'in_progress':
      return (
        <svg viewBox="0 0 14 14" className={clsx(cls, 'text-warning')}>
          <circle cx="7" cy="7" r="5.5" fill="none" stroke="currentColor" strokeWidth="1.5" />
          <path d="M7 3.5a3.5 3.5 0 0 1 0 7Z" fill="currentColor" />
        </svg>
      )
    case 'done':
      return (
        <svg viewBox="0 0 14 14" className={clsx(cls, 'text-accent')}>
          <circle cx="7" cy="7" r="6.25" fill="currentColor" />
          <path d="m4.4 7.2 1.8 1.8 3.5-3.6" fill="none" stroke="var(--surface)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      )
  }
}

export function PriorityIcon({ priority, className }: { priority: number; className?: string }) {
  const cls = clsx('size-3.5 shrink-0', className)
  if (priority === 4)
    return (
      <svg viewBox="0 0 14 14" className={clsx(cls, 'text-danger')}>
        <rect x="1" y="1" width="12" height="12" rx="3" fill="currentColor" />
        <path d="M7 3.8v3.6M7 9.6v.2" stroke="#fff" strokeWidth="1.6" strokeLinecap="round" />
      </svg>
    )
  if (priority === 0)
    return (
      <svg viewBox="0 0 14 14" className={clsx(cls, 'text-subtle')}>
        {[2, 6, 10].map((x) => (
          <rect key={x} x={x} y="6.25" width="2" height="1.5" rx=".5" fill="currentColor" />
        ))}
      </svg>
    )
  return (
    <svg viewBox="0 0 14 14" className={clsx(cls, 'text-muted')}>
      {[
        [1.5, 8, 4],
        [5.75, 5, 7],
        [10, 2, 10],
      ].map(([x, y, h], i) => (
        <rect key={i} x={x} y={y} width="2.5" height={h} rx=".6" fill="currentColor" opacity={i < priority ? 1 : 0.25} />
      ))}
    </svg>
  )
}

export function LabelChip({ label }: { label: Label }) {
  return (
    <span className="inline-flex h-5 max-w-32 items-center gap-1 rounded-full border border-border px-1.5 text-[11px] text-muted">
      <span className="size-1.5 shrink-0 rounded-full" style={{ background: label.color }} />
      <span className="truncate">{label.name}</span>
    </span>
  )
}

export function DueBadge({ date, done }: { date: string; done?: boolean }) {
  const state = done ? 'later' : dueState(date)
  return (
    <span
      className={clsx(
        'inline-flex h-5 items-center gap-1 rounded px-1 text-[11px] font-medium whitespace-nowrap',
        state === 'overdue' && 'bg-danger-soft text-danger',
        state === 'today' && 'bg-warning-soft text-warning',
        (state === 'soon' || state === 'later') && 'text-muted',
      )}
    >
      <CalendarDays className="size-3" />
      {relativeDay(date)}
    </span>
  )
}

export function PointsPill({ points }: { points: number | null }) {
  if (points == null) return null
  return (
    <span className="inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-surface-3 px-1.5 text-[11px] font-semibold tabular text-muted" title={`${points} points`}>
      {points}
    </span>
  )
}

export function RecurringMark() {
  return <Repeat className="size-3 shrink-0 text-subtle" aria-label="Recurring chore" />
}

export function TaskMeta({ task, compact }: { task: Task; compact?: boolean }) {
  const { labelById } = useSession()
  return (
    <>
      {!compact && task.labels.map((id) => labelById.get(id)).filter(Boolean).slice(0, 2).map((l) => <LabelChip key={l!.id} label={l!} />)}
      {task.dueDate && <DueBadge date={task.dueDate} done={task.status === 'done'} />}
      <PointsPill points={task.points} />
    </>
  )
}

// ---- pickers ----

type PickerProps<T> = { value: T; onChange: (v: T) => void; children: ReactNode }

export function StatusMenu({ value, onChange, children }: PickerProps<TaskStatus>) {
  return (
    <Menu trigger={children} width="w-48">
      {statuses.map((s) => (
        <MenuItem key={s.value} icon={<StatusIcon status={s.value} />} checked={value === s.value} onSelect={() => onChange(s.value)}>
          {s.label}
        </MenuItem>
      ))}
    </Menu>
  )
}

export function PriorityMenu({ value, onChange, children }: PickerProps<number>) {
  return (
    <Menu trigger={children} width="w-44">
      {[...priorities].reverse().map((p) => (
        <MenuItem key={p.value} icon={<PriorityIcon priority={p.value} />} checked={value === p.value} onSelect={() => onChange(p.value)}>
          {p.label}
        </MenuItem>
      ))}
    </Menu>
  )
}

export function AssigneeMenu({ value, onChange, children }: PickerProps<string | null>) {
  const { members, user } = useSession()
  return (
    <Menu trigger={children} width="w-52">
      <MenuItem icon={<Avatar size={16} />} checked={value === null} onSelect={() => onChange(null)}>
        Unassigned
      </MenuItem>
      {members.map((m) => (
        <MenuItem key={m.id} icon={<Avatar name={m.displayName} color={m.color} size={16} />} checked={value === m.id} onSelect={() => onChange(m.id)}>
          {m.displayName}
          {m.id === user.id && <span className="text-subtle"> (you)</span>}
        </MenuItem>
      ))}
    </Menu>
  )
}

export function LabelsMenu({ value, onChange, children }: PickerProps<string[]>) {
  const { labels } = useSession()
  const toggle = (id: string) => onChange(value.includes(id) ? value.filter((x) => x !== id) : [...value, id])
  return (
    <Menu trigger={children} width="w-52">
      {labels.length === 0 && <MenuLabel>No labels yet — add them in Settings</MenuLabel>}
      {labels.map((l) => (
        <MenuItem key={l.id} keepOpen icon={<span className="size-2 rounded-full" style={{ background: l.color }} />} checked={value.includes(l.id)} onSelect={() => toggle(l.id)}>
          {l.name}
        </MenuItem>
      ))}
    </Menu>
  )
}

export function SprintMenu({ value, onChange, children }: PickerProps<string | null>) {
  const { active, planned } = useSprints()
  return (
    <Menu trigger={children} width="w-60">
      <MenuItem checked={value === null} onSelect={() => onChange(null)}>
        Backlog
      </MenuItem>
      <MenuSeparator />
      {active && (
        <MenuItem checked={value === active.id} onSelect={() => onChange(active.id)}>
          Sprint {active.number} <span className="text-subtle">· current · {sprintRange(active)}</span>
        </MenuItem>
      )}
      {planned && (
        <MenuItem checked={value === planned.id} onSelect={() => onChange(planned.id)}>
          Sprint {planned.number} <span className="text-subtle">· next · {sprintRange(planned)}</span>
        </MenuItem>
      )}
    </Menu>
  )
}

/** A compact, borderless button used as a picker trigger inside rows and the drawer. */
export function ChipButton({ children, className, ...rest }: React.ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      type="button"
      className={clsx('inline-flex h-7 max-w-full items-center gap-1.5 rounded-md border border-border bg-surface px-2 text-[12px] text-text hover:bg-surface-2', className)}
      {...rest}
    >
      {children}
    </button>
  )
}
