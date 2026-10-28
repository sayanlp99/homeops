import { QueryClient, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { addDays, format, parseISO } from 'date-fns'
import { toast } from 'sonner'
import { ApiError, clientId, del, get, patch, post } from './api'
import { shrinkImage } from './image'
import type { Diet, GroceryItem, Ingredient, MealItem, PantryItem, Recipe, Slot, Suggestion } from './types'
import { uuid } from './uuid'

export const SLOTS: { value: Slot; label: string }[] = [
  { value: 'breakfast', label: 'Breakfast' },
  { value: 'lunch', label: 'Lunch' },
  { value: 'snacks', label: 'Snacks' },
  { value: 'dinner', label: 'Dinner' },
]
export const slotLabel = (s: Slot) => SLOTS.find((x) => x.value === s)!.label

export const CATEGORIES: { value: string; label: string }[] = [
  { value: 'main', label: 'Main' }, { value: 'dal', label: 'Dal' }, { value: 'curry', label: 'Curry / gravy' },
  { value: 'sabzi', label: 'Sabzi / dry veg' }, { value: 'rice', label: 'Rice' }, { value: 'bread', label: 'Roti / bread' },
  { value: 'breakfast', label: 'Breakfast dish' }, { value: 'snack', label: 'Snack' }, { value: 'side', label: 'Side' },
  { value: 'salad', label: 'Salad / raita' }, { value: 'chutney', label: 'Chutney / pickle' }, { value: 'soup', label: 'Soup' },
  { value: 'dessert', label: 'Dessert' }, { value: 'drink', label: 'Drink' },
]
export const categoryLabel = (c: string) => CATEGORIES.find((x) => x.value === c)?.label ?? c

export const DIETS: { value: Diet; label: string; color: string }[] = [
  { value: 'veg', label: 'Veg', color: '#15803d' },
  { value: 'vegan', label: 'Vegan', color: '#65a30d' },
  { value: 'egg', label: 'Egg', color: '#d97706' },
  { value: 'nonveg', label: 'Non-veg', color: '#b91c1c' },
]

export const fileUrl = (id: string, thumb = false) => `/api/files/${id}${thumb ? '?thumb=1' : ''}`

const FRACTIONS: [number, string][] = [[1 / 8, '⅛'], [1 / 4, '¼'], [1 / 3, '⅓'], [1 / 2, '½'], [2 / 3, '⅔'], [3 / 4, '¾']]

/** "1.5 kg", "2 cups", "1½ tsp", "⅛ tsp" — cooking measures read as kitchen fractions. */
export function formatQty(q: number | null, unit: string) {
  if (q == null) return unit
  const cooking = ['', 'tsp', 'tbsp', 'cup', 'pcs', 'inch'].includes(unit)
  let n = String(Math.round(q * 100) / 100)
  if (cooking) {
    const whole = Math.floor(q + 0.02)
    const rest = q - whole
    const f = FRACTIONS.find(([v]) => Math.abs(rest - v) < 0.03)
    if (rest < 0.03) n = String(whole)
    else if (f) n = `${whole || ''}${f[1]}`
  }
  const label = unit === 'cup' && q > 1 ? 'cups' : unit
  return label ? `${n} ${label}` : n
}

export const weekRange = (start: Date) => ({ from: format(start, 'yyyy-MM-dd'), to: format(addDays(start, 6), 'yyyy-MM-dd') })

// ---- queries ----

export function useRecipes() {
  return useQuery({ queryKey: ['recipes'], queryFn: () => get<Recipe[]>('/recipes') })
}

export function useRecipe(id: string | undefined) {
  const { data, ...rest } = useRecipes()
  return { recipe: data?.find((r) => r.id === id), ...rest }
}

export function useMeals(from: string, to: string) {
  return useQuery({ queryKey: ['meals', from, to], queryFn: () => get<MealItem[]>(`/meals?from=${from}&to=${to}`), placeholderData: (prev) => prev })
}

export function useSuggestions(date: string | null, slot: Slot | null) {
  return useQuery({
    queryKey: ['suggest', date, slot],
    queryFn: () => get<Suggestion[]>(`/meals/suggest?date=${date}&slot=${slot}`),
    enabled: !!date && !!slot,
    staleTime: 10_000,
  })
}

export function useGrocery() {
  return useQuery({ queryKey: ['grocery'], queryFn: () => get<GroceryItem[]>('/grocery') })
}

export function usePantry() {
  return useQuery({ queryKey: ['pantry'], queryFn: () => get<PantryItem[]>('/pantry') })
}

export function useIngredients() {
  return useQuery({ queryKey: ['ingredients'], queryFn: () => get<{ ingredients: Ingredient[]; aisles: string[] }>('/ingredients'), staleTime: 5 * 60_000 })
}

// ---- offline-capable mutations (registered on the client so they survive restarts) ----

type NewMeal = { id: string; date: string; slot: Slot; recipeId?: string | null; title: string; servings?: number; note?: string; portions?: Record<string, number> | null; leftover?: boolean }
type MealPatch = Partial<Pick<MealItem, 'date' | 'slot' | 'servings' | 'note' | 'position' | 'portions' | 'leftover'>> & { cooked?: boolean }
// `text` is parsed on the server; or send the parts directly (as the AI list reader does).
type NewGrocery = { id: string; text?: string; name?: string; quantity?: number | null; unit?: string; note?: string }
type GroceryPatch = { checked?: boolean; quantity?: number | null; unit?: string; note?: string }

const mealsKeys = (qc: QueryClient) => qc.getQueryCache().findAll({ queryKey: ['meals'] }).map((q) => q.queryKey)
const setAllMeals = (qc: QueryClient, fn: (ms: MealItem[]) => MealItem[]) => mealsKeys(qc).forEach((k) => qc.setQueryData<MealItem[]>(k, (old) => (old ? fn(old) : old)))
const setGrocery = (qc: QueryClient, fn: (g: GroceryItem[]) => GroceryItem[]) => qc.setQueryData<GroceryItem[]>(['grocery'], (old) => (old ? fn(old) : old))

const fail = (what: string) => (e: Error) => {
  if (!(e instanceof ApiError && e.status === 0)) toast.error(`${what}: ${e.message}`)
}

export function registerMealMutations(qc: QueryClient) {
  qc.setMutationDefaults(['meal.create'], {
    mutationFn: (m: NewMeal) => post<MealItem>('/meals', m),
    onMutate: async (m: NewMeal) => {
      await qc.cancelQueries({ queryKey: ['meals'] })
      const draft: MealItem = { position: 999, recipeId: null, servings: 2, note: '', cookedAt: null, cookedBy: null, createdAt: new Date().toISOString(), portions: null, leftover: false, eaters: {}, ...m } as MealItem
      mealsKeys(qc).forEach((k) => {
        const [, from, to] = k as [string, string, string]
        if (m.date >= from && m.date <= to) qc.setQueryData<MealItem[]>(k, (old) => (old ? [...old, draft] : old))
      })
    },
    onError: fail("Couldn't add dish"),
    onSettled: () => {
      qc.invalidateQueries({ queryKey: ['meals'] })
      qc.invalidateQueries({ queryKey: ['suggest'] })
    },
  })
  qc.setMutationDefaults<MealItem, Error, { id: string; changes: MealPatch }>(['meal.update'], {
    mutationFn: ({ id, changes }) => patch<MealItem>(`/meals/${id}`, changes),
    onMutate: async ({ id, changes }) => {
      await qc.cancelQueries({ queryKey: ['meals'] })
      setAllMeals(qc, (ms) =>
        ms.map((m) => {
          if (m.id !== id) return m
          const { cooked, ...rest } = changes
          return {
            ...m,
            ...rest,
            ...(changes.portions ? { eaters: changes.portions } : {}),
            ...(cooked === undefined ? {} : { cookedAt: cooked ? new Date().toISOString() : null }),
          }
        }),
      )
    },
    onError: fail("Couldn't update dish"),
    onSettled: (_d, _e, v) => {
      qc.invalidateQueries({ queryKey: ['meals'] })
      qc.invalidateQueries({ queryKey: ['nutrition'] })
      if (v.changes.cooked !== undefined) {
        qc.invalidateQueries({ queryKey: ['pantry'] })
        qc.invalidateQueries({ queryKey: ['recipes'] })
      }
    },
  })
  qc.setMutationDefaults(['meal.delete'], {
    mutationFn: (id: string) => del(`/meals/${id}`),
    onMutate: async (id: string) => {
      await qc.cancelQueries({ queryKey: ['meals'] })
      setAllMeals(qc, (ms) => ms.filter((m) => m.id !== id))
    },
    onError: fail("Couldn't remove dish"),
    onSettled: () => {
      qc.invalidateQueries({ queryKey: ['meals'] })
      qc.invalidateQueries({ queryKey: ['suggest'] })
    },
  })

  qc.setMutationDefaults(['grocery.add'], {
    mutationFn: (g: NewGrocery) => post<GroceryItem>('/grocery', g),
    onMutate: async (g: NewGrocery) => {
      await qc.cancelQueries({ queryKey: ['grocery'] })
      const draft: GroceryItem = {
        id: g.id, ingredientId: null, name: g.name ?? g.text ?? '', quantity: g.quantity ?? null, unit: g.unit ?? '', aisle: 'Other', note: g.note ?? '', source: 'manual',
        checkedAt: null, checkedBy: null, createdBy: null, createdAt: new Date().toISOString(),
      }
      setGrocery(qc, (gs) => [...gs, draft])
    },
    onSuccess: (saved: GroceryItem) => setGrocery(qc, (gs) => gs.map((x) => (x.id === saved.id ? saved : x))),
    onError: fail("Couldn't add item"),
    onSettled: () => qc.invalidateQueries({ queryKey: ['grocery'] }),
  })
  qc.setMutationDefaults<GroceryItem, Error, { id: string; changes: GroceryPatch }>(['grocery.update'], {
    mutationFn: ({ id, changes }) => patch<GroceryItem>(`/grocery/${id}`, changes),
    onMutate: async ({ id, changes }) => {
      await qc.cancelQueries({ queryKey: ['grocery'] })
      setGrocery(qc, (gs) =>
        gs.map((g) => {
          if (g.id !== id) return g
          const { checked, ...rest } = changes
          return { ...g, ...rest, ...(checked === undefined ? {} : { checkedAt: checked ? new Date().toISOString() : null }) }
        }),
      )
    },
    onError: fail("Couldn't update item"),
    onSettled: () => {
      if (qc.isMutating({ mutationKey: ['grocery.update'] }) <= 1) qc.invalidateQueries({ queryKey: ['grocery'] })
    },
  })
  qc.setMutationDefaults(['grocery.delete'], {
    mutationFn: (id: string) => del(`/grocery/${id}`),
    onMutate: async (id: string) => {
      await qc.cancelQueries({ queryKey: ['grocery'] })
      setGrocery(qc, (gs) => gs.filter((g) => g.id !== id))
    },
    onError: fail("Couldn't remove item"),
    onSettled: () => qc.invalidateQueries({ queryKey: ['grocery'] }),
  })
}

export function useMealActions() {
  const create = useMutation<MealItem, Error, NewMeal>({ mutationKey: ['meal.create'] })
  const update = useMutation<MealItem, Error, { id: string; changes: MealPatch }>({ mutationKey: ['meal.update'] })
  const remove = useMutation<void, Error, string>({ mutationKey: ['meal.delete'] })
  return {
    add: (m: Omit<NewMeal, 'id'>) => create.mutate({ ...m, id: uuid() }),
    update: (id: string, changes: MealPatch) => update.mutate({ id, changes }),
    remove: (id: string) => remove.mutate(id),
  }
}

export function useGroceryActions() {
  const add = useMutation<GroceryItem, Error, NewGrocery>({ mutationKey: ['grocery.add'] })
  const update = useMutation<GroceryItem, Error, { id: string; changes: GroceryPatch }>({ mutationKey: ['grocery.update'] })
  const remove = useMutation<void, Error, string>({ mutationKey: ['grocery.delete'] })
  return {
    add: (text: string) => add.mutate({ id: uuid(), text }),
    addItem: (g: Omit<NewGrocery, 'id' | 'text'>) => add.mutate({ ...g, id: uuid() }),
    update: (id: string, changes: GroceryPatch) => update.mutate({ id, changes }),
    remove: (id: string) => remove.mutate(id),
  }
}

/** Online-only actions that invalidate their data on success. */
export function useAction<V, R = unknown>(fn: (v: V) => Promise<R>, keys: string[][], success?: string | ((r: R) => string)) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: (r) => {
      keys.forEach((k) => qc.invalidateQueries({ queryKey: k }))
      if (success) toast.success(typeof success === 'function' ? success(r) : success)
    },
    onError: (e: Error) => toast.error(e.message),
  })
}

export async function uploadPhoto(file: File): Promise<string> {
  const body = new FormData()
  body.append('file', await shrinkImage(file), 'photo.jpg')
  const res = await fetch('/api/files', { method: 'POST', body, headers: { 'X-Requested-With': 'homeops', 'X-Client-Id': clientId } })
  const data = await res.json().catch(() => null)
  if (!res.ok) throw new Error(data?.message ?? 'Upload failed')
  return data.id as string
}

export const isPast = (date: string) => parseISO(date) < parseISO(format(new Date(), 'yyyy-MM-dd'))
