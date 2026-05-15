---
name: eval-tcgdex-language-coverage
description: Cobertura de idiomas do TCGDex por contagem de sets e análise de IDs — dados coletados em 2026-05-14 via API direta
metadata:
  type: project
---

# TCGDex: Cobertura por Idioma (dados de 2026-05-14)

## Tabela de cobertura

| Idioma | Sets totais | Sets exclusivos (IDs novos) | Sets sobrepostos (mesmo ID do EN) | EN ausentes |
|--------|-------------|-----------------------------|------------------------------------|-------------|
| EN     | 208         | 208 (referência)            | —                                  | —           |
| JA     | 164         | 160                         | 4 (neo1-4)                         | —           |
| FR     | 191         | 3 (coleções McDonald's FR)  | 188                                | 20          |
| DE     | 144         | 0                           | 144                                | 64          |
| ES     | 145         | 0                           | 145                                | 63          |
| IT     | 163         | 0                           | 163 (resposta: 163 sets)           | 27 (aprox.) |
| KO     | 88          | 88                          | 0                                  | —           |
| ZH-TW  | 91          | 91                          | 0                                  | —           |
| PT     | 114 (114)   | 0                           | 114                                | 94          |

## Conclusão crítica para schema

**IDs japoneses SÃO distintos dos EN.** Apenas 4 IDs coincidentes (neo1, neo2, neo3, neo4 — sets Neo da era WotC que foram globalizados). Os outros 160 IDs JA usam namespaces completamente diferentes: PMCG, ADV, PCG, SM (maiúsculo), S, SV (maiúsculo), M, XY (maiúsculo), etc.

**Coreano e ZH-TW: 0 sobreposição com EN.** IDs como SM1M, S1H, SV1S, CS2a nunca existem no EN. São sets regionais com releases paralelas.

**Conclusão de schema:** O UNIQUE(code, tcg) ATUAL É SUFICIENTE para suportar JA, KO e ZH-TW sem mudança. Todos os IDs exclusivos de língua asiática são distintos dos IDs EN por construção (namespace diferente). Não é necessário adicionar coluna `language` ao schema.

**Exceção a monitorar:** FR tem 3 IDs exclusivos (2013bw, 2018sm-fr, 2019sm-fr) — coleções McDonald's específicas da França. Esses 3 IDs já cabem no schema atual sem conflito.

## PT: cobertura real

PT tem 114 sets — todos com IDs idênticos ao EN (subconjunto). Os 94 IDs EN ausentes no PT incluem:
- Sets muito antigos (base4, basep, gym1, gym2, bog, etc.)
- Todos os sets McDonald's EN (exceto os que nem aparecem no EN)
- Sets recentes ainda não traduzidos: A3a, A3b, A4, B1

PT cobre bem a era moderna (xy em diante) exceto promo sets, trainer kits e McDonald's collections.

**Why:** Pesquisa feita para decidir expansão multilingual do import-catalog.
**How to apply:** Schema atual suporta expansão JA/KO/ZH-TW sem migration. PT pode ser adicionado como language tag nos card names sem nova coluna de set.
