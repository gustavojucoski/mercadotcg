---
name: ADR Sequence Tracker
description: Controla o número do próximo ADR a ser criado para manter numeração sequencial
type: project
---

Último ADR registrado no CLAUDE.md: **ADR-025** (TCGDex como fonte primária + suporte bilíngue).

Próximo ADR disponível: **ADR-026**.

ADRs atualmente documentados no CLAUDE.md (verificado em 2026-05-14):
- ADR-001: SQL puro com golang-migrate
- ADR-002: shopspring/decimal para monetário
- ADR-003: card_variants como tabela própria
- ADR-004: Particionamento price_history por trimestre
- ADR-005: price_daily separada do raw
- ADR-006: Auditoria cambial dupla
- ADR-007: BRIN em observed_at
- ADR-008: Multi-tenant desde a fundação
- ADR-009: Estoque agregado + log de movimentos
- ADR-010: Matching strict via external_card_refs
- ADR-011: Estratégia por fonte (scraping vs API)
- ADR-012: TCGPlayer product ID (SUPERSEDIDO por ADR-015)
- ADR-013: Preços TCGPlayer por multiplicadores
- ADR-014: Cardmarket multiplicadores por condição
- ADR-015: pokewallet.io como fonte primária TCGPlayer+Cardmarket
- ADR-016: Auth próprio JWT + Google OAuth
- ADR-017: Criação de lojas apenas platform_admin
- ADR-018: Validação de documento CNPJ/CPF
- ADR-019: SiteHeader hover dropdowns
- ADR-020: Registro em duas etapas email-first
- ADR-021: tcg como VARCHAR(32) com CHECK constraint
- ADR-022: card_series como entidade própria
- ADR-023: upload.Provider interface polimórfica Local + S3
- ADR-024: Slug de carta {setCode}-{collectorNumber}
- ADR-025: TCGDex como fonte primária + suporte bilíngue PT-BR/EN

**Why:** Manter numeração sequencial é crítico; ADRs fora de ordem geram confusão no CLAUDE.md.
**How to apply:** Antes de criar qualquer ADR, verificar este arquivo e incrementar o contador.
