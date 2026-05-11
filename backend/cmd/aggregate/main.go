// cmd/aggregate recalcula price_daily a partir de price_history.
//
// Uso:
//
//	aggregate [-date YYYY-MM-DD] [-days N]
//
// Flags:
//
//	-date   dia alvo no formato YYYY-MM-DD; "today" usa a data atual;
//	        padrão: ontem. Quando -days > 1, este é o dia mais recente do range.
//	-days   quantos dias recalcular, contando de -date para trás (padrão: 1).
//
// Exemplos:
//
//	aggregate                          # recalcula ontem
//	aggregate -date today              # recalcula hoje
//	aggregate -date 2025-01-15         # recalcula um dia específico
//	aggregate -days 7                  # recalcula os últimos 7 dias (até ontem)
//	aggregate -date today -days 7      # recalcula os últimos 7 dias até hoje
//
// DATABASE_URL é obrigatório. O binário não chama config.Load() — não requer
// JWT_SECRET nem nenhuma outra variável do servidor HTTP.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/gustavojucoski/mercadotcg/backend/internal/repository/postgres"
)

func main() {
	// Zerolog: saída legível em terminal, com timestamp.
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})

	// ---- flags ---------------------------------------------------------------
	dateFlag := flag.String("date", "", "dia alvo: YYYY-MM-DD ou \"today\" (padrão: ontem)")
	daysFlag := flag.Int("days", 1, "quantos dias recalcular, contando de -date para trás (mín: 1)")
	flag.Parse()

	if *daysFlag < 1 {
		fatalf("flag -days deve ser >= 1, recebido: %d", *daysFlag)
	}

	// ---- banco de dados -------------------------------------------------------
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		fatalf("DATABASE_URL é obrigatório")
	}

	ctx := context.Background()

	pool, err := postgres.Connect(ctx, databaseURL)
	if err != nil {
		fatalf("conectar ao banco: %v", err)
	}
	defer pool.Close()

	repo := postgres.NewPriceDailyRepo(pool)

	// ---- resolver data alvo --------------------------------------------------
	// "today" → dia atual em UTC; "" → ontem; YYYY-MM-DD → parse literal.
	endDay, err := resolveDate(*dateFlag)
	if err != nil {
		fatalf("flag -date inválida: %v", err)
	}

	// ---- executar rebuild para cada dia no range -----------------------------
	// Range: [endDay - (days-1), endDay], do mais antigo para o mais recente.
	total := int64(0)
	jobStart := time.Now()

	for i := *daysFlag - 1; i >= 0; i-- {
		day := endDay.AddDate(0, 0, -i)
		dayStr := day.Format("2006-01-02")

		stepStart := time.Now()
		rows, err := repo.RebuildDay(ctx, day)
		elapsed := time.Since(stepStart)

		if err != nil {
			// Falha em um dia não interrompe os demais; loga e continua.
			// Se quiser comportamento de fail-fast, troque por fatalf.
			log.Error().
				Str("date", dayStr).
				Dur("elapsed", elapsed).
				Err(err).
				Msg("falha ao recalcular dia")
			continue
		}

		log.Info().
			Str("date", dayStr).
			Int64("rows_affected", rows).
			Dur("elapsed", elapsed).
			Msg("price_daily recalculado")

		total += rows
	}

	log.Info().
		Int("days_processed", *daysFlag).
		Int64("total_rows", total).
		Dur("total_elapsed", time.Since(jobStart)).
		Msg("agregação concluída")
}

// resolveDate interpreta o valor da flag -date:
//   - ""       → ontem (UTC)
//   - "today"  → hoje (UTC)
//   - YYYY-MM-DD → parse literal
//
// Devolve meia-noite UTC do dia resultante.
func resolveDate(s string) (time.Time, error) {
	now := time.Now().UTC().Truncate(24 * time.Hour)

	switch s {
	case "":
		return now.AddDate(0, 0, -1), nil
	case "today":
		return now, nil
	default:
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			return time.Time{}, fmt.Errorf("formato esperado YYYY-MM-DD, recebido %q: %w", s, err)
		}
		return t.UTC(), nil
	}
}

// fatalf loga um erro fatal e sai com código 1.
func fatalf(format string, args ...any) {
	log.Fatal().Msgf(format, args...)
}
