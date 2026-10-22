import clsx from 'clsx'
import { Clock, Heart, Link2, Plus, Search } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import { Modal } from '@/components/Dialog'
import { Button, Card, EmptyState, Field, Input, PageHeader, Segmented, Spinner } from '@/components/ui'
import { post } from '@/lib/api'
import { relativeDay } from '@/lib/format'
import { SLOTS, categoryLabel, useRecipes } from '@/lib/meals'
import type { Recipe, Slot } from '@/lib/types'
import { DietMark, MealsNav, RecipeImage, minutesLabel } from '../meals/MealBits'

export const importDraft = { current: null as Recipe | null }

export function RecipesPage() {
  const { data: recipes = [], isLoading } = useRecipes()
  const [params, setParams] = useSearchParams()
  const [q, setQ] = useState('')
  const [importing, setImporting] = useState(false)
  const slot = (params.get('slot') as Slot | 'all' | 'fav') || 'all'
  const setSlot = (v: string) =>
    setParams((p) => {
      if (v === 'all') p.delete('slot')
      else p.set('slot', v)
      return p
    })

  const list = useMemo(() => {
    const term = q.trim().toLowerCase()
    return recipes
      .filter((r) => (slot === 'all' ? true : slot === 'fav' ? r.favorite : r.mealTypes.includes(slot)))
      .filter((r) => !term || r.title.toLowerCase().includes(term) || r.cuisine.toLowerCase().includes(term) || r.tags.some((t) => t.includes(term)) || r.ingredients.some((i) => i.name.toLowerCase().includes(term)))
  }, [recipes, slot, q])

  return (
    <div>
      <PageHeader
        title="Kitchen"
        subtitle={`${recipes.length} recipes in your library`}
        actions={
          <>
            <Button icon={<Link2 className="size-4" />} onClick={() => setImporting(true)}>
              Import from link
            </Button>
            <Link to="/recipes/new">
              <Button variant="primary" icon={<Plus className="size-4" />}>
                New recipe
              </Button>
            </Link>
          </>
        }
      />
      <MealsNav />
      <div className="mb-4 flex flex-wrap items-center gap-2">
        <Segmented
          value={slot}
          onChange={setSlot}
          options={[{ value: 'all', label: 'All' }, ...SLOTS.map((s) => ({ value: s.value, label: s.label })), { value: 'fav', label: '♥ Favourites' }]}
          className="max-w-full overflow-x-auto"
        />
        <label className="flex h-8 min-w-48 flex-1 items-center gap-2 rounded-md border border-border bg-surface px-2.5 sm:max-w-72">
          <Search className="size-3.5 text-subtle" />
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Search name, ingredient, cuisine…" className="min-w-0 flex-1 bg-transparent outline-none placeholder:text-subtle" />
        </label>
      </div>

      {isLoading ? (
        <Spinner className="mx-auto mt-10" />
      ) : recipes.length === 0 ? (
        <Card>
          <EmptyState
            title="Your recipe library is empty"
            description="Paste a link from any recipe site (Hebbar's Kitchen, Archana's Kitchen, NDTV Food…) or type in your family recipes."
            action={
              <div className="flex gap-2">
                <Button onClick={() => setImporting(true)}>Import from link</Button>
                <Link to="/recipes/new">
                  <Button variant="primary">New recipe</Button>
                </Link>
              </div>
            }
          />
        </Card>
      ) : list.length === 0 ? (
        <p className="py-10 text-center text-[13px] text-subtle">No recipes match.</p>
      ) : (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
          {list.map((r) => (
            <RecipeCard key={r.id} r={r} />
          ))}
        </div>
      )}
      <ImportDialog open={importing} onOpenChange={setImporting} />
    </div>
  )
}

function RecipeCard({ r }: { r: Recipe }) {
  const time = minutesLabel(r.prepMinutes, r.cookMinutes)
  return (
    <Link to={`/recipes/${r.id}`} className="group overflow-hidden rounded-lg border border-border bg-surface shadow-card transition-shadow hover:shadow-pop">
      <div className="relative">
        <RecipeImage photo={r.photo} title={r.title} className="aspect-[4/3] w-full" />
        {r.favorite && <Heart className="absolute top-2 right-2 size-4 fill-danger text-danger drop-shadow" />}
      </div>
      <div className="p-2.5">
        <div className="flex items-start gap-1.5">
          <DietMark diet={r.diet} className="mt-0.5" />
          <span className="line-clamp-2 text-[13px] leading-snug font-medium">{r.title}</span>
        </div>
        <div className="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[11px] text-subtle">
          <span>{categoryLabel(r.category)}</span>
          {time && (
            <span className="flex items-center gap-0.5">
              <Clock className="size-3" /> {time}
            </span>
          )}
          {r.lastCooked && <span>Last: {relativeDay(r.lastCooked)}</span>}
        </div>
      </div>
    </Link>
  )
}

function ImportDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const [url, setUrl] = useState('')
  const [busy, setBusy] = useState(false)
  const navigate = useNavigate()
  const submit = async () => {
    setBusy(true)
    try {
      const draft = await post<Recipe>('/recipes/import', { url })
      importDraft.current = draft
      onOpenChange(false)
      setUrl('')
      navigate('/recipes/new?import=1')
      draft.warnings?.forEach((w) => toast.warning(w))
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Import failed')
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title="Import a recipe"
      description="Works with most recipe sites. You'll review everything before saving."
      footer={
        <>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button variant="primary" loading={busy} disabled={!url.trim()} onClick={submit}>
            Import
          </Button>
        </>
      }
    >
      <form
        onSubmit={(e) => {
          e.preventDefault()
          if (url.trim()) submit()
        }}
      >
        <Field label="Recipe link">
          <Input autoFocus type="url" inputMode="url" placeholder="https://…" value={url} onChange={(e) => setUrl(e.target.value)} />
        </Field>
      </form>
      <p className={clsx('mt-3 text-[12px] text-subtle')}>Tip: on your phone, use the site's Share button → Copy link, then paste here.</p>
    </Modal>
  )
}
