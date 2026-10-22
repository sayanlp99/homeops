import clsx from 'clsx'
import { forwardRef, type HTMLAttributes } from 'react'
import { Avatar } from '@/components/ui'
import { taskKey } from '@/lib/format'
import { useSession } from '@/lib/session'
import { useUpdateTask } from '@/lib/tasks'
import type { Task } from '@/lib/types'
import { useTaskParam } from '@/lib/ui'
import { AssigneeMenu, PriorityIcon, RecurringMark, StatusIcon, StatusMenu, TaskMeta } from './TaskBits'

/** Dense one-line row, Jira-backlog style. */
export function TaskRow({ task, className, actions }: { task: Task; className?: string; actions?: React.ReactNode }) {
  const { open } = useTaskParam()
  const update = useUpdateTask()
  const { memberById } = useSession()
  const assignee = task.assigneeId ? memberById.get(task.assigneeId) : undefined
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() => open(task)}
      onKeyDown={(e) => e.key === 'Enter' && open(task)}
      className={clsx(
        'group flex h-11 cursor-default items-center gap-2.5 border-b border-border px-3 last:border-b-0 hover:bg-surface-2 lg:h-9',
        task.number === 0 && 'opacity-70',
        className,
      )}
    >
      <StatusMenu value={task.status} onChange={(status) => update(task.id, { status })}>
        <button className="-m-1 rounded p-1 hover:bg-surface-3" aria-label="Change status" onClick={(e) => e.stopPropagation()}>
          <StatusIcon status={task.status} />
        </button>
      </StatusMenu>
      <span className="hidden w-12 shrink-0 font-mono text-[11px] text-subtle sm:block">{taskKey(task.number)}</span>
      <PriorityIcon priority={task.priority} className="hidden sm:block" />
      <span className={clsx('min-w-0 flex-1 truncate', task.status === 'done' && 'text-subtle line-through decoration-border-strong')}>{task.title}</span>
      {task.recurringId && <RecurringMark />}
      <div className="hidden shrink-0 items-center gap-1.5 md:flex">
        <TaskMeta task={task} />
      </div>
      <div className="flex shrink-0 items-center gap-1.5 md:hidden">
        <TaskMeta task={task} compact />
      </div>
      <AssigneeMenu value={task.assigneeId} onChange={(assigneeId) => update(task.id, { assigneeId })}>
        <button className="shrink-0 rounded-full" aria-label="Change assignee" onClick={(e) => e.stopPropagation()}>
          <Avatar name={assignee?.displayName} color={assignee?.color} size={22} />
        </button>
      </AssigneeMenu>
      {actions && (
        <div className="shrink-0" onClick={(e) => e.stopPropagation()}>
          {actions}
        </div>
      )}
    </div>
  )
}

/** Kanban card. Forwarded ref + props let dnd-kit make it draggable. */
export const TaskCard = forwardRef<HTMLDivElement, { task: Task; dragging?: boolean } & HTMLAttributes<HTMLDivElement>>(function TaskCard(
  { task, dragging, className, ...rest },
  ref,
) {
  const { open } = useTaskParam()
  const { memberById } = useSession()
  const assignee = task.assigneeId ? memberById.get(task.assigneeId) : undefined
  return (
    <div
      ref={ref}
      onClick={() => open(task)}
      className={clsx(
        'cursor-default rounded-md border border-border bg-surface p-2.5 shadow-card transition-shadow select-none hover:border-border-strong',
        dragging && 'rotate-1 shadow-pop ring-2 ring-accent/40',
        task.status === 'done' && 'opacity-70',
        task.number === 0 && 'border-dashed',
        className,
      )}
      {...rest}
    >
      <div className="flex items-start gap-2">
        <span className={clsx('min-w-0 flex-1 text-[13px] leading-snug', task.status === 'done' && 'text-muted line-through decoration-border-strong')}>{task.title}</span>
        <Avatar name={assignee?.displayName} color={assignee?.color} size={20} />
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-1.5">
        <PriorityIcon priority={task.priority} />
        <span className="font-mono text-[11px] text-subtle">{taskKey(task.number)}</span>
        {task.recurringId && <RecurringMark />}
        <TaskMeta task={task} />
      </div>
    </div>
  )
})
