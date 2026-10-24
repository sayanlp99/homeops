import clsx from 'clsx'
import { addDays, format, isSameDay, parseISO } from 'date-fns'
import { ChevronLeft, ChevronRight, HeartPulse, Info, TrendingDown, TrendingUp } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { Avatar, Button, Card, EmptyState, PageHeader, Spinner } from '@/components/ui'
import { weekRange } from '@/lib/meals'
import { GOAL_LABELS, MICROS, fmtAmount, nutrientInfo, useHealth, useNutrition } from '@/lib/nutrition'
import { hasRole, useSession } from '@/lib/session'
import type { PersonReport } from '@/lib/types'
import { weekStart } from '@/lib/weeks'
import { MealsNav } from '../meals/MealBits'
import { MacroStrip, NutrientRow } from './NutritionBits'

const tips: Record<string, string> = {
  sodium: 'Mostly from salt: try ¼ tsp less per dish, skip extra salt at the table, and keep pickle and papad occasional.',
  fat: 'Usually the cooking oil: measure it with a spoon rather than pouring, and keep bhaja to a few times a week.',
  kcal: 'Smaller rice/chapati portions are the easiest lever; the planner sizes them when you auto-fill.',
}

export function NutritionPage() {
  const { household, user } = useSession()
  const [params, setParams] = useSearchParams()
  const start = useMemo(() => {
    const w = params.get('week')
    return w ? parseISO(w) : weekStart(new Date(), household.sprintStartDow)
  }, [params, household.sprintStartDow])
  const { from, to } = weekRange(start)
  const canSee = hasRole(user, 'member')
  const { data: report, isLoading } = useNutrition(from, to, canSee)
  const { data: health } = useHealth(canSee)
  const goWeek = (delta: number) =>
    setParams((p) => {
      p.set('week', format(addDays(start, delta * 7), 'yyyy-MM-dd'))
      return p
    })
  const people = (report ?? []).filter((p) => p.targets)
  const missing = (report ?? []).filter((p) => !p.targets)

  return (
    <div>
      <PageHeader
        title="Kitchen"
        subtitle={`Nutrition · ${format(start, 'd MMM')} – ${format(addDays(start, 6), 'd MMM yyyy')}`}
        actions={
          <div className="flex items-center rounded-md border border-border bg-surface shadow-card">
            <button className="flex size-8 items-center justify-center text-muted hover:text-text" onClick={() => goWeek(-1)} aria-label="Previous week">
              <ChevronLeft className="size-4" />
            </button>
            <button
              className="h-8 border-x border-border px-3 text-[12px] font-medium"
              onClick={() =>
                setParams((p) => {
                  p.delete('week')
                  return p
                })
              }
            >
              This week
            </button>
            <button className="flex size-8 items-center justify-center text-muted hover:text-text" onClick={() => goWeek(1)} aria-label="Next week">
              <ChevronRight className="size-4" />
            </button>
          </div>
        }
      />
      <MealsNav />
      {!canSee ? (
        <Card>
          <EmptyState icon={<HeartPulse className="size-5" />} title="Nutrition is for household members" />
        </Card>
      ) : isLoading ? (
        <Spinner className="mx-auto mt-10" />
      ) : people.length === 0 ? (
        <Card>
          <EmptyState
            icon={<HeartPulse className="size-5" />}
            title="Add your details to track nutrition"
            description="With age, height, weight and a goal for each person, HomeOps sets daily targets, sizes portions when it plans meals, and shows what's running low."
            action={
              <Link to="/settings/health">
                <Button variant="primary">Add details</Button>
              </Link>
            }
          />
        </Card>
      ) : (
        <>
          <div className="grid gap-4 xl:grid-cols-2">
            {people.map((p) => (
              <PersonCard key={p.userId} p={p} start={start} goal={health?.members.find((m) => m.userId === p.userId)?.profile?.goal} />
            ))}
          </div>
          {missing.length > 0 && (
            <p className="mt-3 text-[12px] text-subtle">
              {missing.map((m) => m.name).join(', ')} {missing.length === 1 ? "hasn't" : "haven't"} added details yet —{' '}
              <Link to="/settings/health" className="text-accent hover:underline">
                add them
              </Link>
              .
            </p>
          )}
          <p className="mt-4 flex items-start gap-1.5 text-[12px] text-subtle">
            <Info className="mt-0.5 size-3.5 shrink-0" />
            From the meal plan, using each person's portions. Food values come from the Indian Food Composition Tables (IFCT 2017) and USDA; targets from ICMR-NIN 2020. These are
            estimates to spot patterns, not medical advice. For a health condition, pregnancy or a steep weight-loss goal, check with a doctor or dietitian.
          </p>
        </>
      )}
    </div>
  )
}

