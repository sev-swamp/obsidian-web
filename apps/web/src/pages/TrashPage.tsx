import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { useT } from '../i18n'

export function TrashPage() {
  const t = useT()
  const queryClient = useQueryClient()

  const { data: deleted, isLoading } = useQuery({
    queryKey: ['trash'],
    queryFn: api.trash,
  })

  const restore = useMutation({
    mutationFn: (path: string) => api.trashRestore(path),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['trash'] })
      void queryClient.invalidateQueries({ queryKey: ['tree'] })
      void queryClient.invalidateQueries({ queryKey: ['recent'] })
    },
  })

  return (
    <div className="mx-auto max-w-3xl px-6 py-10">
      <h1 className="text-2xl font-bold">🗑 {t('trash')}</h1>

      {isLoading && <p className="mt-6 text-gray-400">{t('loading')}</p>}
      {deleted && deleted.length === 0 && (
        <p className="mt-6 text-gray-400">{t('trashEmpty')}</p>
      )}

      <ul className="mt-6 space-y-2">
        {deleted?.map((file) => (
          <li
            key={file.path}
            className="flex items-center gap-3 rounded-xl border border-gray-200 px-4 py-3 dark:border-gray-800"
          >
            <div className="min-w-0 flex-1">
              <div className="truncate font-medium">{file.path}</div>
              <div className="text-xs text-gray-400">
                {t('deletedBy')} {file.actor} · {new Date(file.time).toLocaleString()}
              </div>
            </div>
            <button
              onClick={() => restore.mutate(file.path)}
              disabled={restore.isPending}
              className="shrink-0 rounded-lg border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-100 disabled:opacity-50 dark:border-gray-700 dark:hover:bg-gray-800"
            >
              {t('restoreAction')}
            </button>
          </li>
        ))}
      </ul>
    </div>
  )
}
