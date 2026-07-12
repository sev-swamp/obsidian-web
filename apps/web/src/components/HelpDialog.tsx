import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { helpSections, type HelpSection } from '../help/content'

// Wraps every match of `query` in <mark> so search hits are visible.
function highlight(text: string, query: string): ReactNode {
  if (!query) return text
  const lower = text.toLowerCase()
  const q = query.toLowerCase()
  const parts: ReactNode[] = []
  let pos = 0
  for (;;) {
    const idx = lower.indexOf(q, pos)
    if (idx < 0) {
      parts.push(text.slice(pos))
      break
    }
    if (idx > pos) parts.push(text.slice(pos, idx))
    parts.push(<mark key={idx}>{text.slice(idx, idx + q.length)}</mark>)
    pos = idx + q.length
  }
  return parts
}

function matches(haystack: string, q: string): boolean {
  return haystack.toLowerCase().includes(q)
}

// Returns sections filtered by the query: a hit in the title/keywords
// keeps the whole section, otherwise only the matching entries remain.
function filterSections(query: string): HelpSection[] {
  const q = query.trim().toLowerCase()
  if (!q) return helpSections
  const out: HelpSection[] = []
  for (const section of helpSections) {
    if (matches(section.title, q) || matches(section.keywords, q)) {
      out.push(section)
      continue
    }
    const entries = section.entries.filter(
      (e) => matches(e.code, q) || matches(e.text, q),
    )
    if (entries.length > 0) out.push({ ...section, entries })
  }
  return out
}

export function HelpDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [query, setQuery] = useState('')

  useEffect(() => {
    if (open) setQuery('')
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && open) onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, onClose])

  const sections = useMemo(() => filterSections(query), [query])

  if (!open) return null

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-black/40 p-4 pt-14 sm:pt-20"
      onClick={onClose}
    >
      <div
        className="flex max-h-full w-full max-w-2xl flex-col rounded-xl border border-gray-200 bg-white shadow-2xl dark:border-gray-700 dark:bg-gray-900"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-2 border-b border-gray-200 px-4 py-3 dark:border-gray-700">
          <h2 className="text-lg font-semibold">Справка по синтаксису</h2>
          <div className="flex-1" />
          <button
            onClick={onClose}
            aria-label="Закрыть справку"
            className="rounded p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-gray-800 dark:hover:text-gray-200"
          >
            ✕
          </button>
        </div>

        <input
          autoFocus
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Поиск по справке… например: выделение жирным, таблица, ссылка"
          className="border-b border-gray-200 bg-transparent px-4 py-2.5 text-sm outline-none dark:border-gray-700"
        />

        <div className="overflow-y-auto p-4">
          {sections.length === 0 && (
            <p className="py-8 text-center text-sm text-gray-400">
              Ничего не найдено по запросу «{query}»
            </p>
          )}
          {sections.map((section) => (
            <section key={section.id} className="mb-6 last:mb-0">
              <h3 className="mb-2 text-sm font-semibold tracking-wide text-violet-600 uppercase dark:text-violet-400">
                {highlight(section.title, query.trim())}
              </h3>
              <div className="space-y-2">
                {section.entries.map((entry, i) => (
                  <div
                    key={i}
                    className="grid grid-cols-1 gap-1 rounded-lg border border-gray-100 p-3 sm:grid-cols-2 sm:gap-4 dark:border-gray-800"
                  >
                    <pre className="overflow-x-auto rounded bg-gray-50 px-2.5 py-1.5 font-mono text-xs whitespace-pre-wrap text-gray-800 dark:bg-gray-800 dark:text-gray-200">
                      {highlight(entry.code, query.trim())}
                    </pre>
                    <p className="text-sm text-gray-600 dark:text-gray-400">
                      {highlight(entry.text, query.trim())}
                    </p>
                  </div>
                ))}
              </div>
            </section>
          ))}
        </div>
      </div>
    </div>
  )
}
