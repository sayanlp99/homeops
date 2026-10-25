import clsx from 'clsx'
import { addDays, format } from 'date-fns'
import { Check, ListChecks, MoreHorizontal, Plus, RefreshCw, ShoppingBag, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Modal } from '@/components/Dialog'
import { Menu, MenuItem, MenuLabel, MenuSeparator } from '@/components/Menu'
import { Avatar, Badge, Button, Card, EmptyState, Field, IconButton, Input, PageHeader, Spinner, Switch } from '@/components/ui'
import { api, patch, post } from '@/lib/api'
import { formatINR } from '@/lib/format'
import { formatQty, useAction, useGrocery, useGroceryActions, useIngredients } from '@/lib/meals'
import { useSession } from '@/lib/session'
import type { GroceryItem } from '@/lib/types'
import { weekStart } from '@/lib/weeks'
import { MealsNav } from '../meals/MealBits'

const AISLE_ORDER = ['Vegetables', 'Fruits & Nuts', 'Dairy & Eggs', 'Meat & Fish', 'Grains & Flour', 'Pulses & Lentils', 'Spices & Masala', 'Oils & Ghee', 'Bakery', 'Snacks & Packaged', 'Beverages', 'Frozen', 'Household', 'Other']

export function GroceryPage() {
  const { data: items = [], isLoading } = useGrocery()
  const { add } = useGroceryActions()
  const { household } = useSession()
  const [text, setText] = useState('')
  const [finishing, setFinishing] = useState(false)
  const [generating, setGenerating] = useState(false)
  const [clearing, setClearing] = useState(false)

  const { open, checked, byAisle } = useMemo(() => {
    const open = items.filter((i) => !i.checkedAt)
    const groups = new Map<string, GroceryItem[]>()
    for (const it of open) groups.set(it.aisle, [...(groups.get(it.aisle) ?? []), it])
    const byAisle = [...groups.entries()].sort((a, b) => AISLE_ORDER.indexOf(a[0]) - AISLE_ORDER.indexOf(b[0]))
    return { open, checked: items.filter((i) => i.checkedAt).sort((a, b) => (b.checkedAt ?? '').localeCompare(a.checkedAt ?? '')), byAisle }
  }, [items])

  const submit = () => {
    const t = text.trim()
    if (!t) return
    add(t)
    setText('')
  }

  return (
    <div className="mx-auto max-w-3xl">
      <PageHeader
        title="Kitchen"
        subtitle={items.length ? `${open.length} to buy · ${checked.length} in cart` : 'Your shopping list'}
        actions={
          <>
            {items.length > 0 && (
              <Button variant="ghost" icon={<Trash2 className="size-4" />} onClick={() => setClearing(true)}>
                Delete all
              </Button>
            )}
            <Button icon={<RefreshCw className="size-4" />} onClick={() => setGenerating(true)}>
              From meal plan
            </Button>
            <Button variant="primary" icon={<ShoppingBag className="size-4" />} disabled={!checked.length} onClick={() => setFinishing(true)}>
              Finish shopping
            </Button>
          </>
        }
      />
      <MealsNav />

      <form
        className="mb-4 flex gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <Input value={text} onChange={(e) => setText(e.target.value)} placeholder="Add item, e.g. 2 kg onions" enterKeyHint="done" className="h-11 lg:h-9" />
        <Button type="submit" variant="primary" className="h-11 lg:h-9" icon={<Plus className="size-4" />}>
          Add
        </Button>
      </form>

      {isLoading ? (
        <Spinner className="mx-auto mt-10" />
      ) : items.length === 0 ? (
        <Card>
          <EmptyState
            icon={<ListChecks className="size-5" />}
            title="The list is empty"
            description="Plan the week's meals, then build the list in one tap — quantities are added up and what's already in the pantry is skipped."
            action={<Button onClick={() => setGenerating(true)}>Build from meal plan</Button>}
          />
        </Card>
      ) : (
        <div className="space-y-3">
          {byAisle.map(([aisle, list]) => (
            <Card key={aisle} className="overflow-hidden">
              <div className="flex items-center justify-between border-b border-border bg-surface-2/60 px-3 py-1.5">
                <span className="text-[11px] font-semibold tracking-wide text-muted uppercase">{aisle}</span>
                <span className="text-[11px] text-subtle tabular">{list.length}</span>
              </div>
              {list.map((it) => (
                <Row key={it.id} item={it} />
              ))}
            </Card>
          ))}
          {open.length === 0 && <p className="py-4 text-center text-[13px] text-success">Everything's in the cart 🎉</p>}
          {checked.length > 0 && (
            <Card className="overflow-hidden opacity-90">
              <div className="flex items-center justify-between border-b border-border bg-success-soft px-3 py-1.5">
                <span className="text-[11px] font-semibold tracking-wide text-success uppercase">In the cart</span>
                <span className="text-[11px] text-success tabular">{checked.length}</span>
              </div>
              {checked.map((it) => (
                <Row key={it.id} item={it} />
              ))}
            </Card>
          )}
        </div>
      )}
      <FinishDialog open={finishing} onOpenChange={setFinishing} count={checked.length} />
      <ClearDialog open={clearing} onOpenChange={setClearing} count={items.length} />
      <GenerateDialog open={generating} onOpenChange={setGenerating} sprintStartDow={household.sprintStartDow} />
    </div>
  )
}

function Row({ item }: { item: GroceryItem }) {
  const { update, remove } = useGroceryActions()
  const { memberById } = useSession()
  const { data: catalog } = useIngredients()
  const moveAisle = useAction((aisle: string) => patch(`/ingredients/${item.ingredientId}`, { aisle }), [['grocery'], ['ingredients'], ['recipes']])
  const done = !!item.checkedAt
  const who = item.checkedBy ? memberById.get(item.checkedBy) : undefined
  return (
    <div className="flex min-h-12 items-center gap-3 border-b border-border px-3 last:border-b-0">
      <button
        onClick={() => update(item.id, { checked: !done })}
        aria-label={done ? `Uncheck ${item.name}` : `Check ${item.name}`}
        className={clsx('flex size-6 shrink-0 items-center justify-center rounded-md border-2 transition-colors', done ? 'border-success bg-success text-white' : 'border-border-strong hover:border-accent')}
      >
        {done && <Check className="size-4" strokeWidth={3} />}
      </button>
      <button className="min-w-0 flex-1 py-2 text-left" onClick={() => update(item.id, { checked: !done })}>
        <span className={clsx('block truncate text-[14px] lg:text-[13px]', done && 'text-subtle line-through')}>
          {item.name}
          {(item.quantity != null || item.unit) && <span className="ml-1.5 font-semibold text-muted tabular">{formatQty(item.quantity, item.unit)}</span>}
        </span>
        {item.note && <span className="block truncate text-[12px] text-subtle">{item.note}</span>}
      </button>
      {item.source === 'plan' && !done && <Badge tone="accent">Plan</Badge>}
      {item.source === 'pantry' && !done && <Badge tone="warning">Restock</Badge>}
      {done && who && <Avatar name={who.displayName} color={who.color} size={18} />}
      <IconButton label={`Delete ${item.name}`} className="text-subtle hover:text-danger" onClick={() => remove(item.id)}>
        <Trash2 className="size-4" />
      </IconButton>
      <Menu
        align="end"
        trigger={
          <button className="rounded p-1.5 text-subtle hover:bg-surface-2 hover:text-text" aria-label="Item options">
            <MoreHorizontal className="size-4" />
          </button>
        }
      >
        {item.ingredientId && (
          <>
            <MenuLabel>Move to aisle</MenuLabel>
            {(catalog?.aisles ?? AISLE_ORDER).map((a) => (
              <MenuItem key={a} checked={a === item.aisle} onSelect={() => moveAisle.mutate(a)}>
                {a}
              </MenuItem>
            ))}
            <MenuSeparator />
          </>
        )}
        <MenuItem danger icon={<Trash2 className="size-4" />} onSelect={() => remove(item.id)}>
          Delete
        </MenuItem>
      </Menu>
    </div>
  )
}

function ClearDialog({ open, onOpenChange, count }: { open: boolean; onOpenChange: (o: boolean) => void; count: number }) {
  const clear = useAction(
    () => api<{ deleted: number }>('DELETE', '/grocery'),
    [['grocery']],
    (r) => `Deleted ${r.deleted} ${r.deleted === 1 ? 'item' : 'items'}`,
  )
  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title="Delete the whole list?"
      footer={
        <>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button variant="danger" loading={clear.isPending} onClick={() => clear.mutate(undefined, { onSuccess: () => onOpenChange(false) })}>
            Delete all
          </Button>
        </>
      }
    >
      <p className="text-[13px] text-muted">
        All {count} {count === 1 ? 'item' : 'items'} will be removed, including what's in the cart. The pantry and past shopping trips stay as they are.
      </p>
    </Modal>
  )
}

