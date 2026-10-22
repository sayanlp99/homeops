import { differenceInCalendarDays, parseISO } from 'date-fns'
import { ArrowRight, Bell, CalendarRange, ChefHat, Plus, Wallet } from 'lucide-react'
import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import { Avatar, Button, Card, CardHeader, EmptyState, Progress, Stat } from '@/components/ui'
import { sprintRange, todayISO } from '@/lib/format'
import { pushSupport } from '@/lib/push'
import { useSession } from '@/lib/session'
import { useSprints } from '@/lib/sprints'
import { byRank, useTasks } from '@/lib/tasks'
import { openQuickAdd } from '@/lib/ui'
import { TaskRow } from '../tasks/TaskList'

function greeting() {
  const h = new Date().getHours()
  return h < 12 ? 'Good morning' : h < 17 ? 'Good afternoon' : 'Good evening'
}

export function HomePage() {
  const { user, members, partner } = useSession()
  const { active } = useSprints()
  const { data: tasks = [] } = useTasks()
  const today = todayISO()

  const v = useMemo(() => {
    const open = tasks.filter((t) => t.status !== 'done' && t.status !== 'backlog')
    const mine = open.filter((t) => t.assigneeId === user.id || !t.assigneeId)
    const sprint = active ? tasks.filter((t) => t.sprintId === active.id) : []
    return {
      dueToday: mine.filter((t) => t.dueDate === today).sort(byRank),
      overdue: open.filter((t) => t.dueDate && t.dueDate < today).sort((a, b) => a.dueDate!.localeCompare(b.dueDate!)),
      upcoming: mine
        .filter((t) => t.status !== 'in_progress' && t.dueDate && t.dueDate > today && differenceInCalendarDays(parseISO(t.dueDate), new Date()) <= 7)
        .sort((a, b) => a.dueDate!.localeCompare(b.dueDate!))
        .slice(0, 8),
      inProgress: mine.filter((t) => t.status === 'in_progress' && t.dueDate !== today),
      mineOpen: mine.length,
      sprint,
      sprintDone: sprint.filter((t) => t.status === 'done').length,
      sprintPts: sprint.reduce((s, t) => s + (t.points ?? 0), 0),
      sprintDonePts: sprint.filter((t) => t.status === 'done').reduce((s, t) => s + (t.points ?? 0), 0),
    }
  }, [tasks, user.id, active, today])

  const showPushNudge = pushSupport() !== 'unsupported' && (typeof Notification === 'undefined' || Notification.permission !== 'granted')

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-lg font-semibold tracking-tight">
            {greeting()}, {user.displayName.split(' ')[0]}
          </h1>
          <p className="text-[13px] text-muted">{new Date().toLocaleDateString('en-IN', { weekday: 'long', day: 'numeric', month: 'long' })}</p>
        </div>
        <Button variant="primary" className="max-lg:hidden" icon={<Plus className="size-4" />} onClick={() => openQuickAdd()}>
          New task
        </Button>
      </div>

      {showPushNudge && (
        <Card className="flex items-center gap-3 border-accent/30 bg-accent-soft p-3">
          <Bell className="size-4 shrink-0 text-accent" />
          <p className="min-w-0 flex-1 text-[13px]">
            {pushSupport() === 'needs-install' ? 'Add HomeOps to your Home Screen to get reminders on this iPhone.' : 'Turn on notifications to get task and planning reminders on this device.'}
          </p>
          <Link to="/settings/notifications">
            <Button size="sm" variant="primary">
              Set up
            </Button>
          </Link>
        </Card>
      )}

      <Card className="grid grid-cols-2 gap-4 p-4 lg:grid-cols-4">
        <Stat label="Due today" value={v.dueToday.length} sub="assigned to you" />
        <Stat label="Overdue" value={v.overdue.length} tone={v.overdue.length ? 'danger' : undefined} sub="household" />
        <Stat label="Your open tasks" value={v.mineOpen} sub="in sprints" />
        <div className="min-w-0">
          <div className="text-[11px] font-medium tracking-wide text-subtle uppercase">{active ? `Sprint ${active.number}` : 'Sprint'}</div>
          <div className="mt-1 text-xl font-semibold tabular">
            {v.sprintPts ? `${v.sprintDonePts}/${v.sprintPts}` : `${v.sprintDone}/${v.sprint.length}`}
            <span className="ml-1 text-[12px] font-normal text-subtle">{v.sprintPts ? 'pts' : 'done'}</span>
          </div>
          <Progress className="mt-2" tone="success" value={v.sprintPts ? v.sprintDonePts / v.sprintPts : v.sprint.length ? v.sprintDone / v.sprint.length : 0} />
        </div>
      </Card>

      <div className="grid gap-4 lg:grid-cols-3">
        <div className="space-y-4 lg:col-span-2">
          {v.overdue.length > 0 && (
            <Card className="overflow-hidden">
              <CardHeader title={<span className="text-danger">Overdue</span>} subtitle="Reschedule or knock these out first" />
              {v.overdue.map((t) => (
                <TaskRow key={t.id} task={t} />
              ))}
            </Card>
          )}
          <Card className="overflow-hidden">
            <CardHeader
              title="Today"
              subtitle={`${v.dueToday.length + v.inProgress.length} on your plate`}
              actions={
                <Link to="/board?mine=1" className="flex items-center gap-1 text-[12px] text-accent hover:underline">
                  Board <ArrowRight className="size-3" />
                </Link>
              }
            />
            {v.dueToday.length + v.inProgress.length === 0 ? (
              <EmptyState title="Nothing due today 🎉" description="Pick something from the board, or enjoy the free time." />
            ) : (
              <>
                {v.inProgress.map((t) => (
                  <TaskRow key={t.id} task={t} />
                ))}
                {v.dueToday.map((t) => (
                  <TaskRow key={t.id} task={t} />
                ))}
              </>
            )}
          </Card>
          <Card className="overflow-hidden">
            <CardHeader title="Next 7 days" />
            {v.upcoming.length === 0 ? <p className="px-4 py-6 text-center text-[13px] text-subtle">Nothing scheduled this week.</p> : v.upcoming.map((t) => <TaskRow key={t.id} task={t} />)}
          </Card>
        </div>

        <div className="space-y-4">
          {active && (
            <Card>
              <CardHeader title={`Sprint ${active.number}`} subtitle={sprintRange(active)} actions={<CalendarRange className="size-4 text-subtle" />} />
              <div className="space-y-3 p-4">
                {active.goal && <p className="text-[13px]">🎯 {active.goal}</p>}
                {members.map((m) => {
                  const theirs = v.sprint.filter((t) => t.assigneeId === m.id)
                  const done = theirs.filter((t) => t.status === 'done').length
                  return (
                    <div key={m.id} className="flex items-center gap-2.5">
                      <Avatar name={m.displayName} color={m.color} size={24} />
                      <div className="min-w-0 flex-1">
                        <div className="flex justify-between text-[12px]">
                          <span className="font-medium">{m.displayName}</span>
                          <span className="text-muted tabular">
                            {done}/{theirs.length}
                          </span>
                        </div>
                        <Progress className="mt-1" value={theirs.length ? done / theirs.length : 0} />
                      </div>
                    </div>
                  )
                })}
                {!partner && (
                  <Link to="/settings/household" className="block text-[12px] text-accent hover:underline">
                    Invite your partner →
                  </Link>
                )}
              </div>
            </Card>
          )}
          <Card>
            <CardHeader title="Today's menu" actions={<ChefHat className="size-4 text-subtle" />} />
            <p className="px-4 py-5 text-[13px] text-subtle">Meal planning arrives in Phase 2.</p>
          </Card>
          <Card>
            <CardHeader title="Budget this month" actions={<Wallet className="size-4 text-subtle" />} />
            <p className="px-4 py-5 text-[13px] text-subtle">Budgets arrive in Phase 3.</p>
          </Card>
        </div>
      </div>
    </div>
  )
}
