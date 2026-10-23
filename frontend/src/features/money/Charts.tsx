import clsx from 'clsx'
import { format, parse } from 'date-fns'
import { AlertTriangle, CheckCircle2, XCircle } from 'lucide-react'
import { useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { rupees, rupeesCompact } from '@/lib/money'

// Charts follow the dataviz spec: 2px lines, 10% area wash, hairline solid gridlines,
// thin bars with 4px rounded data-ends, text in ink tokens (never series colour),
// a tooltip on every chart, and a legend whenever there are two or more series.

export function useWidth<T extends HTMLElement>() {
  const ref = useRef<T>(null)
  const [w, setW] = useState(0)
  useLayoutEffect(() => {
    if (!ref.current) return
    const ro = new ResizeObserver(([e]) => setW(e!.contentRect.width))
    ro.observe(ref.current)
    return () => ro.disconnect()
  }, [])
  return [ref, w] as const
}

export function niceMax(v: number) {
  if (v <= 0) return 100
  const p = Math.pow(10, Math.floor(Math.log10(v)))
  for (const m of [1, 1.5, 2, 2.5, 3, 4, 5, 6, 8, 10]) if (m * p >= v) return m * p
  return 10 * p
}

export function Tooltip({ x, y, width, children }: { x: number; y: number; width: number; children: ReactNode }) {
  const left = Math.min(Math.max(x - 80, 0), width - 160)
  return (
    <div className="pointer-events-none absolute z-10 w-40 rounded-md border border-border bg-surface px-2.5 py-2 text-[12px] shadow-pop" style={{ left, top: Math.max(0, y - 70) }}>
      {children}
    </div>
  )
}

export function LegendKey({ color, dashed, label }: { color: string; dashed?: boolean; label: string }) {
  return (
    <span className="flex items-center gap-1.5 text-[12px] text-muted">
      <svg width="16" height="8" aria-hidden>
        <line x1="1" y1="4" x2="15" y2="4" stroke={color} strokeWidth="2" strokeLinecap="round" strokeDasharray={dashed ? '3 3' : undefined} />
      </svg>
      {label}
    </span>
  )
}

/**
 * Cumulative spend this month against an even pace to the budget. The even-pace line
 * is a reference, drawn in a neutral tone so the data line is the loud one.
 */
export function PaceChart({ daily, budget, month }: { daily: number[]; budget: number; month: string }) {
  const [ref, width] = useWidth<HTMLDivElement>()
  const [hover, setHover] = useState<number | null>(null)
  const h = 180
  const pad = { l: 44, r: 12, t: 12, b: 22 }
  const days = daily.length
  const now = new Date()
  const isCurrent = month === format(now, 'yyyy-MM')
  const lastDay = isCurrent ? now.getDate() : days
  const cum: number[] = []
  daily.slice(0, lastDay).reduce((acc, v, i) => ((cum[i] = acc + v), acc + v), 0)
  const yMax = niceMax(Math.max(budget, cum[cum.length - 1] ?? 0) * 1.05)
  const iw = Math.max(0, width - pad.l - pad.r)
  const ih = h - pad.t - pad.b
  const x = (d: number) => pad.l + (iw * d) / Math.max(1, days - 1)
  const y = (v: number) => pad.t + ih - (ih * v) / yMax
  const line = cum.map((v, i) => `${i ? 'L' : 'M'}${x(i).toFixed(1)},${y(v).toFixed(1)}`).join('')
  const area = cum.length ? `${line}L${x(cum.length - 1)},${y(0)}L${x(0)},${y(0)}Z` : ''
  const ticks = [0, yMax / 2, yMax]
  const hi = hover ?? null

  return (
    <div>
      <div className="mb-2 flex flex-wrap gap-4">
        <LegendKey color="var(--series-1)" label="Spent so far" />
        {budget > 0 && <LegendKey color="var(--viz-ref)" dashed label="Even pace to budget" />}
      </div>
      <div ref={ref} className="relative" onMouseLeave={() => setHover(null)}>
        {width > 0 && (
          <svg width={width} height={h} role="img" aria-label="Cumulative spending this month">
            {ticks.map((t) => (
              <g key={t}>
                <line x1={pad.l} x2={width - pad.r} y1={y(t)} y2={y(t)} stroke="var(--viz-grid)" strokeWidth="1" />
                <text x={pad.l - 6} y={y(t) + 4} textAnchor="end" className="fill-[var(--subtle)] text-[10px] tabular">
                  {rupeesCompact(t)}
                </text>
              </g>
            ))}
            {[1, Math.ceil(days / 2), days].map((d) => (
              <text key={d} x={x(d - 1)} y={h - 6} textAnchor="middle" className="fill-[var(--subtle)] text-[10px] tabular">
                {d}
              </text>
            ))}
            {budget > 0 && <line x1={x(0)} y1={y(budget / days)} x2={x(days - 1)} y2={y(budget)} stroke="var(--viz-ref)" strokeWidth="1.5" strokeDasharray="4 4" />}
            {area && <path d={area} fill="var(--series-1)" opacity="0.1" />}
            {line && <path d={line} fill="none" stroke="var(--series-1)" strokeWidth="2" strokeLinejoin="round" strokeLinecap="round" />}
            {cum.length > 0 && hi === null && <circle cx={x(cum.length - 1)} cy={y(cum[cum.length - 1]!)} r="4" fill="var(--series-1)" stroke="var(--surface)" strokeWidth="2" />}
            {hi !== null && hi < cum.length && (
              <>
                <line x1={x(hi)} x2={x(hi)} y1={pad.t} y2={pad.t + ih} stroke="var(--border-strong)" strokeWidth="1" />
                <circle cx={x(hi)} cy={y(cum[hi]!)} r="4" fill="var(--series-1)" stroke="var(--surface)" strokeWidth="2" />
              </>
            )}
            {/* Hit area: one column per day, wider than the mark. */}
            {Array.from({ length: days }, (_, d) => (
              <rect key={d} x={x(d) - iw / days / 2} y={pad.t} width={iw / days} height={ih} fill="transparent" onMouseEnter={() => setHover(d)} onTouchStart={() => setHover(d)} />
            ))}
          </svg>
        )}
        {hi !== null && hi < cum.length && (
          <Tooltip x={x(hi)} y={y(cum[hi]!)} width={width}>
            <div className="font-medium text-text">{format(parse(`${month}-${String(hi + 1).padStart(2, '0')}`, 'yyyy-MM-dd', new Date()), 'EEE d MMM')}</div>
            <div className="mt-1 flex justify-between text-muted">
              Spent that day <span className="text-text tabular">{rupees(daily[hi]!)}</span>
            </div>
            <div className="flex justify-between text-muted">
              Month so far <span className="text-text tabular">{rupees(cum[hi]!)}</span>
            </div>
            {budget > 0 && (
              <div className="flex justify-between text-muted">
                Even pace <span className="text-text tabular">{rupees((budget * (hi + 1)) / days)}</span>
              </div>
            )}
          </Tooltip>
        )}
      </div>
    </div>
  )
}

/** Six months of spending as columns; the current month is labelled at its cap. */
export function TrendChart({ trend, current }: { trend: { month: string; spent: number }[]; current: string }) {
  const [ref, width] = useWidth<HTMLDivElement>()
  const [hover, setHover] = useState<number | null>(null)
  const h = 150
  const pad = { l: 44, r: 8, t: 18, b: 22 }
  const max = niceMax(Math.max(...trend.map((t) => t.spent), 1))
  const iw = Math.max(0, width - pad.l - pad.r)
  const ih = h - pad.t - pad.b
  const band = iw / Math.max(1, trend.length)
  const bw = Math.min(24, band * 0.6)
  const y = (v: number) => pad.t + ih - (ih * v) / max
  return (
    <div ref={ref} className="relative" onMouseLeave={() => setHover(null)}>
      {width > 0 && (
        <svg width={width} height={h} role="img" aria-label="Monthly spending, last six months">
          {[0, max / 2, max].map((t) => (
            <g key={t}>
              <line x1={pad.l} x2={width - pad.r} y1={y(t)} y2={y(t)} stroke="var(--viz-grid)" strokeWidth="1" />
              <text x={pad.l - 6} y={y(t) + 4} textAnchor="end" className="fill-[var(--subtle)] text-[10px] tabular">
                {rupeesCompact(t)}
              </text>
            </g>
          ))}
          {trend.map((t, i) => {
            const cx = pad.l + band * i + band / 2
            const top = y(t.spent)
            const bh = pad.t + ih - top
            const r = Math.min(4, bh)
            return (
              <g key={t.month} onMouseEnter={() => setHover(i)} onTouchStart={() => setHover(i)}>
                <rect x={cx - band / 2} y={pad.t} width={band} height={ih} fill="transparent" />
                {bh > 0 && (
                  <path
                    d={`M${cx - bw / 2},${pad.t + ih}V${top + r}Q${cx - bw / 2},${top} ${cx - bw / 2 + r},${top}H${cx + bw / 2 - r}Q${cx + bw / 2},${top} ${cx + bw / 2},${top + r}V${pad.t + ih}Z`}
                    fill="var(--series-1)"
                    opacity={t.month === current || hover === i ? 1 : 0.55}
                  />
                )}
                {t.month === current && t.spent > 0 && (
                  <text x={cx} y={top - 5} textAnchor="middle" className="fill-[var(--muted)] text-[10px] font-medium tabular">
                    {rupeesCompact(t.spent)}
                  </text>
                )}
                <text x={cx} y={h - 6} textAnchor="middle" className="fill-[var(--subtle)] text-[10px]">
                  {format(parse(t.month, 'yyyy-MM', new Date()), 'MMM')}
                </text>
              </g>
            )
          })}
        </svg>
      )}
      {hover !== null && trend[hover] && (
        <Tooltip x={pad.l + band * hover + band / 2} y={y(trend[hover]!.spent)} width={width}>
          <div className="font-medium text-text">{format(parse(trend[hover]!.month, 'yyyy-MM', new Date()), 'MMMM yyyy')}</div>
          <div className="mt-1 flex justify-between text-muted">
            Spent <span className="text-text tabular">{rupees(trend[hover]!.spent)}</span>
          </div>
        </Tooltip>
      )}
    </div>
  )
}

/** Share of spending by who paid: one bar, a 2px surface gap between segments, legend below. */
export function SplitBar({ parts }: { parts: { label: string; value: number; color: string }[] }) {
  const total = parts.reduce((s, p) => s + p.value, 0)
  const [hover, setHover] = useState<number | null>(null)
  if (total <= 0) return <p className="text-[13px] text-subtle">No spending yet this month.</p>
  return (
    <div>
      <div className="flex h-3 gap-[2px] overflow-hidden rounded" onMouseLeave={() => setHover(null)}>
        {parts.filter((p) => p.value > 0).map((p, i) => (
          <div
            key={p.label}
            onMouseEnter={() => setHover(i)}
            title={`${p.label}: ${rupees(p.value)} (${Math.round((p.value / total) * 100)}%)`}
            className={clsx('h-full transition-opacity', hover !== null && hover !== i && 'opacity-50')}
            style={{ width: `${(p.value / total) * 100}%`, background: p.color }}
          />
        ))}
      </div>
      <ul className="mt-2.5 space-y-1.5">
        {parts.map((p) => (
          <li key={p.label} className="flex items-center gap-2 text-[13px]">
            <span className="size-2.5 shrink-0 rounded-sm" style={{ background: p.color }} />
            <span className="min-w-0 flex-1 truncate text-muted">{p.label}</span>
            <span className="tabular">{rupees(p.value)}</span>
            <span className="w-10 text-right text-[12px] text-subtle tabular">{Math.round((p.value / total) * 100)}%</span>
          </li>
        ))}
      </ul>
    </div>
  )
}

export type MeterState = 'ok' | 'pace' | 'warn' | 'over' | 'none'

/** How a category is doing: over budget, ahead of an even pace, or fine. */
export function meterState(spent: number, available: number, paceFraction: number): MeterState {
  if (available <= 0) return 'none'
  const used = spent / available
  if (used > 1) return 'over'
  if (used >= 0.8) return 'warn'
  if (used > paceFraction + 0.1) return 'pace'
  return 'ok'
}

/** Budget meter: fill severity on a lighter track; status always has an icon + words. */
export function Meter({ spent, available, state }: { spent: number; available: number; state: MeterState }) {
  const frac = available > 0 ? Math.min(1, spent / available) : 0
  const fill = { ok: 'bg-accent', pace: 'bg-accent', warn: 'bg-warning', over: 'bg-danger', none: 'bg-surface-3' }[state]
  return (
    <div className={clsx('h-1.5 overflow-hidden rounded-full', state === 'over' ? 'bg-danger-soft' : state === 'warn' ? 'bg-warning-soft' : 'bg-accent-soft')}>
      <div className={clsx('h-full rounded-full transition-[width] duration-500', fill)} style={{ width: `${frac * 100}%` }} />
    </div>
  )
}

export function MeterLabel({ state, left }: { state: MeterState; left: number }) {
  if (state === 'none') return <span className="text-[12px] text-subtle">No budget set</span>
  if (state === 'over')
    return (
      <span className="flex items-center gap-1 text-[12px] font-medium text-danger">
        <XCircle className="size-3.5" /> Over by {rupees(-left)}
      </span>
    )
  if (state === 'warn')
    return (
      <span className="flex items-center gap-1 text-[12px] font-medium text-warning">
        <AlertTriangle className="size-3.5" /> {rupees(left)} left
      </span>
    )
  if (state === 'pace')
    return (
      <span className="flex items-center gap-1 text-[12px] text-muted">
        <AlertTriangle className="size-3.5 text-warning" /> Ahead of pace · {rupees(left)} left
      </span>
    )
  return (
    <span className="flex items-center gap-1 text-[12px] text-muted">
      <CheckCircle2 className="size-3.5 text-success" /> {rupees(left)} left
    </span>
  )
}
