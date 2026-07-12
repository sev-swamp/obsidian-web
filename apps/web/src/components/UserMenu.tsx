import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { useAuthStore } from '../store/auth'
import { useLangStore, type Lang } from '../store/lang'
import { useT } from '../i18n'

const langNames: Record<Lang, string> = { en: 'English', ru: 'Русский' }

// UserMenu sits at the bottom of the sidebar: the current user's name
// opens an upward dropdown with the language switcher and sign-out.
export function UserMenu() {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  const token = useAuthStore((s) => s.token)
  const username = useAuthStore((s) => s.username)
  const role = useAuthStore((s) => s.role)
  const logout = useAuthStore((s) => s.logout)
  const { lang, setLang } = useLangStore()
  const t = useT()
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  useEffect(() => {
    if (!open) return
    const onClickOutside = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', onClickOutside)
    window.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onClickOutside)
      window.removeEventListener('keydown', onKey)
    }
  }, [open])

  const name = username ?? t('guest')

  return (
    <div ref={ref} className="relative border-t border-gray-200 pt-2 dark:border-gray-800">
      {open && (
        <div className="absolute bottom-full left-0 z-30 mb-1 w-56 rounded-xl border border-gray-200 bg-white p-1.5 shadow-xl dark:border-gray-700 dark:bg-gray-900">
          <p className="px-2.5 pt-1 pb-1.5 text-xs font-semibold tracking-wide text-gray-400 uppercase">
            {t('language')}
          </p>
          {(Object.keys(langNames) as Lang[]).map((code) => (
            <button
              key={code}
              onClick={() => {
                setLang(code)
                setOpen(false)
              }}
              className={`flex w-full items-center justify-between rounded-lg px-2.5 py-1.5 text-sm hover:bg-gray-100 dark:hover:bg-gray-800 ${
                lang === code ? 'font-medium text-violet-600 dark:text-violet-400' : ''
              }`}
            >
              {langNames[code]}
              {lang === code && <span aria-hidden>✓</span>}
            </button>
          ))}
          {token && (
            <>
              <div className="my-1.5 border-t border-gray-100 dark:border-gray-800" />
              <button
                onClick={() => {
                  setOpen(false)
                  logout()
                  queryClient.clear() // drop data cached for the previous user
                  navigate('/login')
                }}
                className="flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-sm text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-950"
              >
                <span aria-hidden>⎋</span>
                {t('signOut')}
              </button>
            </>
          )}
        </div>
      )}

      <button
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="menu"
        aria-expanded={open}
        className="flex w-full items-center gap-2.5 rounded-lg px-2 py-2 text-left hover:bg-gray-100 dark:hover:bg-gray-800"
      >
        <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-violet-600 text-xs font-bold text-white uppercase">
          {name.slice(0, 1)}
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-medium">{name}</span>
          {role && <span className="block text-xs text-gray-400">{role}</span>}
        </span>
        <span className="text-xs text-gray-400" aria-hidden>
          {open ? '▾' : '▴'}
        </span>
      </button>
    </div>
  )
}
