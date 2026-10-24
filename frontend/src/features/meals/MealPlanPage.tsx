import clsx from 'clsx'
import { addDays, format, isSameDay, parseISO } from 'date-fns'
import { CheckCircle2, ChevronLeft, ChevronRight, Circle, ExternalLink, ListTodo, Minus, Plus, RotateCcw, ShoppingCart, Trash2, Users, Wand2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Modal } from '@/components/Dialog'
import { Menu, MenuItem, MenuLabel, MenuSeparator } from '@/components/Menu'
import { Avatar, Button, Card, PageHeader, Switch } from '@/components/ui'
import { post } from '@/lib/api'
import { useIsDesktop } from '@/lib/hooks'
import { SLOTS, isPast, slotLabel, useAction, useMealActions, useMeals, useRecipes, weekRange } from '@/lib/meals'
import type { TaskPatch } from '@/lib/tasks'
import { openQuickAdd } from '@/lib/ui'
import { weekStart } from '@/lib/weeks'
import { hasRole, useSession } from '@/lib/session'
import { fmtAmount, useNutrition } from '@/lib/nutrition'
import type { MealItem, PersonReport, Recipe, Slot } from '@/lib/types'
import { AddDishDialog } from './AddDishDialog'
import { DietMark, MealsNav } from './MealBits'

