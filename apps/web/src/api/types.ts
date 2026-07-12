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
  contentHash: string
  html?: string
  frontmatter?: Record<string, unknown>
  backlinks?: Backlink[]
  access?: 'read' | 'write'
}

export interface Revision {
  id: string
  actor: string
  action: string
  message: string
  time: string
}

export interface DeletedFile {
  path: string
  actor: string
  time: string
  restoreRev: string
}

export interface ConflictInfo {
  currentHash: string
  currentContent: string
  changedBy?: string
  changedAt?: string
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
