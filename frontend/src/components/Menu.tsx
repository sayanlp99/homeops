import * as DM from '@radix-ui/react-dropdown-menu'
import clsx from 'clsx'
import { Check } from 'lucide-react'
import type { ReactNode } from 'react'

export function Menu({ trigger, children, align = 'start', width = 'w-56' }: { trigger: ReactNode; children: ReactNode; align?: 'start' | 'end'; width?: string }) {
  return (
    <DM.Root modal={false}>
      <DM.Trigger asChild>{trigger}</DM.Trigger>
      <DM.Portal>
        <DM.Content
          align={align}
          sideOffset={4}
          collisionPadding={8}
          className={clsx(
            'z-50 max-h-[min(var(--radix-dropdown-menu-content-available-height),420px)] overflow-y-auto rounded-md border border-border bg-surface p-1 shadow-pop scrollbar-thin',
            'data-[state=open]:animate-[pop-in_100ms_ease-out]',
            width,
          )}
          onClick={(e) => e.stopPropagation()}
        >
          {children}
        </DM.Content>
      </DM.Portal>
    </DM.Root>
  )
}

export function MenuItem({ children, onSelect, icon, checked, danger, shortcut, keepOpen }: { children: ReactNode; onSelect: () => void; icon?: ReactNode; checked?: boolean; danger?: boolean; shortcut?: string; keepOpen?: boolean }) {
  return (
    <DM.Item
      onSelect={(e) => {
        if (keepOpen) e.preventDefault()
        onSelect()
      }}
      className={clsx(
        'flex h-9 cursor-default items-center gap-2 rounded px-2 text-[13px] outline-none select-none data-[highlighted]:bg-surface-2 lg:h-7',
        danger ? 'text-danger' : 'text-text',
      )}
    >
      {icon && <span className="flex size-4 items-center justify-center text-subtle">{icon}</span>}
      <span className="min-w-0 flex-1 truncate">{children}</span>
      {shortcut && <span className="text-[11px] text-subtle">{shortcut}</span>}
      {checked && <Check className="size-3.5 text-accent" />}
    </DM.Item>
  )
}

export const MenuLabel = ({ children }: { children: ReactNode }) => (
  <DM.Label className="px-2 pt-1.5 pb-1 text-[11px] font-medium text-subtle">{children}</DM.Label>
)

export const MenuSeparator = () => <DM.Separator className="-mx-1 my-1 h-px bg-border" />
