import { useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import { useAuthStore } from '../store/auth'
import { useT } from '../i18n'
import { LockIcon } from '../components/icons'

export function LoginPage() {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const setSession = useAuthStore((s) => s.setSession)
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const t = useT()

  const { data: status } = useQuery({ queryKey: ['auth-status'], queryFn: api.authStatus })
  const { data: providers } = useQuery({
    queryKey: ['auth-providers'],
    queryFn: api.authProviders,
  })

  // Finish an SSO redirect: an external plugin bounces the browser back
  // with a one-time login code (?code=) or an error (?sso_error=).
  useEffect(() => {
    const ssoError = searchParams.get('sso_error')
    if (ssoError) {
      setError(ssoError)
      setSearchParams({}, { replace: true })
      return
    }
    const code = searchParams.get('code')
    if (!code) return
    setSearchParams({}, { replace: true })
    void api
      .exchangeLoginCode(code)
      .then((res) => {
        setSession(res.token, res.username, res.role, res.permissions ?? [])
        navigate('/')
      })
      .catch((err: Error) => setError(err.message))
  }, [searchParams, setSearchParams, setSession, navigate])

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    try {
      const res = await api.login(username, password)
      setSession(res.token, res.username, res.role, res.permissions ?? [])
      navigate('/')
    } catch (err) {
      setError((err as Error).message)
    }
  }

  if (status && !status.authEnabled) {
    navigate('/')
    return null
  }

  return (
    <div className="flex h-full items-center justify-center bg-gray-50 dark:bg-gray-950">
      <form
        onSubmit={submit}
        className="w-full max-w-sm rounded-2xl border border-gray-200 bg-white p-8 shadow-sm dark:border-gray-800 dark:bg-gray-900"
      >
        <h1 className="mb-6 text-center text-xl font-bold text-gray-900 dark:text-gray-100">
          <span className="text-violet-600 dark:text-violet-400">◈</span> Obsidian Web
        </h1>
        <label className="block text-sm text-gray-700 dark:text-gray-300">
          {t('usernameLabel')}
          <input
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoFocus
            className="mt-1 mb-3 w-full rounded-lg border border-gray-300 bg-transparent px-3 py-2 outline-none focus:border-violet-500 focus:ring-2 focus:ring-violet-500/30 dark:border-gray-700 dark:text-gray-100"
          />
        </label>
        <label className="block text-sm text-gray-700 dark:text-gray-300">
          {t('passwordLabel')}
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="mt-1 w-full rounded-lg border border-gray-300 bg-transparent px-3 py-2 outline-none focus:border-violet-500 focus:ring-2 focus:ring-violet-500/30 dark:border-gray-700 dark:text-gray-100"
          />
        </label>
        {error && <p className="mt-3 text-sm text-red-600 dark:text-red-400">{error}</p>}
        <button
          type="submit"
          className="mt-6 w-full rounded-lg bg-violet-600 py-2 font-medium text-white hover:bg-violet-700"
        >
          {t('signIn')}
        </button>

        {(providers?.providers.length ?? 0) > 0 && (
          <>
            <div className="my-4 flex items-center gap-3 text-xs text-gray-500 dark:text-gray-400">
              <div className="h-px flex-1 bg-gray-200 dark:bg-gray-700" />
              ·
              <div className="h-px flex-1 bg-gray-200 dark:bg-gray-700" />
            </div>
            <div className="space-y-2">
              {providers?.providers.map((p) => (
                <a
                  key={p.id}
                  href={p.url}
                  className="flex w-full items-center justify-center gap-2 rounded-lg border border-gray-300 py-2 text-center text-sm font-medium text-gray-700 hover:bg-gray-100 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800"
                >
                  <LockIcon size={15} /> {p.name}
                </a>
              ))}
            </div>
          </>
        )}
      </form>
    </div>
  )
}
