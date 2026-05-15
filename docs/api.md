# API Reference — MercadoTCG

## Endpoints HTTP

```
GET  /healthz

# Auth (público)
POST /api/v1/auth/register          # { email } — envia link; email não verificado → reenvia
POST /api/v1/auth/login
GET  /api/v1/auth/google
GET  /api/v1/auth/google/callback
POST /api/v1/auth/verify-email      # { token, password, display_name } — completa cadastro + auto-login
POST /api/v1/auth/forgot-password
POST /api/v1/auth/reset-password
POST /api/v1/auth/refresh
POST /api/v1/auth/logout
GET  /api/v1/auth/me                (RequireAuth)

# Catálogo (público)
GET  /api/v1/series?tcg=
GET  /api/v1/sets/{tcg}?series_id=&page=&limit=   # limit máx 500
GET  /api/v1/sets/{tcg}/{code}
GET  /api/v1/sets/{tcg}/{code}/cards?page=&limit=  # limit máx 200
GET  /api/v1/cards/autocomplete?q=&tcg=&limit=
GET  /api/v1/cards/{slug}                          # slug = UUID | {setCode}-{collectorNumber}
GET  /api/v1/cards/search
GET  /api/v1/cards/lookup

# Busca externa (RequirePlatformAdmin)
GET  /api/v1/external-search?number=&set=

# Admin — catálogo PT-BR (RequirePlatformAdmin)
GET    /api/v1/admin/series
PATCH  /api/v1/admin/series/{id}/name-pt
PATCH  /api/v1/admin/sets/{id}/name-pt
PATCH  /api/v1/admin/cards/{id}/name-pt

# Admin — usuários (RequirePlatformAdmin)
GET    /api/v1/admin/users
GET    /api/v1/admin/users/search?q=
POST   /api/v1/admin/users
PATCH  /api/v1/admin/users/{id}/role
DELETE /api/v1/admin/users/{id}

# Admin — lojas (RequirePlatformAdmin)
GET    /api/v1/admin/stores
POST   /api/v1/admin/stores
GET    /api/v1/admin/stores/cnpj-lookup?cnpj=    # rota literal ANTES de {id}
GET    /api/v1/admin/stores/{id}
PATCH  /api/v1/admin/stores/{id}
POST   /api/v1/admin/stores/{id}/verify-document
POST   /api/v1/admin/stores/{id}/logo
GET    /api/v1/admin/stores/{id}/audit-log
POST   /api/v1/admin/stores/{id}/members
PATCH  /api/v1/admin/stores/{id}/members/{userId}/role
DELETE /api/v1/admin/stores/{id}/members/{userId}
GET    /api/v1/admin/stores/{id}/members

# Lojas (RequireAuth / RequireStoreRole)
GET    /api/v1/stores/{id}                              # público
GET    /api/v1/stores/me
GET    /api/v1/stores/{id}/my-role
GET    /api/v1/stores/{id}/members                      # viewer+
POST   /api/v1/stores/{id}/members                      # admin
PATCH  /api/v1/stores/{id}/members/{userId}/role        # admin
DELETE /api/v1/stores/{id}/members/{userId}             # admin
PATCH  /api/v1/stores/{id}/profile                      # admin
POST   /api/v1/stores/{id}/logo                         # admin
GET    /api/v1/stores/{id}/stock
POST   /api/v1/stores/{id}/stock/purchase               # stock_manager+
POST   /api/v1/stores/{id}/stock-items/{itemID}/sale    # stock_manager+

# Variantes
GET  /api/v1/variants/{id}/signal
```

## Rotas Frontend

| Rota | Acesso | Conteúdo |
|---|---|---|
| `/` | público | Homepage |
| `/auth/login` | público | Email+senha |
| `/auth/register` | público | Cadastro email-only (step 1) |
| `/auth/verify-email?token=` | público | Define nome + senha (step 2, auto-login) |
| `/auth/callback?access_token=&refresh_token=` | público | Google OAuth redirect |
| `/auth/forgot-password` | público | Solicita reset |
| `/auth/reset-password?token=` | público | Nova senha |
| `/admin` | platform_admin | Busca Externa |
| `/admin/lojas` | platform_admin | Lista de lojas |
| `/admin/lojas/nova` | platform_admin | Cadastro de loja |
| `/admin/lojas/[id]` | platform_admin | Edição + audit log |
| `/lojas/me` | autenticado | Redireciona para loja do usuário |
| `/lojas/[id]/perfil` | membro | Edição de perfil |
| `/lojas/[id]/membros` | membro | Gestão de membros |
| `/lojas/[id]/selados` | membro | Placeholder |
| `/lojas/[id]/singles` | membro | Placeholder |
| `/sets` | público | Hub de TCGs |
| `/sets/[tcg]` | público | Listagem de sets com filtro |
| `/sets/[tcg]/[code]` | público | Grade de cartas (filtro client-side) |
| `/cards/[slug]` | público | Detalhe da carta |
