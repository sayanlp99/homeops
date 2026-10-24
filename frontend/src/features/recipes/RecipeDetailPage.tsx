import clsx from 'clsx'
import { format } from 'date-fns'
import { ArrowLeft, BellRing, CalendarPlus, Check, Clock, ExternalLink, Heart, Minus, Pencil, Plus, ShoppingCart, Star, Trash2, Users } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import { Modal } from '@/components/Dialog'
import { Badge, Button, Card, EmptyState, Field, IconButton, Input, Select, Spinner } from '@/components/ui'
import { del, patch } from '@/lib/api'
import { formatDate, relativeDay } from '@/lib/format'
import { SLOTS, categoryLabel, formatQty, useAction, useGroceryActions, useMealActions, usePantry, useRecipe } from '@/lib/meals'
import type { Recipe, Slot } from '@/lib/types'
import { DietMark, RecipeImage, minutesLabel } from '../meals/MealBits'
import { RecipeNutritionCard, seasonLabel } from '../nutrition/NutritionBits'

export function RecipeDetailPage() {
  const { id } = useParams()
  const { recipe, isLoading } = useRecipe(id)
  const [params] = useSearchParams()
  const [servings, setServings] = useState<number | null>(params.get('servings') ? Number(params.get('servings')) : null)
  const [planning, setPlanning] = useState(false)
  const [done, setDone] = useState<Set<number>>(new Set())
  const { data: pantry = [] } = usePantry()
  const grocery = useGroceryActions()
  const navigate = useNavigate()
  const fav = useAction((v: { favorite?: boolean; rating?: number }) => patch(`/recipes/${id}`, v), [['recipes']])
  const remove = useAction(() => del(`/recipes/${id}`), [['recipes'], ['meals']], 'Recipe deleted')

  const inStock = useMemo(() => new Set(pantry.filter((p) => p.quantity == null || p.quantity > 0).map((p) => p.ingredientId)), [pantry])

  if (isLoading) return <Spinner className="mx-auto mt-16" />
  if (!recipe)
    return (
      <Card>
        <EmptyState title="Recipe not found" action={<Link to="/recipes"><Button>Back to recipes</Button></Link>} />
      </Card>
    )

  const srv = servings ?? recipe.servings
  const scale = srv / recipe.servings
  const missing = recipe.ingredients.filter((i) => i.ingredientId && !inStock.has(i.ingredientId))
  const time = minutesLabel(recipe.prepMinutes, recipe.cookMinutes)

  return (
    <div className="mx-auto max-w-5xl">
      <div className="mb-3 flex items-center justify-between gap-2">
        <Link to="/recipes" className="flex items-center gap-1 text-[13px] text-muted hover:text-text">
          <ArrowLeft className="size-4" /> Recipes
        </Link>
        <div className="flex items-center gap-1">
          <IconButton label={recipe.favorite ? 'Remove from favourites' : 'Add to favourites'} onClick={() => fav.mutate({ favorite: !recipe.favorite })}>
            <Heart className={clsx('size-4', recipe.favorite && 'fill-danger text-danger')} />
          </IconButton>
          <Link to={`/recipes/${recipe.id}/edit`}>
            <IconButton label="Edit">
              <Pencil className="size-4" />
            </IconButton>
          </Link>
          <IconButton label="Delete" onClick={() => confirm(`Delete “${recipe.title}”? Past meal plans keep the dish name.`) && remove.mutate(undefined, { onSuccess: () => navigate('/recipes') })}>
            <Trash2 className="size-4" />
          </IconButton>
          <Button variant="primary" icon={<CalendarPlus className="size-4" />} onClick={() => setPlanning(true)}>
            Add to plan
          </Button>
        </div>
      </div>

      <Card className="overflow-hidden">
        <div className="grid md:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
          <RecipeImage photo={recipe.photo} title={recipe.title} thumb={false} className="aspect-[4/3] w-full md:aspect-auto md:h-full md:max-h-96" />
          <div className="p-5">
            <div className="flex items-center gap-2">
              <DietMark diet={recipe.diet} />
              <span className="text-[12px] text-muted">
                {categoryLabel(recipe.category)}
                {recipe.cuisine && ` · ${recipe.cuisine}`}
              </span>
            </div>
            <h1 className="mt-1.5 text-xl font-semibold tracking-tight">{recipe.title}</h1>
            {recipe.description && <p className="mt-2 text-[13px] leading-relaxed text-muted">{recipe.description}</p>}
            <div className="mt-4 flex flex-wrap gap-x-5 gap-y-2 text-[13px]">
              {time && (
                <span className="flex items-center gap-1.5 text-muted">
                  <Clock className="size-4" /> {time}
                </span>
              )}
              <span className="flex items-center gap-1.5">
                <Users className="size-4 text-muted" />
                <span className="flex items-center rounded-md border border-border">
                  <button className="px-1.5 py-0.5 text-muted hover:text-text" onClick={() => setServings(Math.max(1, srv - 1))} aria-label="Fewer servings">
                    <Minus className="size-3.5" />
                  </button>
                  <span className="w-16 text-center tabular">{srv} servings</span>
                  <button className="px-1.5 py-0.5 text-muted hover:text-text" onClick={() => setServings(Math.min(50, srv + 1))} aria-label="More servings">
                    <Plus className="size-3.5" />
                  </button>
                </span>
              </span>
              <span className="flex items-center gap-0.5" aria-label={`Rating ${recipe.rating} of 5`}>
                {[1, 2, 3, 4, 5].map((n) => (
                  <button key={n} onClick={() => fav.mutate({ rating: n === recipe.rating ? 0 : n })} aria-label={`${n} stars`}>
                    <Star className={clsx('size-4', n <= recipe.rating ? 'fill-warning text-warning' : 'text-border-strong')} />
                  </button>
                ))}
              </span>
            </div>
            {recipe.prepNote && (
              <div className="mt-4 flex items-start gap-2 rounded-md bg-warning-soft px-3 py-2 text-[13px] text-warning">
                <BellRing className="mt-0.5 size-4 shrink-0" />
                <span>
                  {recipe.prepNote}
                  {recipe.prepLeadHours > 0 && <span className="opacity-80"> — you'll get a reminder {recipe.prepLeadHours} h before the meal.</span>}
                </span>
              </div>
            )}
            <div className="mt-4 flex flex-wrap gap-1.5">
              {seasonLabel(recipe.months) && <Badge tone="success">In season {seasonLabel(recipe.months)}</Badge>}
              {recipe.tags.map((t) => (
                <Badge key={t}>{t}</Badge>
              ))}
            </div>
            <p className="mt-4 text-[12px] text-subtle">
              {recipe.timesCooked ? `Cooked ${recipe.timesCooked} ${recipe.timesCooked === 1 ? 'time' : 'times'} · last ${relativeDay(recipe.lastCooked!)}` : 'Not cooked yet'}
              {recipe.sourceUrl && (
                <>
                  {' · '}
                  <a href={recipe.sourceUrl} target="_blank" rel="noreferrer noopener" className="inline-flex items-center gap-0.5 text-accent hover:underline">
                    Source <ExternalLink className="size-3" />
                  </a>
                </>
              )}
            </p>
          </div>
        </div>
      </Card>

      <div className="mt-4 grid gap-4 lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
        <div className="space-y-4">
          <Card className="h-fit">
            <div className="flex items-center justify-between border-b border-border px-4 py-3">
              <h2 className="text-[13px] font-semibold">Ingredients</h2>
              {missing.length > 0 && (
                <Button
                  size="sm"
                  icon={<ShoppingCart className="size-3.5" />}
                  onClick={() => {
                    missing.forEach((i) => grocery.add(`${i.quantity != null ? formatQty(i.quantity * scale, i.unit) + ' ' : ''}${i.name}`))
                    toast.success(`${missing.length} missing ${missing.length === 1 ? 'item' : 'items'} added to the grocery list`)
                  }}
                >
                  Add {missing.length} missing
                </Button>
              )}
            </div>
            <ul className="divide-y divide-border">
              {recipe.ingredients.map((i, idx) => {
                const have = !!i.ingredientId && inStock.has(i.ingredientId)
                return (
                  <li key={idx} className="flex items-baseline gap-3 px-4 py-2 text-[13px]">
                    <span className="w-20 shrink-0 text-right font-medium tabular">{i.quantity != null ? formatQty(i.quantity * scale, i.unit) : i.unit}</span>
                    <span className="min-w-0 flex-1">
                      {i.name}
                      {i.note && <span className="text-subtle">, {i.note}</span>}
                    </span>
                    {have && <Check className="size-3.5 shrink-0 text-success" aria-label="In pantry" />}
                  </li>
                )
              })}
              {recipe.ingredients.length === 0 && <li className="px-4 py-5 text-[13px] text-subtle">No ingredients listed.</li>}
            </ul>
          </Card>
          <RecipeNutritionCard recipe={recipe} />
        </div>
        <Card className="h-fit">
          <div className="border-b border-border px-4 py-3">
            <h2 className="text-[13px] font-semibold">Method</h2>
          </div>
          <ol className="space-y-1 p-2">
            {recipe.steps.map((s, i) => (
              <li key={i}>
                <button
                  onClick={() => setDone((d) => new Set(d.has(i) ? [...d].filter((x) => x !== i) : [...d, i]))}
                  className={clsx('flex w-full gap-3 rounded-md px-2 py-2 text-left text-[14px] leading-relaxed hover:bg-surface-2', done.has(i) && 'text-subtle line-through')}
                >
                  <span className={clsx('flex size-6 shrink-0 items-center justify-center rounded-full text-[12px] font-semibold', done.has(i) ? 'bg-success text-white' : 'bg-surface-3 text-muted')}>
                    {done.has(i) ? <Check className="size-3.5" /> : i + 1}
                  </span>
                  <span>{s}</span>
                </button>
              </li>
            ))}
            {recipe.steps.length === 0 && <li className="px-2 py-3 text-[13px] text-subtle">No steps written down.</li>}
          </ol>
        </Card>
      </div>
      <PlanDialog open={planning} onOpenChange={setPlanning} recipe={recipe} servings={srv} />
    </div>
  )
}

function PlanDialog({ open, onOpenChange, recipe, servings }: { open: boolean; onOpenChange: (o: boolean) => void; recipe: Recipe; servings: number }) {
  const { add } = useMealActions()
  const [date, setDate] = useState(format(new Date(), 'yyyy-MM-dd'))
  const [slot, setSlot] = useState<Slot>(recipe.mealTypes[0] ?? 'lunch')
  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title={`Plan ${recipe.title}`}
      footer={
        <>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            variant="primary"
            onClick={() => {
              add({ date, slot, recipeId: recipe.id, title: recipe.title, servings })
              toast.success(`Added to ${SLOTS.find((s) => s.value === slot)!.label.toLowerCase()} on ${formatDate(date)}`)
              onOpenChange(false)
            }}
          >
            Add to plan
          </Button>
        </>
      }
    >
      <div className="grid grid-cols-2 gap-3">
        <Field label="Day">
          <Input type="date" value={date} onChange={(e) => setDate(e.target.value)} />
        </Field>
        <Field label="Meal">
          <Select value={slot} onChange={(e) => setSlot(e.target.value as Slot)}>
            {SLOTS.map((s) => (
              <option key={s.value} value={s.value}>
                {s.label}
              </option>
            ))}
          </Select>
        </Field>
      </div>
      <p className="mt-3 text-[12px] text-subtle">{servings} servings</p>
    </Modal>
  )
}
