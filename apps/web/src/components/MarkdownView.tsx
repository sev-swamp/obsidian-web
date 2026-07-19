import { useEffect, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import mermaid from 'mermaid'
import { useAuthStore } from '../store/auth'
import { useThemeStore } from '../store/theme'

declare global {
  interface Window {
    MathJax?: {
      typesetPromise?: (elements?: Element[]) => Promise<void>
    }
    __mathJaxReady?: Promise<void>
  }
}

// Serializes typeset calls so overlapping edits/renders never race MathJax
// against itself (e.g. React StrictMode's double effect invocation in dev).
let mathJaxQueue: Promise<void> = Promise.resolve()

// MarkdownView displays server-rendered HTML and activates the
// client-side pieces: internal link routing, Mermaid diagrams, MathJax.
export function MarkdownView({ html }: { html: string }) {
  const ref = useRef<HTMLDivElement>(null)
  const navigate = useNavigate()
  const theme = useThemeStore((s) => s.theme)

  useEffect(() => {
    const el = ref.current
    if (!el) return

    // Media elements cannot send the Authorization header, so attachment
    // URLs get the session token appended (the only endpoint accepting it).
    const token = useAuthStore.getState().token
    if (token) {
      el.querySelectorAll<HTMLElement>('img, audio, video, source').forEach((media) => {
        const src = media.getAttribute('src')
        if (src && src.startsWith('/api/attachment/') && !src.includes('token=')) {
          media.setAttribute('src', `${src}?token=${encodeURIComponent(token)}`)
        }
      })
    }

    mermaid.initialize({
      startOnLoad: false,
      theme: theme === 'dark' ? 'dark' : 'default',
    })
    const diagrams = el.querySelectorAll<HTMLElement>('pre.mermaid')
    if (diagrams.length > 0) {
      void mermaid.run({ nodes: diagrams }).catch(() => {})
    }
    const ready = window.__mathJaxReady ?? Promise.resolve()
    mathJaxQueue = mathJaxQueue
      .then(() => ready)
      .then(() => window.MathJax?.typesetPromise?.([el]))
      .catch(() => {})
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
