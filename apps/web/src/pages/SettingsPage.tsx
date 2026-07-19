import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import type { AclRule, SsoConfig } from '../api/types'
import { useAuthStore, type Permission } from '../store/auth'
import { usePrefsStore } from '../store/prefs'
import { useT, type TKey } from '../i18n'
import { SettingsIcon, BanIcon } from '../components/icons'
import { useConfirm } from '../components/ConfirmDialog'

const inputCls =
  'w-full rounded-lg border border-gray-300 bg-transparent px-3 py-2 text-sm outline-none focus:border-violet-500 focus:ring-2 focus:ring-violet-500/30 dark:border-gray-700'
const btnCls =
  'rounded-lg border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-100 disabled:opacity-50 dark:border-gray-700 dark:hover:bg-gray-800'
const primaryBtnCls =
  'rounded-lg bg-violet-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-violet-700 disabled:opacity-50'

type Tab = 'general' | 'users' | 'roles' | 'groups' | 'access' | 'tokens' | 'plugins' | 'sso'

// "General" holds personal preferences and is visible to everyone;
// the remaining tabs drive admin APIs and need settings:write.
const generalTab: { id: Tab; label: TKey } = { id: 'general', label: 'tabGeneral' }
const adminTabs: { id: Tab; label: TKey }[] = [
  { id: 'users', label: 'tabUsers' },
  { id: 'roles', label: 'tabRoles' },
  { id: 'groups', label: 'tabGroups' },
  { id: 'access', label: 'tabAccess' },
  { id: 'tokens', label: 'tabTokens' },
  { id: 'plugins', label: 'tabPlugins' },
  { id: 'sso', label: 'tabSSO' },
]

export function SettingsPage() {
  const [tab, setTab] = useState<Tab>('general')
  const can = useAuthStore((s) => s.can)
  const tabs = can('settings:write') ? [generalTab, ...adminTabs] : [generalTab]
  const t = useT()

  return (
    <div className="mx-auto max-w-4xl px-6 py-10">
      <h1 className="flex items-center gap-2 text-2xl font-bold">
        <SettingsIcon size={22} /> {t('settingsTitle')}
      </h1>

      <nav className="mt-6 flex flex-wrap gap-1 border-b border-gray-200 dark:border-gray-800">
        {tabs.map((item) => (
          <button
            key={item.id}
            onClick={() => setTab(item.id)}
            className={`rounded-t-lg px-4 py-2 text-sm font-medium ${
              tab === item.id
                ? 'border border-b-0 border-gray-200 bg-white text-violet-600 dark:border-gray-800 dark:bg-gray-950 dark:text-violet-400'
                : 'text-gray-500 hover:text-gray-800 dark:hover:text-gray-200'
            }`}
          >
            {t(item.label)}
          </button>
        ))}
      </nav>

      <div className="pt-6">
        {tab === 'general' && <GeneralSection />}
        {tab === 'users' && <UsersSection />}
        {tab === 'roles' && <RolesSection />}
        {tab === 'groups' && <GroupsSection />}
        {tab === 'access' && <AccessSection />}
        {tab === 'tokens' && <TokensSection />}
        {tab === 'plugins' && <PluginsSection />}
        {tab === 'sso' && <SsoSection />}
      </div>
    </div>
  )
}

function GeneralSection() {
  const t = useT()
  const lineNumbers = usePrefsStore((s) => s.lineNumbers)
  const openInEdit = usePrefsStore((s) => s.openInEdit)
  const showProperties = usePrefsStore((s) => s.showProperties)
  const setLineNumbers = usePrefsStore((s) => s.setLineNumbers)
  const setOpenInEdit = usePrefsStore((s) => s.setOpenInEdit)
  const setShowProperties = usePrefsStore((s) => s.setShowProperties)

  return (
    <section className="max-w-2xl">
      <h2 className="text-lg font-semibold">{t('editorSection')}</h2>
      <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">{t('prefsHint')}</p>
      <label className="mt-4 flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          checked={lineNumbers}
          onChange={(e) => setLineNumbers(e.target.checked)}
          className="h-4 w-4 accent-violet-600"
        />
        {t('lineNumbersToggle')}
      </label>
      <label className="mt-3 flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          checked={openInEdit}
          onChange={(e) => setOpenInEdit(e.target.checked)}
          className="h-4 w-4 accent-violet-600"
        />
        {t('openInEditToggle')}
      </label>
      <label className="mt-3 flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          checked={showProperties}
          onChange={(e) => setShowProperties(e.target.checked)}
          className="h-4 w-4 accent-violet-600"
        />
        {t('showPropertiesToggle')}
      </label>
    </section>
  )
}

