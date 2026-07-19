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
  searchPlaceholder: 'Search notes… (tag:x, path:x, prop:key=value)',
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
  confirm: 'Confirm',
  folderPlaceholder: 'Existing or new folder…',

  // Folder / tree actions
  newFolder: 'New folder',
  newFolderTitle: 'New folder',
  newFolderPlaceholder: 'folder or path/to/folder',
  newNoteHere: 'New note here',
  newFolderHere: 'New folder here',
  folderNameLabel: 'Folder name',

  // Missing note (broken link target)
  createThisNote: 'Create this note',
  createNoteInFolder: 'Create it in',
  noFolderAccess: 'You have no write access to this folder — the note cannot be created here.',
  checkingAccess: 'Checking access…',

  // Login page
  usernameLabel: 'Username',
  passwordLabel: 'Password',
  signIn: 'Sign in',

  // History & conflicts
  historyBtn: 'History',
  noHistory: 'No history yet',
  restoreAction: 'Restore',
  restoreConfirm: 'Restore this version?',
  rollbackAction: 'Undo this change',
  rollbackConfirm: 'Undo this change and return the note to its previous version?',
  restoreDeletedAction: 'Restore deleted content',
  restoreDeletedConfirm: 'Restore the note content as it was before this deletion?',
  restoreUnchanged: 'The content already matches this version — nothing to restore.',
  restoredFrom: 'restored from',
  conflictTitle: 'Save conflict',
  conflictBody: 'This note was changed while you were editing it.',
  changedBy: 'Changed by',
  overwriteMine: 'Overwrite with my version',
  takeTheirs: 'Take the new version',
  close: 'Close',

  // Presence
  editingNow: 'Editing now',
  viewingNow: 'Viewing',

  // Vault statistics (vault-stats plugin)
  statsTitle: 'Vault statistics',
  statsNotes: 'Notes',
  statsFolders: 'Folders',
  statsAttachments: 'Attachments',
  statsLinks: 'Links',
  statsBrokenLinks: 'Broken links',

  // Trash
  trash: 'Trash',
  trashEmpty: 'Trash is empty',
  deletedBy: 'deleted by',
  purgeAction: 'Remove from trash',
  purgeAllAction: 'Clear trash',
  purgeConfirm: 'Remove this entry from the trash?',
  purgeNote: 'The note content stays in history; only the trash entry is removed.',
  purgeAllConfirm: 'Remove all entries from the trash?',
  deleteNoHistoryWarning:
    'History is disabled — the note cannot be restored after deletion.',
  deleteExternalHistoryWarning:
    'History is managed by an external git repository — the note will not appear in the trash and cannot be restored here.',

  // Help dialog
  helpTitle: 'Syntax reference',
  helpPlaceholder: 'Search the reference… e.g.: bold, table, link',
  helpNothingFound: 'Nothing found for',
  helpClose: 'Close reference',

  // Settings
  settingsTitle: 'Settings',
  tabUsers: 'Users',
  tabRoles: 'Roles',
  tabGroups: 'Groups',
  tabAccess: 'Access (ACL)',
  tabTokens: 'API tokens',
  tabSSO: 'SSO',
  tabPlugins: 'Plugins',
  tabGeneral: 'General',
  editorSection: 'Editor',
  prefsHint: 'These preferences are personal and stored in this browser.',
  lineNumbersToggle: 'Show line numbers when editing',
  openInEditToggle: 'Open notes in edit mode right away',
  pluginSettingsTitle: 'Plugin settings',
  rolesHint: 'Roles bundle permissions. The three built-in roles cannot be deleted; you can create your own. Permission changes apply the next time a user signs in.',
  roleNameLabel: 'Role name',
  roleDescriptionLabel: 'Description',
  createRole: 'Create role',
  saveRole: 'Save',
  deleteRoleBtn: 'Delete',
  roleBuiltin: 'built-in',
  roleAllPermissions: 'all permissions',
  roleAdminFixed: 'The admin role always has full access and cannot be restricted.',
  pluginKindBackend: 'server',
  pluginKindUI: 'interface',
  pluginEnabled: 'Enabled',
  groupNameLabel: 'Group name',
  addGroupBtn: 'Add group',
  membersLabel: 'members',
  noGroups: 'No groups yet',
  ssoEnabledLabel: 'Enable SSO (OpenID Connect)',
  ssoNameLabel: 'Provider name (shown on the login button)',
  issuerLabel: 'Issuer URL',
  clientIdLabel: 'Client ID',
  clientSecretLabel: 'Client secret',
  secretKept: 'leave empty to keep the current one',
  redirectUrlLabel: 'Redirect URL (empty = auto: <host>/api/auth/sso/callback)',
  defaultRoleLabel: 'Role for new SSO users',
  autoProvisionLabel: 'Create accounts automatically on first sign-in',
  ssoSaveBtn: 'Save SSO settings',
  ssoLoginWith: 'Sign in with',
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

  searchPlaceholder: 'Поиск по заметкам… (tag:x, path:x, prop:ключ=значение)',
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
  confirm: 'Подтвердить',
  folderPlaceholder: 'Существующая или новая папка…',

  // Folder / tree actions
  newFolder: 'Новая папка',
  newFolderTitle: 'Новая папка',
  newFolderPlaceholder: 'папка или путь/к/папке',
  newNoteHere: 'Новая заметка здесь',
  newFolderHere: 'Новая папка здесь',
  folderNameLabel: 'Имя папки',

  // Missing note (broken link target)
  createThisNote: 'Создать эту заметку',
  createNoteInFolder: 'Будет создана в',
  noFolderAccess: 'Нет прав на запись в эту папку — заметку здесь создать нельзя.',
  checkingAccess: 'Проверка доступа…',

  usernameLabel: 'Имя пользователя',
  passwordLabel: 'Пароль',
  signIn: 'Войти',

  historyBtn: 'История',
  noHistory: 'Истории пока нет',
  restoreAction: 'Восстановить',
  restoreConfirm: 'Восстановить эту версию?',
  rollbackAction: 'Откатить это изменение',
  rollbackConfirm: 'Откатить это изменение и вернуть заметку к предыдущей версии?',
  restoreDeletedAction: 'Восстановить удалённое',
  restoreDeletedConfirm: 'Восстановить содержимое заметки, каким оно было до этого удаления?',
  restoreUnchanged: 'Содержимое уже совпадает с этой версией — восстанавливать нечего.',
  restoredFrom: 'восстановлено из',
  conflictTitle: 'Конфликт сохранения',
  conflictBody: 'Заметку изменили, пока вы её редактировали.',
  changedBy: 'Изменил(а)',
  overwriteMine: 'Перезаписать моей версией',
  takeTheirs: 'Взять новую версию',
  close: 'Закрыть',

  editingNow: 'Сейчас редактирует',
  viewingNow: 'Просматривают',

  statsTitle: 'Статистика хранилища',
  statsNotes: 'Заметки',
  statsFolders: 'Папки',
  statsAttachments: 'Вложения',
  statsLinks: 'Ссылки',
  statsBrokenLinks: 'Битые ссылки',

  trash: 'Корзина',
  trashEmpty: 'Корзина пуста',
  deletedBy: 'удалил(а)',
  purgeAction: 'Убрать из корзины',
  purgeAllAction: 'Очистить корзину',
  purgeConfirm: 'Убрать эту запись из корзины?',
  purgeNote: 'Содержимое заметки остаётся в истории; удаляется только запись корзины.',
  purgeAllConfirm: 'Убрать все записи из корзины?',
  deleteNoHistoryWarning:
    'История выключена — после удаления заметку нельзя будет восстановить.',
  deleteExternalHistoryWarning:
    'Историей управляет внешний git-репозиторий — заметка не попадёт в корзину, восстановить её здесь будет нельзя.',

  helpTitle: 'Справка по синтаксису',
  helpPlaceholder: 'Поиск по справке… например: выделение жирным, таблица, ссылка',
  helpNothingFound: 'Ничего не найдено по запросу',
  helpClose: 'Закрыть справку',

  settingsTitle: 'Настройки',
  tabUsers: 'Пользователи',
  tabRoles: 'Роли',
  tabGroups: 'Группы',
  tabAccess: 'Доступ (ACL)',
  tabTokens: 'API-токены',
  tabSSO: 'SSO',
  tabPlugins: 'Плагины',
  tabGeneral: 'Общие',
  editorSection: 'Редактор',
  prefsHint: 'Эти настройки персональные и хранятся в этом браузере.',
  lineNumbersToggle: 'Показывать номера строк при редактировании',
  openInEditToggle: 'Открывать заметки сразу в режиме редактирования',
  pluginSettingsTitle: 'Настройки плагина',
  rolesHint: 'Роли объединяют разрешения. Три встроенные роли нельзя удалить; можно создавать свои. Изменения разрешений применяются при следующем входе пользователя.',
  roleNameLabel: 'Название роли',
  roleDescriptionLabel: 'Описание',
  createRole: 'Создать роль',
  saveRole: 'Сохранить',
  deleteRoleBtn: 'Удалить',
  roleBuiltin: 'встроенная',
  roleAllPermissions: 'все разрешения',
  roleAdminFixed: 'Роль admin всегда имеет полный доступ и не может быть ограничена.',
  pluginKindBackend: 'серверный',
  pluginKindUI: 'интерфейсный',
  pluginEnabled: 'Включён',
  groupNameLabel: 'Название группы',
  addGroupBtn: 'Добавить группу',
  membersLabel: 'участники',
  noGroups: 'Групп пока нет',
  ssoEnabledLabel: 'Включить SSO (OpenID Connect)',
  ssoNameLabel: 'Название провайдера (текст кнопки входа)',
  issuerLabel: 'Issuer URL',
  clientIdLabel: 'Client ID',
  clientSecretLabel: 'Client secret',
  secretKept: 'оставьте пустым, чтобы не менять',
  redirectUrlLabel: 'Redirect URL (пусто = авто: <host>/api/auth/sso/callback)',
  defaultRoleLabel: 'Роль для новых SSO-пользователей',
  autoProvisionLabel: 'Создавать учётки автоматически при первом входе',
  ssoSaveBtn: 'Сохранить настройки SSO',
  ssoLoginWith: 'Войти через',
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
