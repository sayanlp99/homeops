import { useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import { ArrowDown, ArrowLeft, ArrowUp, ClipboardPaste, ImagePlus, Plus, Trash2, X } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import { Modal } from '@/components/Dialog'
import { Button, Card, CardHeader, Field, IconButton, Input, Select, Spinner, Textarea } from '@/components/ui'
import { post, api } from '@/lib/api'
import { CATEGORIES, DIETS, SLOTS, fileUrl, uploadPhoto, useRecipe } from '@/lib/meals'
import type { Recipe, RecipeIngredient, Slot } from '@/lib/types'
import { importDraft } from './RecipesPage'

type Draft = Omit<Recipe, 'id' | 'createdAt' | 'updatedAt' | 'timesCooked' | 'lastCooked'> & { id?: string }

const blank: Draft = {
  title: '', description: '', category: 'main', mealTypes: ['lunch', 'dinner'], diet: 'veg', cuisine: '', tags: [], servings: 2,
  prepMinutes: null, cookMinutes: null, prepNote: '', prepLeadHours: 0, steps: [''], sourceUrl: '', photo: '', favorite: false, rating: 0,
  ingredients: [{ name: '', quantity: null, unit: '', note: '' }],
}

const UNITS = ['', 'g', 'kg', 'ml', 'l', 'tsp', 'tbsp', 'cup', 'pcs', 'clove', 'inch', 'bunch', 'pinch', 'packet', 'can', 'sprig']

export function RecipeEditorPage() {
  const { id } = useParams()
  const [params] = useSearchParams()
  const { recipe, isLoading } = useRecipe(id)
  const [d, setD] = useState<Draft | null>(null)
  const [saving, setSaving] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [pasting, setPasting] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)
  const navigate = useNavigate()
  const qc = useQueryClient()

  useEffect(() => {
    if (d) return
    if (id && recipe) setD({ ...recipe, steps: recipe.steps.length ? recipe.steps : [''], ingredients: recipe.ingredients.length ? recipe.ingredients : blank.ingredients })
    else if (!id && params.get('import') && importDraft.current) {
      const r = importDraft.current
      setD({ ...blank, ...r, id: undefined, steps: r.steps.length ? r.steps : [''], ingredients: r.ingredients.length ? r.ingredients : blank.ingredients })
      importDraft.current = null
    } else if (!id) setD(blank)
  }, [id, recipe, params, d])

  if (id && isLoading) return <Spinner className="mx-auto mt-16" />
  if (!d) return null
  const set = (p: Partial<Draft>) => setD({ ...d, ...p })
  const setIng = (i: number, p: Partial<RecipeIngredient>) => set({ ingredients: d.ingredients.map((x, j) => (j === i ? { ...x, ...p } : x)) })
  const move = <T,>(arr: T[], i: number, by: number) => {
    const j = i + by
    if (j < 0 || j >= arr.length) return arr
    const next = [...arr]
    ;[next[i], next[j]] = [next[j]!, next[i]!]
    return next
  }

  const save = async () => {
    if (!d.title.trim()) return toast.error('Give the recipe a name')
    setSaving(true)
    try {
      const body = { ...d, ingredients: d.ingredients.filter((i) => i.name.trim()), steps: d.steps.filter((s) => s.trim()) }
      const saved = await api<Recipe>(id ? 'PUT' : 'POST', id ? `/recipes/${id}` : '/recipes', body)
      await qc.invalidateQueries({ queryKey: ['recipes'] })
      toast.success('Recipe saved')
      navigate(`/recipes/${saved.id}`, { replace: true })
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Save failed')
    } finally {
      setSaving(false)
    }
  }

  const onPhoto = async (f: File | undefined) => {
    if (!f) return
    setUploading(true)
    try {
      set({ photo: await uploadPhoto(f) })
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Upload failed')
    } finally {
      setUploading(false)
    }
  }

  return (
    <div className="mx-auto max-w-4xl pb-20">
      <div className="mb-4 flex items-center justify-between gap-2">
        <Link to={id ? `/recipes/${id}` : '/recipes'} className="flex items-center gap-1 text-[13px] text-muted hover:text-text">
          <ArrowLeft className="size-4" /> {id ? 'Back to recipe' : 'Recipes'}
        </Link>
        <Button variant="primary" loading={saving} onClick={save}>
          Save recipe
        </Button>
      </div>

      <div className="space-y-4">
        <Card>
          <CardHeader title={id ? 'Edit recipe' : 'New recipe'} subtitle={d.sourceUrl ? `Imported from ${new URL(d.sourceUrl).hostname}` : undefined} />
          <div className="grid gap-4 p-4 md:grid-cols-[200px_1fr]">
            <div>
              <button
                type="button"
                onClick={() => fileRef.current?.click()}
                className="relative flex aspect-[4/3] w-full items-center justify-center overflow-hidden rounded-md border border-dashed border-border-strong bg-surface-2 text-subtle hover:border-accent hover:text-accent"
              >
                {uploading ? <Spinner /> : d.photo ? <img src={fileUrl(d.photo, true)} alt="" className="absolute inset-0 size-full object-cover" /> : (
                  <span className="flex flex-col items-center gap-1 text-[12px]">
                    <ImagePlus className="size-6" /> Add photo
                  </span>
                )}
              </button>
              {d.photo && (
                <button className="mt-1 text-[12px] text-subtle hover:text-danger" onClick={() => set({ photo: '' })}>
                  Remove photo
                </button>
              )}
              <input ref={fileRef} type="file" accept="image/*" className="hidden" onChange={(e) => onPhoto(e.target.files?.[0])} />
            </div>
            <div className="space-y-3">
              <Field label="Name">
                <Input value={d.title} onChange={(e) => set({ title: e.target.value })} placeholder="e.g. Maa's chicken curry" maxLength={150} />
              </Field>
              <Field label="Short description">
                <Textarea value={d.description} onChange={(e) => set({ description: e.target.value })} className="min-h-16" />
              </Field>
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
                <Field label="Type">
                  <Select value={d.category} onChange={(e) => set({ category: e.target.value })}>
                    {CATEGORIES.map((c) => (
                      <option key={c.value} value={c.value}>
                        {c.label}
                      </option>
                    ))}
                  </Select>
                </Field>
                <Field label="Diet">
                  <Select value={d.diet} onChange={(e) => set({ diet: e.target.value as Draft['diet'] })}>
                    {DIETS.map((x) => (
                      <option key={x.value} value={x.value}>
                        {x.label}
                      </option>
                    ))}
                  </Select>
                </Field>
                <Field label="Cuisine">
                  <Input value={d.cuisine} onChange={(e) => set({ cuisine: e.target.value })} placeholder="Bengali" />
                </Field>
                <Field label="Serves">
                  <Input type="number" min={1} max={50} value={d.servings} onChange={(e) => set({ servings: Math.max(1, Number(e.target.value) || 1) })} />
                </Field>
              </div>
              <Field label="Good for">
                <div className="flex flex-wrap gap-1.5">
                  {SLOTS.map((s) => {
                    const on = d.mealTypes.includes(s.value)
                    return (
                      <button
                        key={s.value}
                        type="button"
                        onClick={() => set({ mealTypes: on ? d.mealTypes.filter((x) => x !== s.value) : [...d.mealTypes, s.value as Slot] })}
                        className={clsx('h-8 rounded-md border px-3 text-[12px] font-medium', on ? 'border-accent bg-accent-soft text-accent' : 'border-border text-muted hover:bg-surface-2')}
                      >
                        {s.label}
                      </button>
                    )
                  })}
                </div>
              </Field>
            </div>
          </div>
          <div className="grid grid-cols-2 gap-3 border-t border-border p-4 sm:grid-cols-4">
            <Field label="Prep (min)">
              <Input type="number" min={0} value={d.prepMinutes ?? ''} onChange={(e) => set({ prepMinutes: e.target.value ? Number(e.target.value) : null })} />
            </Field>
            <Field label="Cook (min)">
              <Input type="number" min={0} value={d.cookMinutes ?? ''} onChange={(e) => set({ cookMinutes: e.target.value ? Number(e.target.value) : null })} />
            </Field>
            <Field label="Tags" className="col-span-2" hint="Comma separated, e.g. quick, fasting, party">
              <Input value={d.tags.join(', ')} onChange={(e) => set({ tags: e.target.value.split(',').map((t) => t.trimStart()) })} />
            </Field>
            <Field label="Advance prep" className="col-span-2 sm:col-span-3" hint="You'll get a push reminder ahead of the meal">
              <Input value={d.prepNote} onChange={(e) => set({ prepNote: e.target.value })} placeholder="e.g. Soak rajma overnight, marinate chicken" />
            </Field>
            <Field label="Remind (hours before)">
              <Select value={d.prepLeadHours} onChange={(e) => set({ prepLeadHours: Number(e.target.value) })}>
                {[0, 1, 2, 4, 6, 8, 10, 12, 24].map((h) => (
                  <option key={h} value={h}>
                    {h === 0 ? 'No reminder' : `${h} h before`}
                  </option>
                ))}
              </Select>
            </Field>
          </div>
        </Card>

        <Card>
          <CardHeader
            title="Ingredients"
            subtitle={`For ${d.servings} servings`}
            actions={
              <Button size="sm" icon={<ClipboardPaste className="size-3.5" />} onClick={() => setPasting(true)}>
                Paste a list
              </Button>
            }
          />
          <div className="divide-y divide-border">
            <div className="hidden grid-cols-[70px_90px_1fr_1fr_64px] gap-2 px-4 py-1.5 text-[11px] font-medium text-subtle uppercase sm:grid">
              <span>Qty</span>
              <span>Unit</span>
              <span>Ingredient</span>
              <span>Note</span>
              <span />
            </div>
            {d.ingredients.map((ing, i) => (
              <div key={i} className="grid grid-cols-[64px_84px_1fr_auto] items-center gap-2 px-3 py-2 sm:grid-cols-[70px_90px_1fr_1fr_64px] sm:px-4">
                <Input type="number" step="any" min={0} inputMode="decimal" aria-label="Quantity" value={ing.quantity ?? ''} onChange={(e) => setIng(i, { quantity: e.target.value ? Number(e.target.value) : null })} className="px-2" />
                <Select aria-label="Unit" value={UNITS.includes(ing.unit) ? ing.unit : ing.unit} onChange={(e) => setIng(i, { unit: e.target.value })} className="px-1.5">
                  {[...new Set([...UNITS, ing.unit])].map((u) => (
                    <option key={u} value={u}>
                      {u || '—'}
                    </option>
                  ))}
                </Select>
                <Input aria-label="Ingredient" value={ing.name} onChange={(e) => setIng(i, { name: e.target.value })} placeholder="e.g. Toor dal" />
                <Input aria-label="Note" value={ing.note} onChange={(e) => setIng(i, { note: e.target.value })} placeholder="chopped" className="col-span-3 col-start-1 row-start-2 sm:col-span-1 sm:col-start-auto sm:row-start-auto" />
                <div className="flex justify-end">
                  <IconButton label="Remove ingredient" onClick={() => set({ ingredients: d.ingredients.filter((_, j) => j !== i) })}>
                    <X className="size-4" />
                  </IconButton>
                </div>
              </div>
            ))}
          </div>
          <div className="p-3">
            <Button size="sm" variant="ghost" icon={<Plus className="size-3.5" />} onClick={() => set({ ingredients: [...d.ingredients, { name: '', quantity: null, unit: '', note: '' }] })}>
              Add ingredient
            </Button>
          </div>
        </Card>

        <Card>
          <CardHeader title="Method" />
          <ol className="space-y-2 p-4">
            {d.steps.map((s, i) => (
              <li key={i} className="flex gap-2">
                <span className="mt-2 flex size-6 shrink-0 items-center justify-center rounded-full bg-surface-3 text-[12px] font-semibold text-muted">{i + 1}</span>
                <Textarea value={s} onChange={(e) => set({ steps: d.steps.map((x, j) => (j === i ? e.target.value : x)) })} className="min-h-16 flex-1" placeholder="Describe this step" />
                <div className="flex flex-col">
                  <IconButton label="Move up" onClick={() => set({ steps: move(d.steps, i, -1) })}>
                    <ArrowUp className="size-3.5" />
                  </IconButton>
                  <IconButton label="Move down" onClick={() => set({ steps: move(d.steps, i, 1) })}>
                    <ArrowDown className="size-3.5" />
                  </IconButton>
                  <IconButton label="Remove step" onClick={() => set({ steps: d.steps.filter((_, j) => j !== i) })}>
                    <Trash2 className="size-3.5" />
                  </IconButton>
                </div>
              </li>
            ))}
          </ol>
          <div className="px-4 pb-4">
            <Button size="sm" variant="ghost" icon={<Plus className="size-3.5" />} onClick={() => set({ steps: [...d.steps, ''] })}>
              Add step
            </Button>
          </div>
        </Card>
      </div>
      <PasteDialog
        open={pasting}
        onOpenChange={setPasting}
        onParsed={(rows) => set({ ingredients: [...d.ingredients.filter((i) => i.name.trim()), ...rows] })}
      />
    </div>
  )
}

function PasteDialog({ open, onOpenChange, onParsed }: { open: boolean; onOpenChange: (o: boolean) => void; onParsed: (rows: RecipeIngredient[]) => void }) {
  const [text, setText] = useState('')
  const [busy, setBusy] = useState(false)
  const run = async () => {
    setBusy(true)
    try {
      const rows = await post<RecipeIngredient[]>('/ingredients/parse', { lines: text.split('\n') })
      onParsed(rows.map((r) => ({ ...r, note: r.note ?? '' })))
      setText('')
      onOpenChange(false)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Could not parse')
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title="Paste ingredients"
      description="One per line — quantities, units and notes are picked out automatically."
      footer={
        <>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button variant="primary" loading={busy} disabled={!text.trim()} onClick={run}>
            Add ingredients
          </Button>
        </>
      }
    >
      <Textarea autoFocus value={text} onChange={(e) => setText(e.target.value)} className="min-h-48 font-mono text-[13px]" placeholder={'1 cup toor dal\n2 tomatoes, chopped\n½ tsp turmeric powder\nSalt to taste'} />
    </Modal>
  )
}
