import clsx from 'clsx'
import { format, parseISO } from 'date-fns'
import { ListTodo, Plus, User } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Avatar, Badge, Button, Card, CardHeader } from '@/components/ui'
import { taskKey } from '@/lib/format'
import { SLOTS, isPast, slotLabel } from '@/lib/meals'
import { hasRole, useSession } from '@/lib/session'
import { useSprints } from '@/lib/sprints'
import { type TaskPatch, useUpdateTask } from '@/lib/tasks'
import type { MealItem, Recipe, Sprint, Task } from '@/lib/types'
import { openQuickAdd, useTaskParam } from '@/lib/ui'
import { StatusIcon } from '../tasks/TaskBits'
import { DietMark } from './MealBits'

/** A task to cook a planned dish, due that day; opened in quick add to pick who cooks it. */
export function cookingTask(meal: MealItem, recipe: Recipe | undefined, time?: string): TaskPatch {
  const when = `${slotLabel(meal.slot)} on ${format(parseISO(meal.date), 'EEE d MMM')}${time ? ` at ${time}` : ''}`
  const lines = [`${when} · ${meal.servings} ${meal.servings === 1 ? 'serving' : 'servings'}`]
  if (recipe) lines.push(`Recipe: ${window.location.origin}/recipes/${recipe.id}?servings=${meal.servings}`)
  return { title: `Cook ${meal.title}`.slice(0, 200), description: lines.join('\n\n'), dueDate: meal.date, mealId: meal.id }
}

/** Each planned dish's cooking task, newest first when there are several. */
export function useTaskByMeal(tasks: Task[]) {
  return useMemo(() => {
    const m = new Map<string, Task>()
    for (const t of tasks) {
      const prev = t.mealId ? m.get(t.mealId) : undefined
      if (t.mealId && (!prev || t.createdAt > prev.createdAt)) m.set(t.mealId, t)
    }
    return m
  }, [tasks])
}

export type CookingState = 'none' | 'backlog' | 'sprint' | 'done'

export function cookingState(task: Task | undefined): CookingState {
  if (!task) return 'none'
  if (task.status === 'done') return 'done'
  return task.sprintId ? 'sprint' : 'backlog'
}

/** The small mark on a dish chip: grey = backlog, accent = in a sprint, green = done. */
export function TaskMark({ task }: { task?: Task }) {
  const state = cookingState(task)
  if (state === 'none') return null
  return (
    <ListTodo
      className={clsx('size-3.5 shrink-0', { backlog: 'text-warning', sprint: 'text-accent', done: 'text-success' }[state])}
      aria-label={{ backlog: 'Cooking task in the backlog', sprint: 'Cooking task in the sprint', done: 'Cooking task done' }[state]}
    />
  )
}

/** The sprint a dish falls in: the open sprint whose dates cover its day, else the current one. */
export function sprintCovering(sprints: Sprint[], date: string): Sprint | undefined {
  return sprints.find((s) => s.status !== 'completed' && s.startsOn <= date && date <= s.endsOn) ?? sprints.find((s) => s.status === 'active')
}

/** Quick add, filled in for cooking this dish and put in the sprint that covers its day. */
export function addCookingTask(meal: MealItem, recipe: Recipe | undefined, sprints: Sprint[], time?: string) {
  const sprint = sprintCovering(sprints, meal.date)
  openQuickAdd({ ...cookingTask(meal, recipe, time), ...(sprint ? { sprintId: sprint.id } : {}) })
}

const stateTone = { none: 'neutral', backlog: 'warning', sprint: 'accent', done: 'success' } as const

const slotOrder = (m: MealItem) => SLOTS.findIndex((s) => s.value === m.slot)

/**
 * The dishes to cook, each with its cooking task: in a sprint, still in the backlog, or
 * none yet. Leftovers aren't cooked, so they're left out.
 */
