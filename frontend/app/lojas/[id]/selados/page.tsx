export default function SeladosPage() {
  return (
    <div className="mx-auto max-w-3xl px-4 py-16 text-center">
      <div className="inline-flex items-center justify-center w-12 h-12 rounded-xl bg-zinc-100 dark:bg-zinc-800 mb-4">
        <svg className="w-6 h-6 text-zinc-400" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" d="M20.25 7.5l-.625 10.632a2.25 2.25 0 01-2.247 2.118H6.622a2.25 2.25 0 01-2.247-2.118L3.75 7.5M10 11.25h4M3.375 7.5h17.25c.621 0 1.125-.504 1.125-1.125v-1.5c0-.621-.504-1.125-1.125-1.125H3.375c-.621 0-1.125.504-1.125 1.125v1.5c0 .621.504 1.125 1.125 1.125z" />
        </svg>
      </div>
      <h2 className="text-base font-semibold text-zinc-800 dark:text-zinc-200 mb-2">
        Estoque de Selados
      </h2>
      <p className="text-sm text-zinc-500 max-w-sm mx-auto">
        Gerencie booster boxes, ETBs, tins e outros produtos selados do seu estoque.
        Em breve.
      </p>
    </div>
  )
}