function PersonCard({ p, start, goal }: { p: PersonReport; start: Date; goal?: keyof typeof GOAL_LABELS }) {
  const t = p.targets!
  const lows = p.gaps.filter((g) => g.kind === 'low')
  const highs = p.gaps.filter((g) => g.kind === 'high')
  return (
    <Card>
      <div className="flex items-center justify-between gap-3 border-b border-border px-4 py-3">
        <div className="flex min-w-0 items-center gap-2.5">
          <Avatar name={p.name} color={p.color} size={28} />
          <div className="min-w-0">
            <h2 className="truncate text-[14px] font-semibold">{p.name}</h2>
            <p className="truncate text-[12px] text-subtle">
              {fmtAmount(t.kcal)} kcal · {fmtAmount(t.protein)} g protein a day{goal ? ` · ${GOAL_LABELS[goal].label.toLowerCase()}` : ''}
            </p>
          </div>
        </div>
      </div>
      <div className="space-y-5 p-4">
        {p.counted === 0 ? (
          <p className="text-[13px] text-muted">Nothing planned this week yet{p.partial ? ' (only meals out or dishes without nutrition)' : ''}.</p>
        ) : (
          <>
            <section>
              <h3 className="mb-2 text-[12px] font-medium text-muted">
                Daily average · {p.counted} {p.counted === 1 ? 'day' : 'days'}
                {p.partial > 0 && <span className="text-subtle"> ({p.partial} with a meal out left out)</span>}
              </h3>
              <MacroStrip n={p.average} targets={t} />
            </section>
            <section>
              <h3 className="mb-2 text-[12px] font-medium text-muted">Calories by day</h3>
              <DailyBars p={p} start={start} />
            </section>
            <section>
              <h3 className="mb-2 text-[12px] font-medium text-muted">Vitamins & minerals (daily average)</h3>
              <div className="space-y-2">
                {MICROS.map((x) => (
                  <NutrientRow key={x.key} k={x.key} value={p.average[x.key]} target={t[x.key]} />
                ))}
              </div>
            </section>
            {(lows.length > 0 || highs.length > 0) && (
              <section className="space-y-2">
                <h3 className="text-[12px] font-medium text-muted">To balance</h3>
                {lows.map((g) => (
                  <div key={g.nutrient} className="rounded-md bg-surface-2 px-3 py-2 text-[13px]">
                    <p className="flex items-center gap-1.5">
                      <TrendingDown className="size-3.5 text-warning" />
                      <span className="font-medium">{nutrientInfo(g.nutrient).label}</span>
                      <span className="text-subtle">at {g.percent}% of target</span>
                    </p>
                    {g.ideas.length > 0 && (
                      <p className="mt-1 text-[12px] text-muted">
                        Good sources you cook:{' '}
                        {g.ideas.map((i, n) => (
                          <span key={i.recipeId}>
                            {n > 0 && ', '}
                            <Link to={`/recipes/${i.recipeId}`} className="text-accent hover:underline">
                              {i.title}
                            </Link>{' '}
                            <span className="text-subtle">({i.percent}%)</span>
                          </span>
                        ))}
                      </p>
                    )}
                    {g.nutrient === 'b12' && <p className="mt-1 text-[12px] text-subtle">B12 comes only from animal foods (milk, curd, eggs, fish, chicken); if you eat little of these, ask a doctor about a supplement.</p>}
                    {g.nutrient === 'iron' && <p className="mt-1 text-[12px] text-subtle">Have lemon, guava or amla with dal and greens, and keep tea away from meals — vitamin C helps iron absorb, tea blocks it.</p>}
                  </div>
                ))}
                {highs.map((g) => (
                  <div key={g.nutrient} className="rounded-md bg-surface-2 px-3 py-2 text-[13px]">
                    <p className="flex items-center gap-1.5">
                      <TrendingUp className="size-3.5 text-danger" />
                      <span className="font-medium">{nutrientInfo(g.nutrient).label}</span>
                      <span className="text-subtle">at {g.percent}% of the {g.nutrient === 'kcal' ? 'target' : 'limit'}</span>
                    </p>
                    {tips[g.nutrient] && <p className="mt-1 text-[12px] text-muted">{tips[g.nutrient]}</p>}
                  </div>
                ))}
              </section>
            )}
          </>
        )}
      </div>
    </Card>
  )
}

