// internal/service/matching/score.go
package matching

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper"
)

// scorePass descreve um único passo de matching com seu score associado.
type scorePass struct {
	variantIDs []uuid.UUID
	score      int
}

// scoreResult reúne os candidatos encontrados e o score mais alto atingido.
type scoreResult struct {
	variantIDs []uuid.UUID
	score      int
}

// scoreResult executa os 4 passes de matching em ordem de prioridade e
// devolve o melhor resultado encontrado. O primeiro passo que encontrar
// ao menos uma variante interrompe a cadeia.
//
// Os passes são, em ordem:
//
//	1. set_code exato + number exato         → score 95
//	2. set_name normalizado (ILIKE) + number → score 80
//	3. trigram similarity no nome + number   → score 70
//	4. number only (dentro do set informado) → score 60
func ScoreResult(ctx context.Context, pool *pgxpool.Pool, q scraper.Query) (scoreResult, error) {
	passes := []func(context.Context, *pgxpool.Pool, scraper.Query) ([]uuid.UUID, error){
		pass1ExactSetCodeAndNumber,
		pass2SetNameIlikeAndNumber,
		pass3TrigramNameAndNumber,
		pass4NumberOnly,
	}
	scores := []int{95, 80, 70, 60}

	for i, fn := range passes {
		ids, err := fn(ctx, pool, q)
		if err != nil {
			// log e continua para o próximo passo — uma query com parâmetros
			// vazios pode retornar 0 linhas mas não deve abortar o scoring.
			log.Warn().Err(err).Int("pass", i+1).Msg("matching: erro no passo de scoring, continuando")
			continue
		}
		if len(ids) > 0 {
			return scoreResult{variantIDs: ids, score: scores[i]}, nil
		}
	}

	// Nenhum passo encontrou candidatos.
	return scoreResult{score: 0}, nil
}

// pass1ExactSetCodeAndNumber: set_code case-insensitive + card.collector_number exato.
// Retorna todas as variantes do card encontrado (pode ser mais de uma,
// e.g., holo + reverse holo do mesmo número).
func pass1ExactSetCodeAndNumber(ctx context.Context, pool *pgxpool.Pool, q scraper.Query) ([]uuid.UUID, error) {
	if q.SetCode == "" || q.Number == "" {
		return nil, nil
	}
	const sql = `
		SELECT cv.id
		FROM card_variants cv
		JOIN cards c ON c.id = cv.card_id
		JOIN card_sets cs ON cs.id = c.set_id
		WHERE LOWER(cs.code) = LOWER($1)
		  AND c.collector_number = $2`

	return queryVariantIDs(ctx, pool, sql, q.SetCode, q.Number)
}

// pass2SetNameIlikeAndNumber: set_name com ILIKE (tolerante a case/espaços extras)
// + card.collector_number exato.
func pass2SetNameIlikeAndNumber(ctx context.Context, pool *pgxpool.Pool, q scraper.Query) ([]uuid.UUID, error) {
	if q.SetName == "" || q.Number == "" {
		return nil, nil
	}
	const sql = `
		SELECT cv.id
		FROM card_variants cv
		JOIN cards c ON c.id = cv.card_id
		JOIN card_sets cs ON cs.id = c.set_id
		WHERE cs.name ILIKE $1
		  AND c.collector_number = $2`

	return queryVariantIDs(ctx, pool, sql, "%"+q.SetName+"%", q.Number)
}

// pass3TrigramNameAndNumber: usa pg_trgm para comparar o nome da carta
// com o título bruto do resultado. Retorna as top-5 variantes com
// similarity > 0.3 ordenadas por sim DESC.
func pass3TrigramNameAndNumber(ctx context.Context, pool *pgxpool.Pool, q scraper.Query) ([]uuid.UUID, error) {
	if q.Name == "" || q.Number == "" {
		return nil, nil
	}
	const sql = `
		SELECT cv.id
		FROM card_variants cv
		JOIN cards c ON c.id = cv.card_id
		WHERE c.collector_number = $2
		  AND similarity(c.name, $1) > 0.3
		ORDER BY similarity(c.name, $1) DESC
		LIMIT 5`

	return queryVariantIDs(ctx, pool, sql, q.Name, q.Number)
}

