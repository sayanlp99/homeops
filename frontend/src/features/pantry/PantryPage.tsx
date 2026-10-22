import clsx from 'clsx'
import { differenceInCalendarDays, parseISO } from 'date-fns'
import { Minus, Package, Plus, Search, ShoppingCart, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Modal } from '@/components/Dialog'
import { Badge, Button, Card, EmptyState, Field, IconButton, Input, PageHeader, Segmented, Select, Spinner } from '@/components/ui'
import { del, patch, post } from '@/lib/api'
import { formatShortDate } from '@/lib/format'
import { formatQty, useAction, usePantry } from '@/lib/meals'
import type { PantryItem } from '@/lib/types'
import { MealsNav } from '../meals/MealBits'

type Loc = 'all' | PantryItem['location']
const UNITS = ['', 'g', 'kg', 'ml', 'l', 'pcs', 'packet', 'bunch', 'dozen', 'cup', 'tbsp', 'tsp', 'can', 'bottle']

function status(p: PantryItem): { label: string; tone: 'danger' | 'warning' | 'neutral' } | null {
  if (p.expiresOn) {
    const d = differenceInCalendarDays(parseISO(p.expiresOn), new Date())
    if (d < 0) return { label: 'Expired', tone: 'danger' }
    if (d <= 2) return { label: d === 0 ? 'Expires today' : `Expires in ${d}d`, tone: 'warning' }
  }
  if (p.quantity === 0) return { label: 'Out', tone: 'danger' }
  if (p.quantity != null && p.lowThreshold != null && p.quantity <= p.lowThreshold) return { label: 'Low', tone: 'warning' }
  return null
}

export function PantryPage() {
  const { data: items = [], isLoading } = usePantry()
  const [loc, setLoc] = useState<Loc>('all')
  const [q, setQ] = useState('')
  const [editing, setEditing] = useState<Partial<PantryItem> | null>(null)
  const restock = useAction(() => post<{ added: number }>('/pantry/restock'), [['grocery']], (r) => (r.added ? `${r.added} items added to the grocery list` : 'Nothing is low right now'))

  const list = useMemo(
    () => items.filter((p) => (loc === 'all' || p.location === loc) && (!q || p.name.toLowerCase().includes(q.toLowerCase()))),
    [items, loc, q],
  )
  const attention = items.filter((p) => status(p)).length

  return (
    <div>
      <PageHeader
        title="Kitchen"
        subtitle={`${items.length} items tracked${attention ? ` · ${attention} need attention` : ''}`}
        actions={
          <>
            <Button icon={<ShoppingCart className="size-4" />} loading={restock.isPending} onClick={() => restock.mutate()}>
              Restock low items
            </Button>
            <Button variant="primary" icon={<Plus className="size-4" />} onClick={() => setEditing({ location: 'pantry', unit: '' })}>
              Add item
            </Button>
          </>
        }
      />
      <MealsNav />
      <div className="mb-4 flex flex-wrap items-center gap-2">
        <Segmented<Loc>
          value={loc}
          onChange={setLoc}
          options={[
            { value: 'all', label: 'All' },
            { value: 'pantry', label: 'Pantry' },
            { value: 'fridge', label: 'Fridge' },
            { value: 'freezer', label: 'Freezer' },
          ]}
        />
        <label className="flex h-8 min-w-48 flex-1 items-center gap-2 rounded-md border border-border bg-surface px-2.5 sm:max-w-64">
          <Search className="size-3.5 text-subtle" />
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Search" className="min-w-0 flex-1 bg-transparent outline-none placeholder:text-subtle" />
        </label>
      </div>

      {isLoading ? (
        <Spinner className="mx-auto mt-10" />
      ) : items.length === 0 ? (
        <Card>
          <EmptyState
            icon={<Package className="size-5" />}
            title="Nothing tracked yet"
            description="Finishing a shopping trip stocks the pantry automatically. Cooking a planned meal uses it up. Add staples here by hand too."
            action={<Button onClick={() => setEditing({ location: 'pantry', unit: '' })}>Add item</Button>}
          />
        </Card>
      ) : (
        <Card className="overflow-hidden">
          <div className="hidden grid-cols-[1fr_160px_110px_110px_90px_36px] gap-3 border-b border-border px-4 py-2 text-[11px] font-medium tracking-wide text-subtle uppercase md:grid">
            <span>Item</span>
            <span>Quantity</span>
            <span>Low at</span>
            <span>Expires</span>
            <span>Where</span>
            <span />
          </div>
          {list.map((p) => (
            <PantryRow key={p.id} p={p} onEdit={() => setEditing(p)} />
          ))}
          {list.length === 0 && <p className="px-4 py-6 text-center text-[13px] text-subtle">No items here.</p>}
        </Card>
      )}
      <PantryEditor value={editing} onClose={() => setEditing(null)} />
    </div>
  )
}

