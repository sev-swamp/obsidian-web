export interface NoteMeta {
  path: string
  title: string
  tags?: string[]
  aliases?: string[]
  modTime: string
  size: number
}

export interface Backlink {
  source: string
  title: string
}

export interface Note extends NoteMeta {
  content: string
  html?: string
  frontmatter?: Record<string, unknown>
  backlinks?: Backlink[]
}

export interface TreeNode {
  name: string
  path: string
  isDir: boolean
  children?: TreeNode[]
}

export interface SearchResult {
  path: string
  title: string
  snippet?: string
  tags?: string[]
  score: number
}

export interface NoteRules {
  defaultFolder: string
  typeFolders: Record<string, string> | null
  autoFrontmatter: boolean
}

export interface Settings {
  notes: NoteRules
  vault: { templatesDir: string; attachmentsDir: string }
}

export interface CreateNoteRequest {
  title: string
  folder?: string
  type?: string
  template?: string
  variables?: Record<string, string>
  content?: string
}

export interface VaultEvent {
  type:
    | 'file.created'
    | 'file.changed'
    | 'file.deleted'
    | 'tree.changed'
    | 'index.updated'
  path?: string
}
