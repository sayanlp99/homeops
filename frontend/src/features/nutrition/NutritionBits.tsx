import clsx from 'clsx'
import { AlertTriangle, Leaf } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { Avatar, Card, Progress, Select } from '@/components/ui'
import { patch } from '@/lib/api'
import { useAction } from '@/lib/meals'
import { MICROS, NUTRIENTS, fmtAmount, nutrientInfo, progressTone, useFoods, useHealth } from '@/lib/nutrition'
import { hasRole, useSession } from '@/lib/session'
import type { MemberHealth, NutrientKey, Nutrients, Recipe, Targets } from '@/lib/types'

export const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

/** "Nov–Feb", "Mar–Jun"; empty for all year. Runs wrap around the new year. */
export function seasonLabel(months: number[]) {
  if (!months.length || months.length === 12) return ''
  const set = new Set(months)
  const prev = (m: number) => (m === 1 ? 12 : m - 1)
  const next = (m: number) => (m % 12) + 1
  return [...set]
    .filter((m) => !set.has(prev(m)))
    .sort((a, b) => a - b)
    .map((start) => {
      let end = start
      while (set.has(next(end))) end = next(end)
      return end === start ? MONTHS[start - 1] : `${MONTHS[start - 1]}–${MONTHS[end - 1]}`
    })
    .join(', ')
}

export function MonthPicker({ value, onChange }: { value: number[]; onChange: (v: number[]) => void }) {
  const all = value.length === 0
  return (
    <div className="flex flex-wrap gap-1">
      <button
        type="button"
        onClick={() => onChange([])}
        className={clsx('h-7 rounded-md border px-2 text-[12px]', all ? 'border-accent bg-accent-soft text-accent' : 'border-border text-muted hover:text-text')}
      >
        All year
      </button>
      {MONTHS.map((m, i) => {
        const on = value.includes(i + 1)
        return (
          <button
            key={m}
            type="button"
            onClick={() => onChange(on ? value.filter((x) => x !== i + 1) : [...value, i + 1].sort((a, b) => a - b))}
            className={clsx('h-7 w-10 rounded-md border text-[12px]', on ? 'border-accent bg-accent-soft text-accent' : 'border-border text-muted hover:text-text')}
          >
            {m}
          </button>
        )
      })}
    </div>
  )
}

/** One nutrient as amount + bar against a person's daily target. */
export function NutrientRow({ k, value, target, compact }: { k: NutrientKey; value: number; target?: number; compact?: boolean }) {
  const info = nutrientInfo(k)
  const pct = target ? Math.round((value / target) * 100) : null
  return (
    <div className={clsx('grid items-center gap-x-3', compact ? 'grid-cols-[84px_minmax(0,1fr)_88px]' : 'grid-cols-[96px_minmax(0,1fr)_112px]')}>
      <span className="truncate text-[12px] text-muted">{info.label}</span>
      {target ? <Progress value={value / target} tone={progressTone(value, target, info.limit)} /> : <span />}
      <span className="text-right text-[12px] tabular">
        {fmtAmount(value, info.unit)}
        {pct != null && (
          <span className="text-subtle">
            {' · '}
            {info.limit ? `${pct}% of max` : `${pct}%`}
          </span>
        )}
      </span>
    </div>
  )
}

/** The macro headline: calories, protein, carbs, fat, fibre — amount, then share of target. */
export function MacroStrip({ n, targets }: { n: Nutrients; targets?: Targets | null }) {
  return (
    <div className="grid grid-cols-5 gap-1.5">
      {NUTRIENTS.filter((x) => x.macro).map((x) => {
        const pct = targets && targets[x.key] ? Math.round((n[x.key] / targets[x.key]) * 100) : null
        return (
          <div key={x.key} className="min-w-0 rounded-md bg-surface-2 px-1.5 py-1.5 text-center">
            <div className="text-[15px] leading-tight font-semibold tabular">
              {fmtAmount(n[x.key])}
              <span className="ml-0.5 text-[10px] font-normal text-subtle">{x.key === 'kcal' ? '' : x.unit}</span>
            </div>
            <div className="truncate text-[11px] text-muted">{x.key === 'kcal' ? 'kcal' : x.short}</div>
            {pct != null && <div className="text-[10px] text-subtle tabular">{pct}%{x.limit ? ' of max' : ''}</div>}
          </div>
        )
      })}
    </div>
  )
}

