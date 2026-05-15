---
name: Critical Gaps — MercadoTCG Backend
description: Lacunas arquiteturais críticas que bloqueiam o primeiro loop de valor do produto
type: project
---

Estado em 2026-05-11:

1. **Pipeline scraping → price_history não existe.** Scrapers retornam ao caller (handler) mas não persistem. `InsertBatch` está implementado no repo mas não é chamado por nenhum job.

2. **Matching service ausente.** `external_card_refs` são inseridos apenas via seed manual. Sem matching automático, observações de novas cartas nunca viram `price_history`.

3. **Upload em disco local.** `upload.Provider` interface existe e está bem desenhada, mas apenas `LocalProvider` implementado. Qualquer deploy stateless perde logos.

4. **Partições de price_history até 2026-Q4.** Não há job criando partições futuras. Em 2027-Q1 inserções começarão a falhar silenciosamente.

5. **price_daily só tem dados de seed.** `RebuildDay` está implementado em `PriceDailyRepo` mas `cmd/aggregate` não existe ainda; o job nunca rodou com dados reais.

**Why:** Bloqueia o ciclo de dados completo: sem matching + ingestão + agregação, o produto não tem dados reais de preço para exibir.
**How to apply:** Qualquer nova feature de catálogo ou pricing depende de desbloquear esses gaps primeiro.
