// In-app syntax reference shown by HelpDialog, localized (en/ru).
// Keywords hold extra search terms in both languages that are not
// visible in the UI.

import type { Lang } from '../store/lang'

export type Localized = Record<Lang, string>

export interface HelpEntry {
  code: string
  text: Localized
}

export interface HelpSection {
  id: string
  title: Localized
  keywords: string
  entries: HelpEntry[]
}

export const helpSections: HelpSection[] = [
  {
    id: 'headings',
    title: { en: 'Headings', ru: 'Заголовки' },
    keywords: 'headings заголовок h1 h2 h3 структура structure',
    entries: [
      { code: '# Heading 1', text: { en: 'First-level heading', ru: 'Заголовок первого уровня' } },
      { code: '## Heading 2', text: { en: 'Second-level heading', ru: 'Заголовок второго уровня' } },
      {
        code: '### Heading 3',
        text: {
          en: 'Third-level heading (and so on up to ######)',
          ru: 'Заголовок третьего уровня (и так далее до ######)',
        },
      },
    ],
  },
  {
    id: 'text',
    title: { en: 'Text formatting', ru: 'Форматирование текста' },
    keywords:
      'bold italic strikethrough выделение жирным жирный курсив зачёркнутый моноширинный код formatting monospace',
    entries: [
      { code: '**bold text**', text: { en: 'Bold', ru: 'Выделение жирным' } },
      { code: '*italic*', text: { en: 'Italic', ru: 'Выделение курсивом' } },
      { code: '***bold italic***', text: { en: 'Bold and italic together', ru: 'Жирный и курсив одновременно' } },
      { code: '~~strikethrough~~', text: { en: 'Strikethrough', ru: 'Зачёркнутый текст' } },
      {
        code: '`inline code`',
        text: { en: 'Monospace fragment inside a sentence', ru: 'Моноширинный фрагмент внутри предложения' },
      },
      { code: '---', text: { en: 'Horizontal rule (on its own line)', ru: 'Горизонтальная линия (на отдельной строке)' } },
    ],
  },
  {
    id: 'lists',
    title: { en: 'Lists', ru: 'Списки' },
    keywords: 'lists маркированный нумерованный вложенный bullet numbered nested',
    entries: [
      { code: '- item\n- another item', text: { en: 'Bullet list', ru: 'Маркированный список' } },
      { code: '1. first\n2. second', text: { en: 'Numbered list', ru: 'Нумерованный список' } },
      {
        code: '- item\n    - nested',
        text: { en: 'Nest with a 4-space indent', ru: 'Вложенность — отступ в 4 пробела' },
      },
    ],
  },
  {
    id: 'tasks',
    title: { en: 'Tasks (checklists)', ru: 'Задачи (чек-листы)' },
    keywords: 'tasks checklist todo чекбокс галочка checkbox задача',
    entries: [
      { code: '- [ ] to do', text: { en: 'Open task', ru: 'Невыполненная задача' } },
      { code: '- [x] done', text: { en: 'Completed task', ru: 'Выполненная задача' } },
    ],
  },
  {
    id: 'links',
    title: { en: 'Links', ru: 'Ссылки' },
    keywords:
      'links wiki wikilink вики ссылка алиас заголовок блок embed вложение картинка изображение url alias image attachment',
    entries: [
      {
        code: '[[Note]]',
        text: {
          en: 'Wiki-link to a note by name (folder is optional)',
          ru: 'Ссылка на заметку по имени (папку указывать не обязательно)',
        },
      },
      { code: '[[Folder/Note]]', text: { en: 'Wiki-link by path', ru: 'Ссылка на заметку по пути' } },
      {
        code: '[[Note|my text]]',
        text: { en: 'Link with custom text (alias)', ru: 'Ссылка с собственным текстом (алиас)' },
      },
      {
        code: '[[Note#Section]]',
        text: { en: 'Link to a heading inside a note', ru: 'Ссылка на конкретный заголовок внутри заметки' },
      },
      { code: '[[Note#^abc123]]', text: { en: 'Link to a block by id', ru: 'Ссылка на блок по идентификатору' } },
      {
        code: '![[image.png]]',
        text: {
          en: 'Embed an attachment: image, PDF, audio, video',
          ru: 'Встраивание вложения: изображение, PDF, аудио, видео',
        },
      },
      { code: '[text](https://example.com)', text: { en: 'External link', ru: 'Обычная внешняя ссылка' } },
    ],
  },
  {
    id: 'quotes',
    title: { en: 'Quotes', ru: 'Цитаты' },
    keywords: 'quote blockquote цитата',
    entries: [{ code: '> Plain quote', text: { en: 'Blockquote', ru: 'Блок цитаты' } }],
  },
  {
    id: 'callouts',
    title: { en: 'Callouts (colored blocks)', ru: 'Callouts (цветные блоки)' },
    keywords:
      'callout цветной блок цвет синий зелёный жёлтый красный оранжевый фиолетовый серый голубой blue green yellow red orange purple gray cyan note info todo tip hint success check done warning caution attention danger error bug fail failure question help faq example quote cite abstract summary tldr выполнено готово совет предупреждение опасность вопрос пример',
    entries: [
      {
        code: '> [!note] Title\n> Block body.\n> Can span multiple lines.',
        text: {
          en: 'General form: type in [!brackets], then an optional title. The title and color are derived from the type',
          ru: 'Общий вид: тип в [!скобках], после него — свой заголовок (не обязателен). Заголовок и цвет подставляются по типу',
        },
      },
      {
        code: '> [!note] / [!info] / [!todo]',
        text: { en: '🔵 Blue — note, info, plan', ru: '🔵 Синий — заметка, информация, план' },
      },
      {
        code: '> [!tip] / [!hint] / [!success] / [!check] / [!done]',
        text: {
          en: '🟢 Green — tip, success, done. Example:\n> [!done] Shipped',
          ru: '🟢 Зелёный — совет, успех, выполнено. Пример:\n> [!done] Shipped',
        },
      },
      {
        code: '> [!warning] / [!caution] / [!attention]',
        text: { en: '🟡 Yellow — warning, caution', ru: '🟡 Жёлтый — предупреждение, осторожно' },
      },
      {
        code: '> [!danger] / [!error] / [!bug] / [!fail] / [!failure]',
        text: { en: '🔴 Red — danger, error, bug', ru: '🔴 Красный — опасность, ошибка, баг' },
      },
      {
        code: '> [!question] / [!help] / [!faq]',
        text: { en: '🟠 Orange — question, help', ru: '🟠 Оранжевый — вопрос, помощь' },
      },
      { code: '> [!example]', text: { en: '🟣 Purple — example', ru: '🟣 Фиолетовый — пример' } },
      {
        code: '> [!quote] / [!cite]',
        text: { en: '⚪ Gray — quote with attribution', ru: '⚪ Серый — цитата с указанием источника' },
      },
      {
        code: '> [!abstract] / [!summary] / [!tldr]',
        text: { en: '🩵 Cyan — summary, TL;DR', ru: '🩵 Голубой — краткое содержание, выжимка' },
      },
      {
        code: '> [!my-type] Custom block',
        text: {
          en: 'Unknown types work too — rendered blue (default). A custom color is one CSS line, see docs/syntax.md',
          ru: 'Неизвестный тип тоже работает — рендерится синим (цветом по умолчанию). Свой цвет добавляется одной CSS-строкой, см. docs/syntax.md',
        },
      },
    ],
  },
  {
    id: 'tables',
    title: { en: 'Tables', ru: 'Таблицы' },
    keywords: 'tables таблица столбец строка колонка column row',
    entries: [
      {
        code: '| Name | Role |\n| ---- | ---- |\n| Ivan | admin |',
        text: {
          en: 'Table: first row is the header, second is the separator',
          ru: 'Таблица: первая строка — шапка, вторая — разделитель',
        },
      },
    ],
  },
  {
    id: 'code',
    title: { en: 'Code blocks', ru: 'Блоки кода' },
    keywords: 'code блок кода подсветка syntax highlighting язык программирование language',
    entries: [
      {
        code: '```go\nfunc main() {}\n```',
        text: {
          en: 'Code block with highlighting; put the language after ``` (go, python, js, sql…)',
          ru: 'Блок кода с подсветкой; укажите язык после ``` (go, python, js, sql…)',
        },
      },
      { code: '```\nplain text\n```', text: { en: 'Block without highlighting', ru: 'Блок без подсветки' } },
    ],
  },
  {
    id: 'mermaid',
    title: { en: 'Mermaid diagrams', ru: 'Диаграммы Mermaid' },
    keywords: 'mermaid диаграмма график схема flowchart sequence gantt diagram chart',
    entries: [
      {
        code: '```mermaid\ngraph LR\n  A --> B\n```',
        text: {
          en: 'Diagram block. Flowchart, sequence, gantt, pie and other Mermaid types are supported',
          ru: 'Диаграмма-схема. Поддерживаются flowchart, sequence, gantt, pie и другие типы Mermaid',
        },
      },
    ],
  },
  {
    id: 'math',
    title: { en: 'Math (MathJax)', ru: 'Формулы (MathJax)' },
    keywords: 'math формула латех latex mathjax уравнение дробь интеграл equation formula',
    entries: [
      { code: '$e^{i\\pi} + 1 = 0$', text: { en: 'Inline formula', ru: 'Формула внутри строки' } },
      {
        code: '$$\n\\int_0^1 x^2 dx\n$$',
        text: { en: 'Block formula, centered', ru: 'Формула отдельным блоком, по центру' },
      },
    ],
  },
  {
    id: 'tags',
    title: { en: 'Tags', ru: 'Теги' },
    keywords: 'tags тег метка hashtag поиск по тегам label',
    entries: [
      {
        code: '#project #work/meetings',
        text: {
          en: 'Inline tags; nesting via / is allowed',
          ru: 'Теги в тексте заметки; допускаются вложенные через /',
        },
      },
      {
        code: 'tags: [project, idea]',
        text: { en: 'Tags in frontmatter (see Frontmatter)', ru: 'Теги в frontmatter (см. раздел Frontmatter)' },
      },
    ],
  },
  {
    id: 'frontmatter',
    title: { en: 'Frontmatter (note properties)', ru: 'Frontmatter (свойства заметки)' },
    keywords: 'frontmatter yaml свойства метаданные title aliases алиасы шапка properties metadata created updated author',
    entries: [
      {
        code: '---\ntitle: Title\ntags: [project]\naliases: [Other name]\n---',
        text: {
          en: 'YAML block at the very top of the file: title, tags, aliases (alternative names for [[links]])',
          ru: 'YAML-блок в самом начале файла: заголовок, теги, алиасы (альтернативные имена для [[ссылок]])',
        },
      },
      {
        code: 'created: "2026-07-18 16:00"\nauthor: Ivan',
        text: {
          en: 'All YAML fields are shown automatically under the note title. Settings → Notes toggles the panel, hides properties or renames them.',
          ru: 'Все YAML-поля автоматически показываются под заголовком заметки. В «Настройки → Заметки» панель можно выключить, скрыть или переименовать свойства.',
        },
      },
    ],
  },
  {
    id: 'templates',
    title: { en: 'Template variables', ru: 'Переменные шаблонов' },
    keywords: 'templates шаблон переменная дата время подстановка date time title variable',
    entries: [
      { code: '{{title}}', text: { en: 'Title of the note being created', ru: 'Название создаваемой заметки' } },
      { code: '{{date}}', text: { en: 'Current date (2026-07-12)', ru: 'Текущая дата (2026-07-12)' } },
      { code: '{{time}}', text: { en: 'Current time (14:30)', ru: 'Текущее время (14:30)' } },
      {
        code: '{{date:YYYY-MM-DD}}',
        text: {
          en: 'Date in a custom format (YYYY, MM, DD, HH, mm, ss)',
          ru: 'Дата в своём формате (YYYY, MM, DD, HH, mm, ss)',
        },
      },
      { code: '{{datetime}}', text: { en: 'ISO date and time', ru: 'Дата и время в формате ISO' } },
      {
        code: '{{my_variable}}',
        text: {
          en: 'Custom variable — the value is passed when creating a note via the API',
          ru: 'Пользовательская переменная — значение передаётся при создании заметки через API',
        },
      },
      {
        code: '{{currentuser}}',
        text: {
          en: 'Username of the person creating the note',
          ru: 'Имя пользователя, создающего заметку',
        },
      },
    ],
  },
  {
    id: 'search',
    title: { en: 'Searching notes', ru: 'Поиск по заметкам' },
    keywords: 'search поиск фильтр tag path prop свойства метаданные горячие клавиши hotkey cmd k filter',
    entries: [
      {
        code: '⌘K / Ctrl+K',
        text: { en: 'Open quick search across all notes', ru: 'Открыть быстрый поиск по всем заметкам' },
      },
      {
        code: 'tag:project report',
        text: {
          en: 'Search “report” only in notes tagged “project”',
          ru: 'Искать «report» только в заметках с тегом «project»',
        },
      },
      {
        code: 'path:Projects plan',
        text: {
          en: 'Search “plan” only inside the Projects folder',
          ru: 'Искать «plan» только в папке Projects',
        },
      },
      {
        code: 'prop:author=Ivan',
        text: {
          en: 'Search by a frontmatter property. Use prop:key:value for a partial match. Typing prop: suggests keys and values.',
          ru: 'Поиск по свойству frontmatter. Для частичного совпадения: prop:ключ:значение. При вводе prop: подсказываются ключи и значения.',
        },
      },
      {
        code: 'prop:created>=2026-07-01\nprop:created="2026-07-18 16:00"',
        text: {
          en: 'Date/number ranges via >, >=, <, <=; quote values that contain spaces.',
          ru: 'Диапазоны дат и чисел через >, >=, <, <=; значения с пробелами берите в кавычки.',
        },
      },
    ],
  },
]
