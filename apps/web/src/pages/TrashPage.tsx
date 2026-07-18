import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { useAuthStore } from '../store/auth'
import { useT } from '../i18n'
import { TrashIcon } from '../components/icons'
import { useConfirm } from '../components/ConfirmDialog'

export function TrashPage() {
  const t = useT()
  const confirm = useConfirm()
  const queryClient = useQueryClient()
  const can = useAuthStore((s) => s.can)
  const canPurge = can('trash:purge')

  const { data: deleted, isLoading } = useQuery({
    queryKey: ['trash'],
    queryFn: api.trash,
  })

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ['trash'] })
    void queryClient.invalidateQueries({ queryKey: ['tree'] })
    void queryClient.invalidateQueries({ queryKey: ['recent'] })
  }

  const restore = useMutation({
    mutationFn: (path: string) => api.trashRestore(path),
    onSuccess: invalidate,
  })

  const purge = useMutation({
    mutationFn: (path: string) => api.trashPurge(path),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['trash'] }),
  })

  const purgeAll = useMutation({
    mutationFn: () => api.trashPurgeAll(),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['trash'] }),
  })

  const isPending = restore.isPending || purge.isPending || purgeAll.isPending

  return (
    <div className="mx-auto max-w-3xl px-6 py-10">
      <div className="flex items-center justify-between">
        <h1 className="flex items-center gap-2 text-2xl font-bold">
          <TrashIcon size={22} /> {t('trash')}
        </h1>
        {canPurge && deleted && deleted.length > 0 && (
          <button
            onClick={() =>
              void confirm({
                title: t('purgeAllConfirm'),
                message: t('purgeNote'),
                confirmLabel: t('purgeAllAction'),
                danger: true,
              }).then((ok) => ok && purgeAll.mutate())
            }
            disabled={isPending}
            className="rounded-lg border border-red-300 px-3 py-1.5 text-sm text-red-600 hover:bg-red-50 disabled:opacity-50 dark:border-red-800 dark:text-red-400 dark:hover:bg-red-950"
          >
            {t('purgeAllAction')}
          </button>
        )}
      </div>

      {isLoading && <p className="mt-6 text-gray-500 dark:text-gray-400">{t('loading')}</p>}
      {deleted && deleted.length === 0 && (
        <p className="mt-6 text-gray-500 dark:text-gray-400">{t('trashEmpty')}</p>
      )}

      <ul className="mt-6 space-y-2">
        {deleted?.map((file) => (
          <li
            key={`${file.path}@${file.deleteRev}`}
            className="flex items-center gap-3 rounded-xl border border-gray-200 px-4 py-3 dark:border-gray-800"
          >
            <div className="min-w-0 flex-1">
              <div className="truncate font-medium">{file.path}</div>
              <div className="text-xs text-gray-500 dark:text-gray-400">
                {t('deletedBy')} {file.actor} · {new Date(file.time).toLocaleString()}
              </div>
            </div>
            <button
              onClick={() => restore.mutate(file.path)}
              disabled={isPending}
              className="shrink-0 rounded-lg border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-100 disabled:opacity-50 dark:border-gray-700 dark:hover:bg-gray-800"
            >
              {t('restoreAction')}
            </button>
            {canPurge && (
              <button
                onClick={() =>
                  void confirm({
                    title: t('purgeConfirm'),
                    message: t('purgeNote'),
                    confirmLabel: t('purgeAction'),
                    danger: true,
                  }).then((ok) => ok && purge.mutate(file.path))
                }
                disabled={isPending}
                className="shrink-0 rounded-lg border border-red-300 px-3 py-1.5 text-sm text-red-600 hover:bg-red-50 disabled:opacity-50 dark:border-red-800 dark:text-red-400 dark:hover:bg-red-950"
              >
                {t('purgeAction')}
              </button>
            )}
          </li>
        ))}
      </ul>
    </div>
  )
}
