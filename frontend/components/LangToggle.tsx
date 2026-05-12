'use client'

import { useLang } from '@/lib/locale'

export function LangToggle() {
  const { lang, setLang } = useLang()

  return (
    <div className="flex items-center gap-0.5 text-xs select-none" aria-label="Idioma do catálogo">
      <button
        onClick={() => setLang('pt')}
        className={`px-2 py-1 rounded-md transition-colors ${
          lang === 'pt'
            ? 'text-zinc-900 dark:text-zinc-50 font-semibold'
            : 'text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-300'
        }`}
        aria-pressed={lang === 'pt'}
        title="Exibir nomes em Português"
      >
        PT
      </button>
      <span className="text-zinc-300 dark:text-zinc-600 select-none" aria-hidden="true">·</span>
      <button
        onClick={() => setLang('en')}
        className={`px-2 py-1 rounded-md transition-colors ${
          lang === 'en'
            ? 'text-zinc-900 dark:text-zinc-50 font-semibold'
            : 'text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-300'
        }`}
        aria-pressed={lang === 'en'}
        title="Display names in English"
      >
        EN
      </button>
    </div>
  )
}
