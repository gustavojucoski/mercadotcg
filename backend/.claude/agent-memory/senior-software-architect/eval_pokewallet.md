---
name: Tech Evaluation — pokewallet.io
description: Avaliação de risco da dependência pokewallet.io como fonte primária TCGPlayer+Cardmarket
type: project
---

**Status:** Em uso em produção como fonte primária (ADR-015).

Riscos identificados em 2026-05-11:
- Free tier: 100 req/hora sem SLA documentado
- Sem versionamento de API público; mudanças de schema podem quebrar silenciosamente
- Scraper legado `internal/scraper/cardmarket/` com FlareSolverr existe mas não está registrado em main.go
- Scraper legado `internal/scraper/tcgplayer/` também existe mas não registrado

Decisão: implementar multi-source fallback (ADR-022) antes do primeiro deploy de produção.
Estratégia: `SourceRegistry` com prioridade primária/fallback por fonte, com circuit breaker simples.

**Why:** pokewallet.io sem SLA é aceitável em dev mas não em produção com usuários reais.
**How to apply:** Ao discutir scraping ou fontes externas, sempre mencionar o risco e o plano de fallback.
