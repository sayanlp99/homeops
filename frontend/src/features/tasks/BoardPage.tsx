import {
  DndContext,
  DragOverlay,
  KeyboardSensor,
  PointerSensor,
  closestCorners,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragOverEvent,
  type DragStartEvent,
} from '@dnd-kit/core'
import { SortableContext, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { differenceInCalendarDays, parseISO } from 'date-fns'
import { ListTodo, Plus, Tag } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { Menu, MenuItem } from '@/components/Menu'
import { Button, Card, EmptyState, PageHeader, Progress, Segmented, Spinner } from '@/components/ui'
import { sprintRange } from '@/lib/format'
import { useIsDesktop } from '@/lib/hooks'
import { useSession } from '@/lib/session'
import { useSprints } from '@/lib/sprints'
import { byRank, rankBetween, useTasks, useUpdateTask } from '@/lib/tasks'
import type { Task, TaskStatus } from '@/lib/types'
import { openQuickAdd } from '@/lib/ui'
import { StatusIcon } from './TaskBits'
import { TaskCard } from './TaskList'

const COLUMNS: { status: TaskStatus; label: string }[] = [
  { status: 'todo', label: 'To Do' },
  { status: 'in_progress', label: 'In Progress' },
  { status: 'done', label: 'Done' },
]

type Who = 'all' | 'mine' | 'partner' | 'unassigned'

export function BoardPage() {
  const { user, partner, labelById, labels } = useSession()
  const { active, isLoading: sprintsLoading } = useSprints()
  const { data: tasks, isLoading } = useTasks()
  const [params, setParams] = useSearchParams()
  const who = (params.get('mine') ? 'mine' : (params.get('who') as Who)) || 'all'
  const labelFilter = params.get('label')
  const isDesktop = useIsDesktop()
  const [mobileCol, setMobileCol] = useState<TaskStatus>('todo')

  const setWho = (w: Who) =>
    setParams((p) => {
      p.delete('mine')
      if (w === 'all') p.delete('who')
      else p.set('who', w)
      return p
    })
  const setLabel = (id: string | null) =>
    setParams((p) => {
      if (id) p.set('label', id)
      else p.delete('label')
      return p
    })

  const sprintTasks = useMemo(() => {
    if (!active || !tasks) return []
    return tasks
      .filter((t) => t.sprintId === active.id && t.status !== 'backlog')
      .filter((t) => (who === 'mine' ? t.assigneeId === user.id : who === 'partner' ? t.assigneeId === partner?.id : who === 'unassigned' ? !t.assigneeId : true))
      .filter((t) => !labelFilter || t.labels.includes(labelFilter))
      .sort(byRank)
  }, [tasks, active, who, labelFilter, user.id, partner?.id])

  if (isLoading || sprintsLoading) return <Spinner className="mx-auto mt-16" />
  if (!active)
    return (
      <Card>
        <EmptyState
          icon={<ListTodo className="size-5" />}
          title="No active sprint"
          description="Start the next sprint from the planning page."
          action={
            <Link to="/backlog">
              <Button variant="primary">Go to planning</Button>
            </Link>
          }
        />
      </Card>
    )

  const all = tasks?.filter((t) => t.sprintId === active.id) ?? []
  const points = all.reduce((s, t) => s + (t.points ?? 0), 0)
  const donePoints = all.filter((t) => t.status === 'done').reduce((s, t) => s + (t.points ?? 0), 0)
  const doneCount = all.filter((t) => t.status === 'done').length
  const daysLeft = differenceInCalendarDays(parseISO(active.endsOn), new Date())

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <PageHeader
        title={`Sprint ${active.number}`}
        subtitle={
          <>
            {sprintRange(active)} · {daysLeft > 0 ? `${daysLeft} ${daysLeft === 1 ? 'day' : 'days'} left` : daysLeft === 0 ? 'Last day' : 'Ended — complete it in Planning'}
            {active.goal && <span className="hidden sm:inline"> · 🎯 {active.goal}</span>}
          </>
        }
        actions={
          <>
            <Link to="/backlog" className="hidden sm:block">
              <Button>Planning</Button>
            </Link>
            <Button variant="primary" className="max-lg:hidden" icon={<Plus className="size-4" />} onClick={() => openQuickAdd({ sprintId: active.id })}>
              New task
            </Button>
          </>
        }
      />

      <div className="mb-3 flex flex-wrap items-center gap-2">
        <Segmented<Who>
          value={who}
          onChange={setWho}
          options={[
            { value: 'all', label: 'Everyone' },
            { value: 'mine', label: 'Mine' },
            ...(partner ? [{ value: 'partner' as Who, label: partner.displayName }] : []),
            { value: 'unassigned', label: 'Unassigned' },
          ]}
        />
        <Menu
          trigger={
            <Button size="sm" icon={<Tag className="size-3.5" />}>
              {labelFilter ? labelById.get(labelFilter)?.name ?? 'Label' : 'All labels'}
            </Button>
          }
        >
          <MenuItem checked={!labelFilter} onSelect={() => setLabel(null)}>
            All labels
          </MenuItem>
          {labels.map((l) => (
            <MenuItem key={l.id} checked={labelFilter === l.id} icon={<span className="size-2 rounded-full" style={{ background: l.color }} />} onSelect={() => setLabel(l.id)}>
              {l.name}
            </MenuItem>
          ))}
        </Menu>
        <div className="ml-auto flex min-w-48 flex-1 items-center gap-2 sm:max-w-72 sm:flex-none">
          <Progress value={points ? donePoints / points : all.length ? doneCount / all.length : 0} tone="success" className="flex-1" />
          <span className="text-[12px] whitespace-nowrap text-muted tabular">
            {points ? `${donePoints}/${points} pts` : `${doneCount}/${all.length} done`}
          </span>
        </div>
      </div>

      {isDesktop ? (
        <DesktopBoard tasks={sprintTasks} />
      ) : (
        <>
          <Segmented<TaskStatus>
            className="mb-3 w-full"
            value={mobileCol}
            onChange={setMobileCol}
            options={COLUMNS.map((c) => ({ value: c.status, label: `${c.label} · ${sprintTasks.filter((t) => t.status === c.status).length}` }))}
          />
          <div className="space-y-2 pb-4">
            {sprintTasks
              .filter((t) => t.status === mobileCol)
              .map((t) => (
                <TaskCard key={t.id} task={t} />
              ))}
            {sprintTasks.filter((t) => t.status === mobileCol).length === 0 && <p className="py-10 text-center text-[13px] text-subtle">Nothing here</p>}
          </div>
        </>
      )}
    </div>
  )
}

