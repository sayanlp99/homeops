import { useQuery } from '@tanstack/react-query'
import { get } from './api'
import type { Food, MemberHealth, NutrientKey, Nutrients, PersonReport } from './types'

export interface NutrientInfo {
  key: NutrientKey
  label: string
  short: string
  unit: string
  /** A ceiling rather than a goal. */
  limit?: boolean
  macro?: boolean
}

export const NUTRIENTS: NutrientInfo[] = [
  { key: 'kcal', label: 'Calories', short: 'kcal', unit: 'kcal', macro: true },
  { key: 'protein', label: 'Protein', short: 'Protein', unit: 'g', macro: true },
  { key: 'carbs', label: 'Carbs', short: 'Carbs', unit: 'g', macro: true },
  { key: 'fat', label: 'Fat', short: 'Fat', unit: 'g', macro: true, limit: true },
  { key: 'fibre', label: 'Fibre', short: 'Fibre', unit: 'g', macro: true },
  { key: 'iron', label: 'Iron', short: 'Iron', unit: 'mg' },
  { key: 'calcium', label: 'Calcium', short: 'Calcium', unit: 'mg' },
  { key: 'vitC', label: 'Vitamin C', short: 'Vit C', unit: 'mg' },
  { key: 'vitA', label: 'Vitamin A', short: 'Vit A', unit: 'µg' },
  { key: 'folate', label: 'Folate (B9)', short: 'Folate', unit: 'µg' },
  { key: 'b12', label: 'Vitamin B12', short: 'B12', unit: 'µg' },
  { key: 'zinc', label: 'Zinc', short: 'Zinc', unit: 'mg' },
  { key: 'magnesium', label: 'Magnesium', short: 'Magnesium', unit: 'mg' },
  { key: 'potassium', label: 'Potassium', short: 'Potassium', unit: 'mg' },
  { key: 'sodium', label: 'Sodium', short: 'Sodium', unit: 'mg', limit: true },
]
export const nutrientInfo = (k: NutrientKey) => NUTRIENTS.find((n) => n.key === k)!
export const MICROS = NUTRIENTS.filter((n) => !n.macro)

export const AVOID_LABELS: Record<string, string> = { fish: 'Fish', seafood: 'Prawns & seafood', chicken: 'Chicken', mutton: 'Mutton', egg: 'Egg', dairy: 'Dairy' }
export const ACTIVITY_LABELS = {
  sedentary: { label: 'Mostly sitting', hint: 'Desk work, little exercise' },
  light: { label: 'Lightly active', hint: 'Housework, walks or exercise 1–3 days a week' },
  moderate: { label: 'Active', hint: 'Exercise 3–5 days a week' },
  active: { label: 'Very active', hint: 'Hard exercise or a physical job most days' },
} as const
export const GOAL_LABELS = {
  lose: { label: 'Lose weight gently', hint: 'About 15% below maintenance (at most 400 kcal), protein 1.2 g/kg' },
  maintain: { label: 'Stay at this weight', hint: 'Maintenance calories, protein 1.1 g/kg' },
  gain: { label: 'Gain / build muscle', hint: '+250 kcal, protein 1.4 g/kg' },
} as const

/** "1,460", "12.5", "0.8" — enough precision to be useful, no more. */
export function fmtAmount(v: number, unit?: string) {
  const n = v >= 100 ? Math.round(v).toLocaleString('en-IN') : v >= 10 ? String(Math.round(v)) : String(Math.round(v * 10) / 10)
  return unit ? `${n} ${unit}` : n
}

/** Share of a target, with tone: ceilings turn red when exceeded, goals turn green when met. */
export function progressTone(value: number, target: number, limit?: boolean): 'success' | 'warning' | 'danger' | 'accent' {
  if (!target) return 'accent'
  const r = value / target
  if (limit) return r > 1.1 ? 'danger' : r > 0.9 ? 'warning' : 'success'
  return r >= 0.9 ? 'success' : r >= 0.6 ? 'accent' : 'warning'
}

export function useHealth(enabled = true) {
  return useQuery({ queryKey: ['health'], queryFn: () => get<{ members: MemberHealth[]; avoidGroups: string[] }>('/health'), enabled, staleTime: 60_000 })
}

export function useNutrition(from: string, to: string, enabled = true) {
  return useQuery({ queryKey: ['nutrition', from, to], queryFn: () => get<PersonReport[]>(`/nutrition?from=${from}&to=${to}`), enabled, placeholderData: (p) => p })
}

export function useFoods() {
  return useQuery({ queryKey: ['foods'], queryFn: () => get<Food[]>('/nutrition/foods'), staleTime: Infinity })
}

export const scaleNutrients = (n: Nutrients, f: number) => Object.fromEntries(Object.entries(n).map(([k, v]) => [k, v * f])) as unknown as Nutrients
