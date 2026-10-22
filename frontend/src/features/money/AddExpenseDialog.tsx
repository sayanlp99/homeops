import clsx from 'clsx'
import { format, subDays } from 'date-fns'
import { CalendarDays, Camera, Check, Lock, Paperclip, Users, X } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'
import { Modal } from '@/components/Dialog'
import { Avatar, Button, Input, Segmented, Select, Spinner } from '@/components/ui'
import { uploadPhoto, fileUrl } from '@/lib/meals'
import { classify, parseRupees, rupees, useAccounts, useAddTxn, useMoneyCategories } from '@/lib/money'
import { useSession } from '@/lib/session'
import type { TxnKind } from '@/lib/types'
import { quickExpense } from '@/lib/ui'
import { CategoryIcon } from './MoneyBits'

/** Three taps: amount → category → Save. Everything else is optional and defaults sensibly. */
export function AddExpenseDialog() {
  const { open, defaults } = quickExpense.use()
  const { user, members } = useSession()
  const { data: categories = [] } = useMoneyCategories()
  const { data: accounts = [] } = useAccounts()
  const add = useAddTxn()
  const amountRef = useRef<HTMLInputElement>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  const [kind, setKind] = useState<TxnKind>('expense')
  const [amount, setAmount] = useState('')
  const [categoryId, setCategoryId] = useState<string | null>(null)
  const [payee, setPayee] = useState('')
  const [date, setDate] = useState(format(new Date(), 'yyyy-MM-dd'))
  const [paidBy, setPaidBy] = useState(user.id)
  const [personal, setPersonal] = useState(false)
  const [accountId, setAccountId] = useState('')
  const [note, setNote] = useState('')
  const [receipt, setReceipt] = useState('')
  const [uploading, setUploading] = useState(false)
  const [autoCat, setAutoCat] = useState(false)

  useEffect(() => {
    if (!open) return
    setKind('expense')
    setAmount(defaults.amount ? String(defaults.amount / 100) : '')
    setCategoryId(defaults.categoryId ?? null)
    setPayee(defaults.payee ?? '')
    setDate(format(new Date(), 'yyyy-MM-dd'))
    setPaidBy(user.id)
    setPersonal(false)
    setAccountId('')
    setNote('')
    setReceipt('')
    setAutoCat(false)
  }, [open, defaults, user.id])

  const visibleCats = categories.filter((c) => !c.archived && c.kind === (kind === 'income' ? 'income' : 'expense') && (personal ? !!c.ownerId : !c.ownerId))
  const close = () => quickExpense.set({ open: false, defaults: {} })
  const paise = parseRupees(amount)

  const suggest = async () => {
    if (!payee.trim() || (categoryId && !autoCat)) return
    try {
      const r = await classify(payee)
      if (r.categoryId && visibleCats.some((c) => c.id === r.categoryId)) {
        setCategoryId(r.categoryId)
        setAutoCat(true)
      }
    } catch {
      /* offline: skip the suggestion */
    }
  }

  const save = () => {
    if (!paise) return amountRef.current?.focus()
    add.mutate(
      {
        id: crypto.randomUUID(), kind, amount: paise, date, payee: payee.trim(), note: note.trim(), categoryId: kind === 'transfer' ? null : categoryId,
        accountId: accountId || null, paidBy, receipt, personal,
      },
      { onSuccess: () => toast.success(`${kind === 'income' ? 'Income' : kind === 'transfer' ? 'Transfer' : 'Expense'} of ${rupees(paise, true)} saved`) },
    )
    if (!navigator.onLine) toast.message('Saved offline — will sync when you reconnect')
    close()
  }

  const onReceipt = async (f?: File) => {
    if (!f) return
    setUploading(true)
    try {
      setReceipt(await uploadPhoto(f))
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Upload failed')
    } finally {
      setUploading(false)
    }
  }

  const days = [
    { label: 'Today', value: format(new Date(), 'yyyy-MM-dd') },
    { label: 'Yesterday', value: format(subDays(new Date(), 1), 'yyyy-MM-dd') },
  ]

  return (
    <Modal
      open={open}
      onOpenChange={(o) => !o && close()}
      title="Add expense"
      width="sm:max-w-lg"
      footer={
        <div className="flex w-full items-center justify-between gap-2">
          <span className="text-[12px] text-subtle">{paise ? rupees(paise, true) : ''}</span>
          <div className="flex gap-2">
            <Button variant="ghost" onClick={close}>
              Cancel
            </Button>
            <Button variant="primary" onClick={save} disabled={!paise}>
              Save
            </Button>
          </div>
        </div>
      }
    >
      <form
        onSubmit={(e) => {
          e.preventDefault()
          save()
        }}
        className="space-y-4"
      >
        <Segmented<TxnKind>
          value={kind}
          onChange={(k) => {
            setKind(k)
            setCategoryId(null)
          }}
          className="w-full"
          options={[
            { value: 'expense', label: 'Expense' },
            { value: 'income', label: 'Income' },
            { value: 'transfer', label: 'Transfer' },
          ]}
        />
        <label className="flex items-center gap-2 border-b-2 border-border pb-1 focus-within:border-accent">
          <span className="text-2xl font-semibold text-subtle">₹</span>
          <input
            ref={amountRef}
            autoFocus
            inputMode="decimal"
            enterKeyHint="done"
            placeholder="0"
            value={amount}
            onChange={(e) => setAmount(e.target.value.replace(/[^0-9.,]/g, ''))}
            className="min-w-0 flex-1 bg-transparent text-3xl font-semibold tracking-tight outline-none placeholder:text-border-strong"
            aria-label="Amount in rupees"
          />
        </label>

        {kind !== 'transfer' && (
          <div>
            <div className="mb-1.5 flex items-center justify-between text-[12px] text-muted">
              <span>Category</span>
              {autoCat && categoryId && <span className="text-accent">Suggested from payee</span>}
            </div>
            {personal && visibleCats.length === 0 && (
              <p className="rounded-md border border-dashed border-border px-3 py-2 text-[12px] text-subtle">No personal categories yet — add them under Budget → Categories & accounts. You can still save it uncategorized.</p>
            )}
            <div className="grid grid-cols-4 gap-1.5 sm:grid-cols-6">
              {visibleCats.map((c) => (
                <button
                  key={c.id}
                  type="button"
                  onClick={() => {
                    setCategoryId(c.id === categoryId ? null : c.id)
                    setAutoCat(false)
                  }}
                  className={clsx(
                    'relative flex flex-col items-center gap-1 rounded-md border px-1 py-2 text-center text-[11px] leading-tight',
                    c.id === categoryId ? 'border-accent bg-accent-soft text-text' : 'border-border text-muted hover:bg-surface-2',
                  )}
                >
                  <CategoryIcon category={c} size={26} />
                  <span className="line-clamp-2">{c.name}</span>
                  {c.id === categoryId && <Check className="absolute top-1 right-1 size-3 text-accent" />}
                  {c.ownerId && <Lock className="absolute top-1 left-1 size-2.5 text-subtle" />}
                </button>
              ))}
            </div>
          </div>
        )}

        <div className="grid grid-cols-2 gap-2">
          <Input placeholder={kind === 'income' ? 'From (e.g. Salary)' : 'Paid to (e.g. Swiggy)'} value={payee} onChange={(e) => setPayee(e.target.value)} onBlur={suggest} maxLength={80} />
          <div className="flex gap-1">
            {days.map((d) => (
              <button
                key={d.label}
                type="button"
                onClick={() => setDate(d.value)}
                className={clsx('h-10 flex-1 rounded-md border px-2 text-[12px] lg:h-8', date === d.value ? 'border-accent bg-accent-soft text-accent' : 'border-border text-muted')}
              >
                {d.label}
              </button>
            ))}
            <label
              className={clsx('relative flex h-10 w-10 shrink-0 items-center justify-center rounded-md border lg:h-8', days.some((d) => d.value === date) ? 'border-border text-muted' : 'border-accent bg-accent-soft text-accent')}
              title={date}
            >
              <CalendarDays className="size-4" />
              <input type="date" aria-label="Pick another date" value={date} onChange={(e) => setDate(e.target.value || days[0]!.value)} className="absolute inset-0 opacity-0" />
            </label>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {members.length > 1 && (
            <div className="flex items-center gap-1 rounded-md border border-border p-0.5" role="radiogroup" aria-label="Paid by">
              {members.map((m) => (
                <button
                  key={m.id}
                  type="button"
                  role="radio"
                  aria-checked={paidBy === m.id}
                  onClick={() => setPaidBy(m.id)}
                  className={clsx('flex h-8 items-center gap-1.5 rounded px-2 text-[12px] lg:h-7', paidBy === m.id ? 'bg-surface-3 font-medium' : 'text-muted')}
                >
                  <Avatar name={m.displayName} color={m.color} size={16} />
                  {m.id === user.id ? 'I paid' : `${m.displayName.split(' ')[0]} paid`}
                </button>
              ))}
            </div>
          )}
          <button
            type="button"
            onClick={() => {
              setPersonal(!personal)
              setCategoryId(null)
            }}
            className={clsx('flex h-9 items-center gap-1.5 rounded-md border px-2.5 text-[12px] lg:h-8', personal ? 'border-accent bg-accent-soft text-accent' : 'border-border text-muted')}
            title="Personal expenses are only visible to you"
          >
            {personal ? <Lock className="size-3.5" /> : <Users className="size-3.5" />}
            {personal ? 'Personal' : 'Shared'}
          </button>
          {accounts.length > 0 && (
            <Select value={accountId} onChange={(e) => setAccountId(e.target.value)} className="h-9 w-auto lg:h-8" aria-label="Account">
              <option value="">No account</option>
              {accounts.filter((a) => !a.archived).map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </Select>
          )}
          <button
            type="button"
            onClick={() => fileRef.current?.click()}
            className={clsx('flex h-9 items-center gap-1.5 rounded-md border px-2.5 text-[12px] lg:h-8', receipt ? 'border-success/40 bg-success-soft text-success' : 'border-border text-muted')}
          >
            {uploading ? <Spinner className="size-3.5" /> : receipt ? <Paperclip className="size-3.5" /> : <Camera className="size-3.5" />}
            {receipt ? 'Receipt added' : 'Receipt'}
          </button>
          {receipt && (
            <>
              <img src={fileUrl(receipt, true)} alt="Receipt" className="size-9 rounded object-cover" />
              <button type="button" onClick={() => setReceipt('')} aria-label="Remove receipt" className="text-subtle hover:text-danger">
                <X className="size-4" />
              </button>
            </>
          )}
          <input ref={fileRef} type="file" accept="image/*" capture="environment" className="hidden" onChange={(e) => onReceipt(e.target.files?.[0])} />
        </div>
        <Input placeholder="Note (optional)" value={note} onChange={(e) => setNote(e.target.value)} maxLength={300} />
      </form>
    </Modal>
  )
}