export function MealPlanPage() {
  const { household } = useSession()
  const [params, setParams] = useSearchParams()
  const isDesktop = useIsDesktop()
  const start = useMemo(() => {
    const w = params.get('week')
    return w ? parseISO(w) : weekStart(new Date(), household.sprintStartDow)
  }, [params, household.sprintStartDow])
  const days = useMemo(() => Array.from({ length: 7 }, (_, i) => addDays(start, i)), [start])
  const { from, to } = weekRange(start)
  const { data: meals = [] } = useMeals(from, to)
  const { data: recipes = [] } = useRecipes()
  const { user } = useSession()
  const { data: report = [] } = useNutrition(from, to, hasRole(user, 'member'))
  const tracked = report.filter((p) => p.targets)
  const recipeById = useMemo(() => new Map(recipes.map((r) => [r.id, r])), [recipes])
  const [target, setTarget] = useState<{ date: string; slot: Slot } | null>(null)
  const [mobileDay, setMobileDay] = useState(() => {
    const i = days.findIndex((d) => isSameDay(d, new Date()))
    return i < 0 ? 0 : i
  })
  const [autofill, setAutofill] = useState(false)
  const navigate = useNavigate()

  const goWeek = (delta: number) =>
    setParams((p) => {
      p.set('week', format(addDays(start, delta * 7), 'yyyy-MM-dd'))
      return p
    })
  const thisWeek = () =>
    setParams((p) => {
      p.delete('week')
      return p
    })

  const cell = (d: Date, slot: Slot) => meals.filter((m) => m.date === format(d, 'yyyy-MM-dd') && m.slot === slot)
  const planned = meals.length
  const groceries = useAction(() => post<{ added: number; covered: number }>('/grocery/generate', { from: format(new Date() > start ? new Date() : start, 'yyyy-MM-dd'), to }), [['grocery']], (r) => `${r.added} items added to the grocery list${r.covered ? ` · ${r.covered} already in the pantry` : ''}`)

  return (
    <div>
      <PageHeader
        title="Kitchen"
        subtitle={`${format(start, 'd MMM')} – ${format(addDays(start, 6), 'd MMM yyyy')} · ${planned} ${planned === 1 ? 'dish' : 'dishes'} planned`}
        actions={
          <>
            <div className="flex items-center rounded-md border border-border bg-surface shadow-card">
              <button className="flex size-8 items-center justify-center text-muted hover:text-text" onClick={() => goWeek(-1)} aria-label="Previous week">
                <ChevronLeft className="size-4" />
              </button>
              <button className="h-8 border-x border-border px-3 text-[12px] font-medium" onClick={thisWeek}>
                This week
              </button>
              <button className="flex size-8 items-center justify-center text-muted hover:text-text" onClick={() => goWeek(1)} aria-label="Next week">
                <ChevronRight className="size-4" />
              </button>
            </div>
            <Button icon={<Wand2 className="size-4" />} onClick={() => setAutofill(true)}>
              Auto-fill
            </Button>
            <Button
              variant="primary"
              icon={<ShoppingCart className="size-4" />}
              loading={groceries.isPending}
              onClick={() => groceries.mutate(undefined, { onSuccess: () => navigate('/grocery') })}
            >
              Grocery list
            </Button>
          </>
        }
      />
      <MealsNav />

      {isDesktop ? (
        <Card className="overflow-hidden">
          <div className="grid grid-cols-[104px_repeat(7,minmax(0,1fr))]">
            <div className="border-b border-border bg-surface-2/60" />
            {days.map((d) => {
              const today = isSameDay(d, new Date())
              return (
                <div key={d.toISOString()} className={clsx('border-b border-l border-border px-2.5 py-2', today ? 'bg-accent-soft' : 'bg-surface-2/60')}>
                  <div className={clsx('text-[11px] font-medium uppercase', today ? 'text-accent' : 'text-subtle')}>{format(d, 'EEE')}</div>
                  <div className={clsx('text-[15px] font-semibold tabular', today && 'text-accent')}>{format(d, 'd')}</div>
                </div>
              )
            })}
            {SLOTS.map((s) => (
              <SlotRow key={s.value} slot={s.value} label={s.label} time={household.mealTimes?.[s.value]} days={days} cell={cell} recipeById={recipeById} onAdd={setTarget} />
            ))}
            {tracked.length > 0 && (
              <>
                <button onClick={() => navigate('/nutrition')} className="border-t border-border bg-surface-2/40 px-3 py-2.5 text-left hover:bg-surface-2">
                  <div className="text-[12px] font-semibold">Nutrition</div>
                  <div className="text-[11px] text-subtle">kcal · protein</div>
                </button>
                {days.map((d) => (
                  <div key={d.toISOString()} className="space-y-1 border-t border-l border-border p-1.5">
                    <DayNutrition people={tracked} date={format(d, 'yyyy-MM-dd')} />
                  </div>
                ))}
              </>
            )}
          </div>
        </Card>
      ) : (
        <>
          <div className="-mx-4 mb-3 flex gap-1.5 overflow-x-auto px-4 pb-1 scrollbar-thin">
            {days.map((d, i) => {
              const count = meals.filter((m) => m.date === format(d, 'yyyy-MM-dd')).length
              const today = isSameDay(d, new Date())
              return (
                <button
                  key={d.toISOString()}
                  onClick={() => setMobileDay(i)}
                  className={clsx(
                    'flex w-12 shrink-0 flex-col items-center rounded-lg border py-1.5',
                    mobileDay === i ? 'border-accent bg-accent text-accent-fg' : today ? 'border-accent/40 bg-accent-soft' : 'border-border bg-surface',
                  )}
                >
                  <span className="text-[10px] font-medium uppercase opacity-80">{format(d, 'EEE')}</span>
                  <span className="text-[16px] font-semibold tabular">{format(d, 'd')}</span>
                  <span className={clsx('mt-0.5 size-1 rounded-full', count ? (mobileDay === i ? 'bg-accent-fg' : 'bg-accent') : 'bg-transparent')} />
                </button>
              )
            })}
          </div>
          <div className="space-y-3">
            {tracked.length > 0 && (
              <Card className="px-3 py-2" onClick={() => navigate('/nutrition')}>
                <DayNutrition people={tracked} date={format(days[mobileDay]!, 'yyyy-MM-dd')} row />
              </Card>
            )}
            {SLOTS.map((s) => {
              const d = days[mobileDay]!
              const items = cell(d, s.value)
              return (
                <Card key={s.value}>
                  <div className="flex items-center justify-between border-b border-border px-3 py-2">
                    <span className="text-[13px] font-semibold">
                      {s.label} <span className="font-normal text-subtle">{household.mealTimes?.[s.value]}</span>
                    </span>
                    <Button size="sm" variant="ghost" icon={<Plus className="size-3.5" />} onClick={() => setTarget({ date: format(d, 'yyyy-MM-dd'), slot: s.value })}>
                      Add
                    </Button>
                  </div>
                  <div className="space-y-1.5 p-2">
                    {items.length === 0 && <p className="px-1 py-1.5 text-[13px] text-subtle">Nothing planned</p>}
                    {items.map((m) => (
                      <DishChip key={m.id} meal={m} recipe={m.recipeId ? recipeById.get(m.recipeId) : undefined} large />
                    ))}
                  </div>
                </Card>
              )
            })}
          </div>
        </>
      )}

      <AddDishDialog target={target} planned={meals} onClose={() => setTarget(null)} />
      <AutofillDialog open={autofill} onOpenChange={setAutofill} from={format(new Date() > start ? new Date() : start, 'yyyy-MM-dd')} to={to} />
    </div>
  )
}