/** One bar per day against a dashed target line; hover or tap a bar for the numbers. */
function DailyBars({ p, start }: { p: PersonReport; start: Date }) {
  const t = p.targets!
  const [hover, setHover] = useState<number | null>(null)
  const W = 420
  const H = 130
  const pad = { top: 14, bottom: 22, left: 4, right: 4 }
  const max = Math.max(t.kcal * 1.3, ...p.days.map((d) => d.totals.kcal))
  const y = (v: number) => pad.top + (H - pad.top - pad.bottom) * (1 - v / max)
  const slot = (W - pad.left - pad.right) / p.days.length
  const bw = Math.min(36, slot * 0.5)
  const sel = hover != null ? p.days[hover] : null
  return (
    <div className="relative">
      <svg viewBox={`0 0 ${W} ${H}`} className="w-full" role="img" aria-label={`Calories by day for ${p.name}`} onMouseLeave={() => setHover(null)}>
        <line x1={pad.left} x2={W - pad.right} y1={y(0)} y2={y(0)} stroke="var(--color-border)" />
        {p.days.map((d, i) => {
          const x = pad.left + slot * i + (slot - bw) / 2
          const v = d.totals.kcal
          const partial = d.unknown.length > 0
          const today = isSameDay(addDays(start, i), new Date())
          return (
            <g key={d.date} onMouseEnter={() => setHover(i)} onClick={() => setHover(hover === i ? null : i)}>
              <rect x={pad.left + slot * i} y={0} width={slot} height={H} fill="transparent" />
              {v > 0 && (
                <path
                  d={`M${x},${y(0)} V${y(v) + 4} a4,4 0 0 1 4,-4 h${bw - 8} a4,4 0 0 1 4,4 V${y(0)} Z`}
                  fill={p.color}
                  opacity={partial ? 0.4 : hover == null || hover === i ? 1 : 0.55}
                />
              )}
              <text x={x + bw / 2} y={H - 6} textAnchor="middle" fontSize="11" fill={today ? 'var(--color-accent)' : 'var(--color-subtle)'} fontWeight={today ? 600 : 400}>
                {format(addDays(start, i), 'EEE')}
              </text>
            </g>
          )
        })}
        <line x1={pad.left} x2={W - pad.right} y1={y(t.kcal)} y2={y(t.kcal)} stroke="var(--color-muted)" strokeDasharray="4 4" strokeWidth={1.5} />
        <text x={pad.left} y={y(t.kcal) - 4} fontSize="11" fill="var(--color-muted)">
          target {fmtAmount(t.kcal)}
        </text>
      </svg>
      {sel && (
        <div className="pointer-events-none absolute top-0 left-1/2 -translate-x-1/2 rounded-md border border-border bg-surface px-2.5 py-1.5 text-[12px] shadow-pop">
          <span className="font-medium">{format(parseISO(sel.date), 'EEE d MMM')}</span>
          <span className="tabular">
            {' · '}
            {fmtAmount(sel.totals.kcal)} kcal · {Math.round(sel.totals.protein)} g protein · {Math.round(sel.totals.fibre)} g fibre
          </span>
          {sel.unknown.length > 0 && <span className="block text-subtle">Not counted: {sel.unknown.join(', ')}</span>}
        </div>
      )}
      <p className={clsx('mt-1 text-[11px] text-subtle')}>Faded bars include a meal with no nutrition (eating out), so they're incomplete.</p>
    </div>
  )
}
