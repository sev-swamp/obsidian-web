import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import { useT } from '../i18n'

export function RecentList({ onNavigate }: { onNavigate: () => void }) {
  const { data: recent } = useQuery({
    queryKey: ['recent'],
    queryFn: () => api.recent(8),
  })
  const t = useT()

  if (!recent?.length) return null

  return (
    <section className="mt-6">
      <h2 className="mb-1 px-2 text-xs font-semibold tracking-wide text-gray-500 dark:text-gray-400 uppercase">
        {t('recentChanges')}
      </h2>
      {recent.map((note) => (
        <Link
          key={note.path}
          to={'/n/' + note.path.replace(/\.md$/i, '').split('/').map(encodeURIComponent).join('/')}
          onClick={onNavigate}
          className="block truncate rounded px-2 py-1 text-sm text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
        >
          {note.title}
          <span className="ml-2 text-xs text-gray-500 dark:text-gray-400">
            {new Date(note.modTime).toLocaleDateString()}
          </span>
        </Link>
      ))}
    </section>
  )
}
