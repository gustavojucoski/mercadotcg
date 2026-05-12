'use client'

import { useMemo } from 'react'
import { LocaleContext, useLocaleState } from '@/lib/locale'
import type { Lang } from '@/lib/locale'

interface LocaleProviderProps {
  children: React.ReactNode
}

export function LocaleProvider({ children }: LocaleProviderProps) {
  const [lang, setLang] = useLocaleState()

  const value = useMemo(
    () => ({
      lang,
      setLang,
      t: (en: string, pt?: string | null): string => {
        if (lang === 'pt' && pt && pt.length > 0) return pt
        return en
      },
    }),
    [lang, setLang],
  )

  return (
    <LocaleContext.Provider value={value}>{children}</LocaleContext.Provider>
  )
}
