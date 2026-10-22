import { CalendarClock } from 'lucide-react'
import { Link } from 'react-router-dom'
import { Badge, Button, Card, CardHeader, EmptyState, PageHeader, Progress, Spinner } from '@/components/ui'
import { sprintRange, weekdays } from '@/lib/format'
import { useSession } from '@/lib/session'
import { useSprints } from '@/lib/sprints'
import type { Sprint } from '@/lib/types'

const tone = { active: 'success', planned: 'accent', completed: 'neutral' } as const

export function SprintsPage() {
  const { sprints, isLoading } = useSprints()
  const { household } = useSession()
  if (isLoading) return <Spinner className="mx-auto mt-16" />
  const completed = sprints.filter((s) => s.status === 'completed')
  const avgVelocity = completed.length ? Math.round(completed.slice(0, 4).reduce((s, x) => s + x.donePoints, 0) / Math.min(4, completed.length)) : null

  return (
    <div>
      <PageHeader
        title="Sprints"
        subtitle={`Weekly sprints start every ${weekdays[household.sprintStartDow]}. Planning: ${weekdays[household.planningDow]} at ${household.planningTime}.`}
        actions={
          <Link to="/backlog">
            <Button variant="primary">Open planning</Button>
          </Link>
        }
      />
      <div className="mb-4 grid grid-cols-2 gap-3 sm:grid-cols-3">
        <Card className="p-4">
          <div className="text-[11px] font-medium tracking-wide text-subtle uppercase">Completed sprints</div>
          <div className="mt-1 text-xl font-semibold tabular">{completed.length}</div>
        </Card>
        <Card className="p-4">
          <div className="text-[11px] font-medium tracking-wide text-subtle uppercase">Avg. velocity (last 4)</div>
          <div className="mt-1 text-xl font-semibold tabular">{avgVelocity ?? '—'} {avgVelocity !== null && <span className="text-[13px] font-normal text-subtle">pts</span>}</div>
        </Card>
        <Card className="col-span-2 p-4 sm:col-span-1">
          <div className="text-[11px] font-medium tracking-wide text-subtle uppercase">Auto-roll</div>
          <div className="mt-1 text-[13px]">{household.autoRoll ? 'On — sprints roll over automatically' : 'Off — start sprints manually'}</div>
        </Card>
      </div>
      <Card>
        <CardHeader title="History" subtitle="Burndown and velocity charts arrive with the retro in Phase 4." />
        {sprints.length === 0 ? (
          <EmptyState icon={<CalendarClock className="size-5" />} title="No sprints yet" />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[560px] text-left text-[13px]">
              <thead className="border-b border-border text-[11px] tracking-wide text-subtle uppercase">
                <tr>
                  <th className="px-4 py-2 font-medium">Sprint</th>
                  <th className="px-4 py-2 font-medium">Dates</th>
                  <th className="px-4 py-2 font-medium">Status</th>
                  <th className="px-4 py-2 text-right font-medium">Tasks</th>
                  <th className="px-4 py-2 text-right font-medium">Points</th>
                  <th className="w-40 px-4 py-2 font-medium">Completion</th>
                </tr>
              </thead>
              <tbody>
                {sprints.map((s) => (
                  <SprintRow key={s.id} s={s} />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </div>
  )
}

function SprintRow({ s }: { s: Sprint }) {
  const ratio = s.points ? s.donePoints / s.points : s.taskCount ? s.doneCount / s.taskCount : 0
  return (
    <tr className="border-b border-border last:border-b-0 hover:bg-surface-2">
      <td className="px-4 py-2.5 font-medium">
        Sprint {s.number}
        {s.goal && <div className="max-w-56 truncate text-[12px] font-normal text-subtle">🎯 {s.goal}</div>}
      </td>
      <td className="px-4 py-2.5 text-muted">{sprintRange(s)}</td>
      <td className="px-4 py-2.5">
        <Badge tone={tone[s.status]}>{s.status[0]!.toUpperCase() + s.status.slice(1)}</Badge>
      </td>
      <td className="px-4 py-2.5 text-right tabular">
        {s.doneCount}/{s.taskCount}
      </td>
      <td className="px-4 py-2.5 text-right tabular">
        {s.donePoints}/{s.points}
      </td>
      <td className="px-4 py-2.5">
        <div className="flex items-center gap-2">
          <Progress value={ratio} tone={ratio >= 0.8 ? 'success' : 'accent'} className="flex-1" />
          <span className="w-9 text-right text-[12px] text-muted tabular">{Math.round(ratio * 100)}%</span>
        </div>
      </td>
    </tr>
  )
}