export function PersonPicker({ people, value, onChange }: { people: MemberHealth[]; value: string; onChange: (id: string) => void }) {
  if (people.length < 2) return null
  return (
    <div className="flex gap-1">
      {people.map((p) => (
        <button
          key={p.userId}
          onClick={() => onChange(p.userId)}
          className={clsx('flex h-7 items-center gap-1.5 rounded-full border pr-2.5 pl-0.5 text-[12px]', value === p.userId ? 'border-accent bg-accent-soft text-text' : 'border-border text-muted')}
        >
          <Avatar name={p.name} color={p.color} size={22} /> {p.name}
        </button>
      ))}
    </div>
  )
}

export function RecipeNutritionCard({ recipe }: { recipe: Recipe }) {
  const { user } = useSession()
  const canSee = hasRole(user, 'member')
  const { data: health } = useHealth(canSee)
  const people = (health?.members ?? []).filter((m) => m.targets)
  const [who, setWho] = useState(user.id)
  const person = people.find((p) => p.userId === who) ?? people[0]
  const n = recipe.nutrition
  if (!n) return null
  const unknown = n.coverage < 0.5
  return (
    <Card className="h-fit">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border px-4 py-3">
        <div>
          <h2 className="text-[13px] font-semibold">Nutrition per serving</h2>
          <p className="text-[12px] text-subtle">{person ? `Share of ${person.userId === user.id ? 'your' : `${person.name}'s`} daily target` : 'Estimated from the ingredients'}</p>
        </div>
        <PersonPicker people={people} value={person?.userId ?? ''} onChange={setWho} />
      </div>
      <div className="space-y-4 p-4">
        {unknown ? (
          <p className="text-[13px] text-muted">Not enough of the ingredients could be measured to estimate nutrition.</p>
        ) : (
          <>
            <MacroStrip n={n.perServing} targets={person?.targets} />
            <div className="space-y-2">
              {MICROS.map((x) => (
                <NutrientRow key={x.key} k={x.key} value={n.perServing[x.key]} target={person?.targets?.[x.key]} compact />
              ))}
            </div>
          </>
        )}
        {n.missing.length > 0 && <MissingFoods recipe={recipe} />}
        {canSee && !people.length && (
          <p className="text-[12px] text-subtle">
            Add height, weight and goals in{' '}
            <Link to="/settings/health" className="text-accent hover:underline">
              Settings → Health
            </Link>{' '}
            to see this against daily targets.
          </p>
        )}
        <p className="flex items-start gap-1.5 text-[11px] text-subtle">
          <Leaf className="mt-px size-3 shrink-0" />
          Estimates from Indian food tables (IFCT 2017) and USDA data. Oil and portions vary from kitchen to kitchen.
        </p>
      </div>
    </Card>
  )
}

/** Ingredients the estimate couldn't use, with a picker to say what each one is. */
function MissingFoods({ recipe }: { recipe: Recipe }) {
  const missing = recipe.ingredients.filter((i) => recipe.nutrition?.missing.includes(i.name))
  return (
    <div className="rounded-md bg-warning-soft px-3 py-2">
      <p className="flex items-center gap-1.5 text-[12px] font-medium text-warning">
        <AlertTriangle className="size-3.5" /> Left out of the estimate
      </p>
      <ul className="mt-1.5 space-y-1.5">
        {missing.map((i) => (
          <li key={i.name} className="flex items-center gap-2 text-[12px]">
            <span className="min-w-0 flex-1 truncate">
              {i.name}
              {i.food && <span className="text-subtle"> · can't weigh “{i.unit || 'pieces'}”</span>}
            </span>
            {i.ingredientId && <FoodPicker ingredientId={i.ingredientId} value={i.foodKey ?? ''} />}
          </li>
        ))}
      </ul>
    </div>
  )
}

export function FoodPicker({ ingredientId, value }: { ingredientId: string; value: string }) {
  const { data: foods = [] } = useFoods()
  const save = useAction((foodKey: string) => patch(`/ingredients/${ingredientId}`, { foodKey }), [['recipes'], ['nutrition']], 'Nutrition updated')
  return (
    <div className="w-44 shrink-0">
      <Select className="h-7 text-[12px] lg:h-7" value={value} onChange={(e) => save.mutate(e.target.value)} disabled={save.isPending} aria-label="What is this ingredient?">
        <option value="">Match by name…</option>
        <option value="-">Leave out (negligible)</option>
        {foods.map((f) => (
          <option key={f.key} value={f.key}>
            {f.name}
          </option>
        ))}
      </Select>
    </div>
  )
}
