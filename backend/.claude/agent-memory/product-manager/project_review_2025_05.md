---
name: Product Review 2025-05
description: Revisão completa de produto realizada em maio/2025 — ROI, gaps de UX, riscos e prioridade do backlog
type: project
---

Revisão realizada em 2025-05-11 cobrindo todo o estado do produto até aquele momento.

**Conclusão central:** A plataforma tem fundação sólida (auth, multi-tenant, variantes, signal de preço), mas está em "dead zone" — lojas podem gerenciar estoque interno, mas não há loop de valor para usuário final nem receita. O desbloqueador crítico é o pipeline scraping→price_history + página pública de preços.

**Ordem de prioridade validada:**
1. Pipeline scraping → price_history (matching service + job de ingestão) — desbloqueador de tudo
2. Página pública de carta com histórico de preços — primeiro valor para usuário anônimo
3. Registro de venda no estoque de singles (frontend) — loop completo para lojista
4. Selados — modelo de dados + CRUD básico
5. Marketplace (listings + checkout + Mercado Pago) — só após ter lojas ativas com estoque real

**Riscos documentados:**
- Pipeline de scraping ainda não persiste nada (scrapers retornam ao caller apenas)
- Singles page exibe variant_id truncado em vez de nome da carta — UX quebrado
- Sem venda registrável no frontend de singles (só compra implementada)
- Logo em disco local — bloqueador para deploy em produção
- Audit log como feature de admin é over-engineered para o estágio atual

**Why:** Esta revisão informou a priorização do Q2/Q3 2025.
**How to apply:** Usar como baseline ao discutir próximos passos ou repropriorizar backlog.
