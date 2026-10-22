import { startOfDay, subDays } from 'date-fns'

/** Most recent date (at or before d) falling on startDow (0 = Sunday), matching the server. */
export function weekStart(d: Date, startDow: number) {
  const offset = (d.getDay() - startDow + 7) % 7
  return startOfDay(subDays(d, offset))
}
