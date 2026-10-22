import clsx from 'clsx'
import { differenceInCalendarDays, format, getDaysInMonth } from 'date-fns'
import { AlertTriangle, CalendarClock, CheckCircle2, Clock, Lock, Pencil, Plus, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Modal } from '@/components/Dialog'
import { Badge, Button, Card, EmptyState, Field, IconButton, Input, PageHeader, Select, Switch } from '@/components/ui'
import { api, del, post } from '@/lib/api'
import { useAction } from '@/lib/meals'
import { parseRupees, rupees, useAccounts, useBills, useMoneyCategories } from '@/lib/money'
import type { Bill } from '@/lib/types'
import { CategoryIcon, MoneyNav } from './MoneyBits'

function billStatus(b: Bill) {
  const now = new Date()
  const month = format(now, 'yyyy-MM')
  if (b.lastPaidMonth === month) return { tone: 'success' as const, label: 'Paid this month', icon: CheckCircle2, days: 0 }
  const due = new Date(now.getFullYear(), now.getMonth(), Math.min(b.dueDay, getDaysInMonth(now)))
  const days = differenceInCalendarDays(due, now)
  if (days < 0) return { tone: 'danger' as const, label: `Overdue by ${-days} ${days === -1 ? 'day' : 'days'}`, icon: AlertTriangle, days }
  if (days === 0) return { tone: 'warning' as const, label: 'Due today', icon: Clock, days }
  if (days <= b.remindDaysBefore) return { tone: 'warning' as const, label: `Due in ${days} ${days === 1 ? 'day' : 'days'}`, icon: Clock, days }
  return { tone: 'neutral' as const, label: `Due on the ${b.dueDay}${['th', 'st', 'nd', 'rd'][b.dueDay % 10 > 3 || [11, 12, 13].includes(b.dueDay % 100) ? 0 : b.dueDay % 10]}`, icon: CalendarClock, days }
}

export function BillsPage() {
  const { data: bills = [] } = useBills()
  const { data: categories = [] } = useMoneyCategories()
  const [editing, setEditing] = useState<Partial<Bill> | null>(null)
  const [paying, setPaying] = useState<Bill | null>(null)
  const catById = new Map(categories.map((c) => [c.id, c]))
  const sorted = [...bills].sort((a, b) => billStatus(a).days - billStatus(b).days || a.dueDay - b.dueDay)
  const unpaid = bills.filter((b) => b.active && b.lastPaidMonth !== format(new Date(), 'yyyy-MM'))

  return (
    <div>
      <PageHeader
        title="Money"
        actions={
          <Button variant="primary" icon={<Plus className="size-4" />} onClick={() => setEditing({ dueDay: 5, remindDaysBefore: 3, active: true, amount: 0 })}>
            Add bill
          </Button>
        }
      />
      <MoneyNav />
      {bills.length > 0 && (
        <p className="mb-3 text-[13px] text-muted">
          {unpaid.length ? `${unpaid.length} unpaid this month · ${rupees(unpaid.reduce((s, b) => s + b.amount, 0))} expected` : 'All bills paid this month 🎉'}
        </p>
      )}
      <Card>
        {bills.length === 0 ? (
          <EmptyState
            icon={<CalendarClock className="size-5" />}
            title="No bills yet"
            description="Electricity, rent, internet, insurance, SIPs… You'll both get a reminder a few days before each is due, and marking it paid records the expense."
            action={<Button onClick={() => setEditing({ dueDay: 5, remindDaysBefore: 3, active: true, amount: 0 })}>Add a bill</Button>}
          />
        ) : (
          <ul className="divide-y divide-border">
            {sorted.map((b) => {
              const st = billStatus(b)
              const cat = b.categoryId ? catById.get(b.categoryId) : undefined
              return (
                <li key={b.id} className={clsx('flex items-center gap-3 px-4 py-3', !b.active && 'opacity-50')}>
                  <CategoryIcon category={cat} size={32} />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-1.5 text-[13px] font-medium">
                      {b.name}
                      {b.ownerId && <Lock className="size-3 text-subtle" />}
                    </div>
                    <div className="mt-0.5 flex flex-wrap items-center gap-2 text-[12px] text-subtle">
                      <Badge tone={st.tone}>
                        <st.icon className="size-3" /> {st.label}
                      </Badge>
                      {cat && <span>{cat.name}</span>}
                    </div>
                  </div>
                  <span className="text-[13px] font-medium tabular">{b.amount ? rupees(b.amount) : 'Varies'}</span>
                  {st.tone !== 'success' && b.active && (
                    <Button size="sm" onClick={() => setPaying(b)}>
                      Mark paid
                    </Button>
                  )}
                  <IconButton label="Edit" onClick={() => setEditing(b)}>
                    <Pencil className="size-3.5" />
                  </IconButton>
                </li>
              )
            })}
          </ul>
        )}
      </Card>
      <BillEditor value={editing} onClose={() => setEditing(null)} />
      <PayDialog bill={paying} onClose={() => setPaying(null)} />
    </div>
  )
}