function DesktopBoard({ tasks }: { tasks: Task[] }) {
  const update = useUpdateTask()
  const byId = useMemo(() => new Map(tasks.map((t) => [t.id, t])), [tasks])
  const derived = useMemo(() => {
    const cols = {} as Record<TaskStatus, string[]>
    for (const c of COLUMNS) cols[c.status] = tasks.filter((t) => t.status === c.status).map((t) => t.id)
    return cols
  }, [tasks])
  // While dragging we track positions locally; otherwise columns come straight from data.
  const [dragCols, setDragCols] = useState<Record<TaskStatus, string[]> | null>(null)
  const [activeId, setActiveId] = useState<string | null>(null)
  const cols = dragCols ?? derived

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  const columnOf = (id: string, c = cols) => (id in c ? (id as TaskStatus) : (Object.keys(c) as TaskStatus[]).find((k) => c[k].includes(id)))

  const onDragStart = (e: DragStartEvent) => {
    setActiveId(String(e.active.id))
    setDragCols(derived)
  }

  const onDragOver = ({ active, over }: DragOverEvent) => {
    if (!over || !dragCols) return
    const from = columnOf(String(active.id), dragCols)
    const to = columnOf(String(over.id), dragCols)
    if (!from || !to || from === to) return
    setDragCols((c) => {
      if (!c) return c
      const target = c[to]
      const overIndex = target.indexOf(String(over.id))
      const index = overIndex >= 0 ? overIndex : target.length
      return {
        ...c,
        [from]: c[from].filter((x) => x !== active.id),
        [to]: [...target.slice(0, index), String(active.id), ...target.slice(index)],
      }
    })
  }

  const onDragEnd = ({ active, over }: DragEndEvent) => {
    const id = String(active.id)
    const c = dragCols
    setActiveId(null)
    setDragCols(null)
    if (!over || !c) return
    const col = columnOf(id, c)
    if (!col) return
    let list = c[col]
    const overIdx = list.indexOf(String(over.id))
    const curIdx = list.indexOf(id)
    if (overIdx >= 0 && overIdx !== curIdx) {
      list = [...list]
      list.splice(curIdx, 1)
      list.splice(overIdx, 0, id)
    }
    const i = list.indexOf(id)
    const before = list[i - 1] ? byId.get(list[i - 1]!)?.rank : undefined
    const after = list[i + 1] ? byId.get(list[i + 1]!)?.rank : undefined
    const task = byId.get(id)!
    const rank = rankBetween(before, after)
    if (task.status !== col || task.rank !== rank) {
      const unchangedOrder = task.status === col && derived[col].indexOf(id) === i
      if (!unchangedOrder) update(id, { status: col, rank })
    }
  }

  const activeTask = activeId ? byId.get(activeId) : undefined

  return (
    <DndContext sensors={sensors} collisionDetection={closestCorners} onDragStart={onDragStart} onDragOver={onDragOver} onDragEnd={onDragEnd} onDragCancel={() => (setActiveId(null), setDragCols(null))}>
      <div className="grid min-h-0 flex-1 grid-cols-3 gap-3">
        {COLUMNS.map((c) => (
          <Column key={c.status} status={c.status} label={c.label} ids={cols[c.status]} byId={byId} />
        ))}
      </div>
      <DragOverlay dropAnimation={{ duration: 150, easing: 'ease-out' }}>{activeTask && <TaskCard task={activeTask} dragging />}</DragOverlay>
    </DndContext>
  )
}

