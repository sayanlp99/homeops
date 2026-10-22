import clsx from 'clsx'
import { ArrowLeftRight, Lock, Paperclip, Plus, Search, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import { Modal } from '@/components/Dialog'
import { Avatar, Badge, Button, Card, EmptyState, Field, Input, PageHeader, Select, Spinner } from '@/components/ui'
import { del } from '@/lib/api'
import { formatShortDate } from '@/lib/format'
import { fileUrl, useAction } from '@/lib/meals'
import { parseRupees, rupees, useAccounts, useMoneyCategories, useTransactions, useUpdateTxn } from '@/lib/money'
import { useSession } from '@/lib/session'
import type { MoneyCategory, Transaction } from '@/lib/types'
import { openQuickExpense } from '@/lib/ui'
import { useMonthParam } from './BudgetPage'
import { CategoryIcon, MoneyNav, MonthSwitcher } from './MoneyBits'

export function TransactionsPage() {
  const [month, setMonth] = useMonthParam()
  const [params, setParams] = useSearchParams()
  const category = params.get('category') ?? ''
  const account = params.get('account') ?? ''
  const paidBy = params.get('paidBy') ?? ''
  const [q, setQ] = useState(params.get('q') ?? '')
  const [debounced, setDebounced] = useState(q)
  useEffect(() => {
    const t = setTimeout(() => setDebounced(q), 250)
    return () => clearTimeout(t)
  }, [q])
  const { data: txns = [], isLoading } = useTransactions({ month: debounced ? undefined : month, category, account, paidBy, q: debounced })
  const { data: categories = [] } = useMoneyCategories()
  const { data: accounts = [] } = useAccounts()
  const { members } = useSession()
  const [editing, setEditing] = useState<Transaction | null>(null)
  const catById = useMemo(() => new Map(categories.map((c) => [c.id, c])), [categories])
  const accById = useMemo(() => new Map(accounts.map((a) => [a.id, a])), [accounts])
  const setParam = (k: string, v: string) =>
    setParams((p) => {
      if (v) p.set(k, v)
      else p.delete(k)
      return p
    })

  const totals = useMemo(() => {
    let out = 0, inn = 0
    for (const t of txns) {
      if (t.kind === 'expense') out += t.amount
      if (t.kind === 'income') inn += t.amount
    }
    return { out, inn }
  }, [txns])

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
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <label className="flex h-8 min-w-48 flex-1 items-center gap-2 rounded-md border border-border bg-surface px-2.5 sm:max-w-64">
          <Search className="size-3.5 text-subtle" />
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Search all months" className="min-w-0 flex-1 bg-transparent outline-none placeholder:text-subtle" />
        </label>
        <Select value={category} onChange={(e) => setParam('category', e.target.value)} className="h-8 w-auto" aria-label="Category">
          <option value="">All categories</option>
          <option value="none">Uncategorized</option>
          {categories.filter((c) => !c.archived).map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
              {c.ownerId ? ' (personal)' : ''}
            </option>
          ))}
        </Select>
        {accounts.length > 0 && (
          <Select value={account} onChange={(e) => setParam('account', e.target.value)} className="h-8 w-auto" aria-label="Account">
            <option value="">All accounts</option>
            {accounts.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
          </Select>
        )}
        {members.length > 1 && (
          <Select value={paidBy} onChange={(e) => setParam('paidBy', e.target.value)} className="h-8 w-auto" aria-label="Paid by">
            <option value="">Anyone paid</option>
            {members.map((m) => (
              <option key={m.id} value={m.id}>
                {m.displayName} paid
              </option>
            ))}
          </Select>
        )}
        <span className="ml-auto text-[12px] text-muted tabular">
          {txns.length} · out {rupees(totals.out)}
          {totals.inn > 0 && ` · in ${rupees(totals.inn)}`}
        </span>
      </div>

      <Card className="overflow-hidden">
        {isLoading ? (
          <Spinner className="mx-auto my-10" />
        ) : txns.length === 0 ? (
          <EmptyState title="No transactions" description="Add an expense, or import a bank statement." action={<Button onClick={() => openQuickExpense()}>Add expense</Button>} />
        ) : (
          <ul className="divide-y divide-border">
            {txns.map((t) => (
              <TxnRow key={t.id} t={t} category={t.categoryId ? catById.get(t.categoryId) : undefined} categories={categories} accountName={t.accountId ? accById.get(t.accountId)?.name : undefined} onOpen={() => setEditing(t)} />
            ))}
          </ul>
        )}
      </Card>
      <EditTxnDialog txn={editing} onClose={() => setEditing(null)} categories={categories} />
    </div>
  )
}

