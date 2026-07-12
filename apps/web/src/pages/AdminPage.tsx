import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import type { AclRule } from '../api/types'
import { useT } from '../i18n'

const inputCls =
  'w-full rounded-lg border border-gray-300 bg-transparent px-3 py-2 text-sm outline-none focus:border-violet-500 dark:border-gray-700'
const btnCls =
  'rounded-lg border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-100 disabled:opacity-50 dark:border-gray-700 dark:hover:bg-gray-800'
const primaryBtnCls =
  'rounded-lg bg-violet-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-violet-700 disabled:opacity-50'

export function AdminPage() {
  const t = useT()
  const queryClient = useQueryClient()
  const invalidateUsers = () =>
    void queryClient.invalidateQueries({ queryKey: ['admin-users'] })

  const { data } = useQuery({ queryKey: ['admin-users'], queryFn: api.adminUsers })
  const { data: aclData } = useQuery({ queryKey: ['admin-acl'], queryFn: api.adminGetACL })

  // New user form
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
      invalidateUsers()
    },
  })

  // ACL editor
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

  // Access checker
  const [check, setCheck] = useState({ user: '', path: '' })
  const [checkResult, setCheckResult] = useState('')

  return (
    <div className="mx-auto max-w-4xl px-6 py-10">
      <h1 className="text-2xl font-bold">⚙️ {t('adminTitle')}</h1>

      {/* ---- users ---- */}
      <section className="mt-8">
        <h2 className="mb-3 text-sm font-semibold tracking-wide text-gray-400 uppercase">
          {t('usersSection')}
        </h2>
        <div className="space-y-2">
          {data?.users.map((u) => (
            <UserRow key={u.username} user={u} onChanged={invalidateUsers} />
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
            <option value="viewer">viewer</option>
            <option value="editor">editor</option>
            <option value="admin">admin</option>
          </select>
          <input
            className={inputCls}
            placeholder={t('groupsLabel')}
            value={newUser.groups}
            onChange={(e) => setNewUser({ ...newUser, groups: e.target.value })}
          />
          {createUser.error && (
            <p className="text-sm text-red-500 sm:col-span-2">
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

      {/* ---- ACL ---- */}
      <section className="mt-10">
        <h2 className="mb-2 text-sm font-semibold tracking-wide text-gray-400 uppercase">
          {t('aclSection')}
        </h2>
        <p className="mb-2 text-xs text-gray-500 dark:text-gray-400">{t('aclHint')}</p>
        <textarea
          value={rulesValue}
          onChange={(e) => setRulesText(e.target.value)}
          spellCheck={false}
          rows={10}
          className={`${inputCls} font-mono text-xs`}
        />
        {aclError && <p className="mt-1 text-sm text-red-500">{aclError}</p>}
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
      </section>

      {/* ---- access check ---- */}
      <section className="mt-10">
        <h2 className="mb-3 text-sm font-semibold tracking-wide text-gray-400 uppercase">
          {t('checkSection')}
        </h2>
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
                .then((r) => setCheckResult(r.access))
                .catch((e: Error) => setCheckResult(e.message))
            }}
            disabled={!check.user || !check.path}
            className={btnCls}
          >
            {t('checkBtn')}
          </button>
          {checkResult && (
            <span
              className={`rounded-full px-3 py-1 text-sm font-medium ${
                checkResult === 'write'
                  ? 'bg-green-100 text-green-800 dark:bg-green-950 dark:text-green-300'
                  : checkResult === 'read'
                    ? 'bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300'
                    : 'bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-300'
              }`}
            >
              {t('accessResult')}: {checkResult}
            </span>
          )}
        </div>
      </section>
    </div>
  )
}

function UserRow({
  user,
  onChanged,
}: {
  user: { username: string; role: string; groups: string[] | null }
  onChanged: () => void
}) {
  const t = useT()
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
        <option value="viewer">viewer</option>
        <option value="editor">editor</option>
        <option value="admin">admin</option>
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
      <button onClick={() => revoke.mutate()} className={btnCls} title={t('revokeSessions')}>
        ⛔
      </button>
      <button
        onClick={() => {
          if (confirm(`${t('deleteUserBtn')} ${user.username}?`)) remove.mutate()
        }}
        className={`${btnCls} text-red-600 dark:text-red-400`}
      >
        {t('deleteUserBtn')}
      </button>
      {(update.error || remove.error) && (
        <span className="w-full text-xs text-red-500">
          {((update.error || remove.error) as Error).message}
        </span>
      )}
    </div>
  )
}

function splitGroups(s: string): string[] {
  return s
    .split(',')
    .map((g) => g.trim())
    .filter(Boolean)
}
