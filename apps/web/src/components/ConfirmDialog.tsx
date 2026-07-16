import { createContext, useCallback, useContext, useState, type ReactNode } from 'react'
import { useT } from '../i18n'

export type ConfirmOptions = {
  title: string
  message?: string
  confirmLabel?: string
  danger?: boolean
}

type Pending = ConfirmOptions & { resolve: (ok: boolean) => void }

const ConfirmContext = createContext<((opts: ConfirmOptions) => Promise<boolean>) | null>(null)

// useConfirm returns an async confirm() — a styled, promise-based drop-in for
// the native window.confirm(). Resolves true when confirmed, false otherwise.
export function useConfirm() {
  const ctx = useContext(ConfirmContext)
  if (!ctx) throw new Error('useConfirm must be used within <ConfirmProvider>')
  return ctx
}

// ConfirmProvider hosts a single dialog instance for the whole app.
export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [pending, setPending] = useState<Pending | null>(null)
  const t = useT()

  const confirm = useCallback(
    (opts: ConfirmOptions) =>
      new Promise<boolean>((resolve) => setPending({ ...opts, resolve })),
    [],
  )

  const close = (ok: boolean) => {
    pending?.resolve(ok)
    setPending(null)
  }

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      {pending && (
        <div
          className="fixed inset-0 z-[60] flex items-center justify-center bg-black/40 p-4"
          onClick={() => close(false)}
        >
          <div
            role="alertdialog"
            aria-modal="true"
            className="w-full max-w-sm rounded-xl border border-gray-200 bg-white p-5 shadow-2xl dark:border-gray-700 dark:bg-gray-900"
            onClick={(e) => e.stopPropagation()}
          >
            <h2 className="text-lg font-semibold">{pending.title}</h2>
            {pending.message && (
              <p className="mt-2 text-sm text-gray-600 dark:text-gray-400">{pending.message}</p>
            )}
            <div className="mt-5 flex justify-end gap-2">
              <button
                autoFocus
                onClick={() => close(false)}
                className="rounded-lg px-3 py-1.5 text-sm hover:bg-gray-100 dark:hover:bg-gray-800"
              >
                {t('cancel')}
              </button>
              <button
                onClick={() => close(true)}
                className={
                  pending.danger
                    ? 'rounded-lg bg-red-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-red-700'
                    : 'rounded-lg bg-violet-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-violet-700'
                }
              >
                {pending.confirmLabel ?? t('confirm')}
              </button>
            </div>
          </div>
        </div>
      )}
    </ConfirmContext.Provider>
  )
}
