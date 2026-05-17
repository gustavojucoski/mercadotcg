import Link from 'next/link'

export default function CatalogoHubPage() {
  const cards = [
    {
      href: '/admin/catalogo/sets?tcg=pokemon',
      title: 'Sets',
      description: 'Gerencie os sets do catálogo — crie, edite e faça upload de imagens.',
      icon: (
        <svg className="size-6" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" d="M6 6.878V6a2.25 2.25 0 0 1 2.25-2.25h7.5A2.25 2.25 0 0 1 18 6v.878m-12 0c.235-.083.487-.128.75-.128h10.5c.263 0 .515.045.75.128m-12 0A2.25 2.25 0 0 0 4.5 9v.878m13.5-3A2.25 2.25 0 0 1 19.5 9v.878m0 0a2.246 2.246 0 0 0-.75-.128H5.25c-.263 0-.515.045-.75.128m15 0A2.25 2.25 0 0 1 21 12v6a2.25 2.25 0 0 1-2.25 2.25H5.25A2.25 2.25 0 0 1 3 18v-6c0-.98.626-1.813 1.5-2.122" />
        </svg>
      ),
    },
    {
      href: '/admin/catalogo/cards',
      title: 'Cartas',
      description: 'Busque e edite cartas individuais, imagens e variantes.',
      icon: (
        <svg className="size-6" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 6A2.25 2.25 0 0 1 6 3.75h2.25A2.25 2.25 0 0 1 10.5 6v2.25a2.25 2.25 0 0 1-2.25 2.25H6a2.25 2.25 0 0 1-2.25-2.25V6ZM3.75 15.75A2.25 2.25 0 0 1 6 13.5h2.25a2.25 2.25 0 0 1 2.25 2.25V18a2.25 2.25 0 0 1-2.25 2.25H6A2.25 2.25 0 0 1 3.75 18v-2.25ZM13.5 6a2.25 2.25 0 0 1 2.25-2.25H18A2.25 2.25 0 0 1 20.25 6v2.25A2.25 2.25 0 0 1 18 10.5h-2.25a2.25 2.25 0 0 1-2.25-2.25V6ZM13.5 15.75a2.25 2.25 0 0 1 2.25-2.25H18a2.25 2.25 0 0 1 2.25 2.25V18A2.25 2.25 0 0 1 18 20.25h-2.25A2.25 2.25 0 0 1 13.5 18v-2.25Z" />
        </svg>
      ),
    },
    {
      href: '/admin/catalogo/series',
      title: 'Series',
      description: 'Gerencie as series que agrupam os sets.',
      icon: (
        <svg className="size-6" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 12h16.5m-16.5 3.75h16.5M3.75 19.5h16.5M5.625 4.5h12.75a1.875 1.875 0 0 1 0 3.75H5.625a1.875 1.875 0 0 1 0-3.75Z" />
        </svg>
      ),
    },
  ]

  return (
    <div className="mx-auto max-w-5xl px-4 py-6">
      <div className="mb-8">
        <h1 className="text-lg font-semibold text-zinc-900 dark:text-zinc-50">Catálogo</h1>
        <p className="text-sm text-zinc-500 mt-1">
          Gerencie sets, cartas e variantes do catálogo TCG.
        </p>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        {cards.map(card => (
          <Link
            key={card.href}
            href={card.href}
            className="group rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5 hover:border-violet-400 dark:hover:border-violet-600 hover:shadow-sm transition-all"
          >
            <div className="flex items-center gap-3 mb-3">
              <span className="inline-flex items-center justify-center w-10 h-10 rounded-lg bg-violet-50 dark:bg-violet-900/30 text-violet-600 dark:text-violet-400 group-hover:bg-violet-100 dark:group-hover:bg-violet-900/50 transition-colors">
                {card.icon}
              </span>
              <h2 className="text-sm font-semibold text-zinc-900 dark:text-zinc-50">
                {card.title}
              </h2>
            </div>
            <p className="text-xs text-zinc-500 leading-relaxed">{card.description}</p>
          </Link>
        ))}
      </div>
    </div>
  )
}
