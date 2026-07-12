import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { useAuthStore, type Permission } from '../store/auth'
import { useT } from '../i18n'

export function TokensPage() {
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
    <div className="mx-auto max-w-3xl px-6 py-10">
      <h1 className="text-2xl font-bold">🔑 {t('tokensTitle')}</h1>

      <ul className="mt-6 space-y-2">
        {tokens?.map((tok) => (
          <li
            key={tok.id}
            className="flex items-center gap-3 rounded-xl border border-gray-200 px-4 py-3 dark:border-gray-800"
          >
            <div className="min-w-0 flex-1">
              <div className="font-medium">
                {tok.name}{' '}
                {tok.revoked && (
                  <span className="text-xs text-red-500">({t('revoked')})</span>
                )}
              </div>
              <div className="text-xs text-gray-400">
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
        className="mt-8 space-y-3 rounded-xl border border-dashed border-gray-300 p-4 dark:border-gray-700"
        onSubmit={(e) => {
          e.preventDefault()
          if (name.trim()) create.mutate()
        }}
      >
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder={t('tokenName')}
          className="w-full rounded-lg border border-gray-300 bg-transparent px-3 py-2 text-sm outline-none focus:border-violet-500 dark:border-gray-700"
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
                    setPerms(
                      e.target.checked ? [...perms, p] : perms.filter((x) => x !== p),
                    )
                  }
                  className="accent-violet-600"
                />
                <code className="text-xs">{p}</code>
              </label>
            ))}
          </div>
        </fieldset>
        {create.error && (
          <p className="text-sm text-red-500">{(create.error as Error).message}</p>
        )}
        <button
          type="submit"
          disabled={!name.trim() || create.isPending}
          className="rounded-lg bg-violet-600 px-4 py-2 text-sm font-medium text-white hover:bg-violet-700 disabled:opacity-50"
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
    </div>
  )
}
