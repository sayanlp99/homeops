import { differenceInCalendarDays, format, parseISO } from 'date-fns'

const inr = new Intl.NumberFormat('en-IN', { style: 'currency', currency: 'INR', maximumFractionDigits: 0 })
export const formatINR = (n: number) => inr.format(n)

export const taskKey = (n: number) => (n > 0 ? `HO-${n}` : 'HO-…')

export const toISODate = (d: Date) => format(d, 'yyyy-MM-dd')
export const todayISO = () => toISODate(new Date())

/** DD/MM/YYYY, the Indian convention. */
export const formatDate = (iso: string) => format(parseISO(iso), 'dd/MM/yyyy')
export const formatShortDate = (iso: string) => format(parseISO(iso), 'd MMM')
export const formatDateTime = (iso: string) => format(parseISO(iso), 'dd/MM/yyyy, h:mm a')

export function relativeDay(iso: string): string {
  const diff = differenceInCalendarDays(parseISO(iso), new Date())
  if (diff === 0) return 'Today'
  if (diff === 1) return 'Tomorrow'
  if (diff === -1) return 'Yesterday'
  if (diff > 1 && diff < 7) return format(parseISO(iso), 'EEEE')
  return formatShortDate(iso)
}

export type DueState = 'overdue' | 'today' | 'soon' | 'later'
export function dueState(iso: string): DueState {
  const diff = differenceInCalendarDays(parseISO(iso), new Date())
  if (diff < 0) return 'overdue'
  if (diff === 0) return 'today'
  if (diff <= 2) return 'soon'
  return 'later'
}

export function timeAgo(iso: string): string {
  const s = Math.max(0, (Date.now() - parseISO(iso).getTime()) / 1000)
  if (s < 60) return 'just now'
  if (s < 3600) return `${Math.floor(s / 60)}m ago`
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`
  if (s < 7 * 86400) return `${Math.floor(s / 86400)}d ago`
  return formatShortDate(iso)
}

export const weekdays = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']
export const weekdaysShort = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

export function sprintRange(s: { startsOn: string; endsOn: string }) {
  return `${formatShortDate(s.startsOn)} – ${formatShortDate(s.endsOn)}`
}

export function initials(name: string) {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((p) => p[0]!.toUpperCase())
    .join('')
}
