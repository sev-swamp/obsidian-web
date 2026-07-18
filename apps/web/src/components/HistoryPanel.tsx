import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { useConfirm } from './ConfirmDialog'
import { useT } from '../i18n'

// HistoryPanel lists revisions of a note; selecting one shows what it
// changed (diff against its parent) and offers a restore. The top
// revision's diff describes the current content, so its button undoes
// the change (restores the previous revision) instead of "restoring"
// a state that is already on disk.
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
  const [notice, setNotice] = useState<string | null>(null)
  const t = useT()
  const confirm = useConfirm()
  const queryClient = useQueryClient()
  const rowRefs = useRef<Map<string, HTMLDivElement>>(new Map())

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
    onSuccess: (data) => {
      if (data.status === 'unchanged') {
        setNotice(t('restoreUnchanged'))
        return
      }
      void queryClient.invalidateQueries({ queryKey: ['note'] })
      void queryClient.invalidateQueries({ queryKey: ['history', path] })
      setSelected(null)
    },
    // A failed restore must never look like a silent no-op.
    onError: (err) => setNotice(err instanceof Error ? err.message : String(err)),
  })

  useEffect(() => {
    if (!notice) return
    const timer = setTimeout(() => setNotice(null), 5000)
    return () => clearTimeout(timer)
  }, [notice])

  // Jump to and highlight the revision a restore came from.
  const showSource = (sourceRev: string) => {
    setSelected(sourceRev)
    rowRefs.current.get(sourceRev)?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
  }

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

      {notice && (
        <p className="border-b border-gray-200 bg-amber-50 px-4 py-2 text-sm text-amber-800 dark:border-gray-800 dark:bg-amber-950 dark:text-amber-300">
          {notice}
        </p>
      )}

      <div className="max-h-72 overflow-y-auto p-2">
        {revisions?.length === 0 && (
          <p className="px-2 py-3 text-sm text-gray-500 dark:text-gray-400">{t('noHistory')}</p>
        )}
        {revisions?.map((rev, index) => {
          // The state *at* a delete revision is "file absent", so its
          // button restores the content as of the previous revision
          // instead. The top revision's diff describes the current
          // content — its button undoes the change (also the previous
          // revision's state). Everything else restores its own state.
          const next: (typeof rev) | undefined = revisions[index + 1]
          const restoreOp =
            rev.action === 'delete'
              ? next && {
                  target: next.id,
                  label: t('restoreDeletedAction'),
                  confirmTitle: t('restoreDeletedConfirm'),
                }
              : index === 0
                ? next && {
                    target: next.id,
                    label: t('rollbackAction'),
                    confirmTitle: t('rollbackConfirm'),
                  }
                : {
                    target: rev.id,
                    label: t('restoreAction'),
                    confirmTitle: t('restoreConfirm'),
                  }
          const sourceInList =
            rev.sourceRev && revisions.some((r) => r.id === rev.sourceRev)
              ? rev.sourceRev
              : null
          return (
            <div
              key={rev.id}
              ref={(el) => {
                if (el) rowRefs.current.set(rev.id, el)
                else rowRefs.current.delete(rev.id)
              }}
            >
              <button
                onClick={() => setSelected(selected === rev.id ? null : rev.id)}
                className={`flex w-full items-baseline gap-2 rounded-lg px-2 py-1.5 text-left text-sm hover:bg-gray-100 dark:hover:bg-gray-800 ${
                  selected === rev.id ? 'bg-violet-50 dark:bg-violet-950' : ''
                }`}
              >
                <span className="font-medium">{rev.actor}</span>
                <span className="text-xs text-gray-500 dark:text-gray-400">{rev.action}</span>
                {rev.sourceRev && (
                  <span
                    title={`${t('restoredFrom')} ${rev.sourceRev}`}
                    onClick={(e) => {
                      if (!sourceInList) return
                      e.stopPropagation()
                      showSource(sourceInList)
                    }}
                    className={`rounded bg-violet-100 px-1 font-mono text-[11px] text-violet-700 dark:bg-violet-900 dark:text-violet-300 ${
                      sourceInList ? 'cursor-pointer hover:underline' : ''
                    }`}
                  >
                    ← {rev.sourceRev.slice(0, 7)}
                  </span>
                )}
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
                  {canEdit && restoreOp && (
                    <button
                      onClick={() =>
                        void confirm({
                          title: restoreOp.confirmTitle,
                          confirmLabel: restoreOp.label,
                        }).then((ok) => ok && restore.mutate(restoreOp.target))
                      }
                      disabled={restore.isPending}
                      className="mt-2 rounded-lg border border-gray-300 px-3 py-1 text-xs hover:bg-gray-100 disabled:opacity-50 dark:border-gray-700 dark:hover:bg-gray-800"
                    >
                      {restoreOp.label}
                    </button>
                  )}
                </div>
              )}
            </div>
          )
        })}
      </div>
    </aside>
  )
}
