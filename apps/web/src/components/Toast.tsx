import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from 'react'

export type ToastVariant = 'success' | 'error'

type ToastItem = { id: number; message: string; variant: ToastVariant }

const ToastContext = createContext<((message: string, variant?: ToastVariant) => void) | null>(
  null,
)

// useToast returns show(message, variant?) — fires a brief, auto-dismissing
// notification. Use for confirming actions that would otherwise fail silently.
export function useToast() {
  const ctx = useContext(ToastContext)
  if (!ctx) throw new Error('useToast must be used within <ToastProvider>')
  return ctx
}

// ToastProvider hosts a single stacked toast list for the whole app.
export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([])
  const nextId = useRef(0)

  const show = useCallback((message: string, variant: ToastVariant = 'success') => {
    const id = nextId.current++
    setItems((prev) => [...prev, { id, message, variant }])
    setTimeout(() => {
      setItems((prev) => prev.filter((item) => item.id !== id))
    }, 3000)
  }, [])

  return (
    <ToastContext.Provider value={show}>
      {children}
      <div className="fixed bottom-4 right-4 z-[70] flex flex-col gap-2">
        {items.map((item) => (
          <div
            key={item.id}
            role="status"
            className={
              item.variant === 'success'
                ? 'rounded-lg bg-green-600 px-4 py-2 text-sm font-medium text-white shadow-lg'
                : 'rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white shadow-lg'
            }
          >
            {item.message}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  )
}