function SlotRow({ slot, label, time, days, cell, recipeById, onAdd }: { slot: Slot; label: string; time?: string; days: Date[]; cell: (d: Date, s: Slot) => MealItem[]; recipeById: Map<string, Recipe>; onAdd: (t: { date: string; slot: Slot }) => void }) {
  return (
    <>
      <div className="border-b border-border bg-surface-2/40 px-3 py-2.5 last:border-b-0">
        <div className="text-[12px] font-semibold">{label}</div>
        {time && <div className="text-[11px] text-subtle tabular">{time}</div>}
      </div>
      {days.map((d) => {
        const items = cell(d, slot)
        const date = format(d, 'yyyy-MM-dd')
        return (
          <div key={date} className={clsx('group min-h-24 space-y-1 border-b border-l border-border p-1.5', isSameDay(d, new Date()) && 'bg-accent-soft/40')}>
            {items.map((m) => (
              <DishChip key={m.id} meal={m} recipe={m.recipeId ? recipeById.get(m.recipeId) : undefined} />
            ))}
            <button
              onClick={() => onAdd({ date, slot })}
              className={clsx(
                'flex h-7 w-full items-center justify-center gap-1 rounded text-[12px] text-subtle hover:bg-surface-2 hover:text-text',
                items.length > 0 && 'opacity-0 group-hover:opacity-100 focus:opacity-100',
              )}
              aria-label={`Add to ${label}`}
            >
              <Plus className="size-3.5" />
              {items.length === 0 && 'Add'}
            </button>
          </div>
        )
      })}
    </>
  )
}

function DishChip({ meal, recipe, large }: { meal: MealItem; recipe?: Recipe; large?: boolean }) {
  const { update, remove } = useMealActions()
  const { members, memberById, user, household } = useSession()
  const navigate = useNavigate()
  const [portions, setPortions] = useState(false)
  const cooked = !!meal.cookedAt
  const eaters = Object.entries(meal.eaters ?? {}).filter(([, v]) => v > 0)
  const everyone = members.filter((m) => m.role !== 'viewer')
  // Show who's eating only when it isn't simply "everyone, one serving each".
  const special = eaters.length !== everyone.length || eaters.some(([, v]) => v !== 1)
  return (
    <>
      <Menu
        width="w-56"
        trigger={
          <button
            className={clsx(
              'flex w-full items-center gap-1.5 rounded-md border px-2 text-left transition-colors',
              large ? 'min-h-10 py-1.5 text-[14px]' : 'min-h-7 py-1 text-[12px] leading-tight',
              cooked ? 'border-success/30 bg-success-soft text-muted' : 'border-border bg-surface hover:border-border-strong',
            )}
          >
            {recipe ? <DietMark diet={recipe.diet} /> : <span className="size-3.5 shrink-0 rounded-[3px] border-[1.5px] border-dashed border-border-strong" />}
            <span className={clsx('min-w-0 flex-1', large ? 'truncate' : 'line-clamp-2', meal.leftover && 'italic')}>
              {meal.leftover && <RotateCcw className="mr-0.5 inline size-3 align-[-1px] text-subtle" aria-label="Leftover" />}
              {meal.title}
            </span>
            {special && eaters.length > 0 && (
              <span className="flex shrink-0 -space-x-1">
                {eaters.map(([id, v]) => {
                  const m = memberById.get(id)
                  return m ? <PortionDot key={id} name={m.displayName} color={m.color} portion={v} /> : null
                })}
              </span>
            )}
            {recipe && meal.servings !== recipe.servings && !special && <span className="shrink-0 text-[10px] text-subtle tabular">×{meal.servings}</span>}
            {cooked && <CheckCircle2 className="size-3.5 shrink-0 text-success" />}
          </button>
        }
      >
        <MenuLabel>
          {meal.title} · {meal.servings} servings
        </MenuLabel>
        {recipe && (
          <MenuItem icon={<ExternalLink className="size-4" />} onSelect={() => navigate(`/recipes/${recipe.id}?servings=${meal.servings}`)}>
            Open recipe
          </MenuItem>
        )}
        <MenuItem icon={cooked ? <Circle className="size-4" /> : <CheckCircle2 className="size-4" />} onSelect={() => update(meal.id, { cooked: !cooked })}>
          {cooked ? 'Mark not cooked' : recipe ? 'Mark cooked (uses pantry)' : 'Mark cooked'}
        </MenuItem>
        {hasRole(user, 'member') && (
          <MenuItem icon={<Users className="size-4" />} onSelect={() => setPortions(true)}>
            Who's eating…
          </MenuItem>
        )}
        {hasRole(user, 'member') && !cooked && !meal.leftover && (
          <MenuItem icon={<ListTodo className="size-4" />} onSelect={() => openQuickAdd(cookingTask(meal, recipe, household.mealTimes?.[meal.slot]))}>
            Add cooking task
          </MenuItem>
        )}
        <MenuItem keepOpen icon={<Plus className="size-4" />} onSelect={() => update(meal.id, { servings: Math.min(50, meal.servings + 1) })}>
          More servings
        </MenuItem>
        <MenuItem keepOpen icon={<Minus className="size-4" />} onSelect={() => update(meal.id, { servings: Math.max(1, meal.servings - 1) })}>
          Fewer servings
        </MenuItem>
        {!isPast(meal.date) && (
          <MenuItem icon={<ChevronRight className="size-4" />} onSelect={() => update(meal.id, { date: format(addDays(parseISO(meal.date), 1), 'yyyy-MM-dd') })}>
            Move to next day
          </MenuItem>
        )}
        <MenuSeparator />
        <MenuItem danger icon={<Trash2 className="size-4" />} onSelect={() => remove(meal.id)}>
          Remove
        </MenuItem>
      </Menu>
      <PortionsDialog open={portions} onOpenChange={setPortions} meal={meal} recipe={recipe} people={everyone} />
    </>
  )
}

