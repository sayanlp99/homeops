import { format, parseISO } from 'date-fns'
import { useState } from 'react'
import type { Burndown, TeamMetrics } from '@/lib/retro'
import type { Member } from '@/lib/types'
import { LegendKey, Tooltip, niceMax, useWidth } from '../money/Charts'

// Same mark specs as the money charts: 2px lines, hairline solid grid, thin columns with
// 4px rounded data-ends, ink-coloured text, a legend for 2+ series, a tooltip on hover.

const axisText = 'fill-[var(--subtle)] text-[10px] tabular'

function columnPath(cx: number, w: number, top: number, base: number, round: boolean) {
  const r = round ? Math.min(4, base - top) : 0
  return `M${cx - w / 2},${base}V${top + r}Q${cx - w / 2},${top} ${cx - w / 2 + r},${top}H${cx + w / 2 - r}Q${cx + w / 2},${top} ${cx + w / 2},${top + r}V${base}Z`
}

/** Work remaining per day against an even burn to zero. */
export function BurndownChart({ data }: { data: Burndown }) {
  const [ref, width] = useWidth<HTMLDivElement>()
  const [hover, setHover] = useState<number | null>(null)
  const h = 170
  const pad = { l: 34, r: 12, t: 10, b: 22 }
  const n = data.days.length
  const max = niceMax(Math.max(data.total, 1))
  const iw = Math.max(0, width - pad.l - pad.r)
  const ih = h - pad.t - pad.b
  const x = (i: number) => pad.l + (iw * i) / Math.max(1, n - 1)
  const y = (v: number) => pad.t + ih - (ih * v) / max
  const pts = data.days.map((d, i) => (d.remaining == null ? null : [x(i), y(d.remaining)] as const))
  const known = pts.filter(Boolean) as (readonly [number, number])[]
  const line = known.map((p, i) => `${i ? 'L' : 'M'}${p[0].toFixed(1)},${p[1].toFixed(1)}`).join('')
  const hd = hover !== null ? data.days[hover] : undefined
  return (
    <div>
      <div className="mb-2 flex flex-wrap gap-4">
        <LegendKey color="var(--series-1)" label={`${data.unit === 'points' ? 'Points' : 'Tasks'} remaining`} />
        <LegendKey color="var(--viz-ref)" dashed label="Ideal" />
      </div>
      <div ref={ref} className="relative" onMouseLeave={() => setHover(null)}>
        {width > 0 && (
          <svg width={width} height={h} role="img" aria-label={`Sprint ${data.sprintNumber} burndown`}>
            {[0, max / 2, max].map((t) => (
              <g key={t}>
                <line x1={pad.l} x2={width - pad.r} y1={y(t)} y2={y(t)} stroke="var(--viz-grid)" strokeWidth="1" />
                <text x={pad.l - 6} y={y(t) + 4} textAnchor="end" className={axisText}>
                  {Math.round(t)}
                </text>
              </g>
            ))}
            {data.days.map((d, i) => (
              <text key={d.date} x={x(i)} y={h - 6} textAnchor="middle" className={axisText}>
                {format(parseISO(d.date), 'EEEEE')}
              </text>
            ))}
            <line x1={x(0)} y1={y(data.total)} x2={x(n - 1)} y2={y(0)} stroke="var(--viz-ref)" strokeWidth="1.5" strokeDasharray="4 4" />
            {line && <path d={line} fill="none" stroke="var(--series-1)" strokeWidth="2" strokeLinejoin="round" strokeLinecap="round" />}
            {known.length > 0 && <circle cx={known[known.length - 1]![0]} cy={known[known.length - 1]![1]} r="4" fill="var(--series-1)" stroke="var(--surface)" strokeWidth="2" />}
            {hover !== null && pts[hover] && <circle cx={pts[hover]![0]} cy={pts[hover]![1]} r="4" fill="var(--series-1)" stroke="var(--surface)" strokeWidth="2" />}
            {data.days.map((d, i) => (
              <rect key={d.date} x={x(i) - iw / n / 2} y={pad.t} width={iw / n} height={ih} fill="transparent" onMouseEnter={() => setHover(i)} onTouchStart={() => setHover(i)} />
            ))}
          </svg>
        )}
        {hd && hover !== null && (
          <Tooltip x={x(hover)} y={pts[hover]?.[1] ?? y(0)} width={width}>
            <div className="font-medium text-text">{format(parseISO(hd.date), 'EEE d MMM')}</div>
            {hd.remaining == null ? (
              <div className="mt-1 text-muted">Still to come</div>
            ) : (
              <>
                <div className="mt-1 flex justify-between text-muted">
                  Remaining <span className="text-text tabular">{hd.remaining}</span>
                </div>
                <div className="flex justify-between text-muted">
                  Done that day <span className="text-text tabular">{hd.done}</span>
                </div>
              </>
            )}
          </Tooltip>
        )}
      </div>
    </div>
  )
}

