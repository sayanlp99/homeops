import { useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import { AlertTriangle, CheckCircle2, FileText, Receipt, Settings2, Undo2, Upload } from 'lucide-react'
import { useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { Avatar, Badge, Button, Card, CardHeader, Field, Input, PageHeader, Segmented, Select, Spinner } from '@/components/ui'
import { clientId, del, post } from '@/lib/api'
import { formatDateTime, formatShortDate } from '@/lib/format'
import { fileUrl, formatQty, useAction } from '@/lib/meals'
import { ACCOUNT_KINDS, parseRupees, rupees, useAccounts, useImports, useMoneyCategories } from '@/lib/money'
import { useSession } from '@/lib/session'
import type { ImportMapping, ImportPreview, ImportRow, MoneyCategory, TxnKind } from '@/lib/types'
import { MoneyNav } from './MoneyBits'

async function upload<T>(path: string, form: FormData): Promise<T> {
  const res = await fetch('/api' + path, { method: 'POST', body: form, headers: { 'X-Requested-With': 'homeops', 'X-Client-Id': clientId } })
  const data = await res.json().catch(() => null)
  if (!res.ok) throw new Error(data?.message ?? 'Upload failed')
  return data as T
}

export function ImportPage() {
  const [mode, setMode] = useState<'statement' | 'invoice'>('statement')
  return (
    <div>
      <PageHeader title="Money" />
      <MoneyNav />
      <Segmented
        value={mode}
        onChange={setMode}
        className="mb-4"
        options={[
          { value: 'statement', label: 'Bank or card statement' },
          { value: 'invoice', label: 'Grocery invoice / bill' },
        ]}
      />
      {mode === 'statement' ? <StatementImport /> : <InvoiceImport />}
      <ImportHistory />
    </div>
  )
}

function DropZone({ accept, onFile, busy, hint }: { accept: string; onFile: (f: File) => void; busy: boolean; hint: string }) {
  const ref = useRef<HTMLInputElement>(null)
  const [over, setOver] = useState(false)
  return (
    <button
      type="button"
      onClick={() => ref.current?.click()}
      onDragOver={(e) => (e.preventDefault(), setOver(true))}
      onDragLeave={() => setOver(false)}
      onDrop={(e) => {
        e.preventDefault()
        setOver(false)
        const f = e.dataTransfer.files[0]
        if (f) onFile(f)
      }}
      className={clsx('flex w-full flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed px-4 py-8 text-center', over ? 'border-accent bg-accent-soft' : 'border-border-strong hover:border-accent')}
    >
      {busy ? <Spinner className="size-5" /> : <Upload className="size-6 text-subtle" />}
      <span className="text-[13px] font-medium">{busy ? 'Reading…' : 'Drop a file here, or tap to choose'}</span>
      <span className="text-[12px] text-subtle">{hint}</span>
      <input
        ref={ref}
        type="file"
        accept={accept}
        className="hidden"
        onChange={(e) => {
          const f = e.target.files?.[0]
          if (f) onFile(f)
          e.target.value = ''
        }}
      />
    </button>
  )
}

// ---- statements ----

const FIELDS: { key: keyof ImportMapping; label: string }[] = [
  { key: 'date', label: 'Date' },
  { key: 'description', label: 'Narration / description' },
  { key: 'debit', label: 'Debit / withdrawal' },
  { key: 'credit', label: 'Credit / deposit' },
  { key: 'amount', label: 'Amount (single column)' },
  { key: 'drCr', label: 'Dr/Cr indicator' },
  { key: 'ref', label: 'Reference no.' },
]

function StatementImport() {
  const { data: accounts = [] } = useAccounts()
  const { data: categories = [] } = useMoneyCategories()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [accountId, setAccountId] = useState('')
  const [newAccount, setNewAccount] = useState({ name: '', kind: 'bank' })
  const [file, setFile] = useState<File | null>(null)
  const [preview, setPreview] = useState<(ImportPreview & { check?: { ok: boolean; problem: string; summary: { debits: number; credits: number; drCount: number; crCount: number } } | null }) | null>(null)
  const [rows, setRows] = useState<ImportRow[]>([])
  const [busy, setBusy] = useState(false)
  const [mapping, setMapping] = useState<ImportMapping | null>(null)
  const [showMapper, setShowMapper] = useState(false)
  const account = accounts.find((a) => a.id === accountId)

  const run = async (f: File, m?: ImportMapping) => {
    let acc = accountId
    if (!acc) {
      if (!newAccount.name.trim()) return toast.error('Pick the account this statement is for')
      const created = await post<{ id: string }>('/money/accounts', newAccount)
      acc = created.id
      setAccountId(acc)
      qc.invalidateQueries({ queryKey: ['money', 'accounts'] })
    }
    setBusy(true)
    setFile(f)
    try {
      const form = new FormData()
      form.append('file', f)
      form.append('accountId', acc)
      if (m) form.append('mapping', JSON.stringify(m))
      const p = await upload<typeof preview & object>('/money/import/preview', form)
      setPreview(p)
      setRows(p.rows ?? [])
      setMapping(p.mapping)
      setShowMapper(!p.rows?.length)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Could not read the file')
    } finally {
      setBusy(false)
    }
  }

  const commit = useAction(
    () => post<{ imported: number; skipped: number }>('/money/import/commit', { accountId, filename: preview?.filename, mapping, rows }),
    [['money']],
    (r) => `Imported ${r.imported} transactions${r.skipped ? ` · ${r.skipped} skipped` : ''}`,
  )
  const set = (i: number, p: Partial<ImportRow>) => setRows((rs) => rs.map((r, j) => (j === i ? { ...r, ...p } : r)))
  const included = rows.filter((r) => r.include)
  const totals = useMemo(() => {
    let out = 0, inn = 0
    for (const r of included) {
      if (r.kind === 'transfer') continue
      if (r.debit) out += r.amount
      else inn += r.amount
    }
    return { out, inn }
  }, [included])

  return (
    <Card>
      <CardHeader title="Import a statement" subtitle="CSV, Excel (.xlsx), HTML-style .xls, or HDFC's text (.txt) download. Re-importing an overlapping period skips rows already imported." />
      <div className="space-y-4 p-4">
        <div className="grid gap-3 sm:grid-cols-[1fr_auto]">
          <Field label="Account">
            <Select value={accountId} onChange={(e) => setAccountId(e.target.value)}>
              <option value="">+ New account…</option>
              {accounts.filter((a) => !a.archived).map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                  {a.ownerId ? ' (personal)' : ''}
                  {a.hasImportMapping ? ' · layout remembered' : ''}
                </option>
              ))}
            </Select>
          </Field>
          {!accountId && (
            <div className="grid grid-cols-[1fr_auto] gap-2 sm:w-96">
              <Field label="New account name">
                <Input placeholder="e.g. HDFC Savings" value={newAccount.name} onChange={(e) => setNewAccount({ ...newAccount, name: e.target.value })} />
              </Field>
              <Field label="Type">
                <Select value={newAccount.kind} onChange={(e) => setNewAccount({ ...newAccount, kind: e.target.value })}>
                  {ACCOUNT_KINDS.map((k) => (
                    <option key={k.value} value={k.value}>
                      {k.label}
                    </option>
                  ))}
                </Select>
              </Field>
            </div>
          )}
        </div>
        {!preview && <DropZone accept=".csv,.txt,.xlsx,.xls,.tsv" onFile={(f) => run(f)} busy={busy} hint="Download it from net banking — no password-protected PDFs needed" />}

        {preview && (
          <>
            {preview.check && (
              <div className={clsx('flex items-start gap-2 rounded-md px-3 py-2 text-[13px]', preview.check.ok ? 'bg-success-soft text-success' : 'bg-warning-soft text-warning')}>
                {preview.check.ok ? <CheckCircle2 className="mt-0.5 size-4 shrink-0" /> : <AlertTriangle className="mt-0.5 size-4 shrink-0" />}
                <span>
                  {preview.check.ok
                    ? `Matches the bank's own summary: ${preview.check.summary.drCount} debits (${rupees(preview.check.summary.debits, true)}) and ${preview.check.summary.crCount} credits (${rupees(preview.check.summary.credits, true)}).`
                    : preview.check.problem}
                </span>
              </div>
            )}
            {preview.warnings?.filter((w) => w !== preview.check?.problem).map((w) => (
              <div key={w} className="flex items-center gap-2 rounded-md bg-warning-soft px-3 py-2 text-[13px] text-warning">
                <AlertTriangle className="size-4 shrink-0" /> {w}
              </div>
            ))}
            <div className="flex flex-wrap items-center gap-2">
              <span className="flex items-center gap-1.5 text-[13px] font-medium">
                <FileText className="size-4 text-subtle" /> {preview.filename}
              </span>
              <span className="text-[12px] text-muted">
                {rows.length} rows · {rows.filter((r) => r.duplicate).length} already imported · {rows.filter((r) => r.alreadyRecorded).length} recorded another way
              </span>
              <div className="ml-auto flex gap-2">
                <Button size="sm" variant="ghost" icon={<Settings2 className="size-3.5" />} onClick={() => setShowMapper(!showMapper)}>
                  Columns
                </Button>
                <Button size="sm" variant="ghost" onClick={() => (setPreview(null), setRows([]))}>
                  Choose another file
                </Button>
              </div>
            </div>
            {showMapper && mapping && preview.sample && (
              <Mapper sample={preview.sample} mapping={mapping} onChange={setMapping} onApply={() => file && run(file, mapping)} />
            )}
            {rows.length > 0 && (
              <div className="-mx-4 overflow-x-auto border-y border-border">
                <table className="w-full min-w-[760px] text-[13px]">
                  <thead className="bg-surface-2/60 text-[11px] tracking-wide text-subtle uppercase">
                    <tr>
                      <th className="w-8 px-3 py-2">
                        <input type="checkbox" aria-label="Select all" checked={rows.every((r) => r.include || r.duplicate)} onChange={(e) => setRows(rows.map((r) => ({ ...r, include: e.target.checked && !r.duplicate })))} className="accent-[var(--accent)]" />
                      </th>
                      <th className="px-2 py-2 text-left font-medium">Date</th>
                      <th className="px-2 py-2 text-left font-medium">Payee</th>
                      <th className="px-2 py-2 text-right font-medium">Amount</th>
                      <th className="px-2 py-2 text-left font-medium">Type</th>
                      <th className="px-2 py-2 text-left font-medium">Category</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-border">
                    {rows.map((r, i) => (
                      <StatementRow key={r.hash} r={r} categories={categories} personal={!!account?.ownerId} onChange={(p) => set(i, p)} />
                    ))}
                  </tbody>
                </table>
              </div>
            )}
            <div className="flex flex-wrap items-center justify-between gap-3">
              <span className="text-[12px] text-muted tabular">
                {included.length} selected · out {rupees(totals.out, true)} · in {rupees(totals.inn, true)}
              </span>
              <Button
                variant="primary"
                disabled={!included.length}
                loading={commit.isPending}
                onClick={() => {
                  // Land on the newest imported month, where any uncategorized rows are.
                  const month = included.map((r) => r.date.slice(0, 7)).sort().pop()
                  commit.mutate(undefined, { onSuccess: () => navigate(`/budget/transactions?category=none${month ? `&month=${month}` : ''}`) })
                }}
              >
                Import {included.length} transactions
              </Button>
            </div>
          </>
        )}
      </div>
    </Card>
  )
}

