import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { Breadcrumbs } from '../components/Breadcrumbs'
import { MarkdownView } from '../components/MarkdownView'
import { useAuthStore } from '../store/auth'
import { useT } from '../i18n'

export function NotePage() {
  const params = useParams()
  const notePath = decodeURIComponent(params['*'] ?? '')
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState('')
  const can = useAuthStore((s) => s.can)
  const canEdit = can('notes:edit')
  const canDelete = can('notes:delete')
  const t = useT()

  const {
    data: note,
    isLoading,
    error,
  } = useQuery({
    queryKey: ['note', notePath.endsWith('.md') ? notePath : notePath + '.md'],
    queryFn: () => api.note(notePath),
    enabled: notePath.length > 0,
  })

  useEffect(() => {
    setEditing(false)
  }, [notePath])

  const save = useMutation({
    mutationFn: () => api.saveNote(notePath, draft),
    onSuccess: () => {
      setEditing(false)
      void queryClient.invalidateQueries({ queryKey: ['note'] })
      void queryClient.invalidateQueries({ queryKey: ['recent'] })
    },
  })

  const remove = useMutation({
    mutationFn: () => api.deleteNote(notePath),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['tree'] })
      navigate('/')
    },
  })

  if (!notePath) return null
  if (isLoading) {
    return <div className="p-8 text-gray-400">{t('loading')}</div>
  }
  if (error || !note) {
    return (
      <div className="mx-auto max-w-3xl p-8">
        <Breadcrumbs path={notePath} />
        <h1 className="text-xl font-semibold">{t('noteNotFound')}</h1>
        <p className="mt-2 text-gray-500">
          “{notePath}” {t('notExistYet')}
        </p>
      </div>
    )
  }

  return (
    <article className="mx-auto max-w-3xl px-6 py-8">
      <Breadcrumbs path={note.path} />

      <div className="mb-6 flex items-start justify-between gap-4">
        <div>
          <h1 className="text-3xl font-bold">{note.title}</h1>
          {note.tags && note.tags.length > 0 && (
            <div className="mt-2 flex flex-wrap gap-1.5">
              {note.tags.map((tag) => (
                <span
                  key={tag}
                  className="rounded-full bg-violet-100 px-2.5 py-0.5 text-xs font-medium text-violet-700 dark:bg-violet-950 dark:text-violet-300"
                >
                  #{tag}
                </span>
              ))}
            </div>
          )}
        </div>
        <div className="flex shrink-0 gap-2">
          {editing ? (
            <>
              <button
                onClick={() => save.mutate()}
                disabled={save.isPending}
                className="rounded-lg bg-violet-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-violet-700 disabled:opacity-50"
              >
                {t('save')}
              </button>
              <button
                onClick={() => setEditing(false)}
                className="rounded-lg px-3 py-1.5 text-sm hover:bg-gray-100 dark:hover:bg-gray-800"
              >
                {t('cancel')}
              </button>
            </>
          ) : (
            <>
              {canEdit && (
                <button
                  onClick={() => {
                    setDraft(note.content)
                    setEditing(true)
                  }}
                  className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-100 dark:border-gray-700 dark:hover:bg-gray-800"
                >
                  {t('edit')}
                </button>
              )}
              {canDelete && (
                <button
                  onClick={() => {
                    if (confirm(`${t('deleteConfirm')} "${note.title}"?`)) remove.mutate()
                  }}
                  className="rounded-lg border border-red-300 px-3 py-1.5 text-sm text-red-600 hover:bg-red-50 dark:border-red-900 dark:text-red-400 dark:hover:bg-red-950"
                >
                  {t('delete')}
                </button>
              )}
            </>
          )}
        </div>
      </div>

      {editing ? (
        <textarea
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          spellCheck={false}
          className="h-[70vh] w-full resize-y rounded-lg border border-gray-300 bg-gray-50 p-4 font-mono text-sm outline-none focus:border-violet-500 dark:border-gray-700 dark:bg-gray-900"
        />
      ) : (
        <MarkdownView html={note.html ?? ''} />
      )}

      {!editing && note.backlinks && note.backlinks.length > 0 && (
        <footer className="mt-12 border-t border-gray-200 pt-4 dark:border-gray-800">
          <h2 className="mb-2 text-xs font-semibold tracking-wide text-gray-400 uppercase">
            {t('linkedMentions')}
          </h2>
          <ul className="space-y-1">
            {note.backlinks.map((bl) => (
              <li key={bl.source}>
                <button
                  onClick={() =>
                    navigate(
                      '/n/' +
                        bl.source.replace(/\.md$/i, '').split('/').map(encodeURIComponent).join('/'),
                    )
                  }
                  className="text-sm text-violet-600 hover:underline dark:text-violet-400"
                >
                  {bl.title}
                </button>
              </li>
            ))}
          </ul>
        </footer>
      )}
    </article>
  )
}
