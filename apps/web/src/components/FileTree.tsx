import { useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import type { TreeNode } from '../api/types'
import { useAuthStore } from '../store/auth'
import { useT } from '../i18n'
import { NewNoteDialog } from './NewNoteDialog'

function notePathToUrl(path: string): string {
  const clean = path.replace(/\.md$/i, '')
  return '/n/' + clean.split('/').map(encodeURIComponent).join('/')
}

// Actions attached to a folder (or the vault root) — create a note or a
// subfolder inside it. Shown only when the user can edit.
type FolderActions = {
  canEdit: boolean
  activeInput: string | null
  onNewNote: (folder: string) => void
  onNewFolder: (folder: string) => void
  onCloseInput: () => void
}

function NewFolderInput({
  parent,
  onDone,
  depth,
}: {
  parent: string
  onDone: () => void
  depth: number
}) {
  const [name, setName] = useState('')
  const queryClient = useQueryClient()
  const t = useT()
  const create = useMutation({
    mutationFn: (path: string) => api.createFolder(path),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['tree'] })
      onDone()
    },
  })

  const submit = () => {
    const trimmed = name.trim().replace(/^\/+|\/+$/g, '')
    if (trimmed) create.mutate(parent ? `${parent}/${trimmed}` : trimmed)
  }

  return (
    <div style={{ paddingLeft: `${depth * 12 + 20}px` }} className="py-1 pr-2">
      <input
        autoFocus
        value={name}
        onChange={(e) => setName(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter') submit()
          if (e.key === 'Escape') onDone()
        }}
        onBlur={onDone}
        placeholder={t('newFolderPlaceholder')}
        className="w-full rounded border border-violet-400 bg-transparent px-2 py-0.5 text-sm outline-none"
      />
      {create.error && (
        <p className="mt-0.5 text-xs text-red-500">{(create.error as Error).message}</p>
      )}
    </div>
  )
}

function TreeEntry({
  node,
  depth,
  onNavigate,
  actions,
}: {
  node: TreeNode
  depth: number
  onNavigate: () => void
  actions: FolderActions
}) {
  const [open, setOpen] = useState(depth < 1)
  const location = useLocation()

  if (node.isDir) {
    return (
      <div>
        <div
          className="group flex items-center rounded pr-1 hover:bg-gray-100 dark:hover:bg-gray-800"
          style={{ paddingLeft: `${depth * 12 + 8}px` }}
        >
          <button
            onClick={() => setOpen((v) => !v)}
            className="flex min-w-0 flex-1 items-center gap-1 py-1 text-left text-sm font-medium text-gray-700 dark:text-gray-300"
          >
            <span className="text-xs text-gray-400">{open ? '▾' : '▸'}</span>
            <span className="truncate">{node.name}</span>
          </button>
          {actions.canEdit && (
            <span className="flex shrink-0 items-center opacity-0 group-hover:opacity-100">
              <FolderActionButtons folder={node.path} actions={actions} />
            </span>
          )}
        </div>
        {actions.activeInput === node.path && (
          <NewFolderInput
            parent={node.path}
            depth={depth + 1}
            onDone={actions.onCloseInput}
          />
        )}
        {open &&
          node.children?.map((child) => (
            <TreeEntry
              key={child.path}
              node={child}
              depth={depth + 1}
              onNavigate={onNavigate}
              actions={actions}
            />
          ))}
      </div>
    )
  }

  const isNote = node.name.toLowerCase().endsWith('.md')
  if (!isNote) return null

  const url = notePathToUrl(node.path)
  const active = decodeURIComponent(location.pathname) === decodeURIComponent(url)
  return (
    <Link
      to={url}
      onClick={onNavigate}
      className={`block truncate rounded px-2 py-1 text-sm ${
        active
          ? 'bg-violet-100 text-violet-800 dark:bg-violet-950 dark:text-violet-300'
          : 'text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800'
      }`}
      style={{ paddingLeft: `${depth * 12 + 20}px` }}
    >
      {node.name.replace(/\.md$/i, '')}
    </Link>
  )
}

function FolderActionButtons({
  folder,
  actions,
}: {
  folder: string
  actions: FolderActions
}) {
  const t = useT()
  return (
    <>
      <button
        onClick={() => actions.onNewNote(folder)}
        title={t('newNoteHere')}
        aria-label={t('newNoteHere')}
        className="rounded px-1 text-sm text-gray-400 hover:text-violet-600 dark:hover:text-violet-400"
      >
        ＋
      </button>
      <button
        onClick={() => actions.onNewFolder(folder)}
        title={t('newFolderHere')}
        aria-label={t('newFolderHere')}
        className="rounded px-1 text-sm text-gray-400 hover:text-violet-600 dark:hover:text-violet-400"
      >
        🗀
      </button>
    </>
  )
}

export function FileTree({ onNavigate }: { onNavigate: () => void }) {
  const { data: tree, isLoading, error } = useQuery({ queryKey: ['tree'], queryFn: api.tree })
  const canEdit = useAuthStore((s) => s.can)('notes:edit')
  const t = useT()

  const [newNote, setNewNote] = useState<{ open: boolean; folder: string }>({
    open: false,
    folder: '',
  })
  // Which folder currently shows the inline "new folder" input ('' = root,
  // null = none). Only one at a time.
  const [folderInput, setFolderInput] = useState<string | null>(null)

  const actions: FolderActions = {
    canEdit,
    activeInput: folderInput,
    onNewNote: (folder) => setNewNote({ open: true, folder }),
    onNewFolder: (folder) => setFolderInput((cur) => (cur === folder ? null : folder)),
    onCloseInput: () => setFolderInput(null),
  }

  if (isLoading) return <p className="px-2 text-sm text-gray-400">{t('loadingVault')}</p>
  if (error) return <p className="px-2 text-sm text-red-500">{t('treeError')}</p>

  return (
    <nav aria-label={t('files')}>
      <div className="mb-1 flex items-center justify-between px-2">
        <h2 className="text-xs font-semibold tracking-wide text-gray-400 uppercase">
          {t('files')}
        </h2>
        {canEdit && (
          <button
            onClick={() => setFolderInput((cur) => (cur === '' ? null : ''))}
            title={t('newFolder')}
            aria-label={t('newFolder')}
            className="rounded px-1 text-sm text-gray-400 hover:text-violet-600 dark:hover:text-violet-400"
          >
            🗀＋
          </button>
        )}
      </div>
      {folderInput === '' && (
        <NewFolderInput parent="" depth={0} onDone={() => setFolderInput(null)} />
      )}
      {tree?.children?.map((child) => (
        <TreeEntry
          key={child.path}
          node={child}
          depth={0}
          onNavigate={onNavigate}
          actions={actions}
        />
      ))}
      <NewNoteDialog
        open={newNote.open}
        initialFolder={newNote.folder}
        onClose={() => setNewNote((s) => ({ ...s, open: false }))}
      />
    </nav>
  )
}
