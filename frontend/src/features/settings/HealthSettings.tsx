import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, HeartPulse, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Avatar, Button, Card, CardHeader, EmptyState, Field, IconButton, Input, Segmented, Select, Spinner } from '@/components/ui'
import { del, put } from '@/lib/api'
import { ACTIVITY_LABELS, AVOID_LABELS, GOAL_LABELS, NUTRIENTS, fmtAmount, useHealth } from '@/lib/nutrition'
import { hasRole, useSession } from '@/lib/session'
import type { Activity, Goal, MemberHealth, NutrientKey, Sex } from '@/lib/types'

interface Form {
  sex: Sex | ''
  age: string
  heightCm: string
  feet: string
  inches: string
  weightKg: string
  activity: Activity
  goal: Goal
  avoid: string[]
  overrides: Partial<Record<NutrientKey, string>>
}

const thisYear = new Date().getFullYear()

function toForm(m: MemberHealth): Form {
  const p = m.profile
  const cm = p?.heightCm ?? 0
  const totalIn = cm / 2.54
  return {
    sex: p?.sex ?? '',
    age: p ? String(thisYear - p.birthYear) : '',
    heightCm: p ? String(cm) : '',
    feet: p ? String(Math.floor(totalIn / 12)) : '',
    inches: p ? String(Math.round(totalIn % 12)) : '',
    weightKg: p ? String(p.weightKg) : '',
    activity: p?.activity ?? 'light',
    goal: p?.goal ?? 'maintain',
    avoid: p?.avoid ?? [],
    overrides: Object.fromEntries(Object.entries(p?.overrides ?? {}).map(([k, v]) => [k, String(v)])),
  }
}

export function HealthSettings() {
  const { user } = useSession()
  const canSee = hasRole(user, 'member')
  const { data, isLoading } = useHealth(canSee)
  if (!canSee)
    return (
      <Card>
        <EmptyState icon={<HeartPulse className="size-5" />} title="Not available for viewers" description="Health details are shared between household members." />
      </Card>
    )
  if (isLoading || !data) return <Spinner className="mx-auto mt-10" />
  // You first, then everyone else.
  const members = [...data.members].sort((a, b) => (a.userId === user.id ? -1 : b.userId === user.id ? 1 : 0))
  return (
    <>
      <p className="text-[13px] text-muted">
        Used to set daily calorie, protein, vitamin and mineral targets, size portions in the meal plan, and keep food someone doesn't eat off their plate. Anyone in the household
        (except viewers) can update these, so you can keep each other's details current.
      </p>
      {members.map((m) => (
        <PersonCard key={m.userId} m={m} avoidGroups={data.avoidGroups} isMe={m.userId === user.id} />
      ))}
    </>
  )
}