/* ---------------------------------------------------------------- */
/* Users                                                              */
/* ---------------------------------------------------------------- */

// useRoleNames lists the role names for the user role dropdowns, always
// including the built-in defaults as a fallback.
function useRoleNames(): string[] {
  const { data } = useQuery({ queryKey: ['admin-roles'], queryFn: api.adminRoles })
  const names = data?.roles.map((r) => r.name) ?? []
  return names.length > 0 ? names : ['viewer', 'editor', 'admin']
}

function UsersSection() {
  const t = useT()
  const queryClient = useQueryClient()
  const invalidate = () => void queryClient.invalidateQueries({ queryKey: ['admin-users'] })
  const { data } = useQuery({ queryKey: ['admin-users'], queryFn: api.adminUsers })
  const roleNames = useRoleNames()

  const [newUser, setNewUser] = useState({ username: '', password: '', role: 'viewer', groups: '' })
  const createUser = useMutation({
    mutationFn: () =>
      api.adminCreateUser({
        username: newUser.username,
        password: newUser.password,
        role: newUser.role,
        groups: splitGroups(newUser.groups),
      }),
    onSuccess: () => {
      setNewUser({ username: '', password: '', role: 'viewer', groups: '' })
      invalidate()
    },
  })

  return (
    <section>
      <div className="space-y-2">
        {data?.users.map((u) => (
          <UserRow key={u.username} user={u} roleNames={roleNames} onChanged={invalidate} />
        ))}
      </div>

      <form
        className="mt-4 grid gap-2 rounded-xl border border-dashed border-gray-300 p-4 sm:grid-cols-2 dark:border-gray-700"
        onSubmit={(e) => {
          e.preventDefault()
          if (newUser.username && newUser.password) createUser.mutate()
        }}
      >
        <input
          className={inputCls}
          placeholder={t('usernameLabel')}
          value={newUser.username}
          onChange={(e) => setNewUser({ ...newUser, username: e.target.value })}
        />
        <input
          className={inputCls}
          type="password"
          placeholder={t('passwordLabel')}
          value={newUser.password}
          onChange={(e) => setNewUser({ ...newUser, password: e.target.value })}
        />
        <select
          className={inputCls}
          value={newUser.role}
          onChange={(e) => setNewUser({ ...newUser, role: e.target.value })}
        >
          {roleNames.map((r) => (
            <option key={r} value={r}>
              {r}
            </option>
          ))}
        </select>
        <input
          className={inputCls}
          placeholder={t('groupsLabel')}
          value={newUser.groups}
          onChange={(e) => setNewUser({ ...newUser, groups: e.target.value })}
        />
        {createUser.error && (
          <p className="text-sm text-red-600 dark:text-red-400 sm:col-span-2">
            {(createUser.error as Error).message}
          </p>
        )}
        <button
          type="submit"
          disabled={!newUser.username || !newUser.password || createUser.isPending}
          className={`${primaryBtnCls} sm:col-span-2`}
        >
          {t('createUser')}
        </button>
      </form>
    </section>
  )
}