function BillEditor({ value, onClose }: { value: Partial<Bill> | null; onClose: () => void }) {
  const { data: categories = [] } = useMoneyCategories()
  const { data: accounts = [] } = useAccounts()
  const [f, setF] = useState<Partial<Bill> & { amountText?: string; personal?: boolean }>({})
  useEffect(() => {
    if (value) setF({ ...value, amountText: value.amount ? String(value.amount / 100) : '', personal: !!value.ownerId })
  }, [value])
  const save = useAction(
    () => {
      const body = { name: f.name, amount: parseRupees(f.amountText ?? '') ?? 0, dueDay: f.dueDay, categoryId: f.categoryId || null, accountId: f.accountId || null, remindDaysBefore: f.remindDaysBefore, active: f.active, personal: f.personal }
      return f.id ? api('PATCH', `/money/bills/${f.id}`, body) : post('/money/bills', body)
    },
    [['money', 'bills']],
    'Bill saved',
  )
  const remove = useAction(() => del(`/money/bills/${f.id}`), [['money', 'bills']], 'Bill deleted')
  return (
    <Modal
      open={!!value}
      onOpenChange={(o) => !o && onClose()}
      title={f.id ? 'Edit bill' : 'New bill'}
      footer={
        <div className="flex w-full justify-between">
          {f.id ? (
            <Button variant="ghost" icon={<Trash2 className="size-4" />} onClick={() => confirm('Delete this bill?') && remove.mutate(undefined, { onSuccess: onClose })}>
              Delete
            </Button>
          ) : (
            <span />
          )}
          <div className="flex gap-2">
            <Button variant="ghost" onClick={onClose}>
              Cancel
            </Button>
            <Button variant="primary" loading={save.isPending} disabled={!f.name?.trim()} onClick={() => save.mutate(undefined, { onSuccess: onClose })}>
              Save
            </Button>
          </div>
        </div>
      }
    >
      <div className="grid grid-cols-2 gap-3">
        <Field label="Name" className="col-span-2">
          <Input autoFocus placeholder="e.g. Electricity (BESCOM)" value={f.name ?? ''} onChange={(e) => setF({ ...f, name: e.target.value })} />
        </Field>
        <Field label="Usual amount (₹)" hint="Leave empty if it varies">
          <Input inputMode="decimal" value={f.amountText ?? ''} onChange={(e) => setF({ ...f, amountText: e.target.value })} />
        </Field>
        <Field label="Due on day">
          <Input type="number" min={1} max={31} value={f.dueDay ?? 5} onChange={(e) => setF({ ...f, dueDay: Math.min(31, Math.max(1, Number(e.target.value) || 1)) })} />
        </Field>
        <Field label="Category">
          <Select value={f.categoryId ?? ''} onChange={(e) => setF({ ...f, categoryId: e.target.value || null })}>
            <option value="">—</option>
            {categories.filter((c) => c.kind === 'expense' && !c.archived && (f.personal ? !!c.ownerId : !c.ownerId)).map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Remind">
          <Select value={f.remindDaysBefore ?? 3} onChange={(e) => setF({ ...f, remindDaysBefore: Number(e.target.value) })}>
            {[0, 1, 2, 3, 5, 7].map((d) => (
              <option key={d} value={d}>
                {d === 0 ? 'On the day' : `${d} days before`}
              </option>
            ))}
          </Select>
        </Field>
        {accounts.length > 0 && (
          <Field label="Paid from" className="col-span-2">
            <Select value={f.accountId ?? ''} onChange={(e) => setF({ ...f, accountId: e.target.value || null })}>
              <option value="">—</option>
              {accounts.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </Select>
          </Field>
        )}
        <label className="col-span-2 flex items-center justify-between">
          <span className="text-[13px]">Personal (only you get reminders)</span>
          <Switch checked={!!f.personal} onChange={(personal) => setF({ ...f, personal, categoryId: null })} label="Personal" />
        </label>
        <label className="col-span-2 flex items-center justify-between">
          <span className="text-[13px]">Active</span>
          <Switch checked={f.active ?? true} onChange={(active) => setF({ ...f, active })} label="Active" />
        </label>
      </div>
    </Modal>
  )
}

function PayDialog({ bill, onClose }: { bill: Bill | null; onClose: () => void }) {
  const [amount, setAmount] = useState('')
  const [date, setDate] = useState(format(new Date(), 'yyyy-MM-dd'))
  useEffect(() => {
    if (bill) {
      setAmount(bill.amount ? String(bill.amount / 100) : '')
      setDate(format(new Date(), 'yyyy-MM-dd'))
    }
  }, [bill])
  const pay = useAction(() => post(`/money/bills/${bill!.id}/pay`, { amount: parseRupees(amount) ?? 0, date }), [['money']], 'Marked paid and recorded as an expense')
  if (!bill) return null
  return (
    <Modal
      open={!!bill}
      onOpenChange={(o) => !o && onClose()}
      title={`Pay ${bill.name}`}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" disabled={!parseRupees(amount)} loading={pay.isPending} onClick={() => pay.mutate(undefined, { onSuccess: onClose })}>
            Mark paid
          </Button>
        </>
      }
    >
      <div className="grid grid-cols-2 gap-3">
        <Field label="Amount paid (₹)">
          <Input autoFocus inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} />
        </Field>
        <Field label="Paid on">
          <Input type="date" value={date} onChange={(e) => setDate(e.target.value)} />
        </Field>
      </div>
    </Modal>
  )
}
