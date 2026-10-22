import clsx from 'clsx'
import { ChefHat } from 'lucide-react'
import { NavLink } from 'react-router-dom'
import { DIETS, fileUrl } from '@/lib/meals'
import type { Diet } from '@/lib/types'

/** The FSSAI-style square mark Indian packaging uses: green dot = veg, red = non-veg. */
export function DietMark({ diet, className }: { diet: Diet; className?: string }) {
  const d = DIETS.find((x) => x.value === diet) ?? DIETS[0]!
  return (
    <span title={d.label} className={clsx('inline-flex size-3.5 shrink-0 items-center justify-center rounded-[3px] border-[1.5px] bg-surface', className)} style={{ borderColor: d.color }}>
      {diet === 'nonveg' ? (
        <svg viewBox="0 0 10 10" className="size-2">
          <path d="M5 1.5 9 8.5H1Z" fill={d.color} />
        </svg>
      ) : (
        <span className="size-1.5 rounded-full" style={{ background: d.color }} />
      )}
    </span>
  )
}

export function RecipeImage({ photo, title, className, thumb = true }: { photo: string; title: string; className?: string; thumb?: boolean }) {
  if (!photo)
    return (
      <div className={clsx('flex items-center justify-center bg-surface-3 text-subtle', className)} aria-label={title}>
        <ChefHat className="size-1/3 max-h-10 max-w-10 opacity-60" />
      </div>
    )
  return <img src={fileUrl(photo, thumb)} alt={title} loading="lazy" decoding="async" className={clsx('bg-surface-3 object-cover', className)} />
}

const tabs = [
  { to: '/meals', label: 'Meal plan' },
  { to: '/recipes', label: 'Recipes' },
  { to: '/grocery', label: 'Grocery' },
  { to: '/pantry', label: 'Pantry' },
]

/** Sub-navigation shared by the four kitchen pages. */
export function MealsNav() {
  return (
    <nav className="-mx-4 mb-4 flex gap-1 overflow-x-auto border-b border-border px-4 scrollbar-thin lg:mx-0 lg:px-0">
      {tabs.map((t) => (
        <NavLink
          key={t.to}
          to={t.to}
          end={t.to === '/meals'}
          className={({ isActive }) =>
            clsx(
              '-mb-px flex h-9 items-center border-b-2 px-3 text-[13px] font-medium whitespace-nowrap',
              isActive ? 'border-accent text-text' : 'border-transparent text-muted hover:text-text',
            )
          }
        >
          {t.label}
        </NavLink>
      ))}
    </nav>
  )
}

export function minutesLabel(prep: number | null, cook: number | null) {
  const total = (prep ?? 0) + (cook ?? 0)
  if (!total) return ''
  return total >= 60 ? `${Math.floor(total / 60)} h${total % 60 ? ` ${total % 60} m` : ''}` : `${total} min`
}
