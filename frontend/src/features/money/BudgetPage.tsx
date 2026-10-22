import clsx from 'clsx'
import { getDaysInMonth, parse } from 'date-fns'
import { Check, Lock, Pencil, Plus, Repeat, Upload } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { Button, Card, CardHeader, EmptyState, Input, PageHeader, Spinner } from '@/components/ui'
import { api, patch } from '@/lib/api'
import { useAction } from '@/lib/meals'
import { monthKey, parseRupees, rupees, useMoneyCategories, useSummary } from '@/lib/money'
import { useSession } from '@/lib/session'
import type { MoneyCategory } from '@/lib/types'
import { openQuickExpense } from '@/lib/ui'
import { Meter, MeterLabel, PaceChart, SplitBar, TrendChart, meterState } from './Charts'
import { CategoryIcon, MoneyNav, MonthSwitcher } from './MoneyBits'

export function useMonthParam() {
  const [params, setParams] = useSearchParams()
  const month = params.get('month') || monthKey()
  const setMonth = (m: string) =>
    setParams((p) => {
      if (m === monthKey()) p.delete('month')
      else p.set('month', m)
      return p
    })
  return [month, setMonth] as const
}

export function BudgetPage() {
  const [month, setMonth] = useMonthParam()
  const { data: s, isLoading } = useSummary(month)
  const { data: categories = [] } = useMoneyCategories()
  const { members, memberById } = useSession()
  const [editing, setEditing] = useState(false)
  const navigate = useNavigate()

  const byId = useMemo(() => new Map(categories.map((c) => [c.id, c])), [categories])
  const d = parse(month, 'yyyy-MM', new Date())
  const days = getDaysInMonth(d)
  const isCurrent = month === monthKey()
  const paceFraction = isCurrent ? new Date().getDate() / days : 1

  if (isLoading || !s) return <Spinner className="mx-auto mt-16" />
  const rows = s.categories.map((c) => ({ ...c, cat: byId.get(c.categoryId) })).filter((r) => r.cat)
  const shared = rows.filter((r) => !r.cat!.ownerId)
  const personal = rows.filter((r) => r.cat!.ownerId)
  // Compare budgets only with spending in budgeted categories; uncategorized or
  // unbudgeted spending is shown in the total but doesn't make you "over budget".
  const budgetedSpent = s.categories.filter((c) => c.limit + c.carry > 0).reduce((t, c) => t + c.spent, 0)
  const left = s.budgeted - budgetedSpent
  const expectedByNow = s.budgeted * paceFraction
  const paceDelta = expectedByNow - budgetedSpent
  const hasData = s.spent > 0 || s.income > 0 || s.budgeted > 0

  return (
    <div>
      <PageHeader
        title="Money"
        actions={
          <>
            <MonthSwitcher month={month} onChange={setMonth} />
            <Button variant="primary" icon={<Plus className="size-4" />} onClick={() => openQuickExpense()} className="max-lg:hidden">
              Add expense
            </Button>
          </>
        }
      />
      <MoneyNav />

      {!hasData ? (
        <Card>
          <EmptyState
            title="Start tracking"
            description="Set monthly budgets for your categories, add expenses as you go (3 taps), or import a bank statement to fill in the month."
            action={
              <div className="flex flex-wrap justify-center gap-2">
                <Button onClick={() => setEditing(true)} icon={<Pencil className="size-4" />}>
                  Set budgets
                </Button>
                <Button onClick={() => navigate('/budget/import')} icon={<Upload className="size-4" />}>
                  Import statement
                </Button>
                <Button variant="primary" onClick={() => openQuickExpense()} icon={<Plus className="size-4" />}>
                  Add expense
                </Button>
              </div>
            }
          />
        </Card>
      ) : (
        <>
          <Card className="grid grid-cols-2 gap-4 p-4 lg:grid-cols-4">
            <Figure label="Spent" value={rupees(s.spent)} sub={s.budgeted ? `${rupees(budgetedSpent)} of ${rupees(s.budgeted)} budgeted` : 'no budgets set'} />
            <Figure
              label={left >= 0 ? 'Left to spend' : 'Over budget'}
              value={rupees(Math.abs(left))}
              tone={left < 0 ? 'danger' : undefined}
              sub={isCurrent && s.budgeted ? (paceDelta >= 0 ? `${rupees(paceDelta)} under pace` : `${rupees(-paceDelta)} ahead of pace`) : undefined}
            />
            <Figure label="Income" value={rupees(s.income)} sub={s.income ? `${rupees(s.income - s.spent)} net` : 'none recorded'} />
            <Link to={`/budget/transactions?category=none${month !== monthKey() ? `&month=${month}` : ''}`} className="min-w-0 rounded-md hover:bg-surface-2">
              <Figure label="Uncategorized" value={String(s.uncategorized)} sub={s.uncategorized ? 'tap to sort them' : 'all sorted'} tone={s.uncategorized ? 'warning' : undefined} />
            </Link>
          </Card>

          <div className="mt-4 grid gap-4 lg:grid-cols-3">
            <Card className="lg:col-span-2">
              <CardHeader title="Spending this month" subtitle={isCurrent ? `Day ${new Date().getDate()} of ${days}` : undefined} />
              <div className="p-4">
                <PaceChart daily={s.daily} budget={s.budgeted} month={month} />
              </div>
            </Card>
            <div className="space-y-4">
              <Card>
                <CardHeader title="Who paid" />
                <div className="p-4">
                  <SplitBar
                    parts={members.map((m) => ({ label: m.displayName, value: s.byPerson[m.id] ?? 0, color: memberById.get(m.id)!.color }))}
                  />
                </div>
              </Card>
              <Card>
                <CardHeader title="Last 6 months" />
                <div className="p-3">
                  <TrendChart trend={s.trend} current={month} />
                </div>
              </Card>
            </div>
          </div>
        </>
      )}

      <Card className="mt-4 overflow-hidden">
        <CardHeader
          title="Budgets"
          subtitle="Monthly envelopes. Rollover categories carry unspent money forward."
          actions={
            <Button size="sm" variant={editing ? 'primary' : 'secondary'} icon={editing ? <Check className="size-3.5" /> : <Pencil className="size-3.5" />} onClick={() => setEditing(!editing)}>
              {editing ? 'Done' : 'Edit budgets'}
            </Button>
          }
        />
        <EnvelopeList rows={shared} month={month} editing={editing} paceFraction={paceFraction} />
        {personal.length > 0 && (
          <>
            <div className="flex items-center gap-1.5 border-y border-border bg-surface-2/60 px-4 py-1.5 text-[11px] font-semibold tracking-wide text-muted uppercase">
              <Lock className="size-3" /> Personal · only you see these
            </div>
            <EnvelopeList rows={personal} month={month} editing={editing} paceFraction={paceFraction} />
          </>
        )}
      </Card>

      {s.topPayees && s.topPayees.length > 0 && (
        <Card className="mt-4">
          <CardHeader title="Top payees" />
          <ul className="divide-y divide-border">
            {s.topPayees.map((p) => (
              <li key={p.payee}>
                <Link to={`/budget/transactions?q=${encodeURIComponent(p.payee)}${month !== monthKey() ? `&month=${month}` : ''}`} className="flex items-center justify-between px-4 py-2 text-[13px] hover:bg-surface-2">
                  <span className="truncate">{p.payee}</span>
                  <span className="flex items-center gap-3">
                    <span className="text-[12px] text-subtle">{p.count}×</span>
                    <span className="w-24 text-right tabular">{rupees(p.amount)}</span>
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        </Card>
      )}
    </div>
  )
}

function Figure({ label, value, sub, tone }: { label: string; value: string; sub?: string; tone?: 'danger' | 'warning' }) {
  return (
    <div className="min-w-0 p-1">
      <div className="truncate text-[11px] font-medium tracking-wide text-subtle uppercase">{label}</div>
      <div className={clsx('mt-1 text-xl font-semibold tracking-tight', tone === 'danger' && 'text-danger', tone === 'warning' && 'text-warning')}>{value}</div>
      {sub && <div className="mt-0.5 truncate text-[12px] text-muted">{sub}</div>}
    </div>
  )
}

type Row = { categoryId: string; limit: number; carry: number; spent: number; count: number; cat?: MoneyCategory }

function EnvelopeList({ rows, month, editing, paceFraction }: { rows: Row[]; month: string; editing: boolean; paceFraction: number }) {
  if (rows.length === 0) return <p className="px-4 py-5 text-[13px] text-subtle">No categories.</p>
  return (
    <ul className="divide-y divide-border">
      {rows.map((r) => (
        <Envelope key={r.categoryId} r={r} month={month} editing={editing} paceFraction={paceFraction} />
      ))}
    </ul>
  )
}

function Envelope({ r, month, editing, paceFraction }: { r: Row; month: string; editing: boolean; paceFraction: number }) {
  const c = r.cat!
  const available = r.limit + r.carry
  const state = meterState(r.spent, available, paceFraction)
  const [value, setValue] = useState(r.limit ? String(r.limit / 100) : '')
  const save = useAction((amount: number) => patch(`/money/categories/${c.id}`, { monthlyLimit: amount }), [['money']])
  const roll = useAction((rollover: boolean) => patch(`/money/categories/${c.id}`, { rollover }), [['money']])
  const saveMonth = useAction((amount: number) => api('PUT', `/money/categories/${c.id}/months/${month}`, { amount }), [['money']], 'Budget set for this month only')
  const isCurrent = month === monthKey()

  return (
    <li className="px-4 py-3">
      <div className="flex items-center gap-3">
        <CategoryIcon category={c} size={30} />
        <div className="min-w-0 flex-1">
          <div className="flex items-baseline justify-between gap-2">
            <Link to={`/budget/transactions?category=${c.id}${isCurrent ? '' : `&month=${month}`}`} className="flex min-w-0 items-center gap-1.5 truncate text-[13px] font-medium hover:underline">
              {c.name}
              {c.rollover && <Repeat className="size-3 shrink-0 text-subtle" aria-label="Rolls over" />}
            </Link>
            <span className="shrink-0 text-[13px] tabular">
              {rupees(r.spent)}
              {available > 0 && <span className="text-subtle"> / {rupees(available)}</span>}
            </span>
          </div>
          {!editing && (
            <>
              <div className="mt-1.5">
                <Meter spent={r.spent} available={available} state={state} />
              </div>
              <div className="mt-1 flex items-center justify-between gap-2">
                <MeterLabel state={state} left={available - r.spent} />
                {r.carry !== 0 && <span className="text-[11px] text-subtle">{r.carry > 0 ? `+${rupees(r.carry)} rolled over` : `${rupees(r.carry)} carried`}</span>}
              </div>
            </>
          )}
        </div>
      </div>
      {editing && (
        <div className="mt-2 flex flex-wrap items-center gap-2 pl-[42px]">
          <label className="flex items-center gap-1.5">
            <span className="text-[12px] text-subtle">Monthly ₹</span>
            <Input inputMode="decimal" value={value} onChange={(e) => setValue(e.target.value)} className="h-8 w-28" placeholder="0" />
          </label>
          <Button size="sm" onClick={() => save.mutate(parseRupees(value) ?? 0)} loading={save.isPending}>
            Save
          </Button>
          <Button size="sm" variant="ghost" onClick={() => saveMonth.mutate(parseRupees(value) ?? 0)} title="Override just this month">
            Only this month
          </Button>
          <label className="ml-auto flex items-center gap-1.5 text-[12px] text-muted">
            <input type="checkbox" checked={c.rollover} onChange={(e) => roll.mutate(e.target.checked)} className="accent-[var(--accent)]" />
            Roll over unspent
          </label>
        </div>
      )}
    </li>
  )
}
