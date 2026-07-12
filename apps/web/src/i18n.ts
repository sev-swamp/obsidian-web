import { useLangStore } from './store/lang'

// UI strings. Add a key to `en` first — its shape defines the valid
// keys — then mirror it in `ru`.
const en = {
  // Header / layout
  searchButton: 'Search…',
  searchAria: 'Search',
  newNote: '+ New note',
  toggleSidebar: 'Toggle sidebar',
  toggleTheme: 'Toggle theme',
  helpButton: 'Syntax reference',

  // Sidebar
  files: 'Files',
  loadingVault: 'Loading vault…',
  treeError: 'Failed to load tree',
  recentChanges: 'Recent changes',

  // User menu
  guest: 'Guest',
  language: 'Language',
  signOut: 'Sign out',

  // Breadcrumbs
  vault: 'Vault',

  // Home page
  welcomeTo: 'Welcome to',
  tagline: 'Your vault, in the browser. Pick a note from the sidebar or search with',
  recentlyUpdated: 'Recently updated',

  // Note page
  loading: 'Loading…',
  noteNotFound: 'Note not found',
  notExistYet: 'does not exist yet.',
  edit: 'Edit',
  delete: 'Delete',
  save: 'Save',
  cancel: 'Cancel',
  linkedMentions: 'Linked mentions',
  deleteConfirm: 'Delete',

  // Search dialog
  searchPlaceholder: 'Search notes… (supports tag:x and path:x)',
  noResults: 'No results',

  // New note dialog
  newNoteTitle: 'New note',
  titleLabel: 'Title',
  titlePlaceholder: 'My new note',
  folderLabel: 'Folder',
  defaultFolder: 'Default',
  vaultRoot: 'vault root',
  templateLabel: 'Template',
  none: 'None',
  create: 'Create',

  // Login page
  usernameLabel: 'Username',
  passwordLabel: 'Password',
  signIn: 'Sign in',

  // History & conflicts
  historyBtn: 'History',
  noHistory: 'No history yet',
  restoreAction: 'Restore',
  restoreConfirm: 'Restore this version?',
  conflictTitle: 'Save conflict',
  conflictBody: 'This note was changed while you were editing it.',
  changedBy: 'Changed by',
  overwriteMine: 'Overwrite with my version',
  takeTheirs: 'Take the new version',
  close: 'Close',

  // Presence
  editingNow: 'Editing now',
  viewingNow: 'Viewing',

  // Trash
  trash: 'Trash',
  trashEmpty: 'Trash is empty',
  deletedBy: 'deleted by',

  // Help dialog
  helpTitle: 'Syntax reference',
  helpPlaceholder: 'Search the reference… e.g.: bold, table, link',
  helpNothingFound: 'Nothing found for',
  helpClose: 'Close reference',

  // Admin
  adminTitle: 'Administration',
  usersSection: 'Users',
  roleLabel: 'Role',
  groupsLabel: 'Groups (comma-separated)',
  createUser: 'Create user',
  deleteUserBtn: 'Delete',
  revokeSessions: 'Revoke sessions',
  resetPassword: 'New password (leave empty to keep)',
  aclSection: 'Folder access rules (ACL)',
  aclHint:
    'JSON list of rules, evaluated top-down — the first matching glob decides. Example: [{"path":"HR/**","allow":[{"group":"hr","access":"write"}],"default":"none"}]. Unmatched paths are unrestricted; the global role remains the ceiling.',
  saveRules: 'Save rules',
  checkSection: 'Check access',
  pathLabel: 'Path',
  checkBtn: 'Check',
  accessResult: 'Access',

  // API tokens
  tokensTitle: 'API tokens',
  tokenName: 'Token name',
  ttlDaysLabel: 'Lifetime, days (0 = no expiry)',
  permissionsLabel: 'Permissions',
  createTokenBtn: 'Create token',
  tokenCreatedOnce: 'Copy the token now — it is shown only once:',
  revokeBtn: 'Revoke',
  revoked: 'revoked',
  neverExpires: 'no expiry',
  expiresLabel: 'expires',
}

