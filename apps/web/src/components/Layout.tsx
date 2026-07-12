import { useEffect, useState } from 'react'
import { Link, Outlet } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import { FileTree } from './FileTree'
import { RecentList } from './RecentList'
import { SearchDialog } from './SearchDialog'
import { NewNoteDialog } from './NewNoteDialog'
import { HelpDialog } from './HelpDialog'
import { UserMenu } from './UserMenu'
import { useAuthStore } from '../store/auth'
import { useThemeStore } from '../store/theme'
import { useT } from '../i18n'

export function Layout() {
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [searchOpen, setSearchOpen] = useState(false)
  const [newNoteOpen, setNewNoteOpen] = useState(false)
  const [helpOpen, setHelpOpen] = useState(false)
  const { theme, toggle } = useThemeStore()
  const canEdit = useAuthStore((s) => s.can)('notes:edit')
  const t = useT()

  const { data: pluginList } = useQuery({ queryKey: ['plugins'], queryFn: api.plugins })
  const recentEnabled =
    pluginList?.find((p) => p.id === 'recent-changes')?.enabled ?? true

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault()
        setSearchOpen((v) => !v)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  return (
    <div className="flex h-full flex-col bg-white text-gray-900 dark:bg-gray-950 dark:text-gray-100">
      <header className="flex h-14 shrink-0 items-center gap-3 border-b border-gray-200 px-4 dark:border-gray-800">
        <button
          className="rounded p-1.5 hover:bg-gray-100 md:hidden dark:hover:bg-gray-800"
          onClick={() => setSidebarOpen((v) => !v)}
          aria-label={t('toggleSidebar')}
        >
          ☰
        </button>
        <Link to="/" className="flex items-center gap-2 font-semibold">
          <span className="text-violet-600 dark:text-violet-400">◈</span>
          Obsidian Web
        </Link>
        <div className="flex-1" />
        <button
          onClick={() => setSearchOpen(true)}
          className="hidden items-center gap-2 rounded-lg border border-gray-300 px-3 py-1.5 text-sm text-gray-500 hover:border-gray-400 sm:flex dark:border-gray-700 dark:text-gray-400"
        >
          {t('searchButton')}
          <kbd className="rounded bg-gray-100 px-1.5 text-xs dark:bg-gray-800">⌘K</kbd>
        </button>
        <button
          onClick={() => setSearchOpen(true)}
          className="rounded p-1.5 hover:bg-gray-100 sm:hidden dark:hover:bg-gray-800"
          aria-label={t('searchAria')}
        >
          🔍
        </button>
        {canEdit && (
          <button
            onClick={() => setNewNoteOpen(true)}
            className="rounded-lg bg-violet-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-violet-700"
          >
            {t('newNote')}
          </button>
        )}
        <button
          onClick={() => setHelpOpen(true)}
          title={t('helpButton')}
          aria-label={t('helpButton')}
          className="flex h-8 w-8 items-center justify-center rounded-full border border-gray-300 text-sm font-semibold text-gray-500 hover:border-violet-500 hover:text-violet-600 dark:border-gray-700 dark:text-gray-400 dark:hover:border-violet-500 dark:hover:text-violet-400"
        >
          ?
        </button>
        <button
          onClick={toggle}
          className="rounded p-1.5 hover:bg-gray-100 dark:hover:bg-gray-800"
          aria-label={t('toggleTheme')}
        >
          {theme === 'dark' ? '☀️' : '🌙'}
        </button>
      </header>

      <div className="flex min-h-0 flex-1">
        <aside
          className={`${
            sidebarOpen ? 'flex' : 'hidden'
          } absolute z-20 h-[calc(100%-3.5rem)] w-72 shrink-0 flex-col border-r border-gray-200 bg-white md:static md:flex md:h-auto dark:border-gray-800 dark:bg-gray-950`}
        >
          <div className="min-h-0 flex-1 overflow-y-auto p-3">
            <FileTree onNavigate={() => setSidebarOpen(false)} />
            {recentEnabled && <RecentList onNavigate={() => setSidebarOpen(false)} />}
          </div>
          <div className="p-2">
            <UserMenu />
          </div>
        </aside>

        <main className="min-w-0 flex-1 overflow-y-auto">
          <Outlet />
        </main>
      </div>

      <SearchDialog open={searchOpen} onClose={() => setSearchOpen(false)} />
      <NewNoteDialog open={newNoteOpen} onClose={() => setNewNoteOpen(false)} />
      <HelpDialog open={helpOpen} onClose={() => setHelpOpen(false)} />
    </div>
  )
}