function PantryRow({ p, onEdit }: { p: PantryItem; onEdit: () => void }) {
  const update = useAction((v: Partial<PantryItem>) => patch(`/pantry/${p.id}`, v), [['pantry'], ['suggest']])
  const remove = useAction(() => del(`/pantry/${p.id}`), [['pantry']])
  const st = status(p)
  const step = p.unit === 'g' || p.unit === 'ml' ? 100 : p.unit === 'kg' || p.unit === 'l' ? 0.5 : 1
  return (
    <div className="grid grid-cols-[1fr_auto] items-center gap-x-3 gap-y-1 border-b border-border px-4 py-2.5 last:border-b-0 md:grid-cols-[1fr_160px_110px_110px_90px_36px]">
      <button onClick={onEdit} className="flex min-w-0 items-center gap-2 text-left">
        <span className="truncate text-[13px] font-medium">{p.name}</span>
        {st && <Badge tone={st.tone}>{st.label}</Badge>}
      </button>
      <div className="flex items-center gap-1 justify-self-end md:justify-self-start">
        {p.quantity != null ? (
          <>
            <button className="rounded p-1 text-muted hover:bg-surface-2" aria-label="Decrease" onClick={() => update.mutate({ quantity: Math.max(0, Math.round((p.quantity! - step) * 100) / 100) })}>
              <Minus className="size-3.5" />
            </button>
            <span className={clsx('min-w-16 text-center text-[13px] tabular', p.quantity === 0 && 'text-danger')}>{formatQty(p.quantity, p.unit)}</span>
            <button className="rounded p-1 text-muted hover:bg-surface-2" aria-label="Increase" onClick={() => update.mutate({ quantity: Math.round((p.quantity! + step) * 100) / 100 })}>
              <Plus className="size-3.5" />
            </button>
          </>
        ) : (
          <span className="text-[12px] text-subtle">In stock</span>
        )}
      </div>
      <span className="hidden text-[12px] text-muted tabular md:block">{p.lowThreshold != null ? formatQty(p.lowThreshold, p.unit) : '—'}</span>
      <span className="hidden text-[12px] text-muted md:block">{p.expiresOn ? formatShortDate(p.expiresOn) : '—'}</span>
      <span className="hidden text-[12px] text-muted capitalize md:block">{p.location}</span>
      <IconButton label="Remove" className="hidden md:inline-flex" onClick={() => confirm(`Stop tracking ${p.name}?`) && remove.mutate()}>
        <Trash2 className="size-3.5" />
      </IconButton>
    </div>
  )
}

function PantryEditor({ value, onClose }: { value: Partial<PantryItem> | null; onClose: () => void }) {
  const [f, setF] = useState<Partial<PantryItem>>({})
  const [key, setKey] = useState<unknown>(null)
  if (value !== key) {
    setKey(value)
    if (value) setF(value)
  }
  const set = (p: Partial<PantryItem>) => setF((x) => ({ ...x, ...p }))
  const save = useAction(
    (v: Partial<PantryItem>) =>
      post('/pantry', { name: v.name, quantity: v.quantity ?? null, unit: v.unit ?? '', lowThreshold: v.lowThreshold ?? null, location: v.location ?? 'pantry', expiresOn: v.expiresOn || null }),
    [['pantry'], ['suggest']],
    'Pantry updated',
  )
  return (
    <Modal
      open={!!value}
      onOpenChange={(o) => !o && onClose()}
      title={f.id ? f.name : 'Add to pantry'}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={save.isPending} disabled={!f.name?.trim()} onClick={() => save.mutate(f, { onSuccess: onClose })}>
            Save
          </Button>
        </>
      }
    >
      <div className="space-y-3">
        {!f.id && (
          <Field label="Item">
            <Input autoFocus value={f.name ?? ''} onChange={(e) => set({ name: e.target.value })} placeholder="e.g. Basmati rice" />
          </Field>
        )}
        <div className="grid grid-cols-2 gap-3">
          <Field label="Quantity" hint="Leave empty to just mark it in stock">
            <Input type="number" step="any" min={0} inputMode="decimal" value={f.quantity ?? ''} onChange={(e) => set({ quantity: e.target.value === '' ? null : Number(e.target.value) })} />
          </Field>
          <Field label="Unit">
            <Select value={f.unit ?? ''} onChange={(e) => set({ unit: e.target.value })}>
              {[...new Set([...UNITS, f.unit ?? ''])].map((u) => (
                <option key={u} value={u}>
                  {u || '—'}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Low-stock alert at" hint="Shown as Low, and added by Restock">
            <Input type="number" step="any" min={0} inputMode="decimal" value={f.lowThreshold ?? ''} onChange={(e) => set({ lowThreshold: e.target.value === '' ? null : Number(e.target.value) })} />
          </Field>
          <Field label="Expires on">
            <Input type="date" value={f.expiresOn ?? ''} onChange={(e) => set({ expiresOn: e.target.value || null })} />
          </Field>
        </div>
        <Field label="Kept in">
          <Segmented
            value={f.location ?? 'pantry'}
            onChange={(location) => set({ location })}
            options={[
              { value: 'pantry', label: 'Pantry' },
              { value: 'fridge', label: 'Fridge' },
              { value: 'freezer', label: 'Freezer' },
            ]}
          />
        </Field>
      </div>
    </Modal>
  )
}
