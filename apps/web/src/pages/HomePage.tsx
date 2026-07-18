import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import { useT } from '../i18n'

export function HomePage() {
  const { data: recent } = useQuery({ queryKey: ['recent'], queryFn: () => api.recent(12) })
  const t = useT()

  // The vault-stats plugin powers this section: disabling it in the
  // settings hides the block (and its endpoint answers 404).
  const { data: pluginList } = useQuery({ queryKey: ['plugins'], queryFn: api.plugins })
  const statsEnabled = pluginList?.find((p) => p.id === 'vault-stats')?.enabled ?? false
  const { data: stats } = useQuery({
    queryKey: ['vault-stats'],
    queryFn: api.vaultStats,
    enabled: statsEnabled,
  })

  const statItems = stats
    ? ([
        [t('statsNotes'), stats.notes],
        [t('statsFolders'), stats.folders],
        [t('statsAttachments'), stats.attachments],
        [t('statsLinks'), stats.links],
        [t('statsBrokenLinks'), stats.brokenLinks],
      ] as const)
    : []

  return (
    <div className="mx-auto max-w-3xl px-6 py-10">
      <h1 className="text-3xl font-bold">
        {t('welcomeTo')} <span className="text-violet-600 dark:text-violet-400">Obsidian Web</span>
      </h1>
      <p className="mt-2 text-gray-500 dark:text-gray-400">
        {t('tagline')}{' '}
        <kbd className="rounded bg-gray-100 px-1.5 text-sm dark:bg-gray-800">⌘K</kbd>.
      </p>

      {statsEnabled && stats && (
        <section className="mt-10">
          <h2 className="mb-3 text-sm font-semibold tracking-wide text-gray-500 dark:text-gray-400 uppercase">
            {t('statsTitle')}
          </h2>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-5">
            {statItems.map(([label, value]) => (
              <div
                key={label}
                className="rounded-xl border border-gray-200 p-3 text-center dark:border-gray-800"
              >
                <div className="text-xl font-semibold">{value}</div>
                <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">{label}</div>
              </div>
            ))}
          </div>
        </section>
      )}

      {recent && recent.length > 0 && (
        <section className="mt-10">
          <h2 className="mb-3 text-sm font-semibold tracking-wide text-gray-500 dark:text-gray-400 uppercase">
            {t('recentlyUpdated')}
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
                <div className="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">{note.path}</div>
                <div className="mt-2 text-xs text-gray-500 dark:text-gray-400">
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
