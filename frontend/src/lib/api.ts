import { uuid } from './uuid'

// Identifies this tab so it can ignore realtime echoes of its own writes.
export const clientId = uuid()

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message)
  }
}

export function isOfflineError(e: unknown) {
  return e instanceof TypeError || (e instanceof ApiError && e.status === 0)
}

export async function api<T = void>(method: string, path: string, body?: unknown): Promise<T> {
  let res: Response
  try {
    res = await fetch('/api' + path, {
      method,
      credentials: 'same-origin',
      headers: {
        'X-Requested-With': 'homeops',
        'X-Client-Id': clientId,
        ...(body !== undefined ? { 'Content-Type': 'application/json' } : {}),
      },
      body: body !== undefined ? JSON.stringify(body) : undefined,
    })
  } catch {
    throw new ApiError(0, 'offline', "You're offline")
  }
  if (res.status === 204) return undefined as T
  const data = await res.json().catch(() => null)
  if (!res.ok) {
    throw new ApiError(res.status, data?.code ?? 'error', data?.message ?? `Request failed (${res.status})`)
  }
  return data as T
}

export const get = <T>(path: string) => api<T>('GET', path)
export const post = <T = void>(path: string, body?: unknown) => api<T>('POST', path, body ?? {})
export const patch = <T = void>(path: string, body: unknown) => api<T>('PATCH', path, body)
export const put = <T = void>(path: string, body: unknown) => api<T>('PUT', path, body)
export const del = (path: string) => api('DELETE', path)
