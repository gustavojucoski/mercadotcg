// URL base da API para chamadas client-side.
// Em Docker (NEXT_PUBLIC_API_URL=""), resulta em "" para que as chamadas
// usem path relativo (/api/v1/...) e sejam interceptadas pelo rewrite do Next.js.
export const API_URL = process.env.NEXT_PUBLIC_API_URL || ''

// URL base para chamadas server-side (RSC, Server Components).
// API_INTERNAL_URL aponta para o hostname interno do Docker (http://api:8080),
// evitando a viagem pela rede externa. Faz fallback para API_URL em dev local.
export const API_INTERNAL =
  process.env.API_INTERNAL_URL ??
  process.env.NEXT_PUBLIC_API_URL ??
  'http://localhost:8080'
