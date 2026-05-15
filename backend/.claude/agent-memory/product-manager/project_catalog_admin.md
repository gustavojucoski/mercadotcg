---
name: catalog-admin-feature
description: Requisitos e decisões de produto para a feature de gerenciamento de catálogo pelo admin (PT-BR + imagens + criação/deleção)
metadata:
  type: project
---

Feature de gerenciamento de catálogo pelo admin definida em 2026-05-14.

**Objetivo:** Permitir que o platform_admin preencha manualmente nomes PT-BR e imagens de séries, sets e cartas — necessário porque TCGDex não tem PT-BR para o Pokémon TCG principal (só Pocket). Adicionalmente, criar e deletar sets/cartas para curadoria fina do catálogo.

**Decisões de produto aprovadas:**

- Navegação drill-down: /admin/catalogo → série → set → cartas (não páginas separadas)
- Inline edit para name_pt nas 3 entidades; modal para upload de imagem
- Modal de imagem com dois tabs: Upload de arquivo e URL externa
- Filtro "só sem PT" na listagem de cartas + indicador de progresso (X/Y traduzidas)
- Paginação 100 cartas/página obrigatória no MVP (sets com 200+ cartas)
- collector_number e tcg fora de escopo permanente — são dados de identidade, não metadados

**Pré-condição crítica antes de qualquer engenharia:**
- Fix no UPSERT do import-catalog para NÃO sobrescrever campos com valor já preenchido manualmente. Verificar UpsertSeries, UpsertCard, UpdateSetImageURL em card_repo.go. SQL pattern: `ON CONFLICT DO UPDATE SET field = EXCLUDED.field WHERE col IS NULL`.
- Verificar se card_sets.series_id é nullable no schema — determina se "criar série" entra no MVP ou V1.

**Novos endpoints necessários (backend):**
- POST /api/v1/admin/sets/{id}/logo — upload multipart
- POST /api/v1/admin/sets/{id}/symbol — upload multipart
- PATCH /api/v1/admin/sets/{id}/logo-url — salva URL externa
- PATCH /api/v1/admin/sets/{id}/symbol-url — salva URL externa
- PATCH /api/v1/admin/sets/{id} — campos escalares: printed_total, release_date (V1)
- POST /api/v1/admin/cards/{id}/image — upload imagem PT (V1)
- PATCH /api/v1/admin/cards/{id}/image-url — URL externa imagem PT (V1)
- POST /api/v1/admin/sets — criar set manualmente (V1)
- DELETE /api/v1/admin/sets/{id} — deletar set; 409 se tiver cartas (V1)
- POST /api/v1/admin/series — criar série manualmente (V1, condicional a series_id NOT NULL)
- POST /api/v1/admin/cards — criar carta no set (V2)
- DELETE /api/v1/admin/cards/{id} — deletar carta; 409 se variante tiver price_history/stock_items (V2)

**MVP scope (~1 semana eng):**
- Rotas frontend: /admin/catalogo + /admin/catalogo/series/{id} + /admin/catalogo/sets/{id}
- Inline edit name_pt nas 3 entidades
- Upload/URL logo e symbol de set
- Filtro "só sem PT" + indicador de progresso
- Fix UPSERT import-catalog

**V1 scope (+3-4 dias eng):**
- Criar set: modal com code (obrigatório, único), name EN (obrigatório), tcg (obrigatório), series_id (opcional), release_date, total_cards, printed_total
- Criar série: modal com name EN + tcg (se series_id for NOT NULL)
- Deletar set: bloquear com 409 se tiver cartas; modal de confirmação se set vazio
- Botão "Novo Set" na página da série (series_id e tcg pré-preenchidos)

**V2 scope (+3-4 dias eng):**
- Criar carta: modal com collector_number + name EN + checkboxes de variantes (Normal, Holo, Reverse Holo, First Edition, WPromo) — sem variantes automáticas
- Deletar carta: bloquear se variante tiver price_history ou stock_items; cascata nas variantes orphans com confirmação

**Regras de deleção (decisão de produto):**
- Deletar set → bloquear se tiver cartas (nunca cascata, risco de price_history/stock_items)
- Deletar carta → bloquear se qualquer card_variant tiver price_history ou stock_items; cascata em variantes sem dados vinculados (com confirmação)
- Nunca soft delete — adiciona complexidade de filtragem desnecessária no early-stage
- Sempre modal de confirmação antes de qualquer deleção

**Criação de variantes:**
- Sem variantes automáticas na criação de carta — combinação correta varia por carta/set
- Admin marca checkboxes no modal de criação; pode criar carta sem variantes e adicionar depois

**Risco principal:** Import sobrescrever edições manuais silenciosamente — mitigado pelo fix de UPSERT como pré-condição hard.
**Risco V1:** Set code duplicado → unique constraint + 409 com mensagem clara.
**Risco V2:** collector_number duplicado no set → unique constraint (card_set_id, collector_number) + 409.

**Why:** Sem PT-BR o catálogo público fica parcialmente em inglês, corroendo o posicionamento BR do produto. Criação/deleção manual permite curadoria sem depender de re-import.
**How to apply:** Qualquer trabalho nesta feature deve começar pelo fix do UPSERT antes de qualquer UI. Criar/deletar é V1/V2 — não bloquear MVP por isso.

Relacionado: [[product-review-2025-05]]
