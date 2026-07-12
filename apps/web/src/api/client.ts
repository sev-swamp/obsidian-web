import type {
  CreateNoteRequest,
  DeletedFile,
  Note,
  NoteMeta,
  Revision,
  SearchResult,
  Settings,
  TreeNode,
} from './types'
import { useAuthStore, type Permission } from '../store/auth'

class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public body?: unknown,
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
    let body: unknown
    try {
      body = await res.json()
      const err = (body as { error?: string }).error
      if (err) message = err
    } catch {
      /* not JSON */
    }
    throw new ApiError(res.status, message, body)
  }
  return res.json() as Promise<T>
}

function encodePath(path: string): string {
  return path.split('/').map(encodeURIComponent).join('/')
}

export const api = {
  tree: () => request<TreeNode>('/api/tree'),
  note: (path: string) => request<Note>(`/api/note/${encodePath(path)}`),
  saveNote: (path: string, content: string, baseHash?: string) =>
    request<{ status: string }>(`/api/note/${encodePath(path)}`, {
      method: 'PUT',
      body: JSON.stringify({ content, baseHash: baseHash ?? '' }),
    }),
  history: (path: string, limit = 50) =>
    request<Revision[]>(`/api/history/${encodePath(path)}?limit=${limit}`),
  diff: (path: string, from: string, to = '') =>
    request<{ diff: string }>(
      `/api/diff/${encodePath(path)}?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`,
    ),
  restore: (path: string, rev: string) =>
    request<{ status: string }>(`/api/restore/${encodePath(path)}`, {
      method: 'POST',
      body: JSON.stringify({ rev }),
    }),
  trash: () => request<DeletedFile[]>('/api/trash'),
  trashRestore: (path: string) =>
    request<{ status: string }>('/api/trash/restore', {
      method: 'POST',
      body: JSON.stringify({ path }),
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