function PersonCard({ m, avoidGroups, isMe }: { m: MemberHealth; avoidGroups: string[]; isMe: boolean }) {
  const qc = useQueryClient()
  const [f, setF] = useState<Form>(() => toForm(m))
  const [unit, setUnit] = useState<'cm' | 'ft'>('cm')
  const [showTargets, setShowTargets] = useState(false)
  useEffect(() => setF(toForm(m)), [m])
  const set = (p: Partial<Form>) => setF((x) => ({ ...x, ...p }))

  const heightCm = unit === 'cm' ? Number(f.heightCm) : Math.round(((Number(f.feet) || 0) * 12 + (Number(f.inches) || 0)) * 2.54 * 10) / 10
  const save = useMutation({
    mutationFn: () =>
      put(`/nutrition/profiles/${m.userId}`, {
        sex: f.sex,
        birthYear: thisYear - Number(f.age),
        heightCm,
        weightKg: Number(f.weightKg),
        activity: f.activity,
        goal: f.goal,
        avoid: f.avoid,
        overrides: Object.fromEntries(Object.entries(f.overrides).filter(([, v]) => v && Number(v) > 0).map(([k, v]) => [k, Number(v)])),
      }),
    onSuccess: () => {
      toast.success(`${isMe ? 'Your' : `${m.name}'s`} details saved`)
      qc.invalidateQueries({ queryKey: ['health'] })
      qc.invalidateQueries({ queryKey: ['nutrition'] })
      qc.invalidateQueries({ queryKey: ['meals'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })
  const remove = useMutation({
    mutationFn: () => del(`/nutrition/profiles/${m.userId}`),
    onSuccess: () => {
      toast.success('Details removed')
      qc.invalidateQueries({ queryKey: ['health'] })
      qc.invalidateQueries({ queryKey: ['nutrition'] })
    },
  })
  const complete = f.sex && Number(f.age) > 0 && heightCm > 0 && Number(f.weightKg) > 0
  const t = m.targets
  const c = m.computed

  return (
    <Card>
      <CardHeader
        title={
          <span className="flex items-center gap-2">
            <Avatar name={m.name} color={m.color} size={22} /> {isMe ? `${m.name} (you)` : m.name}
          </span>
        }
        subtitle={t ? `${fmtAmount(t.kcal)} kcal · ${fmtAmount(t.protein)} g protein a day` : 'No details yet'}
        actions={
          m.profile && (
            <IconButton label="Remove details" onClick={() => confirm(`Remove ${isMe ? 'your' : `${m.name}'s`} health details?`) && remove.mutate()}>
              <Trash2 className="size-4" />
            </IconButton>
          )
        }
      />
      <form
        className="space-y-4 p-4"
        onSubmit={(e) => {
          e.preventDefault()
          save.mutate()
        }}
      >
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <Field label="Sex" className="col-span-2 sm:col-span-1">
            <Segmented
              value={f.sex || ('' as Sex)}
              onChange={(sex) => set({ sex })}
              options={[
                { value: 'female', label: 'Female' },
                { value: 'male', label: 'Male' },
              ]}
              className="w-full"
            />
          </Field>
          <Field label="Age">
            <Input type="number" inputMode="numeric" min={14} max={100} value={f.age} onChange={(e) => set({ age: e.target.value })} required />
          </Field>
          <Field label="Weight (kg)">
            <Input type="number" inputMode="decimal" step="0.1" min={25} max={250} value={f.weightKg} onChange={(e) => set({ weightKg: e.target.value })} required />
          </Field>
          <Field label="Height" className="col-span-2 sm:col-span-1">
            <div className="flex gap-1">
              {unit === 'cm' ? (
                <Input type="number" inputMode="decimal" min={100} max={230} className="min-w-0 flex-1" value={f.heightCm} onChange={(e) => set({ heightCm: e.target.value })} aria-label="Height in cm" required />
              ) : (
                <>
                  <Input type="number" inputMode="numeric" min={3} max={7} className="min-w-0 flex-1" value={f.feet} onChange={(e) => set({ feet: e.target.value })} aria-label="Feet" placeholder="ft" required />
                  <Input type="number" inputMode="numeric" min={0} max={11} className="min-w-0 flex-1" value={f.inches} onChange={(e) => set({ inches: e.target.value })} aria-label="Inches" placeholder="in" />
                </>
              )}
              <div className="w-[76px] shrink-0">
                <Select
                  value={unit}
                  onChange={(e) => {
                    const u = e.target.value as 'cm' | 'ft'
                    if (u === 'cm' && heightCm) set({ heightCm: String(heightCm) })
                    if (u === 'ft' && f.heightCm) {
                      const totalIn = Number(f.heightCm) / 2.54
                      set({ feet: String(Math.floor(totalIn / 12)), inches: String(Math.round(totalIn % 12)) })
                    }
                    setUnit(u)
                  }}
                  aria-label="Height unit"
                >
                  <option value="cm">cm</option>
                  <option value="ft">ft/in</option>
                </Select>
              </div>
            </div>
          </Field>
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Activity" hint={ACTIVITY_LABELS[f.activity].hint}>
            <Select value={f.activity} onChange={(e) => set({ activity: e.target.value as Activity })}>
              {Object.entries(ACTIVITY_LABELS).map(([k, v]) => (
                <option key={k} value={k}>
                  {v.label}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Goal" hint={GOAL_LABELS[f.goal].hint}>
            <Select value={f.goal} onChange={(e) => set({ goal: e.target.value as Goal })}>
              {Object.entries(GOAL_LABELS).map(([k, v]) => (
                <option key={k} value={k}>
                  {v.label}
                </option>
              ))}
            </Select>
          </Field>
        </div>
        <Field label="Doesn't eat" hint="Dishes with these are never planned for this person, and they're left out of the portions.">
          <div className="flex flex-wrap gap-1.5">
            {avoidGroups.map((g) => {
              const on = f.avoid.includes(g)
              return (
                <button
                  key={g}
                  type="button"
                  onClick={() => set({ avoid: on ? f.avoid.filter((x) => x !== g) : [...f.avoid, g] })}
                  className={`h-7 rounded-full border px-3 text-[12px] ${on ? 'border-danger bg-danger-soft text-danger' : 'border-border text-muted hover:text-text'}`}
                >
                  {AVOID_LABELS[g] ?? g}
                </button>
              )
            })}
          </div>
        </Field>

        {c && (
          <div className="rounded-md border border-border">
            <button type="button" className="flex w-full items-center justify-between px-3 py-2 text-left text-[13px]" onClick={() => setShowTargets(!showTargets)}>
              <span>
                <span className="font-medium">Daily targets</span>
                <span className="text-subtle">
                  {' '}
                  · BMR {fmtAmount(c.bmr)} kcal, maintenance {fmtAmount(c.tdee)} kcal
                </span>
              </span>
              <ChevronDown className={`size-4 text-subtle transition-transform ${showTargets ? 'rotate-180' : ''}`} />
            </button>
            {showTargets && (
              <div className="border-t border-border px-3 py-2">
                <p className="mb-2 text-[12px] text-subtle">
                  Calculated from the details above (Mifflin-St Jeor; vitamins and minerals per ICMR-NIN 2020). Type a number to set your own, for example on a dietitian's advice;
                  leave blank to use the calculated one.
                </p>
                <div className="grid gap-x-6 gap-y-1.5 sm:grid-cols-2">
                  {NUTRIENTS.map((n) => (
                    <label key={n.key} className="grid grid-cols-[minmax(0,1fr)_112px] items-center gap-2 text-[12px]">
                      <span className="truncate text-muted">
                        {n.label}
                        {n.limit && <span className="text-subtle"> (max)</span>}
                      </span>
                      <span className="relative">
                        <Input
                          type="number"
                          inputMode="decimal"
                          min={0}
                          step="any"
                          className="h-7 pr-9 text-right text-[12px]"
                          placeholder={fmtAmount(c[n.key])}
                          value={f.overrides[n.key] ?? ''}
                          onChange={(e) => set({ overrides: { ...f.overrides, [n.key]: e.target.value } })}
                        />
                        <span className="pointer-events-none absolute top-1/2 right-2 -translate-y-1/2 text-[10px] text-subtle">{n.unit}</span>
                      </span>
                    </label>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}
        <div className="flex justify-end">
          <Button type="submit" variant="primary" disabled={!complete} loading={save.isPending}>
            Save
          </Button>
        </div>
      </form>
    </Card>
  )
}