/** A task to cook a planned dish, due that day; opened in quick add to pick who cooks it. */
function cookingTask(meal: MealItem, recipe: Recipe | undefined, time?: string): TaskPatch {
  const when = `${slotLabel(meal.slot)} on ${format(parseISO(meal.date), 'EEE d MMM')}${time ? ` at ${time}` : ''}`
  const lines = [`${when} · ${meal.servings} ${meal.servings === 1 ? 'serving' : 'servings'}`]
  if (recipe) lines.push(`Recipe: ${window.location.origin}/recipes/${recipe.id}?servings=${meal.servings}`)
  return { title: `Cook ${meal.title}`.slice(0, 200), description: lines.join('\n\n'), dueDate: meal.date }
}

const portionLabel = (v: number) => {
  const whole = Math.floor(v)
  const frac = { 0: '', 0.25: '¼', 0.5: '½', 0.75: '¾' }[v - whole] ?? ''
  return `${whole || (frac ? '' : '0')}${frac}`
}

function PortionDot({ name, color, portion }: { name: string; color: string; portion: number }) {
  return (
    <span className="relative" title={`${name}: ${portionLabel(portion)} serving${portion === 1 ? '' : 's'}`}>
      <Avatar name={name} color={color} size={16} className="ring-1 ring-surface" />
      {portion !== 1 && <span className="absolute -right-1 -bottom-1 rounded-sm bg-surface px-px text-[8px] leading-none font-semibold text-text tabular">{portionLabel(portion)}</span>}
    </span>
  )
}

function PortionsDialog({ open, onOpenChange, meal, recipe, people }: { open: boolean; onOpenChange: (o: boolean) => void; meal: MealItem; recipe?: Recipe; people: { id: string; displayName: string; color: string }[] }) {
  const { update } = useMealActions()
  const [p, setP] = useState<Record<string, number>>({})
  const [leftover, setLeftover] = useState(meal.leftover)
  useEffect(() => {
    if (!open) return
    setP(Object.fromEntries(people.map((m) => [m.id, meal.eaters?.[m.id] ?? 0])))
    setLeftover(meal.leftover)
  }, [open]) // eslint-disable-line react-hooks/exhaustive-deps -- start from the dish as it is when opened
  const kcal = recipe?.nutrition && recipe.nutrition.coverage >= 0.5 ? recipe.nutrition.perServing.kcal : null
  const total = Object.values(p).reduce((a, b) => a + b, 0)
  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title={`Who's eating ${meal.title}`}
      description="Servings each person eats. The nutrition page and calorie targets use these."
      footer={
        <>
          <Button
            variant="ghost"
            onClick={() => {
              update(meal.id, { portions: null, leftover })
              onOpenChange(false)
            }}
          >
            Reset to default
          </Button>
          <Button
            variant="primary"
            onClick={() => {
              update(meal.id, { portions: p, leftover, ...(total > meal.servings && !leftover ? { servings: Math.min(50, Math.ceil(total)) } : {}) })
              onOpenChange(false)
            }}
          >
            Save
          </Button>
        </>
      }
    >
      <div className="space-y-2">
        {people.map((m) => {
          const v = p[m.id] ?? 0
          return (
            <div key={m.id} className="flex items-center gap-3">
              <Avatar name={m.displayName} color={m.color} size={26} />
              <span className="min-w-0 flex-1 truncate text-[14px]">{m.displayName}</span>
              {kcal != null && v > 0 && <span className="text-[12px] text-subtle tabular">{fmtAmount(kcal * v)} kcal</span>}
              <span className="flex items-center rounded-md border border-border">
                <button className="flex size-8 items-center justify-center text-muted hover:text-text" onClick={() => setP({ ...p, [m.id]: Math.max(0, v - 0.25) })} aria-label={`Less for ${m.displayName}`}>
                  <Minus className="size-3.5" />
                </button>
                <span className="w-10 text-center text-[13px] font-medium tabular">{portionLabel(v)}</span>
                <button className="flex size-8 items-center justify-center text-muted hover:text-text" onClick={() => setP({ ...p, [m.id]: Math.min(10, v + 0.25) })} aria-label={`More for ${m.displayName}`}>
                  <Plus className="size-3.5" />
                </button>
              </span>
            </div>
          )
        })}
      </div>
      <label className="mt-4 flex items-center justify-between gap-3 border-t border-border pt-3 text-[13px]">
        <span>
          Leftover
          <span className="block text-[12px] text-subtle">Eaten again, not cooked again: kept off the grocery list and the pantry.</span>
        </span>
        <Switch checked={leftover} onChange={setLeftover} label="Leftover" />
      </label>
    </Modal>
  )
}