function StatementRow({ r, categories, personal, onChange }: { r: ImportRow; categories: MoneyCategory[]; personal: boolean; onChange: (p: Partial<ImportRow>) => void }) {
  const options = categories.filter((c) => !c.archived && (personal ? true : !c.ownerId) && c.kind === (r.kind === 'income' ? 'income' : 'expense'))
  return (
    <tr className={clsx(!r.include && 'opacity-55')}>
      <td className="px-3 py-1.5">
        <input type="checkbox" checked={r.include} disabled={r.duplicate} onChange={(e) => onChange({ include: e.target.checked })} className="accent-[var(--accent)]" aria-label="Include" />
      </td>
      <td className="px-2 py-1.5 whitespace-nowrap text-muted tabular">{formatShortDate(r.date)}</td>
      <td className="max-w-72 px-2 py-1.5">
        <div className="truncate font-medium">{r.payee}</div>
        <div className="truncate text-[11px] text-subtle" title={r.description}>
          {r.description}
        </div>
        {r.duplicate && <Badge className="mt-0.5">Already imported</Badge>}
        {r.alreadyRecorded && (
          <Badge tone="accent" className="mt-0.5">
            <Receipt className="size-3" /> {r.alreadyRecorded}
          </Badge>
        )}
      </td>
      <td className={clsx('px-2 py-1.5 text-right font-medium whitespace-nowrap tabular', !r.debit && 'text-success')}>
        {r.debit ? '' : '+'}
        {rupees(r.amount, true)}
      </td>
      <td className="px-2 py-1.5">
        <Select value={r.kind} onChange={(e) => onChange({ kind: e.target.value as TxnKind, categoryId: null })} className="h-7 w-28 text-[12px]">
          <option value="expense">{r.debit ? 'Expense' : 'Refund'}</option>
          <option value="income">Income</option>
          <option value="transfer">Transfer</option>
        </Select>
      </td>
      <td className="px-2 py-1.5">
        {r.kind === 'transfer' ? (
          <span className="text-[12px] text-subtle">Not counted in budgets</span>
        ) : (
          <div className="flex items-center gap-1.5">
            <Select value={r.categoryId ?? ''} onChange={(e) => onChange({ categoryId: e.target.value || null })} className={clsx('h-7 w-44 text-[12px]', !r.categoryId && 'border-warning/50')}>
              <option value="">Uncategorized</option>
              {options.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </Select>
            {r.matched === 'rule' && <Badge tone="accent">Learned</Badge>}
            {r.matched === 'builtin' && <Badge>Auto</Badge>}
          </div>
        )}
      </td>
    </tr>
  )
}

function Mapper({ sample, mapping, onChange, onApply }: { sample: string[][]; mapping: ImportMapping; onChange: (m: ImportMapping) => void; onApply: () => void }) {
  const header = sample[mapping.headerRow] ?? sample[0] ?? []
  const cols = Math.max(...sample.map((r) => r.length))
  const label = (i: number) => (header[i] ? `${String.fromCharCode(65 + i)} · ${header[i]}` : `Column ${String.fromCharCode(65 + i)}`)
  return (
    <div className="space-y-3 rounded-md border border-border p-3">
      <p className="text-[12px] text-muted">Tell us which column is which. This layout is remembered for the account.</p>
      <div className="grid gap-2 sm:grid-cols-4">
        <Field label="Header row">
          <Select value={mapping.headerRow} onChange={(e) => onChange({ ...mapping, headerRow: Number(e.target.value) })}>
            {sample.map((r, i) => (
              <option key={i} value={i}>
                Row {i + 1}: {r.slice(0, 3).join(' · ').slice(0, 40)}
              </option>
            ))}
          </Select>
        </Field>
        {FIELDS.map((f) => (
          <Field key={f.key} label={f.label}>
            <Select value={mapping[f.key]} onChange={(e) => onChange({ ...mapping, [f.key]: Number(e.target.value) })}>
              <option value={-1}>—</option>
              {Array.from({ length: cols }, (_, i) => (
                <option key={i} value={i}>
                  {label(i)}
                </option>
              ))}
            </Select>
          </Field>
        ))}
      </div>
      <Button size="sm" onClick={onApply}>
        Apply columns
      </Button>
    </div>
  )
}

// ---- invoices ----

interface InvoicePreview {
  receipt: string
  invoice: null | {
    merchant: string
    orderId: string
    date: string
    total: number
    itemTotal: number
    items: { name: string; qty: number; amount: number; fee: boolean; packQty: number | null; packUnit: string }[]
    warnings: string[] | null
  }
  duplicateOf?: string | null
  categoryId?: string
  pantry?: { name: string; quantity: number | null; unit: string; aisle: string; include: boolean }[] | null
}

function InvoiceImport() {
  const { data: categories = [] } = useMoneyCategories()
  const { data: accounts = [] } = useAccounts()
  const { user, members } = useSession()
  const navigate = useNavigate()
  const [busy, setBusy] = useState(false)
  const [p, setP] = useState<InvoicePreview | null>(null)
  const [f, setF] = useState({ total: '', date: '', payee: '', categoryId: '', paidBy: user.id, accountId: '', note: '' })
  const [pantry, setPantry] = useState<NonNullable<InvoicePreview['pantry']>>([])

  const run = async (file: File) => {
    setBusy(true)
    try {
      const form = new FormData()
      form.append('file', file)
      const r = await upload<InvoicePreview>('/money/invoice/preview', form)
      setP(r)
      const inv = r.invoice
      setF({
        total: inv ? String(inv.total / 100) : '', date: inv?.date ?? new Date().toISOString().slice(0, 10), payee: inv?.merchant ?? '',
        categoryId: r.categoryId ?? '', paidBy: user.id, accountId: '', note: inv?.orderId ? `Order ${inv.orderId}` : '',
      })
      setPantry(r.pantry ?? [])
      inv?.warnings?.forEach((w) => toast.warning(w))
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Could not read the invoice')
    } finally {
      setBusy(false)
    }
  }
  const commit = useAction(
    () =>
      post<{ pantryAdded: number }>('/money/invoice/commit', {
        amount: parseRupees(f.total) ?? 0, date: f.date, payee: f.payee, categoryId: f.categoryId || null, paidBy: f.paidBy, accountId: f.accountId || null,
        note: f.note, receipt: p?.receipt, orderId: p?.invoice?.orderId ?? '', pantry: pantry.filter((x) => x.include),
      }),
    [['money'], ['pantry'], ['suggest']],
    (r) => `Expense saved${r.pantryAdded ? ` · ${r.pantryAdded} items added to the pantry` : ''}`,
  )
  const inv = p?.invoice
  const goods = inv?.items.filter((i) => !i.fee) ?? []
  const fees = inv?.items.filter((i) => i.fee) ?? []
  const feeTotal = fees.reduce((s, i) => s + i.amount, 0)

  return (
    <Card>
      <CardHeader title="Record a grocery order" subtitle="Zepto, Blinkit and other GST invoices (PDF) are read automatically. A photo of a paper bill is attached as the receipt." />
      <div className="space-y-4 p-4">
        {!p && <DropZone accept="application/pdf,image/*" onFile={run} busy={busy} hint="Invoice PDF from the app's order page, or a photo of the bill" />}
        {p && (
          <>
            {p.duplicateOf && (
              <div className="flex items-center gap-2 rounded-md bg-warning-soft px-3 py-2 text-[13px] text-warning">
                <AlertTriangle className="size-4" /> This order is already recorded.
              </div>
            )}
            <div className="grid gap-4 lg:grid-cols-[1fr_320px]">
              <div className="space-y-3">
                {inv ? (
                  <div className="overflow-hidden rounded-md border border-border">
                    <div className="flex items-center justify-between border-b border-border bg-surface-2/60 px-3 py-2 text-[12px]">
                      <span className="font-medium">
                        {inv.merchant || 'Invoice'} {inv.orderId && <span className="text-subtle">· {inv.orderId}</span>}
                      </span>
                      <a href={fileUrl(p.receipt)} target="_blank" rel="noreferrer" className="text-accent hover:underline">
                        View PDF
                      </a>
                    </div>
                    <ul className="divide-y divide-border text-[13px]">
                      {goods.map((it, i) => (
                        <li key={i} className="flex items-baseline gap-3 px-3 py-1.5">
                          <span className="min-w-0 flex-1 truncate">{it.name}</span>
                          <span className="text-[12px] text-subtle tabular">
                            {it.qty !== 1 && `${it.qty} × `}
                            {it.packQty ? formatQty(it.packQty, it.packUnit) : ''}
                          </span>
                          <span className="w-20 text-right tabular">{rupees(it.amount, true)}</span>
                        </li>
                      ))}
                      {fees.length > 0 && (
                        <li className="flex items-baseline gap-3 px-3 py-1.5 text-muted">
                          <span className="min-w-0 flex-1 truncate">Fees ({[...new Set(fees.map((x) => x.name))].join(', ')})</span>
                          <span className="w-20 text-right tabular">{rupees(feeTotal, true)}</span>
                        </li>
                      )}
                      <li className="flex items-baseline justify-between bg-surface-2/60 px-3 py-2 font-semibold">
                        <span>Total</span>
                        <span className="tabular">{rupees(inv.total, true)}</span>
                      </li>
                    </ul>
                  </div>
                ) : (
                  <img src={fileUrl(p.receipt, true)} alt="Bill" className="max-h-72 rounded-md border border-border object-contain" />
                )}
                {pantry.length > 0 && (
                  <div className="rounded-md border border-border">
                    <div className="border-b border-border px-3 py-2 text-[12px] font-medium">Add to pantry</div>
                    <ul className="divide-y divide-border">
                      {pantry.map((it, i) => (
                        <li key={i} className="flex items-center gap-2 px-3 py-1.5 text-[13px]">
                          <input type="checkbox" checked={it.include} onChange={(e) => setPantry(pantry.map((x, j) => (j === i ? { ...x, include: e.target.checked } : x)))} className="accent-[var(--accent)]" />
                          <span className="min-w-0 flex-1 truncate">{it.name}</span>
                          <span className="text-[12px] text-subtle">{it.aisle}</span>
                          <span className="w-16 text-right text-[12px] tabular">{formatQty(it.quantity, it.unit)}</span>
                        </li>
                      ))}
                    </ul>
                  </div>
                )}
              </div>
              <div className="space-y-3">
                <Field label="Total paid (₹)">
                  <Input inputMode="decimal" value={f.total} onChange={(e) => setF({ ...f, total: e.target.value })} className="text-lg font-semibold" />
                </Field>
                <div className="grid grid-cols-2 gap-2">
                  <Field label="Date">
                    <Input type="date" value={f.date} onChange={(e) => setF({ ...f, date: e.target.value })} />
                  </Field>
                  <Field label="Store">
                    <Input value={f.payee} onChange={(e) => setF({ ...f, payee: e.target.value })} />
                  </Field>
                </div>
                <Field label="Category">
                  <Select value={f.categoryId} onChange={(e) => setF({ ...f, categoryId: e.target.value })}>
                    <option value="">Uncategorized</option>
                    {categories.filter((c) => c.kind === 'expense' && !c.archived && !c.ownerId).map((c) => (
                      <option key={c.id} value={c.id}>
                        {c.name}
                      </option>
                    ))}
                  </Select>
                </Field>
                {members.length > 1 && (
                  <Field label="Paid by">
                    <div className="flex gap-1">
                      {members.map((m) => (
                        <button key={m.id} type="button" onClick={() => setF({ ...f, paidBy: m.id })} className={clsx('flex h-8 flex-1 items-center justify-center gap-1.5 rounded-md border text-[12px]', f.paidBy === m.id ? 'border-accent bg-accent-soft' : 'border-border text-muted')}>
                          <Avatar name={m.displayName} color={m.color} size={16} /> {m.displayName.split(' ')[0]}
                        </button>
                      ))}
                    </div>
                  </Field>
                )}
                {accounts.length > 0 && (
                  <Field label="Paid from">
                    <Select value={f.accountId} onChange={(e) => setF({ ...f, accountId: e.target.value })}>
                      <option value="">—</option>
                      {accounts.map((a) => (
                        <option key={a.id} value={a.id}>
                          {a.name}
                        </option>
                      ))}
                    </Select>
                  </Field>
                )}
                <p className="text-[12px] text-subtle">When you later import the bank statement, the matching UPI payment is spotted and skipped automatically.</p>
                <div className="flex gap-2">
                  <Button variant="ghost" onClick={() => setP(null)}>
                    Cancel
                  </Button>
                  <Button variant="primary" className="flex-1" disabled={!parseRupees(f.total) || !!p.duplicateOf} loading={commit.isPending} onClick={() => commit.mutate(undefined, { onSuccess: () => navigate('/budget/transactions') })}>
                    Save expense
                  </Button>
                </div>
              </div>
            </div>
          </>
        )}
      </div>
    </Card>
  )
}

function ImportHistory() {
  const { data: imports = [] } = useImports()
  const { data: accounts = [] } = useAccounts()
  const { memberById } = useSession()
  const undo = useAction((id: string) => del(`/money/imports/${id}`), [['money']], 'Import undone — its transactions were removed')
  if (imports.length === 0) return null
  return (
    <Card className="mt-4">
      <CardHeader title="Recent imports" />
      <ul className="divide-y divide-border">
        {imports.map((im) => (
          <li key={im.id} className="flex items-center gap-3 px-4 py-2 text-[13px]">
            <FileText className="size-4 text-subtle" />
            <span className="min-w-0 flex-1">
              <span className="block truncate font-medium">{im.filename}</span>
              <span className="text-[12px] text-subtle">
                {accounts.find((a) => a.id === im.accountId)?.name ?? 'No account'} · {im.imported} of {im.rowCount} rows · {formatDateTime(im.createdAt)}
                {im.createdBy && ` · ${memberById.get(im.createdBy)?.displayName ?? ''}`}
              </span>
            </span>
            <Button size="sm" variant="ghost" icon={<Undo2 className="size-3.5" />} onClick={() => confirm(`Remove the ${im.imported} transactions from ${im.filename}?`) && undo.mutate(im.id)}>
              Undo
            </Button>
          </li>
        ))}
      </ul>
    </Card>
  )
}

