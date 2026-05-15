# Gaps & Limitações — MercadoTCG

## O que NÃO está pronto

- Não há job criando partições futuras de `price_history`. Hardcoded até 2026-Q4.
- `PriceHistoryRepo.InsertBatch` (CopyFrom) não respeita ON CONFLICT — deduplicação é responsabilidade do pipeline a montante.
- `ListingRepo` não existe — virá com o marketplace.
- `forex.BCBProvider` usa `decimal.NewFromFloat` no parsing — ok para BCB (4 casas), mas trocar para `decimal.NewFromString` se mudar de fonte.
- Cartas gradeadas: `stock_items` agrega por `grade` mas não distingue dois "PSA 10" com cert numbers diferentes (ADR-009).
- Reservas de estoque (`reservation`/`release`): ENUMs declarados em `stock_movement_kind` mas código não os emite.
- Pipeline scraping → matching → `price_history` não existe. Scraper retorna ao caller mas não persiste.
- Emails: `RESEND_API_KEY` obrigatório para envio real; sem chave usa `NoopProvider`. Em produção exige domínio verificado no Resend.
- Google OAuth requer `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `GOOGLE_REDIRECT_URL` e `OAUTH_STATE_HMAC_KEY`.
- `cmd/seed` é stub — não popula dados além do admin (migration 000007).

## Integração de Pagamentos (planejada)

**PSP:** Mercado Pago (default) → Stripe (fase 2). Não usar múltiplos PSPs simultâneos no MVP.

**Modelo de dados a criar:**
- `payment_intents` — id, listing_id, buyer_id, amount_brl, psp, psp_payment_id, status, idempotency_key
- `payment_events` — log append-only de webhooks (payload JSONB)
- `payouts` — seller_id, amount_brl, fee_brl, psp_transfer_id, status, paid_at

**Regras críticas:**
- Idempotência em webhooks (PSP pode reenviar)
- Escrow lógico: liberar payout só após confirmação de recebimento
- `decimal.Decimal` em todo valor — nunca float
- Fees separadas do bruto
- Validar HMAC do PSP antes de qualquer mutação

**Código a criar:**
- `internal/payment/` — interface `Provider`, implementações `mercadopago.go` e `stripe.go`
- `internal/handler/payment_webhook.go` — `POST /webhooks/payments/{psp}`
