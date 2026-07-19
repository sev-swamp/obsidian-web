import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { useAuthStore } from '../store/auth'
import { useLangStore, type Lang } from '../store/lang'
import { useT } from '../i18n'
import { SettingsIcon, GlobeIcon, TrashIcon } from './icons'

const langNames: Record<Lang, string> = { en: 'English', ru: 'Русский' }

// UserMenu sits at the bottom of the sidebar: the current user's name
// opens an upward dropdown with Settings, a Language submenu (flyout),
// Trash and Sign out.
export function UserMenu() {
  const [open, setOpen] = useState(false)
  const [langOpen, setLangOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  const token = useAuthStore((s) => s.token)
  const username = useAuthStore((s) => s.username)
  const role = useAuthStore((s) => s.role)
  const logout = useAuthStore((s) => s.logout)
  const can = useAuthStore((s) => s.can)
  const { lang, setLang } = useLangStore()
  const t = useT()
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  useEffect(() => {
    if (!open) {
      setLangOpen(false)
      return
    }
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
  const itemCls =
    'flex w-full items-center gap-2 rounded-lg px-2.5 py-1.5 text-sm hover:bg-gray-100 dark:hover:bg-gray-800'
  const go = (path: string) => {
    setOpen(false)
    navigate(path)
  }

  return (
    <div ref={ref} className="relative border-t border-gray-200 pt-2 dark:border-gray-800">
      {open && (
        <div className="absolute bottom-full left-0 z-30 mb-1 w-56 rounded-xl border border-gray-200 bg-white p-1.5 shadow-xl dark:border-gray-700 dark:bg-gray-900">
          {/* Settings hold personal preferences too, so everyone gets in;
              admin tabs inside are still gated by settings:write. */}
          <button onClick={() => go('/settings')} className={itemCls}>
            <SettingsIcon size={15} className="text-gray-500 dark:text-gray-400" />
            {t('settingsTitle')}
          </button>

          {/* Language as a submenu with a right-side flyout. */}
          <div className="relative">
            <button
              onClick={() => setLangOpen((v) => !v)}
              aria-haspopup="menu"
              aria-expanded={langOpen}
              className={`${itemCls} justify-between`}
            >
              <span className="flex items-center gap-2">
                <GlobeIcon size={15} className="text-gray-500 dark:text-gray-400" />
                {t('language')}
              </span>
              <span className="text-gray-500 dark:text-gray-400" aria-hidden>
                ›
              </span>
            </button>
            {langOpen && (
              <div className="absolute top-0 left-full z-40 ml-1.5 w-44 rounded-xl border border-gray-200 bg-white p-1.5 shadow-xl dark:border-gray-700 dark:bg-gray-900">
                {(Object.keys(langNames) as Lang[]).map((code) => (
                  <button
                    key={code}
                    onClick={() => {
                      setLang(code)
                      setLangOpen(false)
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
              </div>
            )}
          </div>

          {can('trash:read') && (
            <button onClick={() => go('/trash')} className={itemCls}>
              <TrashIcon size={15} className="text-gray-500 dark:text-gray-400" />
              {t('trash')}
            </button>
          )}

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
          {role && <span className="block text-xs text-gray-500 dark:text-gray-400">{role}</span>}
        </span>
        <span className="text-xs text-gray-500 dark:text-gray-400" aria-hidden>
          {open ? '▾' : '▴'}
        </span>
      </button>
    </div>
  )
}