function UserRow({
  user,
  roleNames,
  onChanged,
}: {
  user: { username: string; role: string; groups: string[] | null }
  roleNames: string[]
  onChanged: () => void
}) {
  const t = useT()
  const confirm = useConfirm()
  const [groups, setGroups] = useState((user.groups ?? []).join(', '))
  const [password, setPassword] = useState('')

  const update = useMutation({
    mutationFn: (patch: { role?: string; groups?: string[]; password?: string }) =>
      api.adminUpdateUser(user.username, patch),
    onSuccess: () => {
      setPassword('')
      onChanged()
    },
  })
  const remove = useMutation({
    mutationFn: () => api.adminDeleteUser(user.username),
    onSuccess: onChanged,
  })
  const revoke = useMutation({
    mutationFn: () => api.adminRevoke(user.username),
    onSuccess: onChanged,
  })

  return (
    <div className="flex flex-wrap items-center gap-2 rounded-xl border border-gray-200 px-3 py-2 dark:border-gray-800">
      <span className="w-28 truncate font-medium">{user.username}</span>
      <select
        value={user.role}
        onChange={(e) => update.mutate({ role: e.target.value })}
        className="rounded-lg border border-gray-300 bg-transparent px-2 py-1 text-sm dark:border-gray-700 dark:bg-gray-950"
      >
        {roleNames.map((r) => (
          <option key={r} value={r}>
            {r}
          </option>
        ))}
      </select>
      <input
        value={groups}
        onChange={(e) => setGroups(e.target.value)}
        onBlur={() => update.mutate({ groups: splitGroups(groups) })}
        placeholder={t('groupsLabel')}
        className="min-w-32 flex-1 rounded-lg border border-gray-300 bg-transparent px-2 py-1 text-sm dark:border-gray-700"
      />
      <input
        type="password"
        value={password}
        onChange={(e) => setPassword(e.target.value)}
        onBlur={() => {
          if (password) update.mutate({ password })
        }}
        placeholder={t('resetPassword')}
        className="w-44 rounded-lg border border-gray-300 bg-transparent px-2 py-1 text-sm dark:border-gray-700"
      />
      <button
        onClick={() => revoke.mutate()}
        className={btnCls}
        title={t('revokeSessions')}
        aria-label={t('revokeSessions')}
      >
        <BanIcon size={16} />
      </button>
      <button
        onClick={() =>
          void confirm({
            title: `${t('deleteUserBtn')} ${user.username}?`,
            confirmLabel: t('deleteUserBtn'),
            danger: true,
          }).then((ok) => ok && remove.mutate())
        }
        className={`${btnCls} text-red-600 dark:text-red-400`}
      >
        {t('deleteUserBtn')}
      </button>
      {(update.error || remove.error) && (
        <span className="w-full text-xs text-red-600 dark:text-red-400">
          {((update.error || remove.error) as Error).message}
        </span>
      )}
    </div>
  )
}

/* ---------------------------------------------------------------- */
/* Roles                                                              */
/* ---------------------------------------------------------------- */

function RolesSection() {
  const t = useT()
  const queryClient = useQueryClient()
  const { data } = useQuery({ queryKey: ['admin-roles'], queryFn: api.adminRoles })
  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ['admin-roles'] })
  }
  const catalog = data?.permissions ?? []

  const [newRole, setNewRole] = useState<{ name: string; description: string; permissions: string[] }>(
    { name: '', description: '', permissions: [] },
  )
  const create = useMutation({
    mutationFn: () => api.adminCreateRole(newRole),
    onSuccess: () => {
      setNewRole({ name: '', description: '', permissions: [] })
      invalidate()
    },
  })

  return (
    <section>
      <p className="mb-4 text-sm text-gray-500">{t('rolesHint')}</p>
      <div className="space-y-3">
        {data?.roles.map((role) => (
          <RoleRow key={role.name} role={role} catalog={catalog} onChanged={invalidate} />
        ))}
      </div>

      <form
        className="mt-4 grid gap-2 rounded-xl border border-dashed border-gray-300 p-4 dark:border-gray-700"
        onSubmit={(e) => {
          e.preventDefault()
          if (newRole.name) create.mutate()
        }}
      >
        <div className="grid gap-2 sm:grid-cols-2">
          <input
            className={inputCls}
            placeholder={t('roleNameLabel')}
            value={newRole.name}
            onChange={(e) => setNewRole({ ...newRole, name: e.target.value })}
          />
          <input
            className={inputCls}
            placeholder={t('roleDescriptionLabel')}
            value={newRole.description}
            onChange={(e) => setNewRole({ ...newRole, description: e.target.value })}
          />
        </div>
        <PermissionPicker
          catalog={catalog}
          selected={newRole.permissions}
          onChange={(permissions) => setNewRole({ ...newRole, permissions })}
        />
        {create.error && <p className="text-sm text-red-600 dark:text-red-400">{(create.error as Error).message}</p>}
        <button type="submit" disabled={!newRole.name || create.isPending} className={primaryBtnCls}>
          {t('createRole')}
        </button>
      </form>
    </section>
  )
}

