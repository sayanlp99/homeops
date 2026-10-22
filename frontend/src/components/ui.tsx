import clsx from 'clsx'
import { Loader2 } from 'lucide-react'
import { forwardRef, type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes, type TextareaHTMLAttributes } from 'react'
import { initials } from '@/lib/format'

type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger'

export const Button = forwardRef<
  HTMLButtonElement,
  ButtonHTMLAttributes<HTMLButtonElement> & { variant?: ButtonVariant; size?: 'sm' | 'md'; loading?: boolean; icon?: ReactNode }
>(function Button({ variant = 'secondary', size = 'md', loading, icon, className, children, disabled, ...rest }, ref) {
  return (
    <button
      ref={ref}
      disabled={disabled || loading}
      className={clsx(
        'inline-flex shrink-0 select-none items-center justify-center gap-1.5 rounded-md font-medium whitespace-nowrap transition-colors disabled:pointer-events-none disabled:opacity-50',
        size === 'sm' ? 'h-7 px-2.5 text-[12px]' : 'h-9 px-3.5 lg:h-8 lg:px-3',
        {
          primary: 'bg-accent text-accent-fg shadow-card hover:bg-accent-hover',
          secondary: 'border border-border bg-surface text-text shadow-card hover:bg-surface-2',
          ghost: 'text-muted hover:bg-surface-2 hover:text-text',
          danger: 'bg-danger text-white shadow-card hover:opacity-90',
        }[variant],
        className,
      )}
      {...rest}
    >
      {loading ? <Loader2 className="size-3.5 animate-spin" /> : icon}
      {children}
    </button>
  )
})

export const IconButton = forwardRef<HTMLButtonElement, ButtonHTMLAttributes<HTMLButtonElement> & { label: string }>(
  function IconButton({ label, className, children, ...rest }, ref) {
    return (
      <button
        ref={ref}
        aria-label={label}
        title={label}
        className={clsx(
          'inline-flex size-9 shrink-0 items-center justify-center rounded-md text-muted transition-colors hover:bg-surface-2 hover:text-text lg:size-8',
          className,
        )}
        {...rest}
      >
        {children}
      </button>
    )
  },
)

const fieldBase =
  'w-full rounded-md border border-border bg-surface px-3 text-text placeholder:text-subtle shadow-card outline-none transition-colors focus:border-accent focus:ring-2 focus:ring-[var(--ring)] disabled:opacity-60'

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(function Input({ className, ...rest }, ref) {
  return <input ref={ref} className={clsx(fieldBase, 'h-10 lg:h-8', className)} {...rest} />
})

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaHTMLAttributes<HTMLTextAreaElement>>(function Textarea(
  { className, ...rest },
  ref,
) {
  return <textarea ref={ref} className={clsx(fieldBase, 'min-h-24 py-2 leading-relaxed', className)} {...rest} />
})

export const Select = forwardRef<HTMLSelectElement, SelectHTMLAttributes<HTMLSelectElement>>(function Select({ className, ...rest }, ref) {
  return <select ref={ref} className={clsx(fieldBase, 'h-10 appearance-auto pr-8 lg:h-8', className)} {...rest} />
})

export function Field({ label, hint, error, children, className }: { label: string; hint?: ReactNode; error?: string; children: ReactNode; className?: string }) {
  return (
    <label className={clsx('block space-y-1.5', className)}>
      <span className="block text-[12px] font-medium text-muted">{label}</span>
      {children}
      {error ? <span className="block text-[12px] text-danger">{error}</span> : hint ? <span className="block text-[12px] text-subtle">{hint}</span> : null}
    </label>
  )
}

export function Card({ className, children, ...rest }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div className={clsx('rounded-lg border border-border bg-surface shadow-card', className)} {...rest}>
      {children}
    </div>
  )
}

export function CardHeader({ title, subtitle, actions }: { title: ReactNode; subtitle?: ReactNode; actions?: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-3 border-b border-border px-4 py-3">
      <div className="min-w-0">
        <h2 className="truncate text-[13px] font-semibold">{title}</h2>
        {subtitle && <p className="truncate text-[12px] text-subtle">{subtitle}</p>}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-1.5">{actions}</div>}
    </div>
  )
}

export function PageHeader({ title, subtitle, actions }: { title: ReactNode; subtitle?: ReactNode; actions?: ReactNode }) {
  return (
    <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
      <div className="min-w-0">
        <h1 className="text-lg font-semibold tracking-tight lg:text-[17px]">{title}</h1>
        {subtitle && <p className="mt-0.5 text-[13px] text-muted">{subtitle}</p>}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  )
}

