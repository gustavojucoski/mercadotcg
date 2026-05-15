---
name: Architectural Review — Maio 2026
description: Resultado da revisão arquitetural completa feita em 2026-05-11 com análise de PM
type: project
---

Revisão completa realizada em 2026-05-11. Principais achados:

**ADRs saudáveis (não revisar):** ADR-001 a ADR-010, ADR-016 a ADR-019. Fundamentação sólida.

**ADRs com ressalvas:**
- ADR-010 (matching strict): correto mas precisa de spec para o matching service antes de implementar.
- ADR-015 (pokewallet.io): risco de dependência única; fallback necessário antes de prod.

**Complexidade prematura identificada (PM + arquiteto concordam):**
- Store audit log (migration 000010): útil, mas overhead desnecessário nesta fase.
- Validação CNPJ automática via ReceitaWS: correto, mas poderia ter sido manual até ter volume.
- Migrations muito granulares (000008, 000009 separadas): não há custo operacional mas cria debt de manutenção.

**Decisões de design pendentes (sub-especificadas):**
- Matching service: qual score mínimo vira auto-approved? (recomendado: 85)
- Selados: compartilha stock_items via variant_id ou tabela própria `sealed_products`?
- Página pública /cartas/[id]: variante default, janela temporal, fonte de "preço de referência"

**Novos ADRs necessários:** 020 (matching), 021 (pipeline ingest), 022 (multi-source fallback), 023 (upload S3/R2), 024 (selados), 025 (particionamento automático).

**Why:** Referência para priorização de próximas sprints.
**How to apply:** Usar como base para qualquer discussão de roadmap ou design de novas features.
