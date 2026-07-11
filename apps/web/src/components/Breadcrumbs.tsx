import { Link } from 'react-router-dom'

export function Breadcrumbs({ path }: { path: string }) {
  const parts = path.replace(/\.md$/i, '').split('/').filter(Boolean)

  return (
    <nav aria-label="Breadcrumb" className="mb-4 text-sm text-gray-500 dark:text-gray-400">
      <Link to="/" className="hover:text-violet-600 dark:hover:text-violet-400">
        Vault
      </Link>
      {parts.map((part, i) => (
        <span key={i}>
          <span className="mx-1.5 text-gray-300 dark:text-gray-600">/</span>
          {i === parts.length - 1 ? (
            <span className="text-gray-900 dark:text-gray-200">{part}</span>
          ) : (
            <span>{part}</span>
          )}
        </span>
      ))}
    </nav>
  )
}