function FinishDialog({ open, onOpenChange, count }: { open: boolean; onOpenChange: (o: boolean) => void; count: number }) {
  const [total, setTotal] = useState('')
  const [store, setStore] = useState('')
  const [toPantry, setToPantry] = useState(true)
  const finish = useAction(
    (v: { total: number | null; store: string; addToPantry: boolean }) => post<{ items: number }>('/grocery/complete', v),
    [['grocery'], ['pantry'], ['suggest']],
    (r) => `Trip saved · ${r.items} items${toPantry ? ' added to the pantry' : ''}`,
  )
  const amount = Number(total.replace(/[^0-9.]/g, ''))
  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title="Finish shopping"
      description={`${count} ${count === 1 ? 'item' : 'items'} in the cart`}
      footer={
        <>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            variant="primary"
            loading={finish.isPending}
            onClick={() =>
              finish.mutate({ total: total ? amount : null, store, addToPantry: toPantry }, { onSuccess: () => (onOpenChange(false), setTotal(''), setStore('')) })
            }
          >
            Save trip
          </Button>
        </>
      }
    >
      <div className="grid grid-cols-2 gap-3">
        <Field label="Bill total (optional)" hint={total && amount ? formatINR(amount) : 'Will feed the budget in Phase 3'}>
          <Input inputMode="decimal" placeholder="₹" value={total} onChange={(e) => setTotal(e.target.value)} />
        </Field>
        <Field label="Store (optional)">
          <Input placeholder="e.g. DMart, BigBasket" value={store} onChange={(e) => setStore(e.target.value)} />
        </Field>
      </div>
      <label className="mt-4 flex items-center justify-between gap-3">
        <span>
          <span className="block text-[13px] font-medium">Add to pantry</span>
          <span className="block text-[12px] text-subtle">Stock levels go up, so next week's list only asks for what's missing.</span>
        </span>
        <Switch checked={toPantry} onChange={setToPantry} label="Add to pantry" />
      </label>
    </Modal>
  )
}

