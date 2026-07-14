import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { useT } from '../i18n'

// HistoryPanel lists revisions of a note; selecting one shows the diff
// against the current content and offers a restore.
export function HistoryPanel({
  path,
  canEdit,
  onClose,
}: {
  path: string
  canEdit: boolean
  onClose: () => void
}) {
  const [selected, setSelected] = useState<string | null>(null)
  const t = useT()
  const queryClient = useQueryClient()

  const { data: revisions } = useQuery({
    queryKey: ['history', path],
    queryFn: () => api.history(path),
  })

  // Show what the selected revision changed (diff against its parent).
  const { data: diff } = useQuery({
    queryKey: ['diff', path, selected],
    queryFn: () => api.diffRev(path, selected!),
    enabled: selected !== null,
  })

  const restore = useMutation({
    mutationFn: (rev: string) => api.restore(path, rev),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['note'] })
      void queryClient.invalidateQueries({ queryKey: ['history', path] })
      setSelected(null)
    },
  })

  return (
    <aside className="mt-8 rounded-xl border border-gray-200 dark:border-gray-800">
      <div className="flex items-center justify-between border-b border-gray-200 px-4 py-2.5 dark:border-gray-800">
        <h2 className="text-sm font-semibold tracking-wide text-gray-500 dark:text-gray-400 uppercase">
          {t('historyBtn')}
        </h2>
        <button
          onClick={onClose}
          aria-label={t('close')}
          className="rounded p-1 text-gray-500 dark:text-gray-400 hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-gray-800 dark:hover:text-gray-200"
        >
          ✕
        </button>
      </div>

      <div className="max-h-72 overflow-y-auto p-2">
        {revisions?.length === 0 && (
          <p className="px-2 py-3 text-sm text-gray-500 dark:text-gray-400">{t('noHistory')}</p>
        )}
        {revisions?.map((rev) => (
          <div key={rev.id}>
            <button
              onClick={() => setSelected(selected === rev.id ? null : rev.id)}
              className={`flex w-full items-baseline gap-2 rounded-lg px-2 py-1.5 text-left text-sm hover:bg-gray-100 dark:hover:bg-gray-800 ${
                selected === rev.id ? 'bg-violet-50 dark:bg-violet-950' : ''
              }`}
            >
              <span className="font-medium">{rev.actor}</span>
              <span className="text-xs text-gray-500 dark:text-gray-400">{rev.action}</span>
              <span className="ml-auto shrink-0 text-xs text-gray-500 dark:text-gray-400">
                {new Date(rev.time).toLocaleString()}
              </span>
            </button>
            {selected === rev.id && (
              <div className="mb-2 rounded-lg border border-gray-100 p-2 dark:border-gray-800">
                <pre className="max-h-56 overflow-auto font-mono text-xs whitespace-pre-wrap">
                  {(diff?.diff ?? '')
                    .split('\n')
                    .map((line, i) => (
                      <span
                        key={i}
                        className={
                          line.startsWith('+ ')
                            ? 'block bg-green-50 text-green-800 dark:bg-green-950 dark:text-green-300'
                            : line.startsWith('- ')
                              ? 'block bg-red-50 text-red-800 dark:bg-red-950 dark:text-red-300'
                              : 'block text-gray-500'
                        }
                      >
                        {line}
                      </span>
                    ))}
                </pre>
                {canEdit && (
                  <button
                    onClick={() => {
                      if (confirm(t('restoreConfirm'))) restore.mutate(rev.id)
                    }}
                    disabled={restore.isPending}
                    className="mt-2 rounded-lg border border-gray-300 px-3 py-1 text-xs hover:bg-gray-100 disabled:opacity-50 dark:border-gray-700 dark:hover:bg-gray-800"
                  >
                    {t('restoreAction')}
                  </button>
                )}
              </div>
            )}
          </div>
        ))}
      </div>
    </aside>
  )
}