function TxnRow({ t, category, categories, accountName, onOpen }: { t: Transaction; category?: MoneyCategory; categories: MoneyCategory[]; accountName?: string; onOpen: () => void }) {
  const { memberById } = useSession()
  const update = useUpdateTxn()
  const payer = t.paidBy ? memberById.get(t.paidBy) : undefined
  const options = categories.filter((c) => !c.archived && (t.ownerId ? !!c.ownerId : !c.ownerId) && (t.kind === 'income' ? c.kind === 'income' : c.kind === 'expense'))
  const recategorize = (categoryId: string) => {
    const learn = t.source === 'import' && !!t.payee && !!categoryId
    update.mutate(
      { id: t.id, changes: { categoryId: categoryId || null, learn } },
      { onSuccess: () => learn && toast.success(`Got it — “${t.payee}” will go to ${categories.find((c) => c.id === categoryId)?.name} from now on`) },
    )
  }
  return (
    <li className="flex items-center gap-3 px-3 py-2 hover:bg-surface-2 sm:px-4">
      <button onClick={onOpen} className="flex min-w-0 flex-1 items-center gap-3 text-left">
        {t.kind === 'transfer' ? (
          <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-surface-3 text-subtle">
            <ArrowLeftRight className="size-4" />
          </span>
        ) : (
          <CategoryIcon category={category} size={32} />
        )}
        <span className="min-w-0 flex-1">
          <span className="flex items-center gap-1.5 truncate text-[13px] font-medium">
            {t.payee || t.description || '—'}
            {t.ownerId && <Lock className="size-3 shrink-0 text-subtle" aria-label="Personal" />}
            {t.receipt && <Paperclip className="size-3 shrink-0 text-subtle" aria-label="Has receipt" />}
          </span>
          <span className="block truncate text-[12px] text-subtle">
            {formatShortDate(t.date)}
            {accountName && ` · ${accountName}`}
            {t.note && ` · ${t.note}`}
            {t.source === 'grocery' && ' · from shopping trip'}
            {t.source === 'bill' && ' · bill'}
          </span>
        </span>
      </button>
      {t.kind !== 'transfer' && (
        <Select
          value={t.categoryId ?? ''}
          onChange={(e) => recategorize(e.target.value)}
          className={clsx('hidden h-7 w-40 text-[12px] md:block', !t.categoryId && 'border-warning/50 text-warning')}
          aria-label="Category"
        >
          <option value="">Uncategorized</option>
          {options.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </Select>
      )}
      {t.kind === 'transfer' && <Badge className="hidden md:inline-flex">Transfer</Badge>}
      {payer && <Avatar name={payer.displayName} color={payer.color} size={20} className="hidden sm:inline-flex" />}
      <span className={clsx('w-24 shrink-0 text-right text-[13px] font-medium tabular', t.kind === 'income' && 'text-success', t.kind === 'transfer' && 'text-subtle', t.amount < 0 && 'text-success')}>
        {t.kind === 'income' || t.amount < 0 ? '+' : ''}
        {rupees(Math.abs(t.amount), true)}
      </span>
    </li>
  )
}

function EditTxnDialog({ txn, onClose, categories }: { txn: Transaction | null; onClose: () => void; categories: MoneyCategory[] }) {
  const update = useUpdateTxn()
  const remove = useAction((id: string) => del(`/money/transactions/${id}`), [['money']], 'Transaction deleted')
  const { members } = useSession()
  const [f, setF] = useState({ amount: '', date: '', payee: '', note: '', categoryId: '', kind: 'expense', paidBy: '' })
  useEffect(() => {
    if (txn) setF({ amount: String(Math.abs(txn.amount) / 100), date: txn.date, payee: txn.payee, note: txn.note, categoryId: txn.categoryId ?? '', kind: txn.kind, paidBy: txn.paidBy ?? '' })
  }, [txn])
  if (!txn) return null
  const options = categories.filter((c) => !c.archived && (txn.ownerId ? !!c.ownerId : !c.ownerId) && (f.kind === 'income' ? c.kind === 'income' : c.kind === 'expense'))
  const save = () => {
    const paise = parseRupees(f.amount)
    if (!paise) return toast.error('Enter an amount')
    update.mutate(
      {
        id: txn.id,
        changes: { amount: txn.amount < 0 ? -paise : paise, date: f.date, payee: f.payee, note: f.note, categoryId: f.categoryId || null, kind: f.kind, paidBy: f.paidBy || undefined, learn: txn.source === 'import' && f.categoryId !== (txn.categoryId ?? '') },
      },
      { onSuccess: onClose },
    )
  }
  return (
    <Modal
      open={!!txn}
      onOpenChange={(o) => !o && onClose()}
      title="Edit transaction"
      description={txn.description && txn.description !== txn.payee ? txn.description : undefined}
      footer={
        <div className="flex w-full justify-between">
          <Button variant="ghost" icon={<Trash2 className="size-4" />} onClick={() => confirm('Delete this transaction?') && remove.mutate(txn.id, { onSuccess: onClose })}>
            Delete
          </Button>
          <div className="flex gap-2">
            <Button variant="ghost" onClick={onClose}>
              Cancel
            </Button>
            <Button variant="primary" loading={update.isPending} onClick={save}>
              Save
            </Button>
          </div>
        </div>
      }
    >
      <div className="grid grid-cols-2 gap-3">
        <Field label="Amount (₹)">
          <Input inputMode="decimal" value={f.amount} onChange={(e) => setF({ ...f, amount: e.target.value })} />
        </Field>
        <Field label="Date">
          <Input type="date" value={f.date} onChange={(e) => setF({ ...f, date: e.target.value })} />
        </Field>
        <Field label="Payee" className="col-span-2">
          <Input value={f.payee} onChange={(e) => setF({ ...f, payee: e.target.value })} />
        </Field>
        <Field label="Type">
          <Select value={f.kind} onChange={(e) => setF({ ...f, kind: e.target.value, categoryId: '' })}>
            <option value="expense">Expense</option>
            <option value="income">Income</option>
            <option value="transfer">Transfer (not counted)</option>
          </Select>
        </Field>
        <Field label="Category">
          <Select value={f.categoryId} onChange={(e) => setF({ ...f, categoryId: e.target.value })} disabled={f.kind === 'transfer'}>
            <option value="">Uncategorized</option>
            {options.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </Select>
        </Field>
        {members.length > 1 && (
          <Field label="Paid by">
            <Select value={f.paidBy} onChange={(e) => setF({ ...f, paidBy: e.target.value })}>
              {members.map((m) => (
                <option key={m.id} value={m.id}>
                  {m.displayName}
                </option>
              ))}
            </Select>
          </Field>
        )}
        <Field label="Note" className={members.length > 1 ? '' : 'col-span-2'}>
          <Input value={f.note} onChange={(e) => setF({ ...f, note: e.target.value })} />
        </Field>
      </div>
      {txn.receipt && (
        <a href={fileUrl(txn.receipt)} target="_blank" rel="noreferrer" className="mt-4 block">
          <img src={fileUrl(txn.receipt, true)} alt="Receipt" className="max-h-56 rounded-md border border-border object-contain" />
        </a>
      )}
      {txn.amount < 0 && <p className="mt-3 text-[12px] text-subtle">This is a refund — it reduces the category's spending.</p>}
    </Modal>
  )
}