function Column({ status, label, ids, byId }: { status: TaskStatus; label: string; ids: string[]; byId: Map<string, Task> }) {
  const { setNodeRef, isOver } = useDroppable({ id: status })
  const pts = ids.reduce((s, id) => s + (byId.get(id)?.points ?? 0), 0)
  return (
    <section className={`flex min-h-0 flex-col rounded-lg border border-border bg-surface-2/60 ${isOver ? 'ring-2 ring-accent/30' : ''}`}>
      <header className="flex h-10 shrink-0 items-center gap-2 px-3">
        <StatusIcon status={status} />
        <span className="text-[12px] font-semibold tracking-wide uppercase">{label}</span>
        <span className="text-[12px] text-subtle tabular">{ids.length}</span>
        {pts > 0 && <span className="ml-auto text-[11px] text-subtle tabular">{pts} pts</span>}
        {status === 'todo' && (
          <button onClick={() => openQuickAdd({ status: 'todo' })} className={`${pts > 0 ? '' : 'ml-auto'} rounded p-0.5 text-subtle hover:bg-surface-3 hover:text-text`} aria-label="Add task">
            <Plus className="size-4" />
          </button>
        )}
      </header>
      <SortableContext items={ids} strategy={verticalListSortingStrategy}>
        <div ref={setNodeRef} className="min-h-24 flex-1 space-y-2 overflow-y-auto px-2 pb-2 scrollbar-thin">
          {ids.map((id) => {
            const t = byId.get(id)
            return t ? <SortableCard key={id} task={t} /> : null
          })}
          {ids.length === 0 && <div className="flex h-20 items-center justify-center rounded-md border border-dashed border-border text-[12px] text-subtle">Drop here</div>}
        </div>
      </SortableContext>
    </section>
  )
}

function SortableCard({ task }: { task: Task }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: task.id })
  return (
    <TaskCard
      ref={setNodeRef}
      task={task}
      style={{ transform: CSS.Translate.toString(transform), transition, opacity: isDragging ? 0.35 : undefined }}
      {...attributes}
      {...listeners}
    />
  )
}
