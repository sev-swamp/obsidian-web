import { useEffect, useState } from 'react'
import { Link, Outlet, useNavigate } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { FileTree } from './FileTree'
import { RecentList } from './RecentList'
import { SearchDialog } from './SearchDialog'
import { NewNoteDialog } from './NewNoteDialog'
import { useAuthStore } from '../store/auth'
import { useThemeStore } from '../store/theme'

export function Layout() {
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [searchOpen, setSearchOpen] = useState(false)
  const [newNoteOpen, setNewNoteOpen] = useState(false)
  const { theme, toggle } = useThemeStore()
  const token = useAuthStore((s) => s.token)
  const username = useAuthStore((s) => s.username)
  const logout = useAuthStore((s) => s.logout)
  const navigate = useNavigate()
  const queryClient = useQueryClient()

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
          aria-label="Toggle sidebar"
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
          Search…
          <kbd className="rounded bg-gray-100 px-1.5 text-xs dark:bg-gray-800">⌘K</kbd>
        </button>
        <button
          onClick={() => setSearchOpen(true)}
          className="rounded p-1.5 hover:bg-gray-100 sm:hidden dark:hover:bg-gray-800"
          aria-label="Search"
        >
          🔍
        </button>
        <button
          onClick={() => setNewNoteOpen(true)}
          className="rounded-lg bg-violet-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-violet-700"
        >
          + New note
        </button>
        <button
          onClick={toggle}
          className="rounded p-1.5 hover:bg-gray-100 dark:hover:bg-gray-800"
          aria-label="Toggle theme"
        >
          {theme === 'dark' ? '☀️' : '🌙'}
        </button>
        {token && (
          <button
            onClick={() => {
              logout()
              queryClient.clear() // drop data cached for the previous user
              navigate('/login')
            }}
            title={username ? `Signed in as ${username} — sign out` : 'Sign out'}
            className="flex items-center gap-1.5 rounded-lg border border-gray-300 px-2.5 py-1.5 text-sm text-gray-600 hover:border-red-400 hover:text-red-600 dark:border-gray-700 dark:text-gray-400 dark:hover:border-red-700 dark:hover:text-red-400"
          >
            {username && <span className="hidden sm:inline">{username}</span>}
            <span aria-hidden>⎋</span>
            <span className="sr-only">Sign out</span>
          </button>
        )}
      </header>

      <div className="flex min-h-0 flex-1">
        <aside
          className={`${
            sidebarOpen ? 'block' : 'hidden'
          } absolute z-20 h-[calc(100%-3.5rem)] w-72 shrink-0 overflow-y-auto border-r border-gray-200 bg-white p-3 md:static md:block md:h-auto dark:border-gray-800 dark:bg-gray-950`}
        >
          <FileTree onNavigate={() => setSidebarOpen(false)} />
          <RecentList onNavigate={() => setSidebarOpen(false)} />
        </aside>

        <main className="min-w-0 flex-1 overflow-y-auto">
          <Outlet />
        </main>
      </div>

      <SearchDialog open={searchOpen} onClose={() => setSearchOpen(false)} />
      <NewNoteDialog open={newNoteOpen} onClose={() => setNewNoteOpen(false)} />
    </div>
  )
}
