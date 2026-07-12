import { create } from 'zustand'
import { persist } from 'zustand/middleware'

export type Lang = 'en' | 'ru'

interface LangState {
  lang: Lang
  setLang: (lang: Lang) => void
}

const initial: Lang = navigator.language?.toLowerCase().startsWith('ru') ? 'ru' : 'en'

export const useLangStore = create<LangState>()(
  persist(
    (set) => ({
      lang: initial,
      setLang: (lang) => set({ lang }),
    }),
    { name: 'obsidianweb-lang' },
  ),
)
