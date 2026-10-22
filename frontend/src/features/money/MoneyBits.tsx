import clsx from 'clsx'
import { addMonths, format, parse } from 'date-fns'
import {
  Baby, Banknote, BookOpen, Briefcase, Car, ChevronLeft, ChevronRight, Circle, Dumbbell, Film, Fuel, Gift, GraduationCap, HeartPulse,
  Home, Landmark, Lock, Plane, Plug, Repeat, Scissors, ShoppingBag, ShoppingBasket, Smartphone, Sofa, Carrot, Utensils, Wallet, Wifi, Dog,
} from 'lucide-react'
import { NavLink } from 'react-router-dom'
import type { MoneyCategory } from '@/lib/types'

export const ICONS: Record<string, typeof Circle> = {
  circle: Circle, 'shopping-basket': ShoppingBasket, carrot: Carrot, utensils: Utensils, plug: Plug, home: Home, car: Car, sofa: Sofa,
  'heart-pulse': HeartPulse, 'shopping-bag': ShoppingBag, repeat: Repeat, wallet: Wallet, fuel: Fuel, plane: Plane, gift: Gift,
  'graduation-cap': GraduationCap, baby: Baby, dumbbell: Dumbbell, film: Film, smartphone: Smartphone, wifi: Wifi, scissors: Scissors,
  briefcase: Briefcase, landmark: Landmark, banknote: Banknote, book: BookOpen, dog: Dog,
}

export function CategoryIcon({ category, size = 28, className }: { category?: Pick<MoneyCategory, 'icon' | 'color'>; size?: number; className?: string }) {
  const Icon = (category && ICONS[category.icon]) || Circle
  return (
    <span
      className={clsx('inline-flex shrink-0 items-center justify-center rounded-md', className)}
      style={{ width: size, height: size, background: category ? `${category.color}1f` : 'var(--surface-3)', color: category?.color ?? 'var(--subtle)' }}
    >
      <Icon style={{ width: size * 0.55, height: size * 0.55 }} />
    </span>
  )
}

export function PrivateMark({ className }: { className?: string }) {
  return <Lock className={clsx('size-3 shrink-0 text-subtle', className)} aria-label="Private to you" />
}

export function MonthSwitcher({ month, onChange }: { month: string; onChange: (m: string) => void }) {
  const d = parse(month, 'yyyy-MM', new Date())
  const isCurrent = month === format(new Date(), 'yyyy-MM')
  return (
    <div className="flex items-center rounded-md border border-border bg-surface shadow-card">
      <button className="flex size-8 items-center justify-center text-muted hover:text-text" onClick={() => onChange(format(addMonths(d, -1), 'yyyy-MM'))} aria-label="Previous month">
        <ChevronLeft className="size-4" />
      </button>
      <button className="h-8 min-w-28 border-x border-border px-3 text-[12px] font-medium" onClick={() => onChange(format(new Date(), 'yyyy-MM'))} title="Go to this month">
        {format(d, 'MMMM yyyy')}
      </button>
      <button
        className="flex size-8 items-center justify-center text-muted hover:text-text disabled:opacity-30"
        disabled={isCurrent}
        onClick={() => onChange(format(addMonths(d, 1), 'yyyy-MM'))}
        aria-label="Next month"
      >
        <ChevronRight className="size-4" />
      </button>
    </div>
  )
}

const tabs = [
  { to: '/budget', label: 'Overview' },
  { to: '/budget/transactions', label: 'Transactions' },
  { to: '/budget/bills', label: 'Bills' },
  { to: '/budget/import', label: 'Import' },
  { to: '/budget/setup', label: 'Categories & accounts' },
]

export function MoneyNav() {
  return (
    <nav className="-mx-4 mb-4 flex gap-1 overflow-x-auto border-b border-border px-4 scrollbar-thin lg:mx-0 lg:px-0">
      {tabs.map((t) => (
        <NavLink
          key={t.to}
          to={t.to}
          end
          className={({ isActive }) =>
            clsx('-mb-px flex h-9 items-center border-b-2 px-3 text-[13px] font-medium whitespace-nowrap', isActive ? 'border-accent text-text' : 'border-transparent text-muted hover:text-text')
          }
        >
          {t.label}
        </NavLink>
      ))}
    </nav>
  )
}