export function CookingTasks({ meals, tasks, recipeById, days }: { meals: MealItem[]; tasks: Task[]; recipeById: Map<string, Recipe>; days: string[] }) {
  const { user, household, memberById } = useSession()
  const { sprints } = useSprints()
  const update = useUpdateTask()
  const { open } = useTaskParam()
  const taskByMeal = useTaskByMeal(tasks)
  const [all, setAll] = useState(false)
  const canEdit = hasRole(user, 'member')

  const dishes = meals
    .filter((m) => !m.leftover && days.includes(m.date))
    .sort((a, b) => a.date.localeCompare(b.date) || slotOrder(a) - slotOrder(b) || a.position - b.position)
  const count = { none: 0, backlog: 0, sprint: 0, done: 0 }
  for (const m of dishes) count[cookingState(taskByMeal.get(m.id))]++
  // Done, cooked or past dishes need nothing more, so they hide unless asked for.
  const settled = (m: MealItem) => !!m.cookedAt || isPast(m.date) || cookingState(taskByMeal.get(m.id)) === 'done'
  const shown = all ? dishes : dishes.filter((m) => !settled(m))
  const hidden = dishes.length - shown.length
  const byDay = days.map((d) => ({ date: d, items: shown.filter((m) => m.date === d) })).filter((g) => g.items.length)

  const sprintFor = (date: string) => sprintCovering(sprints, date)
  const sprintNumber = new Map(sprints.map((s) => [s.id, s.number]))

  if (dishes.length === 0) return null
  return (
    <Card className="mt-4">
      <CardHeader
        title="Cooking tasks"
        subtitle={
          <>
            {count.sprint} in a sprint · <span className={clsx(count.backlog && 'text-warning')}>{count.backlog} in the backlog</span> ·{' '}
            <span className={clsx(count.none && 'font-medium text-text')}>{count.none} without a task</span>
            {count.done > 0 && ` · ${count.done} done`}
          </>
        }
        actions={
          (hidden > 0 || all) && (
            <Button size="sm" variant="ghost" onClick={() => setAll(!all)}>
              {all ? 'Hide done & past' : `Show all (${hidden} more)`}
            </Button>
          )
        }
      />
      {byDay.length === 0 ? (
        <p className="px-4 py-3 text-[13px] text-subtle">Every dish left to cook is done or already cooked.</p>
      ) : (
        <div className="divide-y divide-border">
          {byDay.map((g) => (
            <div key={g.date} className="px-4 py-2">
              <div className="mb-1 text-[11px] font-medium text-subtle uppercase">{format(parseISO(g.date), 'EEE d MMM')}</div>
              <ul className="space-y-0.5">
                {g.items.map((m) => {
                  const recipe = m.recipeId ? recipeById.get(m.recipeId) : undefined
                  const task = taskByMeal.get(m.id)
                  const state = cookingState(task)
                  const assignee = task?.assigneeId ? memberById.get(task.assigneeId) : undefined
                  const target = sprintFor(m.date)
                  return (
                    <li key={m.id} className="flex min-h-9 flex-wrap items-center gap-x-2 gap-y-0.5 py-0.5 text-[13px]">
                      {recipe ? <DietMark diet={recipe.diet} /> : <span className="size-3.5 shrink-0" />}
                      <span className={clsx('min-w-0 flex-1 basis-36 truncate', m.cookedAt && 'text-muted line-through')}>
                        {m.title} <span className="text-[12px] text-subtle">· {slotLabel(m.slot)}</span>
                      </span>
                      {task ? (
                        <button onClick={() => open(task)} className="flex shrink-0 items-center gap-1.5 rounded px-1 py-0.5 hover:bg-surface-2" aria-label={`Open ${taskKey(task.number)}`}>
                          <StatusIcon status={task.status} />
                          <Badge tone={stateTone[state]}>
                            {taskKey(task.number)} · {state === 'backlog' ? 'Backlog' : state === 'done' ? 'Done' : `Sprint ${sprintNumber.get(task.sprintId!) ?? ''}`}
                          </Badge>
                          {assignee ? (
                            <Avatar name={assignee.displayName} color={assignee.color} size={18} />
                          ) : (
                            <span title="Unassigned" className="flex size-[18px] items-center justify-center rounded-full border border-dashed border-border-strong text-subtle">
                              <User className="size-3" />
                            </span>
                          )}
                        </button>
                      ) : (
                        <Badge>No task</Badge>
                      )}
                      {canEdit && state === 'none' && !m.cookedAt && (
                        <Button size="sm" variant="ghost" icon={<Plus className="size-3.5" />} onClick={() => addCookingTask(m, recipe, sprints, household.mealTimes?.[m.slot])}>
                          Add
                        </Button>
                      )}
                      {canEdit && state === 'backlog' && target && (
                        <Button size="sm" variant="ghost" onClick={() => update(task!.id, { sprintId: target.id, status: task!.status === 'backlog' ? 'todo' : task!.status })}>
                          Add to sprint {target.number}
                        </Button>
                      )}
                    </li>
                  )
                })}
              </ul>
            </div>
          ))}
        </div>
      )}
    </Card>
  )
}