function RoleRow({
  role,
  catalog,
  onChanged,
}: {
  role: import('../api/types').RoleRecord
  catalog: string[]
  onChanged: () => void
}) {
  const t = useT()
  const confirm = useConfirm()
  const [open, setOpen] = useState(false)
  const [description, setDescription] = useState(role.description)
  const [permissions, setPermissions] = useState<string[]>(role.permissions ?? [])
  const isAdmin = role.name === 'admin'
  const permCount = role.permissions?.length ?? 0

  const save = useMutation({
    mutationFn: () => api.adminUpdateRole(role.name, { description, permissions }),
    onSuccess: onChanged,
  })
  const remove = useMutation({
    mutationFn: () => api.adminDeleteRole(role.name),
    onSuccess: onChanged,
  })

  return (
    <div className="rounded-xl border border-gray-200 dark:border-gray-800">
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-2 px-4 py-3 text-left"
      >
        <span className="text-gray-500 dark:text-gray-400">{open ? '▾' : '▸'}</span>
        <span className="font-medium">{role.name}</span>
        {role.builtin && (
          <span className="rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-500 dark:bg-gray-800">
            {t('roleBuiltin')}
          </span>
        )}
        <span className="ml-auto truncate text-xs text-gray-500 dark:text-gray-400">
          {isAdmin ? t('roleAllPermissions') : `${permCount} · ${role.description}`}
        </span>
      </button>

      {open && (
        <div className="border-t border-gray-100 px-4 py-3 dark:border-gray-800/60">
          <input
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder={t('roleDescriptionLabel')}
            className={inputCls}
          />
          {isAdmin ? (
            <div className="mt-2">
              <p className="text-xs text-gray-500 dark:text-gray-400">{t('roleAdminFixed')}</p>
              <PermissionPicker catalog={catalog} selected={catalog} onChange={() => {}} readOnly />
            </div>
          ) : (
            <PermissionPicker catalog={catalog} selected={permissions} onChange={setPermissions} />
          )}
          <div className="mt-3 flex items-center gap-2">
            <button onClick={() => save.mutate()} disabled={save.isPending} className={primaryBtnCls}>
              {t('saveRole')}
            </button>
            {!role.builtin && (
              <button
                onClick={() =>
                  void confirm({
                    title: `${t('deleteRoleBtn')} «${role.name}»?`,
                    confirmLabel: t('deleteRoleBtn'),
                    danger: true,
                  }).then((ok) => ok && remove.mutate())
                }
                className={`${btnCls} text-red-600 dark:text-red-400`}
              >
                {t('deleteRoleBtn')}
              </button>
            )}
            {save.error && (
              <span className="text-xs text-red-600 dark:text-red-400">{(save.error as Error).message}</span>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

function PermissionPicker({
  catalog,
  selected,
  onChange,
  readOnly = false,
}: {
  catalog: string[]
  selected: string[]
  onChange: (permissions: string[]) => void
  readOnly?: boolean
}) {
  const toggle = (perm: string, on: boolean) =>
    onChange(on ? [...selected, perm] : selected.filter((p) => p !== perm))

  return (
    <div className="mt-2 grid gap-1 sm:grid-cols-2">
      {catalog.map((perm) => (
        <label key={perm} className="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-400">
          <input
            type="checkbox"
            checked={selected.includes(perm)}
            disabled={readOnly}
            onChange={(e) => toggle(perm, e.target.checked)}
            className="h-4 w-4 accent-violet-600"
          />
          <code className="text-xs">{perm}</code>
        </label>
      ))}
    </div>
  )
}

/* ---------------------------------------------------------------- */
/* Groups                                                             */
/* ---------------------------------------------------------------- */

function GroupsSection() {
  const t = useT()
  const confirm = useConfirm()
  const queryClient = useQueryClient()
  const { data } = useQuery({ queryKey: ['admin-groups'], queryFn: api.adminGroups })
  const [name, setName] = useState('')

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ['admin-groups'] })
    void queryClient.invalidateQueries({ queryKey: ['admin-users'] })
  }
  const add = useMutation({
    mutationFn: () => api.adminAddGroup(name),
    onSuccess: () => {
      setName('')
      invalidate()
    },
  })
  const del = useMutation({
    mutationFn: (group: string) => api.adminDeleteGroup(group),
    onSuccess: invalidate,
  })

  return (
    <section>
      {data?.groups.length === 0 && <p className="text-sm text-gray-500 dark:text-gray-400">{t('noGroups')}</p>}
      <ul className="space-y-2">
        {data?.groups.map((g) => (
          <li
            key={g.name}
            className="flex items-center gap-3 rounded-xl border border-gray-200 px-4 py-3 dark:border-gray-800"
          >
            <div className="min-w-0 flex-1">
              <div className="font-medium">{g.name}</div>
              <div className="truncate text-xs text-gray-500 dark:text-gray-400">
                {g.members.length > 0 ? g.members.join(', ') : `0 ${t('membersLabel')}`}
              </div>
            </div>
            <button
              onClick={() =>
                void confirm({
                  title: `${t('deleteUserBtn')} «${g.name}»?`,
                  confirmLabel: t('deleteUserBtn'),
                  danger: true,
                }).then((ok) => ok && del.mutate(g.name))
              }
              className={`${btnCls} shrink-0 text-red-600 dark:text-red-400`}
            >
              {t('deleteUserBtn')}
            </button>
          </li>
        ))}
      </ul>
      <form
        className="mt-4 flex gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          if (name.trim()) add.mutate()
        }}
      >
        <input
          className={inputCls}
          placeholder={t('groupNameLabel')}
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <button type="submit" disabled={!name.trim() || add.isPending} className={primaryBtnCls}>
          {t('addGroupBtn')}
        </button>
      </form>
      {add.error && <p className="mt-1 text-sm text-red-600 dark:text-red-400">{(add.error as Error).message}</p>}
    </section>
  )
}

/* ---------------------------------------------------------------- */
/* Access rules + checker                                             */
/* ---------------------------------------------------------------- */

function AccessSection() {
  const t = useT()
  const queryClient = useQueryClient()
  const { data: aclData } = useQuery({ queryKey: ['admin-acl'], queryFn: api.adminGetACL })

  const [rulesText, setRulesText] = useState<string | null>(null)
  const [aclError, setAclError] = useState('')
  const rulesValue = rulesText ?? JSON.stringify(aclData?.rules ?? [], null, 2)
  const saveRules = useMutation({
    mutationFn: (rules: AclRule[]) => api.adminPutACL(rules),
    onSuccess: () => {
      setAclError('')
      setRulesText(null)
      void queryClient.invalidateQueries({ queryKey: ['admin-acl'] })
    },
    onError: (err) => setAclError((err as Error).message),
  })

  const [check, setCheck] = useState({ user: '', path: '' })
  const [checkResult, setCheckResult] = useState<{ access: string; role: string } | null>(null)

  return (
    <section>
      <p className="mb-2 text-xs text-gray-500 dark:text-gray-400">{t('aclHint')}</p>
      <textarea
        value={rulesValue}
        onChange={(e) => setRulesText(e.target.value)}
        spellCheck={false}
        rows={10}
        className={`${inputCls} font-mono text-xs`}
      />
      {aclError && <p className="mt-1 text-sm text-red-600 dark:text-red-400">{aclError}</p>}
      <button
        onClick={() => {
          try {
            saveRules.mutate(JSON.parse(rulesValue) as AclRule[])
          } catch (e) {
            setAclError((e as Error).message)
          }
        }}
        disabled={saveRules.isPending}
        className={`${primaryBtnCls} mt-2`}
      >
        {t('saveRules')}
      </button>

      <h3 className="mt-8 mb-3 text-sm font-semibold tracking-wide text-gray-500 dark:text-gray-400 uppercase">
        {t('checkSection')}
      </h3>
      <div className="flex flex-wrap items-center gap-2">
        <input
          className={`${inputCls} max-w-44`}
          placeholder={t('usernameLabel')}
          value={check.user}
          onChange={(e) => setCheck({ ...check, user: e.target.value })}
        />
        <input
          className={`${inputCls} max-w-72`}
          placeholder={`${t('pathLabel')} (HR/Salaries.md)`}
          value={check.path}
          onChange={(e) => setCheck({ ...check, path: e.target.value })}
        />
        <button
          onClick={() => {
            void api
              .adminCheck(check.user, check.path)
              .then((r) => setCheckResult({ access: r.access, role: r.role }))
              .catch((e: Error) => setCheckResult({ access: e.message, role: '' }))
          }}
          disabled={!check.user || !check.path}
          className={btnCls}
        >
          {t('checkBtn')}
        </button>
        {checkResult && (
          <span
            className={`rounded-full px-3 py-1 text-sm font-medium ${
              checkResult.access === 'write'
                ? 'bg-green-100 text-green-800 dark:bg-green-950 dark:text-green-300'
                : checkResult.access === 'read'
                  ? 'bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300'
                  : 'bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-300'
            }`}
          >
            {t('accessResult')}: {checkResult.access}
            {checkResult.role && (
              <span className="ml-1 font-normal opacity-70">({checkResult.role})</span>
            )}
          </span>
        )}
      </div>
    </section>
  )
}

/* ---------------------------------------------------------------- */
/* API tokens                                                         */
/* ---------------------------------------------------------------- */

function TokensSection() {
  const t = useT()
  const queryClient = useQueryClient()
  const myPermissions = useAuthStore((s) => s.permissions) ?? []

  const { data: tokens } = useQuery({ queryKey: ['tokens'], queryFn: api.tokens })

  const [name, setName] = useState('')
  const [ttlDays, setTtlDays] = useState(0)
  const [perms, setPerms] = useState<Permission[]>([])
  const [issued, setIssued] = useState('')

  const create = useMutation({
    mutationFn: () =>
      api.createToken({
        name,
        ttlDays: ttlDays > 0 ? ttlDays : undefined,
        permissions: perms.length > 0 ? perms : undefined,
      }),
    onSuccess: (res) => {
      setIssued(res.token)
      setName('')
      setPerms([])
      void queryClient.invalidateQueries({ queryKey: ['tokens'] })
    },
  })

  const revoke = useMutation({
    mutationFn: (id: string) => api.revokeToken(id),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['tokens'] }),
  })

  return (
    <section>
      <ul className="space-y-2">
        {tokens?.map((tok) => (
          <li
            key={tok.id}
            className="flex items-center gap-3 rounded-xl border border-gray-200 px-4 py-3 dark:border-gray-800"
          >
            <div className="min-w-0 flex-1">
              <div className="font-medium">
                {tok.name}{' '}
                {tok.revoked && <span className="text-xs text-red-600 dark:text-red-400">({t('revoked')})</span>}
              </div>
              <div className="text-xs text-gray-500 dark:text-gray-400">
                {tok.permissions.join(', ')} ·{' '}
                {tok.expiresAt
                  ? `${t('expiresLabel')} ${new Date(tok.expiresAt).toLocaleDateString()}`
                  : t('neverExpires')}
              </div>
            </div>
            {!tok.revoked && (
              <button
                onClick={() => revoke.mutate(tok.id)}
                className="shrink-0 rounded-lg border border-red-300 px-3 py-1.5 text-sm text-red-600 hover:bg-red-50 dark:border-red-900 dark:text-red-400 dark:hover:bg-red-950"
              >
                {t('revokeBtn')}
              </button>
            )}
          </li>
        ))}
      </ul>

      <form
        className="mt-6 space-y-3 rounded-xl border border-dashed border-gray-300 p-4 dark:border-gray-700"
        onSubmit={(e) => {
          e.preventDefault()
          if (name.trim()) create.mutate()
        }}
      >
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder={t('tokenName')}
          className={inputCls}
        />
        <label className="block text-sm text-gray-600 dark:text-gray-400">
          {t('ttlDaysLabel')}
          <input
            type="number"
            min={0}
            value={ttlDays}
            onChange={(e) => setTtlDays(Number(e.target.value))}
            className="mt-1 w-32 rounded-lg border border-gray-300 bg-transparent px-3 py-1.5 dark:border-gray-700"
          />
        </label>
        <fieldset className="text-sm text-gray-600 dark:text-gray-400">
          <legend>{t('permissionsLabel')}</legend>
          <div className="mt-1 flex flex-wrap gap-3">
            {myPermissions.map((p) => (
              <label key={p} className="flex items-center gap-1.5">
                <input
                  type="checkbox"
                  checked={perms.includes(p)}
                  onChange={(e) =>
                    setPerms(e.target.checked ? [...perms, p] : perms.filter((x) => x !== p))
                  }
                  className="accent-violet-600"
                />
                <code className="text-xs">{p}</code>
              </label>
            ))}
          </div>
        </fieldset>
        {create.error && (
          <p className="text-sm text-red-600 dark:text-red-400">{(create.error as Error).message}</p>
        )}
        <button
          type="submit"
          disabled={!name.trim() || create.isPending}
          className={primaryBtnCls}
        >
          {t('createTokenBtn')}
        </button>
      </form>

      {issued && (
        <div className="mt-4 rounded-xl border border-amber-300 bg-amber-50 p-4 dark:border-amber-800 dark:bg-amber-950">
          <p className="mb-2 text-sm font-medium text-amber-800 dark:text-amber-300">
            {t('tokenCreatedOnce')}
          </p>
          <code className="block overflow-x-auto rounded bg-white p-2 text-xs break-all dark:bg-gray-900">
            {issued}
          </code>
        </div>
      )}
    </section>
  )
}

