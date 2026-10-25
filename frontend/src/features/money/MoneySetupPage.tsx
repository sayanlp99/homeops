import clsx from 'clsx'
import { Archive, ArchiveRestore, Lock, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { Modal } from '@/components/Dialog'
import { Badge, Button, Card, CardHeader, EmptyState, Field, IconButton, Input, PageHeader, Select, Switch } from '@/components/ui'
import { api, del, post } from '@/lib/api'
import { useAction } from '@/lib/meals'
import { ACCOUNT_KINDS, parseRupees, rupees, useAccounts, useMoneyCategories, useRules } from '@/lib/money'
import type { MoneyCategory } from '@/lib/types'
import { CategoryIcon, ICONS, MoneyNav } from './MoneyBits'

const COLORS = ['#16a34a', '#65a30d', '#ea580c', '#0891b2', '#7c3aed', '#2563eb', '#a16207', '#dc2626', '#db2777', '#4f46e5', '#64748b', '#0d9488']

export function MoneySetupPage() {
  return (
    <div>
      <PageHeader title="Money" />
      <MoneyNav />
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-[3fr_2fr]">
        <Categories />
        <div className="space-y-4">
          <Accounts />
          <Rules />
        </div>
      </div>
    </div>
  )
}

function Categories() {
  const { data: categories = [] } = useMoneyCategories()
  const [editing, setEditing] = useState<Partial<MoneyCategory> & { personal?: boolean } | null>(null)
  const archive = useAction(({ id, archived }: { id: string; archived: boolean }) => api('PATCH', `/money/categories/${id}`, { archived }), [['money']])
  const groups = [
    { title: 'Shared', items: categories.filter((c) => !c.ownerId && c.kind === 'expense') },
    { title: 'Personal · only you see these', items: categories.filter((c) => c.ownerId), personal: true },
    { title: 'Income', items: categories.filter((c) => !c.ownerId && c.kind === 'income') },
  ]
  return (
    <Card>
      <CardHeader
        title="Categories"
        subtitle="Shared categories are your household budget. Personal ones are private."
        actions={
          <Button size="sm" icon={<Plus className="size-3.5" />} onClick={() => setEditing({ icon: 'circle', color: COLORS[0], kind: 'expense', monthlyLimit: 0, rollover: false })}>
            New
          </Button>
        }
      />
      {groups.map((g) => (
        <div key={g.title}>
          <div className="flex items-center gap-1.5 border-b border-border bg-surface-2/60 px-4 py-1.5 text-[11px] font-semibold tracking-wide text-muted uppercase">
            {g.personal && <Lock className="size-3" />} {g.title}
          </div>
          {g.items.length === 0 && <p className="px-4 py-3 text-[12px] text-subtle">{g.personal ? 'None yet — create one for things like hobbies or personal shopping.' : 'None'}</p>}
          <ul className="divide-y divide-border">
            {g.items.map((c) => (
              <li key={c.id} className={clsx('flex items-center gap-3 px-4 py-2', c.archived && 'opacity-50')}>
                <CategoryIcon category={c} size={28} />
                <button className="min-w-0 flex-1 text-left" onClick={() => setEditing(c)}>
                  <span className="block truncate text-[13px] font-medium">{c.name}</span>
                  <span className="text-[12px] text-subtle">
                    {c.kind === 'income' ? 'Income' : c.monthlyLimit ? `${rupees(c.monthlyLimit)} / month` : 'No budget'}
                    {c.rollover && ' · rolls over'}
                  </span>
                </button>
                {c.archived && <Badge>Archived</Badge>}
                <IconButton label={c.archived ? 'Restore' : 'Archive'} onClick={() => archive.mutate({ id: c.id, archived: !c.archived })}>
                  {c.archived ? <ArchiveRestore className="size-3.5" /> : <Archive className="size-3.5" />}
                </IconButton>
              </li>
            ))}
          </ul>
        </div>
      ))}
      <CategoryEditor value={editing} onClose={() => setEditing(null)} />
    </Card>
  )
}

function CategoryEditor({ value, onClose }: { value: (Partial<MoneyCategory> & { personal?: boolean }) | null; onClose: () => void }) {
  const [f, setF] = useState<Partial<MoneyCategory> & { personal?: boolean; limitText?: string }>({})
  const [key, setKey] = useState<unknown>(null)
  if (value !== key) {
    setKey(value)
    if (value) setF({ ...value, limitText: value.monthlyLimit ? String(value.monthlyLimit / 100) : '' })
  }
  const save = useAction(
    () => {
      const body = { name: f.name, icon: f.icon, color: f.color, kind: f.kind, monthlyLimit: parseRupees(f.limitText ?? '') ?? 0, rollover: f.rollover, personal: f.personal }
      return f.id ? api('PATCH', `/money/categories/${f.id}`, body) : post('/money/categories', body)
    },
    [['money']],
    'Category saved',
  )
  return (
    <Modal
      open={!!value}
      onOpenChange={(o) => !o && onClose()}
      title={f.id ? 'Edit category' : 'New category'}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={save.isPending} disabled={!f.name?.trim()} onClick={() => save.mutate(undefined, { onSuccess: onClose })}>
            Save
          </Button>
        </>
      }
    >
      <div className="space-y-3">
        <div className="flex items-end gap-3">
          <CategoryIcon category={{ icon: f.icon ?? 'circle', color: f.color ?? COLORS[0]! }} size={40} />
          <Field label="Name" className="flex-1">
            <Input autoFocus value={f.name ?? ''} onChange={(e) => setF({ ...f, name: e.target.value })} maxLength={40} />
          </Field>
        </div>
        <Field label="Icon">
          <div className="flex flex-wrap gap-1">
            {Object.entries(ICONS).map(([name, Icon]) => (
              <button key={name} type="button" onClick={() => setF({ ...f, icon: name })} aria-label={name} className={clsx('flex size-8 items-center justify-center rounded-md border', f.icon === name ? 'border-accent bg-accent-soft text-accent' : 'border-border text-muted hover:bg-surface-2')}>
                <Icon className="size-4" />
              </button>
            ))}
          </div>
        </Field>
        <Field label="Colour">
          <div className="flex flex-wrap gap-1.5">
            {COLORS.map((c) => (
              <button key={c} type="button" onClick={() => setF({ ...f, color: c })} aria-label={c} className={clsx('size-7 rounded-full ring-offset-2 ring-offset-surface', f.color === c && 'ring-2 ring-text')} style={{ background: c }} />
            ))}
          </div>
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Type">
            <Select value={f.kind ?? 'expense'} onChange={(e) => setF({ ...f, kind: e.target.value as 'expense' | 'income' })} disabled={!!f.id}>
              <option value="expense">Expense</option>
              <option value="income">Income</option>
            </Select>
          </Field>
          {f.kind !== 'income' && (
            <Field label="Monthly budget (₹)">
              <Input inputMode="decimal" value={f.limitText ?? ''} onChange={(e) => setF({ ...f, limitText: e.target.value })} placeholder="0" />
            </Field>
          )}
        </div>
        {f.kind !== 'income' && (
          <label className="flex items-center justify-between">
            <span>
              <span className="block text-[13px]">Roll over unspent budget</span>
              <span className="block text-[12px] text-subtle">Good for irregular costs: gifts, travel, repairs.</span>
            </span>
            <Switch checked={!!f.rollover} onChange={(rollover) => setF({ ...f, rollover })} label="Rollover" />
          </label>
        )}
        {!f.id && (
          <label className="flex items-center justify-between">
            <span className="text-[13px]">Personal — only visible to you</span>
            <Switch checked={!!f.personal} onChange={(personal) => setF({ ...f, personal })} label="Personal" />
          </label>
        )}
      </div>
    </Modal>
  )
}

