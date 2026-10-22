import * as RD from '@radix-ui/react-dialog'
import { Command } from 'cmdk'
import { Bell, CalendarRange, ChefHat, Home, KanbanSquare, ListChecks, Moon, Plus, Repeat, RotateCcw, Settings, Sun, Wallet } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { taskKey } from '@/lib/format'
import { useTasks } from '@/lib/tasks'
import { setTheme } from '@/lib/theme'
import { openQuickAdd, palette, useTaskParam } from '@/lib/ui'
import { StatusIcon } from '@/features/tasks/TaskBits'

const nav = [
  { to: '/', label: 'Home', icon: Home },
  { to: '/board', label: 'Board', icon: KanbanSquare },
  { to: '/backlog', label: 'Planning & backlog', icon: ListChecks },
  { to: '/sprints', label: 'Sprints', icon: CalendarRange },
  { to: '/recurring', label: 'Recurring chores', icon: Repeat },
  { to: '/meals', label: 'Meals', icon: ChefHat },
  { to: '/budget', label: 'Budget', icon: Wallet },
  { to: '/retro', label: 'Retro', icon: RotateCcw },
  { to: '/settings/notifications', label: 'Notification settings', icon: Bell },
  { to: '/settings/profile', label: 'Settings', icon: Settings },
]

export function CommandPalette() {
  const open = palette.use()
  const navigate = useNavigate()
  const { data: tasks = [] } = useTasks()
  const { open: openTask } = useTaskParam()
  const close = () => palette.set(false)
  const run = (fn: () => void) => {
    close()
    fn()
  }

  const itemCls = 'flex h-9 cursor-default items-center gap-2.5 rounded-md px-2.5 text-[13px] data-[selected=true]:bg-surface-2'

  return (
    <RD.Root open={open} onOpenChange={palette.set}>
      <RD.Portal>
        <RD.Overlay className="fixed inset-0 z-50 bg-black/30" />
        <RD.Content className="fixed top-[10vh] left-1/2 z-50 w-[min(600px,calc(100vw-16px))] -translate-x-1/2 overflow-hidden rounded-lg border border-border bg-surface shadow-pop outline-none data-[state=open]:animate-[pop-in_100ms_ease-out]">
          <RD.Title className="sr-only">Command palette</RD.Title>
          <RD.Description className="sr-only">Search tasks or jump to a page</RD.Description>
          <Command loop>
            <Command.Input autoFocus placeholder="Search tasks, or type a command…" className="h-12 w-full border-b border-border bg-transparent px-4 outline-none placeholder:text-subtle" />
            <Command.List className="max-h-[min(420px,60dvh)] overflow-y-auto p-1.5 scrollbar-thin">
              <Command.Empty className="py-8 text-center text-[13px] text-subtle">No results</Command.Empty>
              <Command.Group heading="Actions" className="[&_[cmdk-group-heading]]:px-2.5 [&_[cmdk-group-heading]]:py-1.5 [&_[cmdk-group-heading]]:text-[11px] [&_[cmdk-group-heading]]:text-subtle">
                <Command.Item className={itemCls} onSelect={() => run(() => openQuickAdd())}>
                  <Plus className="size-4 text-subtle" /> Create task <span className="ml-auto text-[11px] text-subtle">C</span>
                </Command.Item>
                <Command.Item className={itemCls} onSelect={() => run(() => setTheme('dark'))}>
                  <Moon className="size-4 text-subtle" /> Dark theme
                </Command.Item>
                <Command.Item className={itemCls} onSelect={() => run(() => setTheme('light'))}>
                  <Sun className="size-4 text-subtle" /> Light theme
                </Command.Item>
              </Command.Group>
              <Command.Group heading="Go to" className="[&_[cmdk-group-heading]]:px-2.5 [&_[cmdk-group-heading]]:py-1.5 [&_[cmdk-group-heading]]:text-[11px] [&_[cmdk-group-heading]]:text-subtle">
                {nav.map((n) => (
                  <Command.Item key={n.to} className={itemCls} onSelect={() => run(() => navigate(n.to))}>
                    <n.icon className="size-4 text-subtle" /> {n.label}
                  </Command.Item>
                ))}
              </Command.Group>
              <Command.Group heading="Tasks" className="[&_[cmdk-group-heading]]:px-2.5 [&_[cmdk-group-heading]]:py-1.5 [&_[cmdk-group-heading]]:text-[11px] [&_[cmdk-group-heading]]:text-subtle">
                {tasks.slice(0, 300).map((t) => (
                  <Command.Item key={t.id} value={`${taskKey(t.number)} ${t.title}`} className={itemCls} onSelect={() => run(() => openTask(t))}>
                    <StatusIcon status={t.status} />
                    <span className="w-12 shrink-0 font-mono text-[11px] text-subtle">{taskKey(t.number)}</span>
                    <span className="truncate">{t.title}</span>
                  </Command.Item>
                ))}
              </Command.Group>
            </Command.List>
          </Command>
        </RD.Content>
      </RD.Portal>
    </RD.Root>
  )
}
