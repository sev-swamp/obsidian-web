import type {
  AclRule,
  AdminUser,
  ApiTokenRecord,
  GroupInfo,
  PluginStatus,
  RoleRecord,
  SsoConfig,
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
  if (token && !headers.has('Authorization')) {
    headers.set('Authorization', `Bearer ${token}`)
  }

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
  // What a single revision changed (diff against its parent).
  diffRev: (path: string, rev: string) =>
    request<{ diff: string }>(
      `/api/diff/${encodePath(path)}?rev=${encodeURIComponent(rev)}`,
    ),
  restore: (path: string, rev: string) =>
    request<{ status: 'restored' | 'unchanged' }>(`/api/restore/${encodePath(path)}`, {
      method: 'POST',
      body: JSON.stringify({ rev }),
    }),
  trash: () => request<DeletedFile[]>('/api/trash'),
  trashRestore: (path: string) =>
    request<{ status: string }>('/api/trash/restore', {
      method: 'POST',
      body: JSON.stringify({ path }),
    }),
  trashPurge: (path: string) =>
    request<{ status: string }>('/api/trash/purge', {
      method: 'POST',
      body: JSON.stringify({ path }),
    }),
  trashPurgeAll: () =>
    request<{ status: string }>('/api/trash/purge-all', { method: 'POST' }),
  createNote: (req: CreateNoteRequest) =>
    request<Note>('/api/note', { method: 'POST', body: JSON.stringify(req) }),
  createFolder: (path: string) =>
    request<{ path: string }>('/api/folder', {
      method: 'POST',
      body: JSON.stringify({ path }),
    }),
  access: (path: string) =>
    request<{ path: string; access: 'none' | 'read' | 'write' }>(
      `/api/access/${encodePath(path)}`,
    ),
  deleteNote: (path: string) =>
    request<{ status: string }>(`/api/note/${encodePath(path)}`, { method: 'DELETE' }),
  search: (q: string, limit = 20) =>
    request<SearchResult[]>(`/api/search?q=${encodeURIComponent(q)}&limit=${limit}`),
  notes: () => request<NoteMeta[]>('/api/notes'),
  recent: (limit = 10) => request<NoteMeta[]>(`/api/recent?limit=${limit}`),
  templates: () => request<string[]>('/api/templates'),
  settings: () => request<Settings>('/api/settings'),
  authStatus: () => request<{ authEnabled: boolean }>('/api/auth/status'),
  // Admin: user & ACL management
  adminUsers: () => request<{ users: AdminUser[]; groups: string[] }>('/api/admin/users'),
  adminCreateUser: (user: { username: string; password: string; role: string; groups: string[] }) =>
    request<AdminUser>('/api/admin/users', { method: 'POST', body: JSON.stringify(user) }),
  adminUpdateUser: (
    name: string,
    patch: { role?: string; groups?: string[]; password?: string },
  ) =>
    request<AdminUser>(`/api/admin/users/${encodeURIComponent(name)}`, {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),
  adminDeleteUser: (name: string) =>
    request<{ status: string }>(`/api/admin/users/${encodeURIComponent(name)}`, {
      method: 'DELETE',
    }),
  adminRevoke: (name: string) =>
    request<{ tokenVersion: number }>(`/api/admin/users/${encodeURIComponent(name)}/revoke`, {
      method: 'POST',
    }),
  adminGetACL: () => request<{ rules: AclRule[] }>('/api/admin/acl'),
  adminPutACL: (rules: AclRule[]) =>
    request<{ rules: AclRule[] }>('/api/admin/acl', {
      method: 'PUT',
      body: JSON.stringify({ rules }),
    }),
  adminGroups: () => request<{ groups: GroupInfo[] }>('/api/admin/groups'),
  adminAddGroup: (name: string) =>
    request<{ groups: GroupInfo[] }>('/api/admin/groups', {
      method: 'POST',
      body: JSON.stringify({ name }),
    }),
  adminDeleteGroup: (name: string) =>
    request<{ groups: GroupInfo[] }>(`/api/admin/groups/${encodeURIComponent(name)}`, {
      method: 'DELETE',
    }),
  adminRoles: () =>
    request<{ roles: RoleRecord[]; permissions: Permission[] }>('/api/admin/roles'),
  adminCreateRole: (role: { name: string; description: string; permissions: string[] }) =>
    request<RoleRecord>('/api/admin/roles', { method: 'POST', body: JSON.stringify(role) }),
  adminUpdateRole: (
    name: string,
    patch: { description: string; permissions: string[] },
  ) =>
    request<RoleRecord>(`/api/admin/roles/${encodeURIComponent(name)}`, {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),
  adminDeleteRole: (name: string) =>
    request<{ status: string }>(`/api/admin/roles/${encodeURIComponent(name)}`, {
      method: 'DELETE',
    }),
  adminGetSSO: () => request<{ sso: SsoConfig; hasSecret: boolean }>('/api/admin/sso'),
  adminPutSSO: (sso: SsoConfig) =>
    request<{ sso: SsoConfig }>('/api/admin/sso', {
      method: 'PUT',
      body: JSON.stringify({ sso }),
    }),
  ssoStatus: () => request<{ enabled: boolean; name: string }>('/api/auth/sso/status'),
  plugins: () => request<PluginStatus[]>('/api/plugins'),
  adminSetPlugin: (id: string, enabled: boolean) =>
    request<PluginStatus[]>(`/api/admin/plugins/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify({ enabled }),
    }),
  me: (token: string) =>
    request<{ username: string; role: string; permissions: Permission[] }>('/api/auth/me', {
      headers: { Authorization: `Bearer ${token}` },
    }),
  adminCheck: (user: string, path: string) =>
    request<{ access: string; role: string }>(
      `/api/admin/check?user=${encodeURIComponent(user)}&path=${encodeURIComponent(path)}`,
    ),
  // Personal API tokens
  tokens: () => request<ApiTokenRecord[]>('/api/tokens'),
  createToken: (body: { name: string; ttlDays?: number; permissions?: string[] }) =>
    request<{ token: string; record: ApiTokenRecord }>('/api/tokens', {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  revokeToken: (id: string) =>
    request<{ status: string }>(`/api/tokens/${encodeURIComponent(id)}`, { method: 'DELETE' }),
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
