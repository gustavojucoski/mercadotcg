'use client'

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
} from 'react'

export type Lang = 'pt' | 'en'

const STORAGE_KEY = 'mtcg_lang'
const DEFAULT_LANG: Lang = 'pt'

export interface LocaleCtx {
  lang: Lang
  setLang: (l: Lang) => void
  /** Returns pt when lang==="pt" and pt is non-empty, otherwise en. */
  t: (en: string, pt?: string | null) => string
}

export const LocaleContext = createContext<LocaleCtx>({
  lang: DEFAULT_LANG,
  setLang: () => undefined,
  t: (en, pt) => (pt && pt.length > 0 ? pt : en),
})

export function useLang(): LocaleCtx {
  return useContext(LocaleContext)
}

export function useLocaleState(): [Lang, (l: Lang) => void] {
  const [lang, setLangState] = useState<Lang>(DEFAULT_LANG)

  // Hydrate from localStorage after mount (avoids SSR mismatch)
  useEffect(() => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY) as Lang | null
      if (stored === 'pt' || stored === 'en') {
        setLangState(stored)
      }
    } catch {
      // localStorage unavailable (e.g. private browsing with restrictions)
    }
  }, [])

  const setLang = useCallback((l: Lang) => {
    setLangState(l)
    try {
      localStorage.setItem(STORAGE_KEY, l)
    } catch {
      // ignore write failures
    }
  }, [])

  return [lang, setLang]
}
