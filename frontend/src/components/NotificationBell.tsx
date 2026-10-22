import * as Popover from '@radix-ui/react-popover'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import clsx from 'clsx'
import { Bell, CheckCheck } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { get, post } from '@/lib/api'
import { timeAgo } from '@/lib/format'
import type { InboxItem } from '@/lib/types'
import { IconButton } from './ui'

export function useInbox() {
  return useQuery({ queryKey: ['notifications'], queryFn: () => get<{ items: InboxItem[]; unread: number }>('/notifications'), refetchInterval: 5 * 60_000 })
}

export function NotificationBell() {
  const { data } = useInbox()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const unread = data?.unread ?? 0

  // Mirror the unread count on the installed app icon.
  useEffect(() => {
    const nav = navigator as Navigator & { setAppBadge?: (n?: number) => Promise<void>; clearAppBadge?: () => Promise<void> }
    if (unread > 0) nav.setAppBadge?.(unread).catch(() => {})
    else nav.clearAppBadge?.().catch(() => {})
  }, [unread])

  const markRead = useMutation({
    mutationFn: (body: { ids?: string[]; all?: boolean }) => post('/notifications/read', body),
    onMutate: (body) =>
      qc.setQueryData<{ items: InboxItem[]; unread: number }>(['notifications'], (d) => {
        if (!d) return d
        const now = new Date().toISOString()
        const items = d.items.map((n) => (body.all || body.ids?.includes(n.id) ? { ...n, readAt: n.readAt ?? now } : n))
        return { items, unread: items.filter((n) => !n.readAt).length }
      }),
    onSettled: () => qc.invalidateQueries({ queryKey: ['notifications'] }),
  })

  const openItem = (n: InboxItem) => {
    if (!n.readAt) markRead.mutate({ ids: [n.id] })
    setOpen(false)
    navigate(n.url)
  }

  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Popover.Trigger asChild>
        <IconButton label={`Notifications${unread ? ` (${unread} unread)` : ''}`} className="relative">
          <Bell className="size-4" />
          {unread > 0 && (
            <span className="absolute top-1 right-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-danger px-1 text-[10px] font-semibold text-white tabular">
              {unread > 9 ? '9+' : unread}
            </span>
          )}
        </IconButton>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          align="end"
          sideOffset={6}
          collisionPadding={8}
          className="z-50 flex max-h-[70dvh] w-[min(380px,calc(100vw-16px))] flex-col rounded-lg border border-border bg-surface shadow-pop outline-none data-[state=open]:animate-[pop-in_100ms_ease-out]"
        >
          <div className="flex items-center justify-between border-b border-border px-3 py-2">
            <span className="text-[13px] font-semibold">Notifications</span>
            {unread > 0 && (
              <button onClick={() => markRead.mutate({ all: true })} className="flex items-center gap-1 text-[12px] text-accent hover:underline">
                <CheckCheck className="size-3.5" /> Mark all read
              </button>
            )}
          </div>
          <div className="min-h-0 overflow-y-auto scrollbar-thin">
            {!data?.items.length ? (
              <p className="px-4 py-10 text-center text-[13px] text-subtle">You're all caught up.</p>
            ) : (
              data.items.map((n) => (
                <button key={n.id} onClick={() => openItem(n)} className="flex w-full gap-2.5 border-b border-border px-3 py-2.5 text-left last:border-b-0 hover:bg-surface-2">
                  <span className={clsx('mt-1.5 size-1.5 shrink-0 rounded-full', n.readAt ? 'bg-transparent' : 'bg-accent')} />
                  <span className="min-w-0 flex-1">
                    <span className={clsx('block text-[13px]', !n.readAt && 'font-medium')}>{n.title}</span>
                    {n.body && <span className="mt-0.5 block truncate text-[12px] text-muted">{n.body}</span>}
                    <span className="mt-0.5 block text-[11px] text-subtle">{timeAgo(n.createdAt)}</span>
                  </span>
                </button>
              ))
            )}
          </div>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  )
}