/** Planned vs done per sprint: the done column is the story, planned is the reference. */
export function VelocityChart({ sprints }: { sprints: NonNullable<TeamMetrics['sprints']> }) {
  const [ref, width] = useWidth<HTMLDivElement>()
  const [hover, setHover] = useState<number | null>(null)
  const h = 170
  const pad = { l: 34, r: 8, t: 18, b: 22 }
  const max = niceMax(Math.max(1, ...sprints.map((s) => Math.max(s.planned, s.done))))
  const iw = Math.max(0, width - pad.l - pad.r)
  const ih = h - pad.t - pad.b
  const band = iw / Math.max(1, sprints.length)
  const bw = Math.min(16, band * 0.32)
  const y = (v: number) => pad.t + ih - (ih * v) / max
  const base = pad.t + ih
  const avg = sprints.filter((s) => s.status === 'completed').slice(-4)
  const avgDone = avg.length ? Math.round(avg.reduce((t, s) => t + s.done, 0) / avg.length) : null
  return (
    <div>
      <div className="mb-2 flex flex-wrap items-center gap-4">
        <span className="flex items-center gap-1.5 text-[12px] text-muted">
          <span className="size-2.5 rounded-sm bg-[var(--viz-ref)]" /> Planned
        </span>
        <span className="flex items-center gap-1.5 text-[12px] text-muted">
          <span className="size-2.5 rounded-sm bg-[var(--series-1)]" /> Done
        </span>
        {avgDone !== null && <span className="ml-auto text-[12px] text-muted">Average of last {avg.length}: {avgDone} done</span>}
      </div>
      <div ref={ref} className="relative" onMouseLeave={() => setHover(null)}>
        {width > 0 && (
          <svg width={width} height={h} role="img" aria-label="Velocity per sprint">
            {[0, max / 2, max].map((t) => (
              <g key={t}>
                <line x1={pad.l} x2={width - pad.r} y1={y(t)} y2={y(t)} stroke="var(--viz-grid)" strokeWidth="1" />
                <text x={pad.l - 6} y={y(t) + 4} textAnchor="end" className={axisText}>
                  {Math.round(t)}
                </text>
              </g>
            ))}
            {sprints.map((s, i) => {
              const cx = pad.l + band * i + band / 2
              return (
                <g key={s.id} onMouseEnter={() => setHover(i)} onTouchStart={() => setHover(i)} opacity={hover === null || hover === i ? 1 : 0.6}>
                  <rect x={cx - band / 2} y={pad.t} width={band} height={ih} fill="transparent" />
                  {s.planned > 0 && <path d={columnPath(cx - bw / 2 - 1, bw, y(s.planned), base, true)} fill="var(--viz-ref)" />}
                  {s.done > 0 && <path d={columnPath(cx + bw / 2 + 1, bw, y(s.done), base, true)} fill="var(--series-1)" />}
                  {i === sprints.length - 1 && s.done > 0 && (
                    <text x={cx + bw / 2 + 1} y={y(s.done) - 5} textAnchor="middle" className="fill-[var(--muted)] text-[10px] font-medium tabular">
                      {s.done}
                    </text>
                  )}
                  <text x={cx} y={h - 6} textAnchor="middle" className={axisText}>
                    S{s.number}
                  </text>
                </g>
              )
            })}
          </svg>
        )}
        {hover !== null && sprints[hover] && (
          <Tooltip x={pad.l + band * hover + band / 2} y={y(Math.max(sprints[hover]!.planned, sprints[hover]!.done))} width={width}>
            <div className="font-medium text-text">Sprint {sprints[hover]!.number}</div>
            <div className="mt-1 flex justify-between text-muted">
              Planned <span className="text-text tabular">{sprints[hover]!.planned}</span>
            </div>
            <div className="flex justify-between text-muted">
              Done <span className="text-text tabular">{sprints[hover]!.done}</span>
            </div>
          </Tooltip>
        )}
      </div>
    </div>
  )
}