// pass4NumberOnly: fallback — busca qualquer variante com esse número.
// Útil quando SetCode e SetName estão ausentes (e.g., eBay scraper).
// Se SetCode está presente, restringe ao set para reduzir falsos positivos.
func pass4NumberOnly(ctx context.Context, pool *pgxpool.Pool, q scraper.Query) ([]uuid.UUID, error) {
	if q.Number == "" {
		return nil, nil
	}

	if q.SetCode != "" {
		// Restringir ao set reduz ambiguidade: mesmo número em sets diferentes
		// não colidem.
		const sql = `
			SELECT cv.id
			FROM card_variants cv
			JOIN cards c ON c.id = cv.card_id
			JOIN card_sets cs ON cs.id = c.set_id
			WHERE c.collector_number = $1
			  AND LOWER(cs.code) = LOWER($2)`
		return queryVariantIDs(ctx, pool, sql, q.Number, q.SetCode)
	}

	const sql = `
		SELECT cv.id
		FROM card_variants cv
		JOIN cards c ON c.id = cv.card_id
		WHERE c.collector_number = $1`
	return queryVariantIDs(ctx, pool, sql, q.Number)
}

// queryVariantIDs executa uma query paramétrica e coleta os UUIDs retornados.
func queryVariantIDs(ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) ([]uuid.UUID, error) {
	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("matching query: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("matching scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// pickVariant escolhe a variante mais provável dentro de um conjunto de candidatas.
//
// Regras de desempate (aplicadas em ordem):
//  1. Se há apenas uma candidata, devolve ela com +10 de bônus.
//  2. Se o título contém "reverse holo", prefere finish = 'reverse_holo'.
//  3. Se o título contém "holo" (mas não "reverse"), prefere finish = 'holo'.
//  4. Caso nenhuma regra case, devolve a primeira da lista (sem bônus).
func pickVariant(ctx context.Context, pool *pgxpool.Pool, candidates []uuid.UUID, title string, baseScore int) (uuid.UUID, int, error) {
	if len(candidates) == 0 {
		return uuid.Nil, 0, nil
	}
	if len(candidates) == 1 {
		return candidates[0], baseScore + 10, nil
	}

	// Busca o finish de cada candidato para aplicar heurística de título.
	finishes, err := fetchFinishes(ctx, pool, candidates)
	if err != nil {
		// Não é erro fatal — apenas usamos o primeiro candidato sem bônus.
		log.Warn().Err(err).Msg("matching: não foi possível buscar finishes para desempate")
		return candidates[0], baseScore, nil
	}

	titleLower := strings.ToLower(title)
	isReverse := strings.Contains(titleLower, "reverse")
	isHolo := strings.Contains(titleLower, "holo")

	for id, finish := range finishes {
		switch {
		case isReverse && finish == "reverse_holo":
			return id, baseScore, nil
		case isHolo && !isReverse && finish == "holo":
			return id, baseScore, nil
		}
	}

	// Nenhuma heurística casou; retorna o primeiro sem bônus.
	return candidates[0], baseScore, nil
}

// fetchFinishes busca o finish de cada variante. Devolve um map uuid→finish.
func fetchFinishes(ctx context.Context, pool *pgxpool.Pool, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	// pgx aceita []uuid.UUID como array Postgres via pgx.Array.
	const sql = `SELECT id, finish FROM card_variants WHERE id = ANY($1)`
	rows, err := pool.Query(ctx, sql, ids)
	if err != nil {
		return nil, fmt.Errorf("fetch finishes: %w", err)
	}
	defer rows.Close()

	out := make(map[uuid.UUID]string, len(ids))
	for rows.Next() {
		var id uuid.UUID
		var finish string
		if err := rows.Scan(&id, &finish); err != nil {
			return nil, fmt.Errorf("scan finish: %w", err)
		}
		out[id] = finish
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows finish: %w", err)
	}
	return out, nil
}
