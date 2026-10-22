import * as RD from '@radix-ui/react-dialog'
import clsx from 'clsx'
import { X } from 'lucide-react'
import type { ReactNode } from 'react'

/** Centered dialog on desktop, bottom sheet on phones. */
export function Modal({
  open,
  onOpenChange,
  title,
  description,
  children,
  footer,
  width = 'max-w-lg',
}: {
  open: boolean
  onOpenChange: (o: boolean) => void
  title: ReactNode
  description?: ReactNode
  children: ReactNode
  footer?: ReactNode
  width?: string
}) {
  return (
    <RD.Root open={open} onOpenChange={onOpenChange}>
      <RD.Portal>
        <RD.Overlay className="fixed inset-0 z-50 bg-black/40 backdrop-blur-[1px] data-[state=open]:animate-[fade-in_120ms_ease-out]" />
        <RD.Content
          className={clsx(
            'fixed z-50 flex max-h-[92dvh] w-full flex-col border border-border bg-surface shadow-pop outline-none',
            'inset-x-0 bottom-0 rounded-t-xl pb-safe data-[state=open]:animate-[sheet-up_180ms_cubic-bezier(.2,.9,.3,1)]',
            'sm:inset-x-auto sm:bottom-auto sm:top-[12vh] sm:left-1/2 sm:-translate-x-1/2 sm:rounded-lg sm:pb-0 sm:data-[state=open]:animate-[pop-in_140ms_ease-out]',
            width,
          )}
        >
          <div className="flex items-start justify-between gap-4 border-b border-border px-4 py-3">
            <div className="min-w-0">
              <RD.Title className="text-[14px] font-semibold">{title}</RD.Title>
              {description ? <RD.Description className="mt-0.5 text-[12px] text-muted">{description}</RD.Description> : <RD.Description className="sr-only">{title}</RD.Description>}
            </div>
            <RD.Close className="-mr-1 rounded p-1 text-subtle hover:bg-surface-2 hover:text-text" aria-label="Close">
              <X className="size-4" />
            </RD.Close>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4 scrollbar-thin">{children}</div>
          {footer && <div className="flex justify-end gap-2 border-t border-border px-4 py-3">{footer}</div>}
        </RD.Content>
      </RD.Portal>
    </RD.Root>
  )
}

/** Right-hand detail panel on desktop, full-screen page on phones. */
export function Sheet({ open, onOpenChange, title, children, headerActions }: { open: boolean; onOpenChange: (o: boolean) => void; title: ReactNode; children: ReactNode; headerActions?: ReactNode }) {
  return (
    <RD.Root open={open} onOpenChange={onOpenChange}>
      <RD.Portal>
        <RD.Overlay className="fixed inset-0 z-40 bg-black/25 lg:bg-black/10" />
        <RD.Content
          className={clsx(
            'fixed inset-0 z-40 flex flex-col bg-surface outline-none pt-safe pb-safe',
            'lg:inset-y-0 lg:right-0 lg:left-auto lg:w-[560px] lg:border-l lg:border-border lg:pt-0 lg:pb-0 lg:shadow-pop',
            'data-[state=open]:animate-[slide-in_160ms_cubic-bezier(.2,.9,.3,1)]',
          )}
        >
          <div className="flex h-12 shrink-0 items-center justify-between gap-2 border-b border-border px-3">
            <RD.Title className="min-w-0 truncate text-[13px] font-medium text-muted">{title}</RD.Title>
            <RD.Description className="sr-only">Details</RD.Description>
            <div className="flex items-center gap-1">
              {headerActions}
              <RD.Close className="rounded p-1.5 text-subtle hover:bg-surface-2 hover:text-text" aria-label="Close">
                <X className="size-4" />
              </RD.Close>
            </div>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto scrollbar-thin">{children}</div>
        </RD.Content>
      </RD.Portal>
    </RD.Root>
  )
}