/** Each tracked person's calories and protein for one day against their targets. */
function DayNutrition({ people, date, row }: { people: PersonReport[]; date: string; row?: boolean }) {
  return (
    <div className={clsx(row ? 'flex flex-wrap gap-x-4 gap-y-1' : 'space-y-1')}>
      {people.map((p) => {
        const d = p.days.find((x) => x.date === date)
        if (!d || !p.targets) return null
        const empty = d.dishes === 0 && d.unknown.length === 0
        const r = d.totals.kcal / p.targets.kcal
        const tone = empty ? 'text-subtle' : r > 1.1 ? 'text-danger' : r < 0.8 && d.unknown.length === 0 ? 'text-warning' : 'text-success'
        return (
          <div key={p.userId} className="flex items-center gap-1.5 text-[11px] tabular" title={d.unknown.length ? `Not counted: ${d.unknown.join(', ')}` : undefined}>
            <Avatar name={p.name} color={p.color} size={16} />
            {empty ? (
              <span className="text-subtle">—</span>
            ) : (
              <span>
                <span className={clsx('font-medium', tone)}>{fmtAmount(d.totals.kcal)}</span>
                <span className="text-subtle">/{fmtAmount(p.targets.kcal)}</span>
                {d.unknown.length > 0 && <span className="text-subtle">+?</span>}
                <span className="text-muted"> · {Math.round(d.totals.protein)}g</span>
              </span>
            )}
          </div>
        )
      })}
    </div>
  )
}

function AutofillDialog({ open, onOpenChange, from, to }: { open: boolean; onOpenChange: (o: boolean) => void; from: string; to: string }) {
  const [slots, setSlots] = useState<Slot[]>(['breakfast', 'lunch', 'dinner'])
  const run = useAction((v: { from: string; to: string; slots: Slot[] }) => post<{ created: number }>('/meals/autofill', v), [['meals'], ['suggest']], (r) =>
    r.created ? `Planned ${r.created} dishes — tweak anything you like` : 'Nothing to fill: every chosen slot already has a dish (or no recipes fit)',
  )
  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title="Auto-fill the week"
      description="Fills empty slots from today to the end of the week with the top suggestion — favourites, variety, what you usually eat on that day, and what's in the pantry."
      footer={
        <>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button variant="primary" loading={run.isPending} disabled={!slots.length} onClick={() => run.mutate({ from, to, slots }, { onSuccess: () => onOpenChange(false) })}>
            Fill empty slots
          </Button>
        </>
      }
    >
      <div className="grid grid-cols-2 gap-2">
        {SLOTS.map((s) => {
          const on = slots.includes(s.value)
          return (
            <button
              key={s.value}
              onClick={() => setSlots(on ? slots.filter((x) => x !== s.value) : [...slots, s.value])}
              className={clsx('flex h-10 items-center gap-2 rounded-md border px-3 text-[13px]', on ? 'border-accent bg-accent-soft text-accent' : 'border-border text-muted')}
            >
              {on ? <CheckCircle2 className="size-4" /> : <Circle className="size-4" />} {s.label}
            </button>
          )
        })}
      </div>
      <p className="mt-3 text-[12px] text-subtle">Suggestions get smarter as history builds up: after a couple of months they learn your weekday habits and which dishes you pair together.</p>
    </Modal>
  )
}