/* ---------------------------------------------------------------- */
/* Plugins                                                            */
/* ---------------------------------------------------------------- */

function PluginsSection() {
  const queryClient = useQueryClient()
  const { data: pluginList } = useQuery({ queryKey: ['plugins'], queryFn: api.plugins })

  const update = useMutation({
    mutationFn: (vars: {
      id: string
      patch: { enabled?: boolean; settings?: Record<string, string> }
    }) => api.adminSetPlugin(vars.id, vars.patch),
    onSuccess: (statuses) => {
      queryClient.setQueryData(['plugins'], statuses)
      // Plugin settings can change what plugin endpoints serve
      // (e.g. the templates folder), so drop derived caches.
      void queryClient.invalidateQueries({ queryKey: ['templates'] })
    },
  })

  return (
    <section className="space-y-2">
      {pluginList?.map((p) => (
        <PluginRow
          key={p.id}
          plugin={p}
          pending={update.isPending}
          onUpdate={(patch) => update.mutate({ id: p.id, patch })}
        />
      ))}
      {update.error && (
        <p className="text-sm text-red-600 dark:text-red-400">{(update.error as Error).message}</p>
      )}
    </section>
  )
}

function PluginRow({
  plugin: p,
  pending,
  onUpdate,
}: {
  plugin: import('../api/types').PluginStatus
  pending: boolean
  onUpdate: (patch: { enabled?: boolean; settings?: Record<string, string> }) => void
}) {
  const t = useT()
  const [draft, setDraft] = useState<Record<string, string> | null>(null)
  const spec = p.settingsSpec ?? []
  const values = draft ?? p.settings ?? {}

  return (
    <div className="rounded-xl border border-gray-200 px-4 py-3 dark:border-gray-800">
      <div className="flex items-center gap-3">
        <div className="min-w-0 flex-1">
          <div className="font-medium">
            {p.name}{' '}
            <span className="text-xs font-normal text-gray-500 dark:text-gray-400">
              v{p.version} ·{' '}
              {p.kind === 'backend' ? t('pluginKindBackend') : t('pluginKindUI')}
            </span>
          </div>
          <div className="text-xs text-gray-500 dark:text-gray-400">{p.description}</div>
        </div>
        <label className="flex shrink-0 items-center gap-2 text-sm text-gray-600 dark:text-gray-400">
          <input
            type="checkbox"
            checked={p.enabled}
            disabled={pending}
            onChange={(e) => onUpdate({ enabled: e.target.checked })}
            className="h-4 w-4 accent-violet-600"
          />
          {t('pluginEnabled')}
        </label>
      </div>

      {spec.length > 0 && (
        <div className="mt-3 border-t border-gray-100 pt-3 dark:border-gray-800/60">
          <h4 className="mb-2 text-xs font-semibold tracking-wide text-gray-500 dark:text-gray-400 uppercase">
            {t('pluginSettingsTitle')}
          </h4>
          <div className="grid gap-2 sm:grid-cols-2">
            {spec.map((s) => (
              <label key={s.key} className="block text-sm text-gray-600 dark:text-gray-400">
                {s.label || s.key}
                <input
                  className={`${inputCls} mt-1`}
                  value={values[s.key] ?? ''}
                  placeholder={s.default}
                  onChange={(e) => setDraft({ ...values, [s.key]: e.target.value })}
                />
              </label>
            ))}
          </div>
          <button
            type="button"
            className={`${primaryBtnCls} mt-2`}
            disabled={pending || draft === null}
            onClick={() => {
              onUpdate({ settings: values })
              setDraft(null)
            }}
          >
            {t('save')}
          </button>
        </div>
      )}
    </div>
  )
}

