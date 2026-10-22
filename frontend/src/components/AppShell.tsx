import * as RD from '@radix-ui/react-dialog'
import { useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import {
  CalendarRange,
  ChefHat,
  Home,
  KanbanSquare,
  ListChecks,
  LogOut,
  Menu as MenuIcon,
  Package,
  ShoppingCart,
  UtensilsCrossed,
  Plus,
  Repeat,
  RotateCcw,
  Search,
  Settings,
  WifiOff,
  Wallet,
} from 'lucide-react'
import { useEffect, useMemo, type ReactNode } from 'react'
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { post } from '@/lib/api'
import { useHotkeys, useOnline } from '@/lib/hooks'
import { syncPush } from '@/lib/push'
import { clearLocalData } from '@/lib/queryClient'
import { useRealtime, type LiveState } from '@/lib/realtime'
import { useSession } from '@/lib/session'
import { openQuickAdd, openQuickExpense, palette } from '@/lib/ui'
import { AddExpenseDialog } from '@/features/money/AddExpenseDialog'
import { QuickAddTask } from '@/features/tasks/QuickAddTask'
import { TaskDrawer } from '@/features/tasks/TaskDrawer'
import { CommandPalette } from './CommandPalette'
import { Menu, MenuItem, MenuLabel, MenuSeparator } from './Menu'
import { NotificationBell } from './NotificationBell'
import { Avatar, Kbd } from './ui'

type NavItem = { to: string; label: string; icon: typeof Home; soon?: boolean }

const workspace: NavItem[] = [
  { to: '/', label: 'Home', icon: Home },
  { to: '/board', label: 'Board', icon: KanbanSquare },
  { to: '/backlog', label: 'Planning', icon: ListChecks },
  { to: '/sprints', label: 'Sprints', icon: CalendarRange },
  { to: '/recurring', label: 'Recurring chores', icon: Repeat },
]
const household: NavItem[] = [
  { to: '/meals', label: 'Meal plan', icon: UtensilsCrossed },
  { to: '/recipes', label: 'Recipes', icon: ChefHat },
  { to: '/grocery', label: 'Grocery list', icon: ShoppingCart },
  { to: '/pantry', label: 'Pantry', icon: Package },
  { to: '/budget', label: 'Budget', icon: Wallet },
  { to: '/retro', label: 'Retro', icon: RotateCcw, soon: true },
]
const tabs: NavItem[] = [
  { to: '/', label: 'Home', icon: Home },
  { to: '/board', label: 'Board', icon: KanbanSquare },
  { to: '/backlog', label: 'Plan', icon: ListChecks },
  { to: '/meals', label: 'Kitchen', icon: UtensilsCrossed },
]

export function AppShell() {
  const session = useSession()
  const live = useRealtime(true)
  const online = useOnline()
  const location = useLocation()

  // Keep this device's push subscription fresh on every launch.
  useEffect(() => {
    syncPush(session.vapidPublicKey)
  }, [session.vapidPublicKey, session.user.id])

  // The service worker tells us when a push arrived, or that a notification was tapped.
  const navigate = useNavigate()
  const qc = useQueryClient()
  useEffect(() => {
    const onMsg = (e: MessageEvent) => {
      if (e.data?.type === 'navigate') navigate(e.data.url)
      if (e.data?.type === 'push') qc.invalidateQueries({ queryKey: ['notifications'] })
    }
    navigator.serviceWorker?.addEventListener('message', onMsg)
    return () => navigator.serviceWorker?.removeEventListener('message', onMsg)
  }, [navigate, qc])

  const keys = useMemo(
    () => ({
      'mod+k': () => palette.set(!palette.get()),
      '/': () => palette.set(true),
      c: () => openQuickAdd(),
      e: () => openQuickExpense(),
    }),
    [],
  )
  useHotkeys(keys)

  const fullHeight = location.pathname === '/board'

  return (
    <div className="min-h-dvh lg:pl-56">
      <Sidebar live={live} />
      <div className="flex min-h-dvh flex-col">
        <Topbar />
        {!online && (
          <div className="flex items-center justify-center gap-2 bg-warning-soft px-4 py-1.5 text-[12px] font-medium text-warning">
            <WifiOff className="size-3.5" /> Offline — showing saved data. Changes will sync when you reconnect.
          </div>
        )}
        <main className={clsx('mx-auto w-full max-w-[1400px] flex-1 px-4 pt-4 pb-28 lg:px-6 lg:pt-5 lg:pb-8', fullHeight && 'lg:flex lg:h-[calc(100dvh-48px)] lg:flex-col lg:overflow-hidden')}>
          <Outlet />
        </main>
      </div>
      <MobileTabs />
      <button
        onClick={() => (location.pathname.startsWith('/budget') ? openQuickExpense() : openQuickAdd())}
        aria-label={location.pathname.startsWith('/budget') ? 'Add expense' : 'New task'}
        className="fixed right-4 bottom-[calc(72px+env(safe-area-inset-bottom))] z-30 flex size-13 items-center justify-center rounded-full bg-accent text-accent-fg shadow-pop active:scale-95 lg:hidden"
      >
        <Plus className="size-6" />
      </button>
      <QuickAddTask />
      <AddExpenseDialog />
      <TaskDrawer />
      <CommandPalette />
    </div>
  )
}

function Sidebar({ live }: { live: LiveState }) {
  const { household: h } = useSession()
  return (
    <aside className="fixed inset-y-0 left-0 z-20 hidden w-56 flex-col border-r border-border bg-surface lg:flex">
      <div className="flex h-12 items-center gap-2.5 border-b border-border px-4">
        <img src="/logo.svg" alt="" className="size-6" />
        <div className="min-w-0">
          <div className="truncate text-[13px] leading-tight font-semibold">HomeOps</div>
          <div className="truncate text-[11px] leading-tight text-subtle">{h.name}</div>
        </div>
        <LiveDot state={live} />
      </div>
      <nav className="flex-1 space-y-5 overflow-y-auto px-2 py-3 scrollbar-thin">
        <NavGroup title="Workspace" items={workspace} />
        <NavGroup title="Household" items={household} />
      </nav>
      <div className="border-t border-border p-2">
        <SideLink item={{ to: '/settings', label: 'Settings', icon: Settings }} />
      </div>
    </aside>
  )
}

function LiveDot({ state }: { state: LiveState }) {
  const label = { live: 'Live — changes sync instantly', connecting: 'Reconnecting…', offline: 'Offline' }[state]
  return (
    <span title={label} className="ml-auto flex size-4 items-center justify-center">
      <span className={clsx('size-2 rounded-full', state === 'live' ? 'bg-success' : state === 'connecting' ? 'animate-pulse bg-warning' : 'bg-subtle')} />
    </span>
  )
}

function NavGroup({ title, items }: { title: string; items: NavItem[] }) {
  return (
    <div>
      <div className="px-2 pb-1 text-[11px] font-medium tracking-wide text-subtle uppercase">{title}</div>
      <div className="space-y-px">
        {items.map((i) => (
          <SideLink key={i.to} item={i} />
        ))}
      </div>
    </div>
  )
}

function SideLink({ item }: { item: NavItem }) {
  return (
    <NavLink
      to={item.to}
      end={item.to === '/'}
      className={({ isActive }) =>
        clsx(
          'flex h-8 items-center gap-2.5 rounded-md px-2 text-[13px] transition-colors',
          isActive ? 'bg-accent-soft font-medium text-accent' : 'text-muted hover:bg-surface-2 hover:text-text',
        )
      }
    >
      <item.icon className="size-4 shrink-0" />
      <span className="flex-1 truncate">{item.label}</span>
      {item.soon && <span className="rounded bg-surface-3 px-1 text-[10px] text-subtle">Soon</span>}
    </NavLink>
  )
}

function Topbar() {
  return (
    <header className="sticky top-0 z-20 border-b border-border bg-surface/90 pt-safe backdrop-blur supports-[backdrop-filter]:bg-surface/75">
      <div className="flex h-12 items-center gap-2 px-3 lg:px-6">
        <div className="flex items-center gap-2 lg:hidden">
          <img src="/logo.svg" alt="" className="size-6" />
          <span className="text-[14px] font-semibold">HomeOps</span>
        </div>
        <button
          onClick={() => palette.set(true)}
          className="ml-auto flex h-8 items-center gap-2 rounded-md border border-border bg-surface-2 px-2.5 text-[13px] text-subtle hover:border-border-strong lg:ml-0 lg:w-80"
        >
          <Search className="size-3.5" />
          <span className="hidden lg:inline">Search or jump to…</span>
          <span className="ml-auto hidden items-center gap-0.5 lg:flex">
            <Kbd>Ctrl</Kbd>
            <Kbd>K</Kbd>
          </span>
        </button>
        <div className="flex items-center gap-1 lg:ml-auto">
          <NotificationBell />
          <UserMenu />
        </div>
      </div>
    </header>
  )
}

function UserMenu() {
  const { user } = useSession()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const signOut = async () => {
    await post('/auth/logout').catch(() => {})
    await clearLocalData()
    qc.invalidateQueries()
    navigate('/', { replace: true })
  }
  return (
    <Menu
      align="end"
      trigger={
        <button className="ml-1 rounded-full" aria-label="Account menu">
          <Avatar name={user.displayName} color={user.color} size={28} />
        </button>
      }
    >
      <MenuLabel>
        {user.displayName} · @{user.username}
      </MenuLabel>
      <MenuSeparator />
      <MenuItem icon={<Settings className="size-4" />} onSelect={() => navigate('/settings/profile')}>
        Settings
      </MenuItem>
      <MenuItem icon={<LogOut className="size-4" />} onSelect={signOut} danger>
        Sign out
      </MenuItem>
    </Menu>
  )
}

function MobileTabs() {
  return (
    <nav className="fixed inset-x-0 bottom-0 z-30 border-t border-border bg-surface/95 pb-safe backdrop-blur lg:hidden">
      <div className="grid h-14 grid-cols-5">
        {tabs.map((t) => (
          <TabLink key={t.to} to={t.to} label={t.label} icon={<t.icon className="size-5" />} />
        ))}
        <MoreSheet />
      </div>
    </nav>
  )
}

function TabLink({ to, label, icon }: { to: string; label: string; icon: ReactNode }) {
  const { pathname } = useLocation()
  const kitchen = to === '/meals' && ['/recipes', '/grocery', '/pantry'].some((k) => pathname.startsWith(k))
  return (
    <NavLink
      to={to}
      end={to === '/'}
      className={({ isActive }) => clsx('flex flex-col items-center justify-center gap-0.5 text-[10px] font-medium', isActive || kitchen ? 'text-accent' : 'text-subtle')}
    >
      {icon}
      {label}
    </NavLink>
  )
}

function MoreSheet() {
  const more = [...workspace.slice(3), ...household.filter((h) => h.to !== '/meals'), { to: '/settings', label: 'Settings', icon: Settings }]
  const kitchen = ['/recipes', '/grocery', '/pantry']
  const location = useLocation()
  const active = more.some((m) => location.pathname.startsWith(m.to) && !kitchen.some((k) => location.pathname.startsWith(k)))
  return (
    <RD.Root>
      <RD.Trigger className={clsx('flex flex-col items-center justify-center gap-0.5 text-[10px] font-medium', active ? 'text-accent' : 'text-subtle')}>
        <MenuIcon className="size-5" />
        More
      </RD.Trigger>
      <RD.Portal>
        <RD.Overlay className="fixed inset-0 z-50 bg-black/40" />
        <RD.Content className="fixed inset-x-0 bottom-0 z-50 rounded-t-xl border-t border-border bg-surface p-2 pb-safe shadow-pop outline-none data-[state=open]:animate-[sheet-up_180ms_cubic-bezier(.2,.9,.3,1)]">
          <RD.Title className="px-3 pt-2 pb-1 text-[12px] font-medium text-subtle">More</RD.Title>
          <RD.Description className="sr-only">More sections</RD.Description>
          <div className="grid grid-cols-3 gap-1 pb-2">
            {more.map((m) => (
              <RD.Close asChild key={m.to}>
                <NavLink to={m.to} className={({ isActive }) => clsx('flex flex-col items-center gap-1.5 rounded-lg px-2 py-3 text-[12px]', isActive ? 'bg-accent-soft text-accent' : 'text-muted active:bg-surface-2')}>
                  <m.icon className="size-5" />
                  {m.label}
                </NavLink>
              </RD.Close>
            ))}
          </div>
        </RD.Content>
      </RD.Portal>
    </RD.Root>
  )
}
