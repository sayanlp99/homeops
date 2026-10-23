import clsx from 'clsx'
import { Bell, Home, KeyRound, Palette, Tag, User, Users } from 'lucide-react'
import { NavLink, Navigate, useParams } from 'react-router-dom'
import { PageHeader } from '@/components/ui'
import { AppearanceSettings, HouseholdSettings, LabelSettings, ProfileSettings } from './GeneralSettings'
import { MembersSettings } from './MembersSettings'
import { NotificationSettings } from './NotificationSettings'
import { SecuritySettings } from './SecuritySettings'

const tabs = [
  { id: 'profile', label: 'Profile', icon: User, el: <ProfileSettings /> },
  { id: 'notifications', label: 'Notifications', icon: Bell, el: <NotificationSettings /> },
  { id: 'security', label: 'Sign-in & security', icon: KeyRound, el: <SecuritySettings /> },
  { id: 'household', label: 'Household', icon: Home, el: <HouseholdSettings /> },
  { id: 'members', label: 'Members', icon: Users, el: <MembersSettings /> },
  { id: 'labels', label: 'Labels', icon: Tag, el: <LabelSettings /> },
  { id: 'appearance', label: 'Appearance', icon: Palette, el: <AppearanceSettings /> },
]

export function SettingsPage() {
  const { tab } = useParams()
  const current = tabs.find((t) => t.id === tab)
  if (!current) return <Navigate to="/settings/profile" replace />
  return (
    <div>
      <PageHeader title="Settings" />
      <div className="flex flex-col gap-4 lg:flex-row lg:gap-6">
        <nav className="-mx-4 flex shrink-0 gap-1 overflow-x-auto px-4 scrollbar-thin lg:mx-0 lg:w-52 lg:flex-col lg:overflow-visible lg:px-0">
          {tabs.map((t) => (
            <NavLink
              key={t.id}
              to={`/settings/${t.id}`}
              className={({ isActive }) =>
                clsx(
                  'flex h-8 shrink-0 items-center gap-2 rounded-md px-2.5 text-[13px] whitespace-nowrap',
                  isActive ? 'bg-surface-3 font-medium text-text' : 'text-muted hover:bg-surface-2 hover:text-text',
                )
              }
            >
              <t.icon className="size-4" />
              {t.label}
            </NavLink>
          ))}
        </nav>
        <div className="min-w-0 flex-1 space-y-4 lg:max-w-2xl">{current.el}</div>
      </div>
    </div>
  )
}