/* ---------------------------------------------------------------- */
/* SSO                                                                */
/* ---------------------------------------------------------------- */

function SsoSection() {
  const t = useT()
  const queryClient = useQueryClient()
  const { data } = useQuery({ queryKey: ['admin-sso'], queryFn: api.adminGetSSO })

  const [form, setForm] = useState<SsoConfig | null>(null)
  const cfg: SsoConfig = form ??
    data?.sso ?? {
      enabled: false,
      name: '',
      issuer: '',
      clientId: '',
      redirectUrl: '',
      defaultRole: 'viewer',
      autoProvision: true,
    }

  const save = useMutation({
    mutationFn: () => api.adminPutSSO(cfg),
    onSuccess: () => {
      setForm(null)
      void queryClient.invalidateQueries({ queryKey: ['admin-sso'] })
    },
  })

  const set = (patch: Partial<SsoConfig>) => setForm({ ...cfg, ...patch })

  return (
    <section className="max-w-xl space-y-3">
      <label className="flex items-center gap-2 text-sm font-medium">
        <input
          type="checkbox"
          checked={cfg.enabled}
          onChange={(e) => set({ enabled: e.target.checked })}
          className="accent-violet-600"
        />
        {t('ssoEnabledLabel')}
      </label>

      <label className="block text-sm text-gray-600 dark:text-gray-400">
        {t('ssoNameLabel')}
        <input
          className={`${inputCls} mt-1`}
          value={cfg.name}
          onChange={(e) => set({ name: e.target.value })}
          placeholder="Keycloak / Google / …"
        />
      </label>
      <label className="block text-sm text-gray-600 dark:text-gray-400">
        {t('issuerLabel')}
        <input
          className={`${inputCls} mt-1`}
          value={cfg.issuer}
          onChange={(e) => set({ issuer: e.target.value })}
          placeholder="https://accounts.google.com"
        />
      </label>
      <label className="block text-sm text-gray-600 dark:text-gray-400">
        {t('clientIdLabel')}
        <input
          className={`${inputCls} mt-1`}
          value={cfg.clientId}
          onChange={(e) => set({ clientId: e.target.value })}
        />
      </label>
      <label className="block text-sm text-gray-600 dark:text-gray-400">
        {t('clientSecretLabel')}
        {data?.hasSecret && (
          <span className="ml-1 text-xs text-gray-500 dark:text-gray-400">({t('secretKept')})</span>
        )}
        <input
          className={`${inputCls} mt-1`}
          type="password"
          value={cfg.clientSecret ?? ''}
          onChange={(e) => set({ clientSecret: e.target.value })}
        />
      </label>
      <label className="block text-sm text-gray-600 dark:text-gray-400">
        {t('redirectUrlLabel')}
        <input
          className={`${inputCls} mt-1`}
          value={cfg.redirectUrl}
          onChange={(e) => set({ redirectUrl: e.target.value })}
        />
      </label>
      <label className="block text-sm text-gray-600 dark:text-gray-400">
        {t('defaultRoleLabel')}
        <select
          className={`${inputCls} mt-1`}
          value={cfg.defaultRole || 'viewer'}
          onChange={(e) => set({ defaultRole: e.target.value })}
        >
          <option value="viewer">viewer</option>
          <option value="editor">editor</option>
          <option value="admin">admin</option>
        </select>
      </label>
      <label className="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-400">
        <input
          type="checkbox"
          checked={cfg.autoProvision}
          onChange={(e) => set({ autoProvision: e.target.checked })}
          className="accent-violet-600"
        />
        {t('autoProvisionLabel')}
      </label>

      {save.error && <p className="text-sm text-red-600 dark:text-red-400">{(save.error as Error).message}</p>}
      <button
        onClick={() => save.mutate()}
        disabled={save.isPending || form === null}
        className={primaryBtnCls}
      >
        {t('ssoSaveBtn')}
      </button>
    </section>
  )
}

function splitGroups(s: string): string[] {
  return s
    .split(',')
    .map((g) => g.trim())
    .filter(Boolean)
}
