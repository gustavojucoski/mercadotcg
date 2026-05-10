import { NextResponse } from 'next/server'
import type { NextRequest } from 'next/server'

// Rotas que requerem autenticação (verificadas no cliente via useAuth).
// Como o refresh token está em localStorage (não cookie), o proxy não consegue
// inspecioná-lo. A proteção real é o RequirePlatformAdmin no Go backend.
// O proxy apenas redireciona para /auth/login se a rota de autenticação for
// acessada enquanto a sessão estiver marcada via cookie de sessão.
const AUTH_ROUTES = ['/auth/login', '/auth/register']
const PROTECTED_ROUTES = ['/']

export function proxy(request: NextRequest) {
  const { pathname } = request.nextUrl
  const sessionCookie = request.cookies.get('mtcg_session')?.value

  // Se já está autenticado e tenta acessar login/register, redirecionar para home.
  if (sessionCookie && AUTH_ROUTES.some(r => pathname.startsWith(r))) {
    return NextResponse.redirect(new URL('/', request.url))
  }

  return NextResponse.next()
}

export const config = {
  matcher: ['/((?!_next/static|_next/image|favicon.ico|api).*)'],
}
