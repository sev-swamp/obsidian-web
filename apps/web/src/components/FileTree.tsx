import { useState } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '../api/client'
import type { TreeNode } from '../api/types'
import { useT } from '../i18n'

function notePathToUrl(path: string): string {
  const clean = path.replace(/\.md$/i, '')
  return '/n/' + clean.split('/').map(encodeURIComponent).join('/')
}

function TreeEntry({
  node,
  depth,
  onNavigate,
}: {
  node: TreeNode
  depth: number
  onNavigate: () => void
}) {
  const [open, setOpen] = useState(depth < 1)
  const location = useLocation()

  if (node.isDir) {
    return (
      <div>
        <button
          onClick={() => setOpen((v) => !v)}
          className="flex w-full items-center gap-1 rounded px-2 py-1 text-left text-sm font-medium text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-800"
          style={{ paddingLeft: `${depth * 12 + 8}px` }}
        >
          <span className="text-xs text-gray-400">{open ? '▾' : '▸'}</span>
          {node.name}
        </button>
        {open &&
          node.children?.map((child) => (
            <TreeEntry key={child.path} node={child} depth={depth + 1} onNavigate={onNavigate} />
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

export function FileTree({ onNavigate }: { onNavigate: () => void }) {
  const { data: tree, isLoading, error } = useQuery({ queryKey: ['tree'], queryFn: api.tree })
  const t = useT()

  if (isLoading) return <p className="px-2 text-sm text-gray-400">{t('loadingVault')}</p>
  if (error) return <p className="px-2 text-sm text-red-500">{t('treeError')}</p>

  return (
    <nav aria-label={t('files')}>
      <h2 className="mb-1 px-2 text-xs font-semibold tracking-wide text-gray-400 uppercase">
        {t('files')}
      </h2>
      {tree?.children?.map((child) => (
        <TreeEntry key={child.path} node={child} depth={0} onNavigate={onNavigate} />
      ))}
    </nav>
  )
}
