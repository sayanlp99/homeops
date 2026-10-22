import { useSyncExternalStore } from 'react'

export type Theme = 'light' | 'dark' | 'system'

const media = window.matchMedia('(prefers-color-scheme: dark)')
const listeners = new Set<() => void>()

function read(): Theme {
  try {
    return (localStorage.getItem('theme') as Theme) || 'system'
  } catch {
    return 'system'
  }
}

let current = read()

function apply() {
  const dark = current === 'dark' || (current === 'system' && media.matches)
  document.documentElement.classList.toggle('dark', dark)
  document.querySelectorAll('meta[name="theme-color"]').forEach((m) => m.setAttribute('content', dark ? '#0b0d12' : '#f6f7f9'))
}

media.addEventListener('change', apply)
apply()

export function setTheme(t: Theme) {
  current = t
  try {
    localStorage.setItem('theme', t)
  } catch {
    /* private mode */
  }
  apply()
  listeners.forEach((l) => l())
}

export function useTheme() {
  const theme = useSyncExternalStore(
    (cb) => {
      listeners.add(cb)
      return () => listeners.delete(cb)
    },
    () => current,
  )
  return [theme, setTheme] as const
}