function Accounts() {
  const { data: accounts = [] } = useAccounts()
  const [name, setName] = useState('')
  const [kind, setKind] = useState('bank')
  const [personal, setPersonal] = useState(false)
  const add = useAction(() => post('/money/accounts', { name, kind, personal }), [['money', 'accounts']], 'Account added')
  const archive = useAction(({ id, archived }: { id: string; archived: boolean }) => api('PATCH', `/money/accounts/${id}`, { archived }), [['money', 'accounts']])
  return (
    <Card>
      <CardHeader title="Accounts" subtitle="Bank accounts, cards and wallets you import statements from." />
      <ul className="divide-y divide-border">
        {accounts.map((a) => (
          <li key={a.id} className={clsx('flex items-center gap-2 px-4 py-2 text-[13px]', a.archived && 'opacity-50')}>
            <span className="min-w-0 flex-1 truncate font-medium">{a.name}</span>
            {a.ownerId && <Lock className="size-3 text-subtle" />}
            <span className="text-[12px] text-subtle">{ACCOUNT_KINDS.find((k) => k.value === a.kind)?.label}</span>
            <IconButton label={a.archived ? 'Restore' : 'Archive'} onClick={() => archive.mutate({ id: a.id, archived: !a.archived })}>
              {a.archived ? <ArchiveRestore className="size-3.5" /> : <Archive className="size-3.5" />}
            </IconButton>
          </li>
        ))}
      </ul>
      <form
        className="grid grid-cols-[1fr_auto] gap-2 border-t border-border p-3"
        onSubmit={(e) => {
          e.preventDefault()
          if (name.trim()) add.mutate(undefined, { onSuccess: () => setName('') })
        }}
      >
        <Input placeholder="e.g. ICICI Amazon Pay card" value={name} onChange={(e) => setName(e.target.value)} />
        <Button type="submit">Add</Button>
        <div className="col-span-2 flex items-center gap-2">
          <Select value={kind} onChange={(e) => setKind(e.target.value)} className="h-8 w-auto">
            {ACCOUNT_KINDS.map((k) => (
              <option key={k.value} value={k.value}>
                {k.label}
              </option>
            ))}
          </Select>
          <label className="ml-auto flex items-center gap-2 text-[12px] text-muted">
            Personal <Switch checked={personal} onChange={setPersonal} label="Personal account" />
          </label>
        </div>
      </form>
    </Card>
  )
}

function Rules() {
  const { data: rules = [] } = useRules()
  const { data: categories = [] } = useMoneyCategories()
  const remove = useAction((id: string) => del(`/money/rules/${id}`), [['money', 'rules']])
  const catName = (id: string) => categories.find((c) => c.id === id)?.name ?? '—'
  return (
    <Card>
      <CardHeader title="Learned rules" subtitle="Created when you re-categorise an imported transaction." />
      {rules.length === 0 ? (
        <EmptyState title="No rules yet" description="Fix a category on an imported transaction and it's remembered for next time." />
      ) : (
        <ul className="divide-y divide-border">
          {rules.map((r) => (
            <li key={r.id} className="flex items-center gap-2 px-4 py-2 text-[13px]">
              <span className="min-w-0 flex-1 truncate">
                “{r.pattern}” → <span className="font-medium">{r.kind === 'transfer' ? 'Transfer' : catName(r.categoryId)}</span>
              </span>
              <IconButton label="Delete rule" onClick={() => remove.mutate(r.id)}>
                <Trash2 className="size-3.5" />
              </IconButton>
            </li>
          ))}
        </ul>
      )}
    </Card>
  )
}
