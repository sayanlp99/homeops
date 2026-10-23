import { useQuery } from '@tanstack/react-query'
import { get } from './api'

export type RetroCol = 'went_well' | 'improve' | 'action'

export interface RetroCard {
  id: string
  col: RetroCol
  text: string
  authorId: string
  assigneeId: string | null
  taskId: string | null
  votes: number
  voters: string[]
  createdAt: string
}

export interface RetroState {
  retro: { id: string; sprintId: string; status: 'collecting' | 'revealed' | 'closed'; revealedAt: string | null; closedAt: string | null }
  cards: RetroCard[]
  hidden: Record<string, number>
}

export interface RetroSummary {
  id: string
  sprintId: string
  sprintNumber: number
  status: string
  cards: number
  actions: number
}

export interface Burndown {
  sprintNumber: number
  total: number
  unit: 'points' | 'tasks'
  days: { date: string; remaining: number | null; done: number }[]
}

export interface Kudos {
  id: string
  fromId: string
  toId: string
  taskId: string | null
  taskTitle: string
  emoji: string
  message: string
  createdAt: string
}

export interface TeamMetrics {
  sprints: { id: string; number: number; startsOn: string; status: string; planned: number; done: number; byMember: Record<string, number> }[] | null
  streaks: Record<string, { current: number; best: number; today: boolean }>
  kudos: Kudos[] | null
  kudosReceived: Record<string, number>
}

export const KUDOS_EMOJI = ['👏', '🙌', '❤️', '🌟', '💪', '🎉', '🙏', '🔥']

export const RETRO_COLS: { value: RetroCol; label: string; hint: string; tone: string }[] = [
  { value: 'went_well', label: 'Went well', hint: 'What made this week good?', tone: 'var(--success)' },
  { value: 'improve', label: 'To improve', hint: 'What was hard or annoying?', tone: 'var(--warning)' },
  { value: 'action', label: 'Actions', hint: 'What will we do differently? Becomes a task.', tone: 'var(--accent)' },
]

export const useRetro = (sprintId: string | undefined) =>
  useQuery({ queryKey: ['retro', sprintId], queryFn: () => get<RetroState>(`/retro?sprint=${sprintId}`), enabled: !!sprintId })
export const useRetroList = () => useQuery({ queryKey: ['retro', 'list'], queryFn: () => get<RetroSummary[]>('/retros') })
export const useBurndown = (sprintId: string | undefined) =>
  useQuery({ queryKey: ['team', 'burndown', sprintId], queryFn: () => get<Burndown>(`/metrics/burndown?sprint=${sprintId}`), enabled: !!sprintId })
export const useTeam = () => useQuery({ queryKey: ['team'], queryFn: () => get<TeamMetrics>('/metrics/team?sprints=8') })
