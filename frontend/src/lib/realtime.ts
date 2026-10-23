import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { clientId } from './api'

const topicKeys: Record<string, string[][]> = {
  tasks: [['tasks'], ['sprints'], ['team']],
  sprints: [['sprints'], ['tasks']],
  recurring: [['recurring']],
  labels: [['bootstrap']],
  members: [['bootstrap']],
  household: [['bootstrap'], ['sprints'], ['meals']],
  notifications: [['notifications']],
  recipes: [['recipes'], ['suggest']],
  meals: [['meals'], ['suggest'], ['recipes']],
  grocery: [['grocery']],
  pantry: [['pantry'], ['suggest']],
  money: [['money']],
  retro: [['retro'], ['team']],
}

export type LiveState = 'connecting' | 'live' | 'offline'

/**
 * Keeps one SSE connection open while the app is visible. Each event names the data that
 * changed; we refetch just that. After any reconnect we refetch everything, since events
 * may have been missed while disconnected.
 */
export function useRealtime(enabled: boolean): LiveState {
  const qc = useQueryClient()
  const [state, setState] = useState<LiveState>('connecting')

  useEffect(() => {
    if (!enabled) return
    let es: EventSource | null = null
    let firstConnect = true

    const open = () => {
      if (es || document.visibilityState !== 'visible') return
      setState('connecting')
      es = new EventSource('/api/events')
      es.addEventListener('ready', () => {
        setState('live')
        if (!firstConnect) qc.invalidateQueries()
        firstConnect = false
      })
      es.onmessage = (e) => {
        try {
          const ev = JSON.parse(e.data) as { topic: string; origin?: string }
          if (ev.origin === clientId) return
          for (const key of topicKeys[ev.topic] ?? []) qc.invalidateQueries({ queryKey: key })
        } catch {
          /* ignore malformed events */
        }
      }
      es.onerror = () => {
        setState(navigator.onLine ? 'connecting' : 'offline')
        // EventSource retries on its own unless the server rejected us (e.g. 401).
        if (es?.readyState === EventSource.CLOSED) {
          es = null
          setTimeout(open, 5000)
        }
      }
    }
    const close = () => {
      es?.close()
      es = null
    }
    const onVisibility = () => {
      if (document.visibilityState === 'visible') {
        firstConnect = false // treat as reconnect: refresh everything
        open()
      } else {
        close()
      }
    }
    const onOffline = () => setState('offline')
    const onOnline = () => {
      close()
      firstConnect = false
      open()
    }

    open()
    document.addEventListener('visibilitychange', onVisibility)
    window.addEventListener('online', onOnline)
    window.addEventListener('offline', onOffline)
    return () => {
      close()
      document.removeEventListener('visibilitychange', onVisibility)
      window.removeEventListener('online', onOnline)
      window.removeEventListener('offline', onOffline)
    }
  }, [enabled, qc])

  return state
}
