import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'

export function HomePage() {
  const { data: recent } = useQuery({ queryKey: ['recent'], queryFn: () => api.recent(12) })

  return (
    <div className="mx-auto max-w-3xl px-6 py-10">
      <h1 className="text-3xl font-bold">
        Welcome to <span className="text-violet-600 dark:text-violet-400">Obsidian Web</span>
      </h1>
      <p className="mt-2 text-gray-500 dark:text-gray-400">
        Your vault, in the browser. Pick a note from the sidebar or search with{' '}
        <kbd className="rounded bg-gray-100 px-1.5 text-sm dark:bg-gray-800">⌘K</kbd>.
      </p>

      {recent && recent.length > 0 && (
        <section className="mt-10">
          <h2 className="mb-3 text-sm font-semibold tracking-wide text-gray-400 uppercase">
            Recently updated
          </h2>
          <div className="grid gap-3 sm:grid-cols-2">
            {recent.map((note) => (
              <Link
                key={note.path}
                to={
                  '/n/' +
                  note.path.replace(/\.md$/i, '').split('/').map(encodeURIComponent).join('/')
                }
                className="rounded-xl border border-gray-200 p-4 transition-colors hover:border-violet-400 dark:border-gray-800 dark:hover:border-violet-600"
              >
                <div className="font-medium">{note.title}</div>
                <div className="mt-1 truncate text-xs text-gray-400">{note.path}</div>
                <div className="mt-2 text-xs text-gray-400">
                  {new Date(note.modTime).toLocaleString()}
                </div>
              </Link>
            ))}
          </div>
        </section>
      )}
    </div>
  )
}
