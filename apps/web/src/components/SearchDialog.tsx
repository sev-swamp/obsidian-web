import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import type { PropertyInfo } from '../api/types'
import { useSearchStore } from '../store/search'
import { useT } from '../i18n'

/** Quote a filter value when it contains spaces: prop:created="2026-07-18 16:00". */
export function propertyQuery(key: string, value: string) {
  return `prop:${key}=${/\s/.test(value) ? `"${value}"` : value}`
}

interface Suggestion {
  label: string
  /** Replacement for the token currently being typed. */
  insert: string
  /** Keep focus in the value position (key suggestions end with "="). */
  partial: boolean
}

// The token under the cursor is the tail after the last space that is not
// inside quotes; values may contain spaces only when quoted.
function lastToken(query: string): { head: string; token: string } {
  let at = -1
  let inQuotes = false
  for (let i = 0; i < query.length; i++) {
    if (query[i] === '"') inQuotes = !inQuotes
    else if (query[i] === ' ' && !inQuotes) at = i
  }
  return { head: query.slice(0, at + 1), token: query.slice(at + 1) }
}

function suggestFor(query: string, props: PropertyInfo[] | undefined): Suggestion[] {
  if (!props?.length) return []
  const { token } = lastToken(query)
  if (!token.startsWith('prop:')) return []
  const rest = token.slice(5)
  const m = rest.match(/^([^=:<>]+)(>=|<=|=|>|<|:)(.*)$/)
  if (!m) {
    return props
      .filter((p) => p.key.toLowerCase().startsWith(rest.toLowerCase()))
      .slice(0, 8)
      .map((p) => ({ label: `${p.key} (${p.count})`, insert: `prop:${p.key}=`, partial: true }))
  }
  const [, key, op, prefix] = m
  const bare = prefix.replace(/^"|"$/g, '').toLowerCase()
  const info = props.find((p) => p.key.toLowerCase() === key.toLowerCase())
  return (info?.values ?? [])
    .filter((v) => v.value.toLowerCase().includes(bare))
    .slice(0, 8)
    .map((v) => ({
      label: v.value,
      insert: `prop:${key}${op}${/\s/.test(v.value) ? `"${v.value}"` : v.value}`,
      partial: false,
    }))
}

export function SearchDialog() {
  const open = useSearchStore((s) => s.open)
  const initialQuery = useSearchStore((s) => s.initialQuery)
  const onClose = useSearchStore((s) => s.close)
  const [query, setQuery] = useState('')
  const navigate = useNavigate()
  const t = useT()

  const { data: results } = useQuery({
    queryKey: ['search', query],
    queryFn: () => api.search(query),
    enabled: open && query.trim().length > 0,
  })

  const { data: props } = useQuery({
    queryKey: ['properties'],
    queryFn: api.properties,
    enabled: open,
    staleTime: 60_000,
  })

  useEffect(() => {
    if (open) setQuery(initialQuery)
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && open) onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, initialQuery, onClose])

  if (!open) return null

  const goTo = (path: string) => {
    onClose()
    navigate('/n/' + path.replace(/\.md$/i, '').split('/').map(encodeURIComponent).join('/'))
  }

  const suggestions = suggestFor(query, props)
  const applySuggestion = (s: Suggestion) => {
    const { head } = lastToken(query)
    setQuery(head + s.insert + (s.partial ? '' : ' '))
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
        {suggestions.length > 0 && (
          <div className="flex flex-wrap gap-1.5 border-b border-gray-200 p-2 dark:border-gray-700">
            {suggestions.map((s) => (
              <button
                key={s.insert}
                onClick={() => applySuggestion(s)}
                className="rounded-full bg-gray-100 px-2.5 py-0.5 text-xs text-gray-700 hover:bg-violet-100 hover:text-violet-700 dark:bg-gray-800 dark:text-gray-300 dark:hover:bg-violet-950 dark:hover:text-violet-300"
              >
                {s.label}
              </button>
            ))}
          </div>
        )}
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
