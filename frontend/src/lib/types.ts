export type Role = 'admin' | 'member'

export interface User {
  id: string
  householdId: string
  username: string
  displayName: string
  role: Role
  color: string
  notifyPrefs: Partial<Record<NotifyKind, boolean>>
  quietStart: string | null
  quietEnd: string | null
}

export type NotifyKind = 'task_assigned' | 'task_due' | 'task_overdue' | 'ceremony' | 'meals' | 'budget'

export interface Member {
  id: string
  username: string
  displayName: string
  role: Role
  color: string
}

export interface Household {
  id: string
  name: string
  timezone: string
  sprintStartDow: number
  planningDow: number
  planningTime: string
  autoRoll: boolean
}

export interface Label {
  id: string
  name: string
  color: string
}

export interface Bootstrap {
  setupRequired: boolean
  user: User | null
  household?: Household
  members?: Member[]
  labels?: Label[]
  vapidPublicKey?: string
}

export type TaskStatus = 'backlog' | 'todo' | 'in_progress' | 'done'

export interface Task {
  id: string
  number: number // 0 while an offline-created task hasn't synced yet
  title: string
  description: string
  status: TaskStatus
  priority: number // 0 none, 1 low, 2 medium, 3 high, 4 urgent
  points: number | null
  labels: string[]
  assigneeId: string | null
  reporterId: string | null
  sprintId: string | null
  dueDate: string | null // YYYY-MM-DD
  rank: number
  recurringId: string | null
  completedAt: string | null
  completedBy: string | null
  createdAt: string
  updatedAt: string
}

export type SprintStatus = 'planned' | 'active' | 'completed'

export interface Sprint {
  id: string
  number: number
  goal: string
  startsOn: string
  endsOn: string
  status: SprintStatus
  startedAt: string | null
  completedAt: string | null
  taskCount: number
  doneCount: number
  points: number
  donePoints: number
}

export interface Recurring {
  id: string
  title: string
  description: string
  priority: number
  points: number | null
  labels: string[]
  assigneeId: string | null
  rotate: boolean
  freq: 'daily' | 'weekly' | 'monthly'
  interval: number
  weekdays: number[]
  monthDay: number | null
  anchorDate: string
  active: boolean
}

export interface InboxItem {
  id: string
  kind: string
  title: string
  body: string
  url: string
  createdAt: string
  readAt: string | null
}

export interface PushResult {
  device: string
  ok: boolean
  status: number
  error?: string
}