export function Badge({ children, tone = 'neutral', className }: { children: ReactNode; tone?: 'neutral' | 'accent' | 'success' | 'warning' | 'danger'; className?: string }) {
  return (
    <span
      className={clsx(
        'inline-flex h-5 items-center gap-1 rounded px-1.5 text-[11px] font-medium whitespace-nowrap',
        {
          neutral: 'bg-surface-3 text-muted',
          accent: 'bg-accent-soft text-accent',
          success: 'bg-success-soft text-success',
          warning: 'bg-warning-soft text-warning',
          danger: 'bg-danger-soft text-danger',
        }[tone],
        className,
      )}
    >
      {children}
    </span>
  )
}

export function Avatar({ name, color, size = 22, className }: { name?: string; color?: string; size?: number; className?: string }) {
  if (!name)
    return (
      <span
        title="Unassigned"
        className={clsx('inline-block shrink-0 rounded-full border border-dashed border-border-strong', className)}
        style={{ width: size, height: size }}
      />
    )
  return (
    <span
      title={name}
      className={clsx('inline-flex shrink-0 items-center justify-center rounded-full font-semibold text-white', className)}
      style={{ width: size, height: size, background: color, fontSize: Math.max(9, size * 0.42) }}
    >
      {initials(name)}
    </span>
  )
}

export function Kbd({ children }: { children: ReactNode }) {
  return (
    <kbd className="inline-flex h-5 min-w-5 items-center justify-center rounded border border-border bg-surface-2 px-1 font-sans text-[10px] font-medium text-subtle">
      {children}
    </kbd>
  )
}

export function Spinner({ className }: { className?: string }) {
  return <Loader2 className={clsx('size-4 animate-spin text-subtle', className)} />
}

export function EmptyState({ icon, title, description, action }: { icon?: ReactNode; title: string; description?: ReactNode; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center px-6 py-10 text-center">
      {icon && <div className="mb-3 flex size-10 items-center justify-center rounded-full bg-surface-2 text-subtle">{icon}</div>}
      <p className="font-medium">{title}</p>
      {description && <p className="mt-1 max-w-sm text-[13px] text-muted">{description}</p>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  )
}

export function Switch({ checked, onChange, label, disabled }: { checked: boolean; onChange: (v: boolean) => void; label: string; disabled?: boolean }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={clsx(
        'relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors disabled:opacity-50',
        checked ? 'bg-accent' : 'bg-surface-3 ring-1 ring-border-strong ring-inset',
      )}
    >
      <span className={clsx('inline-block size-4 rounded-full bg-white shadow transition-transform', checked ? 'translate-x-[18px]' : 'translate-x-0.5')} />
    </button>
  )
}

export function Segmented<T extends string>({ value, onChange, options, className }: { value: T; onChange: (v: T) => void; options: { value: T; label: ReactNode }[]; className?: string }) {
  return (
    <div className={clsx('inline-flex rounded-md border border-border bg-surface-2 p-0.5', className)}>
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          onClick={() => onChange(o.value)}
          className={clsx(
            'h-7 flex-1 rounded px-2.5 text-[12px] font-medium whitespace-nowrap transition-colors lg:h-6',
            value === o.value ? 'bg-surface text-text shadow-card' : 'text-muted hover:text-text',
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}

export function Progress({ value, className, tone = 'accent' }: { value: number; className?: string; tone?: 'accent' | 'success' | 'warning' | 'danger' }) {
  return (
    <div className={clsx('h-1.5 overflow-hidden rounded-full bg-surface-3', className)}>
      <div
        className={clsx('h-full rounded-full transition-[width] duration-500', { accent: 'bg-accent', success: 'bg-success', warning: 'bg-warning', danger: 'bg-danger' }[tone])}
        style={{ width: `${Math.min(100, Math.max(0, value * 100))}%` }}
      />
    </div>
  )
}

export function Stat({ label, value, sub, tone }: { label: string; value: ReactNode; sub?: ReactNode; tone?: 'danger' | 'success' }) {
  return (
    <div className="min-w-0">
      <div className="truncate text-[11px] font-medium tracking-wide text-subtle uppercase">{label}</div>
      <div className={clsx('mt-1 text-xl font-semibold tabular tracking-tight', tone === 'danger' && 'text-danger', tone === 'success' && 'text-success')}>{value}</div>
      {sub && <div className="mt-0.5 truncate text-[12px] text-muted">{sub}</div>}
    </div>
  )
}
