import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import type { TreeNode } from '../api/types'
import { useT } from '../i18n'

function collectFolders(node: TreeNode | undefined, acc: string[] = []): string[] {
  if (!node) return acc
  for (const child of node.children ?? []) {
    if (child.isDir) {
      acc.push(child.path)
      collectFolders(child, acc)
    }
  }
  return acc
}

export function NewNoteDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [title, setTitle] = useState('')
  const [folder, setFolder] = useState('')
  const [template, setTemplate] = useState('')
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const t = useT()

  const { data: tree } = useQuery({ queryKey: ['tree'], queryFn: api.tree, enabled: open })
  const { data: templates } = useQuery({
    queryKey: ['templates'],
    queryFn: api.templates,
    enabled: open,
  })
  const { data: settings } = useQuery({
    queryKey: ['settings'],
    queryFn: api.settings,
    enabled: open,
  })

  const folders = useMemo(() => collectFolders(tree), [tree])

  const create = useMutation({
    mutationFn: () => api.createNote({ title, folder, template: template || undefined }),
    onSuccess: (note) => {
      void queryClient.invalidateQueries({ queryKey: ['tree'] })
      setTitle('')
      onClose()
      // Open the created note right away.
      navigate('/n/' + note.path.replace(/\.md$/i, '').split('/').map(encodeURIComponent).join('/'))
    },
  })

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40" onClick={onClose}>
      <div
        className="w-full max-w-md rounded-xl border border-gray-200 bg-white p-5 shadow-2xl dark:border-gray-700 dark:bg-gray-900"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="mb-4 text-lg font-semibold">{t('newNoteTitle')}</h2>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            if (title.trim()) create.mutate()
          }}
          className="space-y-3"
        >
          <label className="block text-sm">
            {t('titleLabel')}
            <input
              autoFocus
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              className="mt-1 w-full rounded-lg border border-gray-300 bg-transparent px-3 py-2 outline-none focus:border-violet-500 dark:border-gray-700"
              placeholder={t('titlePlaceholder')}
            />
          </label>
          <label className="block text-sm">
            {t('folderLabel')}
            <select
              value={folder}
              onChange={(e) => setFolder(e.target.value)}
              className="mt-1 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 dark:border-gray-700 dark:bg-gray-900"
            >
              <option value="">
                {t('defaultFolder')} ({settings?.notes.defaultFolder || t('vaultRoot')})
              </option>
              {folders.map((f) => (
                <option key={f} value={f}>
                  {f}
                </option>
              ))}
            </select>
          </label>
          <label className="block text-sm">
            {t('templateLabel')}
            <select
              value={template}
              onChange={(e) => setTemplate(e.target.value)}
              className="mt-1 w-full rounded-lg border border-gray-300 bg-white px-3 py-2 dark:border-gray-700 dark:bg-gray-900"
            >
              <option value="">{t('none')}</option>
              {templates?.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>
          </label>
          {create.error && (
            <p className="text-sm text-red-500">{(create.error as Error).message}</p>
          )}
          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg px-3 py-1.5 text-sm hover:bg-gray-100 dark:hover:bg-gray-800"
            >
              {t('cancel')}
            </button>
            <button
              type="submit"
              disabled={!title.trim() || create.isPending}
              className="rounded-lg bg-violet-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-violet-700 disabled:opacity-50"
            >
              {t('create')}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
