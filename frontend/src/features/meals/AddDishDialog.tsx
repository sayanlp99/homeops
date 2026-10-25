import clsx from 'clsx'
import { Check, Minus, Plus, Search, Sparkles } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { format, parseISO } from 'date-fns'
import { Modal } from '@/components/Dialog'
import { Badge, Button, Spinner } from '@/components/ui'
import { categoryLabel, slotLabel, useMealActions, useRecipes, useSuggestions } from '@/lib/meals'
import type { MealItem, Slot } from '@/lib/types'
import { DietMark, RecipeImage } from './MealBits'

/**
 * Add dishes to one slot. Stays open so a full Indian plate (dal + sabzi + rice) can be
 * built in one go; suggestions re-rank after each pick to complete the meal.
 */
export function AddDishDialog({ target, planned, onClose }: { target: { date: string; slot: Slot } | null; planned: MealItem[]; onClose: () => void }) {
  const { data: recipes = [] } = useRecipes()
  const { data: suggestions, isFetching } = useSuggestions(target?.date ?? null, target?.slot ?? null)
  const { add, remove } = useMealActions()
  const [q, setQ] = useState('')
  const [servings, setServings] = useState(2)
  useEffect(() => {
    setQ('')
  }, [target?.date, target?.slot])

  const inSlot = target ? planned.filter((m) => m.date === target.date && m.slot === target.slot) : []
  const inSlotIds = new Set(inSlot.map((m) => m.recipeId))
  const matches = useMemo(() => {
    const term = q.trim().toLowerCase()
    if (!term) return []
    return recipes
      .filter((r) => r.title.toLowerCase().includes(term) || r.tags.some((t) => t.includes(term)) || r.cuisine.toLowerCase().includes(term))
      .slice(0, 20)
  }, [q, recipes])

  if (!target) return null
  const pick = (recipeId: string | null, title: string) => {
    add({ date: target.date, slot: target.slot, recipeId, title, servings })
    setQ('')
  }

  return (
    <Modal
      open={!!target}
      onOpenChange={(o) => !o && onClose()}
      title={`${slotLabel(target.slot)} · ${format(parseISO(target.date), 'EEEE d MMM')}`}
      description="Pick one or more dishes. Suggestions update as you add."
      width="sm:max-w-xl"
      footer={
        <Button variant="primary" onClick={onClose}>
          Done
        </Button>
      }
    >
      {inSlot.length > 0 && (
        <div className="mb-3 flex flex-wrap gap-1.5">
          {inSlot.map((m) => (
            <span key={m.id} className="inline-flex h-7 items-center gap-1.5 rounded-full border border-accent/30 bg-accent-soft pr-1 pl-2.5 text-[12px] text-accent">
              <Check className="size-3" /> {m.title}
              <button onClick={() => remove(m.id)} className="rounded-full p-0.5 hover:bg-accent/15" aria-label={`Remove ${m.title}`}>
                <Minus className="size-3" />
              </button>
            </span>
          ))}
        </div>
      )}

      <div className="flex items-center gap-2">
        <label className="flex h-10 flex-1 items-center gap-2 rounded-md border border-border bg-surface px-3 focus-within:border-accent lg:h-9">
          <Search className="size-4 text-subtle" />
          <input
            autoFocus
            value={q}
            onChange={(e) => setQ(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && q.trim()) {
                const exact = recipes.find((r) => r.title.toLowerCase() === q.trim().toLowerCase())
                pick(exact?.id ?? matches[0]?.id ?? null, exact?.title ?? matches[0]?.title ?? q.trim())
              }
            }}
            placeholder="Search recipes or type any dish…"
            className="min-w-0 flex-1 bg-transparent outline-none placeholder:text-subtle"
          />
        </label>
        <div className="flex h-10 items-center rounded-md border border-border lg:h-9" title="Servings">
          <button className="px-2 text-muted hover:text-text" onClick={() => setServings(Math.max(1, servings - 1))} aria-label="Fewer servings">
            <Minus className="size-3.5" />
          </button>
          <span className="w-12 text-center text-[12px] tabular">{servings} srv</span>
          <button className="px-2 text-muted hover:text-text" onClick={() => setServings(Math.min(20, servings + 1))} aria-label="More servings">
            <Plus className="size-3.5" />
          </button>
        </div>
      </div>

      {q.trim() ? (
        <ul className="mt-3 divide-y divide-border overflow-hidden rounded-md border border-border">
          {matches.map((r) => (
            <li key={r.id}>
              <button onClick={() => pick(r.id, r.title)} disabled={inSlotIds.has(r.id)} className="flex w-full items-center gap-3 px-3 py-2 text-left hover:bg-surface-2 disabled:opacity-50">
                <RecipeImage photo={r.photo} title={r.title} className="size-9 shrink-0 rounded" />
                <span className="min-w-0 flex-1">
                  <span className="flex items-center gap-1.5 truncate text-[13px] font-medium">
                    <DietMark diet={r.diet} /> {r.title}
                  </span>
                  <span className="text-[12px] text-subtle">{categoryLabel(r.category)}</span>
                </span>
              </button>
            </li>
          ))}
          <li>
            <button onClick={() => pick(null, q.trim())} className="flex w-full items-center gap-2 px-3 py-2.5 text-left text-[13px] text-accent hover:bg-surface-2">
              <Plus className="size-4" /> Add “{q.trim()}” as a one-off dish
            </button>
          </li>
        </ul>
      ) : (
        <div className="mt-4">
          <div className="mb-2 flex items-center gap-1.5 text-[12px] font-medium text-muted">
            <Sparkles className="size-3.5 text-accent" /> Suggested {inSlot.length ? 'to go with it' : 'for this meal'}
            {isFetching && <Spinner className="size-3" />}
          </div>
          {recipes.length === 0 ? (
            <p className="rounded-md border border-dashed border-border px-4 py-6 text-center text-[13px] text-subtle">Add a few recipes and suggestions will appear here. You can also type any dish above.</p>
          ) : suggestions?.length === 0 ? (
            <p className="px-1 py-4 text-[13px] text-subtle">No more recipes fit this slot. Type a dish above.</p>
          ) : (
            <ul className="grid grid-cols-1 gap-2 sm:grid-cols-2">
              {(suggestions ?? []).map((s) => (
                <li key={s.recipeId}>
                  <button
                    onClick={() => pick(s.recipeId, s.title)}
                    className={clsx('flex h-full w-full items-start gap-2.5 rounded-md border border-border p-2 text-left transition-colors hover:border-accent/50 hover:bg-surface-2')}
                  >
                    <RecipeImage photo={s.photo} title={s.title} className="size-11 shrink-0 rounded" />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-[13px] font-medium">{s.title}</span>
                      <span className="mt-1 flex flex-wrap gap-1">
                        {s.reasons.slice(0, 2).map((r) => (
                          <Badge key={r} tone={r.startsWith('Goes with') || r === 'Completes the meal' ? 'accent' : 'neutral'}>
                            {r}
                          </Badge>
                        ))}
                        {s.reasons.length === 0 && <span className="text-[11px] text-subtle">{categoryLabel(s.category)}</span>}
                      </span>
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </Modal>
  )
}
