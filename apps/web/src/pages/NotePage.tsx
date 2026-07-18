import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError } from '../api/client'
import type { ConflictInfo } from '../api/types'
import { Breadcrumbs } from '../components/Breadcrumbs'
import { HistoryPanel } from '../components/HistoryPanel'
import { MarkdownView } from '../components/MarkdownView'
import { MarkdownEditor } from '../components/MarkdownEditor'
import { useConfirm } from '../components/ConfirmDialog'
import { useAuthStore } from '../store/auth'
import { useThemeStore } from '../store/theme'
import { usePresenceStore } from '../store/presence'
import { useWsStore } from '../store/ws'
import { useT } from '../i18n'
import { sendPresence } from '../ws'
import { PlusIcon, PencilIcon, EyeIcon, AlertTriangleIcon } from '../components/icons'

export function NotePage() {
  const params = useParams()
  const notePath = decodeURIComponent(params['*'] ?? '')
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState('')
  const [baseHash, setBaseHash] = useState('')
  const [conflict, setConflict] = useState<ConflictInfo | null>(null)
  const [historyOpen, setHistoryOpen] = useState(false)
  const confirm = useConfirm()
  const can = useAuthStore((s) => s.can)
  const username = useAuthStore((s) => s.username)
  const theme = useThemeStore((s) => s.theme)
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

  // Candidate notes for the editor's [[wiki-link]] autocomplete.
  const { data: allNotes } = useQuery({
    queryKey: ['notes'],
    queryFn: api.notes,
    enabled: editing,
    staleTime: 60_000,
  })

  const canEdit = can('notes:edit') && note?.access !== 'read'
  const canDelete = can('notes:delete') && note?.access !== 'read'
  const canHistory = can('history:read')

  // With history disabled or managed externally nothing lands in the
  // trash — deletion must warn that it is unrecoverable.
  const { data: settings } = useQuery({
    queryKey: ['settings'],
    queryFn: api.settings,
    staleTime: 60_000,
  })
  const deleteWarning = !settings
    ? undefined
    : !settings.history.enabled
      ? t('deleteNoHistoryWarning')
      : settings.history.mode === 'external'
        ? t('deleteExternalHistoryWarning')
        : undefined

  const canonicalPath = note?.path

  useEffect(() => {
    setEditing(false)
    setConflict(null)
    setHistoryOpen(false)
  }, [notePath])

  const connectionId = useWsStore((s) => s.connectionId)

  // Presence: announce viewing/editing. Re-runs on WS reconnect so the
  // server learns our state again after the old connection dropped.
  useEffect(() => {
    if (!canonicalPath) return
    sendPresence(canonicalPath, editing ? 'editing' : 'viewing')
    return () => sendPresence(canonicalPath, 'left')
  }, [canonicalPath, editing, connectionId])

  const presence = usePresenceStore((s) =>
    canonicalPath ? s.byPath[canonicalPath] : undefined,
  )
  const otherEditors = presence?.editors.filter((u) => u !== username) ?? []
  const otherViewers =
    presence?.viewers.filter((u) => u !== username && !otherEditors.includes(u)) ?? []

  const save = useMutation({
    mutationFn: (vars: { content: string; baseHash: string }) =>
      api.saveNote(notePath, vars.content, vars.baseHash),
    onSuccess: () => {
      setEditing(false)
      setConflict(null)
      void queryClient.invalidateQueries({ queryKey: ['note'] })
      void queryClient.invalidateQueries({ queryKey: ['recent'] })
      void queryClient.invalidateQueries({ queryKey: ['history'] })
    },
    onError: (err) => {
      if (err instanceof ApiError && err.status === 409) {
        setConflict(err.body as ConflictInfo)
      }
    },
  })

  const remove = useMutation({
    mutationFn: () => api.deleteNote(notePath),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['tree'] })
      navigate('/')
    },
  })

  // When a note doesn't exist (e.g. following a broken wiki-link), find out
  // whether the current user may create it in the target folder.
  const notFound = error instanceof ApiError && error.status === 404
  const { data: accessInfo, isLoading: accessLoading } = useQuery({
    queryKey: ['access', notePath],
    queryFn: () => api.access(notePath),
    enabled: notFound && can('notes:edit'),
  })

  const targetFolder = (() => {
    const clean = notePath.replace(/\.md$/i, '')
    const slash = clean.lastIndexOf('/')
    return slash >= 0 ? clean.slice(0, slash) : ''
  })()

  const createMissing = useMutation({
    mutationFn: () => {
      const clean = notePath.replace(/\.md$/i, '')
      const slash = clean.lastIndexOf('/')
      const title = slash >= 0 ? clean.slice(slash + 1) : clean
      return api.createNote({ title, folder: targetFolder })
    },
    onSuccess: (note) => {
      void queryClient.invalidateQueries({ queryKey: ['tree'] })
      void queryClient.invalidateQueries({ queryKey: ['note'] })
      navigate('/n/' + note.path.replace(/\.md$/i, '').split('/').map(encodeURIComponent).join('/'))
    },
  })

  if (!notePath) return null
  if (isLoading) {
    return <div className="p-8 text-gray-500 dark:text-gray-400">{t('loading')}</div>
  }
  if (error || !note) {
    const canCreate = accessInfo?.access === 'write'
    const accessDenied = notFound && can('notes:edit') && !accessLoading && !canCreate
    return (
      <div className="mx-auto max-w-3xl p-8">
        <Breadcrumbs path={notePath} />
        <h1 className="text-xl font-semibold">{t('noteNotFound')}</h1>
        <p className="mt-2 text-gray-500">
          “{notePath}” {t('notExistYet')}
        </p>
        {notFound && can('notes:edit') && (
          <div className="mt-6">
            {accessLoading && <p className="text-sm text-gray-500 dark:text-gray-400">{t('checkingAccess')}</p>}
            {canCreate && (
              <>
                <button
                  onClick={() => createMissing.mutate()}
                  disabled={createMissing.isPending}
                  className="inline-flex items-center gap-1.5 rounded-lg bg-violet-600 px-4 py-2 text-sm font-medium text-white hover:bg-violet-700 disabled:opacity-50"
                >
                  <PlusIcon /> {t('createThisNote')}
                </button>
                <p className="mt-2 text-xs text-gray-500 dark:text-gray-400">
                  {t('createNoteInFolder')} {targetFolder || t('vaultRoot')}
                </p>
                {createMissing.error && (
                  <p className="mt-2 text-sm text-red-600 dark:text-red-400">
                    {(createMissing.error as Error).message}
                  </p>
                )}
              </>
            )}
            {accessDenied && <p className="text-sm text-amber-600 dark:text-amber-400">{t('noFolderAccess')}</p>}
          </div>
        )}
      </div>
    )
  }

  return (
    <article className="mx-auto max-w-3xl px-6 py-8">
      <Breadcrumbs path={note.path} />

      {(otherEditors.length > 0 || otherViewers.length > 0) && (
        <div className="mb-4 flex flex-wrap items-center gap-2 text-xs">
          {otherEditors.length > 0 && (
            <span className="inline-flex items-center gap-1 rounded-full bg-amber-100 px-2.5 py-1 font-medium text-amber-800 dark:bg-amber-950 dark:text-amber-300">
              <PencilIcon size={13} /> {t('editingNow')}: {otherEditors.join(', ')}
            </span>
          )}
          {otherViewers.length > 0 && (
            <span className="inline-flex items-center gap-1 rounded-full bg-gray-100 px-2.5 py-1 text-gray-600 dark:bg-gray-800 dark:text-gray-400">
              <EyeIcon size={13} /> {t('viewingNow')}: {otherViewers.join(', ')}
            </span>
          )}
        </div>
      )}

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
          <NoteProperties
            frontmatter={note.frontmatter}
            definitions={settings?.notes.properties ?? []}
          />
        </div>
        <div className="flex shrink-0 gap-2">
          {editing ? (
            <>
              <button
                onClick={() => save.mutate({ content: draft, baseHash })}
                disabled={save.isPending}
                className="rounded-lg bg-violet-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-violet-700 disabled:opacity-50"
              >
                {t('save')}
              </button>
              <button
                onClick={() => {
                  setEditing(false)
                  setConflict(null)
                }}
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
                    setBaseHash(note.contentHash)
                    setEditing(true)
                  }}
                  className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-100 dark:border-gray-700 dark:hover:bg-gray-800"
                >
                  {t('edit')}
                </button>
              )}
              {canHistory && (
                <button
                  onClick={() => setHistoryOpen((v) => !v)}
                  className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm hover:bg-gray-100 dark:border-gray-700 dark:hover:bg-gray-800"
                >
                  {t('historyBtn')}
                </button>
              )}
              {canDelete && (
                <button
                  onClick={() =>
                    void confirm({
                      title: `${t('deleteConfirm')} "${note.title}"?`,
                      message: deleteWarning,
                      confirmLabel: t('delete'),
                      danger: true,
                    }).then((ok) => ok && remove.mutate())
                  }
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
        <MarkdownEditor
          value={draft}
          onChange={setDraft}
          onSave={(content) => save.mutate({ content, baseHash })}
          notes={allNotes ?? []}
          dark={theme === 'dark'}
        />
      ) : (
        <MarkdownView html={note.html ?? ''} />
      )}

      {historyOpen && canHistory && (
        <HistoryPanel
          path={note.path}
          canEdit={canEdit}
          onClose={() => setHistoryOpen(false)}
        />
      )}

      {!editing && note.backlinks && note.backlinks.length > 0 && (
        <footer className="mt-12 border-t border-gray-200 pt-4 dark:border-gray-800">
          <h2 className="mb-2 text-xs font-semibold tracking-wide text-gray-500 dark:text-gray-400 uppercase">
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

      {conflict && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
          <div className="w-full max-w-lg rounded-xl border border-gray-200 bg-white p-5 shadow-2xl dark:border-gray-700 dark:bg-gray-900">
            <h2 className="mb-2 flex items-center gap-2 text-lg font-semibold text-amber-600 dark:text-amber-400">
              <AlertTriangleIcon size={20} /> {t('conflictTitle')}
            </h2>
            <p className="text-sm text-gray-600 dark:text-gray-400">
              {t('conflictBody')}
              {conflict.changedBy && (
                <>
                  {' '}
                  {t('changedBy')}: <strong>{conflict.changedBy}</strong>
                  {conflict.changedAt &&
                    ` (${new Date(conflict.changedAt).toLocaleString()})`}
                </>
              )}
            </p>
            <div className="mt-4 flex flex-col gap-2">
              <button
                onClick={() =>
                  save.mutate({ content: draft, baseHash: conflict.currentHash })
                }
                className="rounded-lg bg-violet-600 px-3 py-2 text-sm font-medium text-white hover:bg-violet-700"
              >
                {t('overwriteMine')}
              </button>
              <button
                onClick={() => {
                  setDraft(conflict.currentContent)
                  setBaseHash(conflict.currentHash)
                  setConflict(null)
                }}
                className="rounded-lg border border-gray-300 px-3 py-2 text-sm hover:bg-gray-100 dark:border-gray-700 dark:hover:bg-gray-800"
              >
                {t('takeTheirs')}
              </button>
              <button
                onClick={() => setConflict(null)}
                className="rounded-lg px-3 py-2 text-sm text-gray-500 hover:bg-gray-100 dark:hover:bg-gray-800"
              >
                {t('close')}
              </button>
            </div>
          </div>
        </div>
      )}
    </article>
  )
}

function NoteProperties({
  frontmatter,
  definitions,
}: {
  frontmatter?: Record<string, unknown>
  definitions: { key: string; label: string; type: string }[]
}) {
  const values = definitions.flatMap((definition) => {
    const value = frontmatter?.[definition.key]
    if (value === undefined || value === null || value === '') return []
    return [{ ...definition, value }]
  })
  if (values.length === 0) return null

  return (
    <dl className="mt-4 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-sm">
      {values.map((property) => (
        <div key={property.key} className="contents">
          <dt className="text-gray-500 dark:text-gray-400">{property.label || property.key}</dt>
          <dd className="min-w-0 break-words">{formatPropertyValue(property.value, property.type)}</dd>
        </div>
      ))}
    </dl>
  )
}

function formatPropertyValue(value: unknown, type: string): string {
  if (Array.isArray(value)) return value.map(String).join(', ')
  if (type === 'date' && typeof value === 'string') return value.slice(0, 10)
  return String(value)
}
