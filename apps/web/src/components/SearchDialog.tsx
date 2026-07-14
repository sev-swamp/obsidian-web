import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import { useT } from '../i18n'

export function SearchDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [query, setQuery] = useState('')
  const navigate = useNavigate()
  const t = useT()

  const { data: results } = useQuery({
    queryKey: ['search', query],
    queryFn: () => api.search(query),
    enabled: open && query.trim().length > 0,
  })

  useEffect(() => {
    if (open) setQuery('')
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && open) onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, onClose])

  if (!open) return null

  const goTo = (path: string) => {
    onClose()
    navigate('/n/' + path.replace(/\.md$/i, '').split('/').map(encodeURIComponent).join('/'))
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-black/40 pt-24"
      onClick={onClose}
    >
      <div
        className="w-full max-w-xl rounded-xl border border-gray-200 bg-white shadow-2xl dark:border-gray-700 dark:bg-gray-900"
        onClick={(e) => e.stopPropagation()}
      >
        <input
          autoFocus
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder={t('searchPlaceholder')}
          className="w-full border-b border-gray-200 bg-transparent px-4 py-3 outline-none dark:border-gray-700"
        />
        <ul className="max-h-96 overflow-y-auto p-2">
          {results?.map((r) => (
            <li key={r.path}>
              <button
                onClick={() => goTo(r.path)}
                className="w-full rounded-lg px-3 py-2 text-left hover:bg-gray-100 dark:hover:bg-gray-800"
              >
                <div className="flex items-baseline gap-2">
                  <span className="font-medium">{r.title}</span>
                  <span className="truncate text-xs text-gray-500 dark:text-gray-400">{r.path}</span>
                </div>
                {r.snippet && (
                  <p className="mt-0.5 line-clamp-2 text-sm text-gray-500 dark:text-gray-400">
                    {r.snippet}
                  </p>
                )}
              </button>
            </li>
          ))}
          {query.trim() && results?.length === 0 && (
            <li className="px-3 py-4 text-center text-sm text-gray-500 dark:text-gray-400">{t('noResults')}</li>
          )}
        </ul>
      </div>
    </div>
  )
}
