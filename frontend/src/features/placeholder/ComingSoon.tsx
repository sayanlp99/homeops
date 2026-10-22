import type { ReactNode } from 'react'
import { Card, EmptyState, PageHeader } from '@/components/ui'

export function ComingSoon({ title, phase, icon, items }: { title: string; phase: number; icon: ReactNode; items: string[] }) {
  return (
    <div>
      <PageHeader title={title} />
      <Card>
        <EmptyState
          icon={icon}
          title={`Coming in Phase ${phase}`}
          description={
            <ul className="mt-2 space-y-1 text-left">
              {items.map((i) => (
                <li key={i}>• {i}</li>
              ))}
            </ul>
          }
        />
      </Card>
    </div>
  )
}
