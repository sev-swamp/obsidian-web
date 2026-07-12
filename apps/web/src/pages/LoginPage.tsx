import { useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import { useAuthStore } from '../store/auth'
import { useT } from '../i18n'

export function LoginPage() {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const setSession = useAuthStore((s) => s.setSession)
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const t = useT()

  const { data: status } = useQuery({ queryKey: ['auth-status'], queryFn: api.authStatus })
  const { data: sso } = useQuery({ queryKey: ['sso-status'], queryFn: api.ssoStatus })

  // Finish an SSO redirect: the callback hands us a session token.
  useEffect(() => {
    const ssoError = searchParams.get('sso_error')
    if (ssoError) {
      setError(ssoError)
      setSearchParams({}, { replace: true })
      return
    }
    const ssoToken = searchParams.get('sso_token')
    if (!ssoToken) return
    setSearchParams({}, { replace: true })
    void api
      .me(ssoToken)
      .then((me) => {
        setSession(ssoToken, me.username, me.role, me.permissions ?? [])
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
            className="mt-1 mb-3 w-full rounded-lg border border-gray-300 bg-transparent px-3 py-2 outline-none focus:border-violet-500 dark:border-gray-700 dark:text-gray-100"
          />
        </label>
        <label className="block text-sm text-gray-700 dark:text-gray-300">
          {t('passwordLabel')}
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="mt-1 w-full rounded-lg border border-gray-300 bg-transparent px-3 py-2 outline-none focus:border-violet-500 dark:border-gray-700 dark:text-gray-100"
          />
        </label>
        {error && <p className="mt-3 text-sm text-red-500">{error}</p>}
        <button
          type="submit"
          className="mt-6 w-full rounded-lg bg-violet-600 py-2 font-medium text-white hover:bg-violet-700"
        >
          {t('signIn')}
        </button>

        {sso?.enabled && (
          <>
            <div className="my-4 flex items-center gap-3 text-xs text-gray-400">
              <div className="h-px flex-1 bg-gray-200 dark:bg-gray-700" />
              ·
              <div className="h-px flex-1 bg-gray-200 dark:bg-gray-700" />
            </div>
            <a
              href="/api/auth/sso/login"
              className="block w-full rounded-lg border border-gray-300 py-2 text-center text-sm font-medium text-gray-700 hover:bg-gray-100 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800"
            >
              🔐 {t('ssoLoginWith')} {sso.name}
            </a>
          </>
        )}
      </form>
    </div>
  )
}
