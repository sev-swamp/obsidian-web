import { useEffect, useRef } from 'react'
import { EditorState, Compartment } from '@codemirror/state'
import {
  EditorView,
  keymap,
  drawSelection,
  highlightActiveLine,
  lineNumbers,
} from '@codemirror/view'
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
import { markdown } from '@codemirror/lang-markdown'
import {
  autocompletion,
  completionKeymap,
  type CompletionContext,
  type CompletionResult,
} from '@codemirror/autocomplete'
import type { NoteMeta } from '../api/types'

// wikilinkSource completes `[[...` with vault note paths. It triggers on the
// text following the last unclosed `[[` on the line and inserts the closing
// brackets when they aren't already present.
function wikilinkSource(notes: () => NoteMeta[]) {
  return (context: CompletionContext): CompletionResult | null => {
    const before = context.matchBefore(/\[\[[^\]\n]*/)
    if (!before) return null
    const from = before.from + 2 // caret position just after the `[[`
    if (before.from + 2 > context.pos) return null
    const options = notes().map((n) => {
      const label = n.path.replace(/\.md$/i, '')
      return {
        label,
        detail: n.title && n.title !== label ? n.title : undefined,
        type: 'text',
        apply: (view: EditorView, _c: unknown, a: number, b: number) => {
          const hasClose = view.state.sliceDoc(b, b + 2) === ']]'
          const insert = hasClose ? label : label + ']]'
          view.dispatch({
            changes: { from: a, to: b, insert },
            selection: { anchor: a + insert.length + (hasClose ? 2 : 0) },
          })
        },
      }
    })
    return { from, options, validFor: /[^\]\n]*/ }
  }
}

const themeCompartment = new Compartment()
const lineNumbersCompartment = new Compartment()

function editorTheme(dark: boolean) {
  return EditorView.theme(
    {
      '&': { fontSize: '0.875rem', height: '100%' },
      '.cm-scroller': {
        fontFamily:
          'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
        lineHeight: '1.6',
      },
      '.cm-content': { padding: '1rem 0' },
      '.cm-gutters': { border: 'none', background: 'transparent' },
      '&.cm-focused': { outline: 'none' },
    },
    { dark },
  )
}

export function MarkdownEditor({
  value,
  onChange,
  onSave,
  notes,
  dark,
  showLineNumbers = true,
}: {
  value: string
  onChange: (value: string) => void
  onSave?: (content: string) => void
  notes: NoteMeta[]
  dark: boolean
  showLineNumbers?: boolean
}) {
  const parent = useRef<HTMLDivElement>(null)
  const view = useRef<EditorView | null>(null)
  // Keep the latest callbacks/data reachable from the (stable) extensions.
  const onChangeRef = useRef(onChange)
  const onSaveRef = useRef(onSave)
  const notesRef = useRef(notes)
  onChangeRef.current = onChange
  onSaveRef.current = onSave
  notesRef.current = notes

  useEffect(() => {
    if (!parent.current) return
    const state = EditorState.create({
      doc: value,
      extensions: [
        lineNumbersCompartment.of(showLineNumbers ? lineNumbers() : []),
        history(),
        drawSelection(),
        highlightActiveLine(),
        EditorView.lineWrapping,
        markdown(),
        autocompletion({ override: [wikilinkSource(() => notesRef.current)] }),
        keymap.of([
          {
            key: 'Mod-s',
            preventDefault: true,
            run: (v) => {
              onSaveRef.current?.(v.state.doc.toString())
              return true
            },
          },
          ...defaultKeymap,
          ...historyKeymap,
          ...completionKeymap,
          indentWithTab,
        ]),
        themeCompartment.of(editorTheme(dark)),
        EditorView.updateListener.of((u) => {
          if (u.docChanged) onChangeRef.current(u.state.doc.toString())
        }),
      ],
    })
    const v = new EditorView({ state, parent: parent.current })
    view.current = v
    v.focus()
    return () => {
      v.destroy()
      view.current = null
    }
    // Created once; external value/theme changes are handled below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Reflect external value changes (e.g. resolving a conflict) into the doc.
  useEffect(() => {
    const v = view.current
    if (!v) return
    const current = v.state.doc.toString()
    if (value !== current) {
      v.dispatch({ changes: { from: 0, to: current.length, insert: value } })
    }
  }, [value])

  useEffect(() => {
    view.current?.dispatch({
      effects: themeCompartment.reconfigure(editorTheme(dark)),
    })
  }, [dark])

  useEffect(() => {
    view.current?.dispatch({
      effects: lineNumbersCompartment.reconfigure(showLineNumbers ? lineNumbers() : []),
    })
  }, [showLineNumbers])

  return (
    <div
      ref={parent}
      className="h-[70vh] w-full overflow-hidden rounded-lg border border-gray-300 bg-gray-50 focus-within:border-violet-500 dark:border-gray-700 dark:bg-gray-900"
    />
  )
}