const ru: Record<TKey, string> = {
  searchButton: 'Поиск…',
  searchAria: 'Поиск',
  newNote: '+ Новая заметка',
  toggleSidebar: 'Показать/скрыть панель',
  toggleTheme: 'Переключить тему',
  helpButton: 'Справка по синтаксису',

  files: 'Файлы',
  loadingVault: 'Загрузка хранилища…',
  treeError: 'Не удалось загрузить дерево',
  recentChanges: 'Последние изменения',

  guest: 'Гость',
  language: 'Язык',
  signOut: 'Выйти',

  vault: 'Хранилище',

  welcomeTo: 'Добро пожаловать в',
  tagline: 'Ваше хранилище — в браузере. Выберите заметку слева или откройте поиск:',
  recentlyUpdated: 'Недавно обновлённые',

  loading: 'Загрузка…',
  noteNotFound: 'Заметка не найдена',
  notExistYet: 'пока не существует.',
  edit: 'Редактировать',
  delete: 'Удалить',
  save: 'Сохранить',
  cancel: 'Отмена',
  linkedMentions: 'Обратные ссылки',
  deleteConfirm: 'Удалить',

  searchPlaceholder: 'Поиск по заметкам… (поддерживает tag:x и path:x)',
  noResults: 'Ничего не найдено',

  newNoteTitle: 'Новая заметка',
  titleLabel: 'Название',
  titlePlaceholder: 'Моя новая заметка',
  folderLabel: 'Папка',
  defaultFolder: 'По умолчанию',
  vaultRoot: 'корень хранилища',
  templateLabel: 'Шаблон',
  none: 'Без шаблона',
  create: 'Создать',

  usernameLabel: 'Имя пользователя',
  passwordLabel: 'Пароль',
  signIn: 'Войти',

  historyBtn: 'История',
  noHistory: 'Истории пока нет',
  restoreAction: 'Восстановить',
  restoreConfirm: 'Восстановить эту версию?',
  conflictTitle: 'Конфликт сохранения',
  conflictBody: 'Заметку изменили, пока вы её редактировали.',
  changedBy: 'Изменил(а)',
  overwriteMine: 'Перезаписать моей версией',
  takeTheirs: 'Взять новую версию',
  close: 'Закрыть',

  editingNow: 'Сейчас редактирует',
  viewingNow: 'Просматривают',

  trash: 'Корзина',
  trashEmpty: 'Корзина пуста',
  deletedBy: 'удалил(а)',

  helpTitle: 'Справка по синтаксису',
  helpPlaceholder: 'Поиск по справке… например: выделение жирным, таблица, ссылка',
  helpNothingFound: 'Ничего не найдено по запросу',
  helpClose: 'Закрыть справку',

  adminTitle: 'Администрирование',
  usersSection: 'Пользователи',
  roleLabel: 'Роль',
  groupsLabel: 'Группы (через запятую)',
  createUser: 'Создать пользователя',
  deleteUserBtn: 'Удалить',
  revokeSessions: 'Отозвать сессии',
  resetPassword: 'Новый пароль (пусто — не менять)',
  aclSection: 'Правила доступа к папкам (ACL)',
  aclHint:
    'JSON-список правил, проверяются сверху вниз — решает первый совпавший glob. Пример: [{"path":"HR/**","allow":[{"group":"hr","access":"write"}],"default":"none"}]. Пути без правила не ограничены; глобальная роль остаётся потолком.',
  saveRules: 'Сохранить правила',
  checkSection: 'Проверить доступ',
  pathLabel: 'Путь',
  checkBtn: 'Проверить',
  accessResult: 'Доступ',

  tokensTitle: 'API-токены',
  tokenName: 'Название токена',
  ttlDaysLabel: 'Срок жизни, дней (0 — бессрочный)',
  permissionsLabel: 'Разрешения',
  createTokenBtn: 'Создать токен',
  tokenCreatedOnce: 'Скопируйте токен сейчас — он показывается только один раз:',
  revokeBtn: 'Отозвать',
  revoked: 'отозван',
  neverExpires: 'бессрочный',
  expiresLabel: 'истекает',
}

export type TKey = keyof typeof en

const dicts = { en, ru }

// useT returns the translator for the current language.
export function useT() {
  const lang = useLangStore((s) => s.lang)
  return (key: TKey): string => dicts[lang][key]
}