/** Who did the work: stacked columns per sprint, one segment per person. */
export function FairnessChart({ sprints, members }: { sprints: NonNullable<TeamMetrics['sprints']>; members: Member[] }) {
  const [ref, width] = useWidth<HTMLDivElement>()
  const [hover, setHover] = useState<number | null>(null)
  const h = 170
  const pad = { l: 34, r: 8, t: 10, b: 22 }
  const totals = sprints.map((s) => members.reduce((t, m) => t + (s.byMember[m.id] ?? 0), 0))
  const max = niceMax(Math.max(1, ...totals))
  const iw = Math.max(0, width - pad.l - pad.r)
  const ih = h - pad.t - pad.b
  const band = iw / Math.max(1, sprints.length)
  const bw = Math.min(24, band * 0.55)
  const y = (v: number) => pad.t + ih - (ih * v) / max
  const base = pad.t + ih
  const overall = members.map((m) => ({ m, v: sprints.reduce((t, s) => t + (s.byMember[m.id] ?? 0), 0) }))
  const grand = overall.reduce((t, o) => t + o.v, 0)
  return (
    <div>
      <div className="mb-2 flex flex-wrap gap-4">
        {overall.map(({ m, v }) => (
          <span key={m.id} className="flex items-center gap-1.5 text-[12px] text-muted">
            <span className="size-2.5 rounded-sm" style={{ background: m.color }} /> {m.displayName}
            <span className="tabular">{grand ? `${Math.round((v / grand) * 100)}%` : ''}</span>
          </span>
        ))}
      </div>
      <div ref={ref} className="relative" onMouseLeave={() => setHover(null)}>
        {width > 0 && (
          <svg width={width} height={h} role="img" aria-label="Work done per person per sprint">
            {[0, max / 2, max].map((t) => (
              <g key={t}>
                <line x1={pad.l} x2={width - pad.r} y1={y(t)} y2={y(t)} stroke="var(--viz-grid)" strokeWidth="1" />
                <text x={pad.l - 6} y={y(t) + 4} textAnchor="end" className={axisText}>
                  {Math.round(t)}
                </text>
              </g>
            ))}
            {sprints.map((s, i) => {
              const cx = pad.l + band * i + band / 2
              let acc = 0
              const segs = members.map((m) => ({ m, v: s.byMember[m.id] ?? 0 })).filter((x) => x.v > 0)
              return (
                <g key={s.id} onMouseEnter={() => setHover(i)} onTouchStart={() => setHover(i)} opacity={hover === null || hover === i ? 1 : 0.6}>
                  <rect x={cx - band / 2} y={pad.t} width={band} height={ih} fill="transparent" />
                  {segs.map((seg, k) => {
                    const bottom = y(acc)
                    acc += seg.v
                    const top = y(acc)
                    const isTop = k === segs.length - 1
                    // 2px surface gap between stacked segments.
                    const segBase = k === 0 ? base : bottom - 2
                    return isTop ? (
                      <path key={seg.m.id} d={columnPath(cx, bw, top, segBase, true)} fill={seg.m.color} />
                    ) : (
                      <rect key={seg.m.id} x={cx - bw / 2} y={top} width={bw} height={Math.max(0, segBase - top)} fill={seg.m.color} />
                    )
                  })}
                  <text x={cx} y={h - 6} textAnchor="middle" className={axisText}>
                    S{s.number}
                  </text>
                </g>
              )
            })}
          </svg>
        )}
        {hover !== null && sprints[hover] && (
          <Tooltip x={pad.l + band * hover + band / 2} y={y(totals[hover]!)} width={width}>
            <div className="font-medium text-text">Sprint {sprints[hover]!.number}</div>
            {members.map((m) => (
              <div key={m.id} className="mt-1 flex items-center justify-between text-muted">
                <span className="flex items-center gap-1.5">
                  <span className="size-2 rounded-sm" style={{ background: m.color }} /> {m.displayName}
                </span>
                <span className="text-text tabular">{sprints[hover]!.byMember[m.id] ?? 0}</span>
              </div>
            ))}
          </Tooltip>
        )}
      </div>
    </div>
  )
}
