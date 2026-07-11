import { useEffect, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import mermaid from 'mermaid'
import { useThemeStore } from '../store/theme'

declare global {
  interface Window {
    MathJax?: {
      typesetPromise?: (elements?: Element[]) => Promise<void>
    }
  }
}

// MarkdownView displays server-rendered HTML and activates the
// client-side pieces: internal link routing, Mermaid diagrams, MathJax.
export function MarkdownView({ html }: { html: string }) {
  const ref = useRef<HTMLDivElement>(null)
  const navigate = useNavigate()
  const theme = useThemeStore((s) => s.theme)

  useEffect(() => {
    const el = ref.current
    if (!el) return

    mermaid.initialize({
      startOnLoad: false,
      theme: theme === 'dark' ? 'dark' : 'default',
    })
    const diagrams = el.querySelectorAll<HTMLElement>('pre.mermaid')
    if (diagrams.length > 0) {
      void mermaid.run({ nodes: diagrams }).catch(() => {})
    }
    void window.MathJax?.typesetPromise?.([el])?.catch(() => {})
  }, [html, theme])

  const onClick = (e: React.MouseEvent) => {
    const anchor = (e.target as HTMLElement).closest('a')
    if (!anchor) return
    const href = anchor.getAttribute('href')
    if (!href) return
    if (href.startsWith('/n/')) {
      e.preventDefault()
      navigate(href)
    }
  }

  return (
    <div
      ref={ref}
      className="markdown"
      onClick={onClick}
      dangerouslySetInnerHTML={{ __html: html }}
    />
  )
}
