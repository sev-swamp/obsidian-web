// In-app syntax reference shown by HelpDialog. Keywords hold extra
// search terms (Russian + English) that are not visible in the UI.

export interface HelpEntry {
  code: string
  text: string
}

export interface HelpSection {
  id: string
  title: string
  keywords: string
  entries: HelpEntry[]
}

export const helpSections: HelpSection[] = [
  {
    id: 'headings',
    title: 'Заголовки',
    keywords: 'headings заголовок h1 h2 h3 структура',
    entries: [
      { code: '# Заголовок 1', text: 'Заголовок первого уровня' },
      { code: '## Заголовок 2', text: 'Заголовок второго уровня' },
      { code: '### Заголовок 3', text: 'Заголовок третьего уровня (и так далее до ######)' },
    ],
  },
  {
    id: 'text',
    title: 'Форматирование текста',
    keywords: 'bold italic strikethrough выделение жирным жирный курсив зачёркнутый моноширинный код formatting',
    entries: [
      { code: '**жирный текст**', text: 'Выделение жирным' },
      { code: '*курсив*', text: 'Выделение курсивом' },
      { code: '***жирный курсив***', text: 'Жирный и курсив одновременно' },
      { code: '~~зачёркнутый~~', text: 'Зачёркнутый текст' },
      { code: '`код в строке`', text: 'Моноширинный фрагмент внутри предложения' },
      { code: '---', text: 'Горизонтальная линия (на отдельной строке)' },
    ],
  },
  {
    id: 'lists',
    title: 'Списки',
    keywords: 'lists маркированный нумерованный вложенный bullet numbered',
    entries: [
      { code: '- пункт\n- ещё пункт', text: 'Маркированный список' },
      { code: '1. первый\n2. второй', text: 'Нумерованный список' },
      { code: '- пункт\n    - вложенный', text: 'Вложенность — отступ в 4 пробела' },
    ],
  },
  {
    id: 'tasks',
    title: 'Задачи (чек-листы)',
    keywords: 'tasks checklist todo чекбокс галочка checkbox',
    entries: [
      { code: '- [ ] сделать', text: 'Невыполненная задача' },
      { code: '- [x] сделано', text: 'Выполненная задача' },
    ],
  },
  {
    id: 'links',
    title: 'Ссылки',
    keywords: 'links wiki wikilink вики ссылка алиас заголовок блок embed вложение картинка изображение url',
    entries: [
      { code: '[[Заметка]]', text: 'Ссылка на заметку по имени (папку указывать не обязательно)' },
      { code: '[[Папка/Заметка]]', text: 'Ссылка на заметку по пути' },
      { code: '[[Заметка|мой текст]]', text: 'Ссылка с собственным текстом (алиас)' },
      { code: '[[Заметка#Раздел]]', text: 'Ссылка на конкретный заголовок внутри заметки' },
      { code: '[[Заметка#^abc123]]', text: 'Ссылка на блок по идентификатору' },
      { code: '![[картинка.png]]', text: 'Встраивание вложения: изображение, PDF, аудио, видео' },
      { code: '[текст](https://example.com)', text: 'Обычная внешняя ссылка' },
    ],
  },
  {
    id: 'quotes',
    title: 'Цитаты',
    keywords: 'quote blockquote цитата',
    entries: [{ code: '> Обычная цитата', text: 'Блок цитаты' }],
  },
  {
    id: 'callouts',
    title: 'Callouts (цветные блоки)',
    keywords:
      'callout цветной блок цвет синий зелёный жёлтый красный оранжевый фиолетовый серый голубой note info todo tip hint success check done warning caution attention danger error bug fail failure question help faq example quote cite abstract summary tldr выполнено готово совет предупреждение опасность вопрос пример',
    entries: [
      {
        code: '> [!note] Заголовок\n> Текст блока.\n> Может быть многострочным.',
        text: 'Общий вид: тип в [!скобках], после него — свой заголовок (не обязателен). Заголовок и цвет подставляются по типу',
      },
      { code: '> [!note] / [!info] / [!todo]', text: '🔵 Синий — заметка, информация, план' },
      {
        code: '> [!tip] / [!hint] / [!success] / [!check] / [!done]',
        text: '🟢 Зелёный — совет, успех, выполнено. Пример:\n> [!done] Shipped',
      },
      {
        code: '> [!warning] / [!caution] / [!attention]',
        text: '🟡 Жёлтый — предупреждение, осторожно',
      },
      {
        code: '> [!danger] / [!error] / [!bug] / [!fail] / [!failure]',
        text: '🔴 Красный — опасность, ошибка, баг',
      },
      { code: '> [!question] / [!help] / [!faq]', text: '🟠 Оранжевый — вопрос, помощь' },
      { code: '> [!example]', text: '🟣 Фиолетовый — пример' },
      { code: '> [!quote] / [!cite]', text: '⚪ Серый — цитата с указанием источника' },
      {
        code: '> [!abstract] / [!summary] / [!tldr]',
        text: '🩵 Голубой — краткое содержание, выжимка',
      },
      {
        code: '> [!мой-тип] Свой блок',
        text: 'Неизвестный тип тоже работает — рендерится синим (цветом по умолчанию). Свой цвет добавляется одной CSS-строкой, см. docs/syntax.md',
      },
    ],
  },
  {
    id: 'tables',
    title: 'Таблицы',
    keywords: 'tables таблица столбец строка колонка',
    entries: [
      {
        code: '| Имя | Роль |\n| --- | ---- |\n| Иван | admin |',
        text: 'Таблица: первая строка — шапка, вторая — разделитель',
      },
    ],
  },
  {
    id: 'code',
    title: 'Блоки кода',
    keywords: 'code блок кода подсветка syntax highlighting язык программирование',
    entries: [
      {
        code: '```go\nfunc main() {}\n```',
        text: 'Блок кода с подсветкой; укажите язык после ``` (go, python, js, sql…)',
      },
      { code: '```\nпростой текст\n```', text: 'Блок без подсветки' },
    ],
  },
  {
    id: 'mermaid',
    title: 'Диаграммы Mermaid',
    keywords: 'mermaid диаграмма график схема flowchart sequence gantt',
    entries: [
      {
        code: '```mermaid\ngraph LR\n  A --> B\n```',
        text: 'Диаграмма-схема. Поддерживаются flowchart, sequence, gantt, pie и другие типы Mermaid',
      },
    ],
  },
  {
    id: 'math',
    title: 'Формулы (MathJax)',
    keywords: 'math формула латех latex mathjax уравнение дробь интеграл',
    entries: [
      { code: '$e^{i\\pi} + 1 = 0$', text: 'Формула внутри строки' },
      { code: '$$\n\\int_0^1 x^2 dx\n$$', text: 'Формула отдельным блоком, по центру' },
    ],
  },
  {
    id: 'tags',
    title: 'Теги',
    keywords: 'tags тег метка hashtag поиск по тегам',
    entries: [
      { code: '#проект #работа/встречи', text: 'Теги в тексте заметки; допускаются вложенные через /' },
      { code: 'tags: [проект, идея]', text: 'Теги в frontmatter (см. раздел Frontmatter)' },
    ],
  },
  {
    id: 'frontmatter',
    title: 'Frontmatter (свойства заметки)',
    keywords: 'frontmatter yaml свойства метаданные title aliases алиасы шапка',
    entries: [
      {
        code: '---\ntitle: Название\ntags: [проект]\naliases: [Другое имя]\n---',
        text: 'YAML-блок в самом начале файла: заголовок, теги, алиасы (альтернативные имена для [[ссылок]])',
      },
    ],
  },
  {
    id: 'templates',
    title: 'Переменные шаблонов',
    keywords: 'templates шаблон переменная дата время подстановка date time title',
    entries: [
      { code: '{{title}}', text: 'Название создаваемой заметки' },
      { code: '{{date}}', text: 'Текущая дата (2026-07-12)' },
      { code: '{{time}}', text: 'Текущее время (14:30)' },
      { code: '{{date:YYYY-MM-DD}}', text: 'Дата в своём формате (YYYY, MM, DD, HH, mm, ss)' },
      { code: '{{datetime}}', text: 'Дата и время в формате ISO' },
      {
        code: '{{моя_переменная}}',
        text: 'Пользовательская переменная — значение передаётся при создании заметки через API',
      },
    ],
  },
  {
    id: 'search',
    title: 'Поиск по заметкам',
    keywords: 'search поиск фильтр tag path горячие клавиши hotkey cmd k',
    entries: [
      { code: '⌘K / Ctrl+K', text: 'Открыть быстрый поиск по всем заметкам' },
      { code: 'tag:проект отчёт', text: 'Искать «отчёт» только в заметках с тегом «проект»' },
      { code: 'path:Projects план', text: 'Искать «план» только в папке Projects' },
    ],
  },
]
