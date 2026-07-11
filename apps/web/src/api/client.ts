import type {
  CreateNoteRequest,
  Note,
  NoteMeta,
  SearchResult,
  Settings,
  TreeNode,
} from './types'
import { useAuthStore, type Permission } from '../store/auth'

class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message)
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const token = useAuthStore.getState().token
  const headers = new Headers(init?.headers)
  headers.set('Content-Type', 'application/json')
  if (token) headers.set('Authorization', `Bearer ${token}`)

  const res = await fetch(path, { ...init, headers })
  if (res.status === 401) {
    useAuthStore.getState().setUnauthorized()
  }
  if (!res.ok) {
    let message = res.statusText
    try {
      const body = (await res.json()) as { error?: string }
      if (body.error) message = body.error
    } catch {
      /* not JSON */
    }
    throw new ApiError(res.status, message)
  }
  return res.json() as Promise<T>
}

function encodePath(path: string): string {
  return path.split('/').map(encodeURIComponent).join('/')
}

export const api = {
  tree: () => request<TreeNode>('/api/tree'),
  note: (path: string) => request<Note>(`/api/note/${encodePath(path)}`),
  saveNote: (path: string, content: string) =>
    request<{ status: string }>(`/api/note/${encodePath(path)}`, {
      method: 'PUT',
      body: JSON.stringify({ content }),
    }),
  createNote: (req: CreateNoteRequest) =>
    request<Note>('/api/note', { method: 'POST', body: JSON.stringify(req) }),
  deleteNote: (path: string) =>
    request<{ status: string }>(`/api/note/${encodePath(path)}`, { method: 'DELETE' }),
  search: (q: string, limit = 20) =>
    request<SearchResult[]>(`/api/search?q=${encodeURIComponent(q)}&limit=${limit}`),
  recent: (limit = 10) => request<NoteMeta[]>(`/api/recent?limit=${limit}`),
  templates: () => request<string[]>('/api/templates'),
  settings: () => request<Settings>('/api/settings'),
  authStatus: () => request<{ authEnabled: boolean }>('/api/auth/status'),
  login: (username: string, password: string) =>
    request<{
      token: string
      username: string
      role: string
      permissions: Permission[]
    }>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),
}

export { ApiError }