function GenerateDialog({ open, onOpenChange, sprintStartDow }: { open: boolean; onOpenChange: (o: boolean) => void; sprintStartDow: number }) {
  const today = new Date()
  const ws = weekStart(today, sprintStartDow)
  const presets = [
    { label: 'Rest of this week', from: today, to: addDays(ws, 6) },
    { label: 'Next 7 days', from: today, to: addDays(today, 6) },
    { label: 'Next week', from: addDays(ws, 7), to: addDays(ws, 13) },
  ]
  const [preset, setPreset] = useState(0)
  const [subtract, setSubtract] = useState(true)
  const gen = useAction(
    (v: { from: string; to: string; subtractPantry: boolean }) => post<{ added: number; covered: number }>('/grocery/generate', v),
    [['grocery']],
    (r) => `${r.added} items on the list${r.covered ? ` · ${r.covered} skipped (in the pantry)` : ''}`,
  )
  const p = presets[preset]!
  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title="Build list from meal plan"
      description="Adds up ingredients of planned, uncooked meals. Items you added by hand or already ticked stay as they are."
      footer={
        <>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            variant="primary"
            loading={gen.isPending}
            onClick={() => gen.mutate({ from: format(p.from, 'yyyy-MM-dd'), to: format(p.to, 'yyyy-MM-dd'), subtractPantry: subtract }, { onSuccess: () => onOpenChange(false) })}
          >
            Build list
          </Button>
        </>
      }
    >
      <div className="space-y-2">
        {presets.map((x, i) => (
          <label key={x.label} className={clsx('flex cursor-pointer items-center justify-between rounded-md border px-3 py-2 text-[13px]', preset === i ? 'border-accent bg-accent-soft' : 'border-border')}>
            <span className="flex items-center gap-2">
              <input type="radio" checked={preset === i} onChange={() => setPreset(i)} className="accent-[var(--accent)]" />
              {x.label}
            </span>
            <span className="text-[12px] text-subtle">
              {format(x.from, 'd MMM')} – {format(x.to, 'd MMM')}
            </span>
          </label>
        ))}
      </div>
      <label className="mt-4 flex items-center justify-between gap-3">
        <span className="text-[13px]">Skip what's already in the pantry</span>
        <Switch checked={subtract} onChange={setSubtract} label="Subtract pantry" />
      </label>
    </Modal>
  )
}
