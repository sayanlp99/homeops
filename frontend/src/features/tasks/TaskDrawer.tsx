import { Link2, PartyPopper, Repeat, Trash2, User } from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'
import { toast } from 'sonner'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Sheet } from '@/components/Dialog'
import { post } from '@/lib/api'
import { Avatar, Button, EmptyState, IconButton, Textarea } from '@/components/ui'
import { formatDateTime, sprintRange, taskKey } from '@/lib/format'
import { useSession } from '@/lib/session'
import { useSprints } from '@/lib/sprints'
import { priorities, statuses, useDeleteTask, useTasks, useUpdateTask } from '@/lib/tasks'
import { findTask, useTaskParam } from '@/lib/ui'
import { AssigneeMenu, ChipButton, LabelChip, LabelsMenu, PriorityIcon, PriorityMenu, SprintMenu, StatusIcon, StatusMenu } from './TaskBits'

const POINTS = [null, 1, 2, 3, 5, 8, 13]

export function TaskDrawer() {
  const { value, close } = useTaskParam()
  const { data: tasks } = useTasks()
  const task = findTask(tasks, value)
  const update = useUpdateTask()
  const remove = useDeleteTask()
  const { memberById, labelById, user } = useSession()
  const qc = useQueryClient()
  const kudos = useMutation({
    mutationFn: (v: { toId: string; taskId: string }) => post('/kudos', { ...v, emoji: '👏' }),
    onSuccess: (_d, v) => {
      toast.success(`👏 Kudos sent to ${memberById.get(v.toId)?.displayName}`)
      qc.invalidateQueries({ queryKey: ['team'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })
  const { sprints } = useSprints()
  const [title, setTitle] = useState('')
  const [desc, setDesc] = useState('')
  const [confirmDelete, setConfirmDelete] = useState(false)

  useEffect(() => {
    if (task) {
      setTitle(task.title)
      setDesc(task.description)
    }
    setConfirmDelete(false)
  }, [task?.id, task?.title, task?.description]) // eslint-disable-line react-hooks/exhaustive-deps

  const open = value !== null
  const sprint = task?.sprintId ? sprints.find((s) => s.id === task.sprintId) : undefined
  const assignee = task?.assigneeId ? memberById.get(task.assigneeId) : undefined

  const saveTitle = () => {
    if (!task) return
    const t = title.trim()
    if (!t) return setTitle(task.title)
    if (t !== task.title) update(task.id, { title: t })
  }
  const saveDesc = () => task && desc !== task.description && update(task.id, { description: desc })

  const copyLink = () => {
    if (!task) return
    navigator.clipboard?.writeText(`${location.origin}/tasks/${task.number}`)
    toast.success('Link copied')
  }

  return (
    <Sheet
      open={open}
      onOpenChange={(o) => !o && close()}
      title={task ? <span className="font-mono">{taskKey(task.number)}</span> : 'Task'}
      headerActions={
        task && (
          <>
            <IconButton label="Copy link" onClick={copyLink}>
              <Link2 className="size-4" />
            </IconButton>
            <IconButton label="Delete task" onClick={() => setConfirmDelete(true)}>
              <Trash2 className="size-4" />
            </IconButton>
          </>
        )
      }
    >
      {!task ? (
        <EmptyState title="Task not found" description="It may have been deleted, or it's from an older sprint." />
      ) : (
        <div className="px-4 py-4 lg:px-5">
          {confirmDelete && (
            <div className="mb-4 flex items-center justify-between gap-3 rounded-md border border-danger/30 bg-danger-soft px-3 py-2 text-[13px]">
              <span className="text-danger">Delete this task permanently?</span>
              <div className="flex gap-2">
                <Button size="sm" variant="ghost" onClick={() => setConfirmDelete(false)}>
                  Cancel
                </Button>
                <Button
                  size="sm"
                  variant="danger"
                  onClick={() => {
                    remove(task.id)
                    close()
                    toast.success(`${taskKey(task.number)} deleted`)
                  }}
                >
                  Delete
                </Button>
              </div>
            </div>
          )}

          <textarea
            value={title}
            onChange={(e) => setTitle(e.target.value.replace(/\n/g, ''))}
            onBlur={saveTitle}
            onKeyDown={(e) => e.key === 'Enter' && (e.preventDefault(), e.currentTarget.blur())}
            rows={1}
            maxLength={200}
            className="field-sizing-content w-full resize-none bg-transparent text-[18px] leading-snug font-semibold outline-none"
            aria-label="Title"
          />
          {task.recurringId && (
            <p className="mt-1 flex items-center gap-1 text-[12px] text-subtle">
              <Repeat className="size-3" /> Generated from a recurring chore
            </p>
          )}

          <dl className="mt-4 grid grid-cols-[96px_1fr] items-center gap-x-3 gap-y-2 text-[13px]">
            <Prop label="Status">
              <StatusMenu value={task.status} onChange={(status) => update(task.id, { status })}>
                <ChipButton>
                  <StatusIcon status={task.status} />
                  {statuses.find((s) => s.value === task.status)?.label}
                </ChipButton>
              </StatusMenu>
            </Prop>
            <Prop label="Priority">
              <PriorityMenu value={task.priority} onChange={(priority) => update(task.id, { priority })}>
                <ChipButton>
                  <PriorityIcon priority={task.priority} />
                  {priorities[task.priority]!.label}
                </ChipButton>
              </PriorityMenu>
            </Prop>
            <Prop label="Assignee">
              <AssigneeMenu value={task.assigneeId} onChange={(assigneeId) => update(task.id, { assigneeId })}>
                <ChipButton>
                  {assignee ? <Avatar name={assignee.displayName} color={assignee.color} size={16} /> : <User className="size-3.5 text-subtle" />}
                  {assignee?.displayName ?? 'Unassigned'}
                </ChipButton>
              </AssigneeMenu>
            </Prop>
            <Prop label="Sprint">
              <SprintMenu value={task.sprintId} onChange={(sprintId) => update(task.id, { sprintId, ...(sprintId === null && task.status !== 'done' ? { status: 'backlog' } : task.status === 'backlog' ? { status: 'todo' } : {}) })}>
                <ChipButton>{sprint ? `Sprint ${sprint.number} · ${sprintRange(sprint)}` : 'Backlog'}</ChipButton>
              </SprintMenu>
            </Prop>
            <Prop label="Due date">
              <input
                type="date"
                value={task.dueDate ?? ''}
                onChange={(e) => update(task.id, { dueDate: e.target.value || null })}
                className="h-7 rounded-md border border-border bg-surface px-2 text-[12px] outline-none focus:border-accent"
              />
            </Prop>
            <Prop label="Points">
              <div className="flex flex-wrap gap-1">
                {POINTS.map((p) => (
                  <button
                    key={String(p)}
                    onClick={() => update(task.id, { points: p })}
                    className={`h-7 min-w-8 rounded-md border px-2 text-[12px] font-semibold tabular ${task.points === p ? 'border-accent bg-accent-soft text-accent' : 'border-border text-muted hover:bg-surface-2'}`}
                  >
                    {p ?? '–'}
                  </button>
                ))}
              </div>
            </Prop>
            <Prop label="Labels">
              <LabelsMenu value={task.labels} onChange={(labels) => update(task.id, { labels })}>
                <button className="flex min-h-7 flex-wrap items-center gap-1 rounded-md px-1 text-left hover:bg-surface-2">
                  {task.labels.length === 0 && <span className="px-1 text-[12px] text-subtle">Add labels</span>}
                  {task.labels.map((id) => labelById.get(id)).filter(Boolean).map((l) => <LabelChip key={l!.id} label={l!} />)}
                </button>
              </LabelsMenu>
            </Prop>
          </dl>

          <div className="mt-5">
            <div className="mb-1.5 text-[12px] font-medium text-muted">Description</div>
            <Textarea value={desc} onChange={(e) => setDesc(e.target.value)} onBlur={saveDesc} placeholder="Add notes, steps, links…" className="min-h-32" />
          </div>

          <div className="mt-6 space-y-1 border-t border-border pt-3 text-[12px] text-subtle">
            {task.reporterId && <p>Created by {memberById.get(task.reporterId)?.displayName ?? 'someone'} · {formatDateTime(task.createdAt)}</p>}
            {!task.reporterId && <p>Created {formatDateTime(task.createdAt)}</p>}
            {task.completedAt && task.completedBy && task.completedBy !== user.id && (
              <div className="my-2">
                <Button size="sm" icon={<PartyPopper className="size-3.5" />} loading={kudos.isPending} onClick={() => kudos.mutate({ toId: task.completedBy!, taskId: task.id })}>
                  Give {memberById.get(task.completedBy)?.displayName ?? 'them'} kudos
                </Button>
              </div>
            )}
            {task.completedAt && (
              <p className="text-success">
                Completed {task.completedBy ? `by ${memberById.get(task.completedBy)?.displayName ?? 'someone'} ` : ''}· {formatDateTime(task.completedAt)}
              </p>
            )}
          </div>
        </div>
      )}
    </Sheet>
  )
}

function Prop({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <dt className="text-[12px] text-subtle">{label}</dt>
      <dd className="min-w-0">{children}</dd>
    </>
  )
}
