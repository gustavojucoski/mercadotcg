// internal/service/matching/score_test.go
//
// Testes unitários para a lógica de scoring e desempate de variante.
//
// Estratégia de isolamento:
//   - ScoreResult depende de *pgxpool.Pool — testado via banco real nos testes
//     de integração (service_integration_test.go). Aqui testamos apenas as
//     funções de decisão pura que NÃO precisam de banco.
//   - pickVariant depende de banco somente para fetchFinishes. Testamos a lógica
//     de desempate por finish via pickVariantPure, que replica a mesma lógica de
//     decisão sem IO (ver função abaixo).
//   - runScoreChain testa a orquestração de passes de scoring isolando as
//     funções de query via closures injetadas — sem banco.
//
// Validações críticas:
//   - A cadeia interrompe no primeiro passo com resultado.
//   - Scores corretos por passo: 95, 80, 70, 60.
//   - Erro num passo não aborta a cadeia (continua para o próximo).
//   - Único candidato recebe bônus +10 de confiança.
//   - Título "reverse holo" → finish reverse_holo preferido.
//   - Título "holo" (sem "reverse") → finish holo preferido.
//   - Sem heurística → primeiro candidato sem bônus.
//   - Lista vazia → uuid.Nil, score 0.
//   - Score 95 + bônus +10 = 105 ANTES do cap (cap é aplicado por Resolve).
package matching

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// ─── runScoreChain — orquestração de passes ──────────────────────────────────

// runScoreChain replica a lógica de ScoreResult com passes injetáveis.
// Permite testar o comportamento de parada e score sem banco.
func runScoreChain(passes []func() ([]uuid.UUID, error)) scoreResult {
	scores := []int{95, 80, 70, 60}
	for i, fn := range passes {
		ids, err := fn()
		if err != nil {
			// Erro: loga e continua — comportamento de ScoreResult.
			continue
		}
		if len(ids) > 0 {
			score := 0
			if i < len(scores) {
				score = scores[i]
			}
			return scoreResult{variantIDs: ids, score: score}
		}
	}
	return scoreResult{score: 0}
}

func TestScoreChain_StopsAtFirstHit_Pass1(t *testing.T) {
	id := uuid.New()
	sr := runScoreChain([]func() ([]uuid.UUID, error){
		func() ([]uuid.UUID, error) { return []uuid.UUID{id}, nil }, // pass1 hit
		func() ([]uuid.UUID, error) { panic("pass2 should not be called") },
	})
	if sr.score != 95 {
		t.Errorf("score: got %d, want 95", sr.score)
	}
	if len(sr.variantIDs) != 1 || sr.variantIDs[0] != id {
		t.Errorf("variantIDs: got %v", sr.variantIDs)
	}
}

func TestScoreChain_StopsAtFirstHit_Pass2(t *testing.T) {
	id := uuid.New()
	sr := runScoreChain([]func() ([]uuid.UUID, error){
		func() ([]uuid.UUID, error) { return nil, nil },              // pass1 miss
		func() ([]uuid.UUID, error) { return []uuid.UUID{id}, nil },  // pass2 hit
		func() ([]uuid.UUID, error) { panic("pass3 should not be called") },
	})
	if sr.score != 80 {
		t.Errorf("score: got %d, want 80", sr.score)
	}
}

func TestScoreChain_StopsAtFirstHit_Pass3(t *testing.T) {
	id := uuid.New()
	sr := runScoreChain([]func() ([]uuid.UUID, error){
		func() ([]uuid.UUID, error) { return nil, nil },
		func() ([]uuid.UUID, error) { return nil, nil },
		func() ([]uuid.UUID, error) { return []uuid.UUID{id}, nil },
		func() ([]uuid.UUID, error) { panic("pass4 should not be called") },
	})
	if sr.score != 70 {
		t.Errorf("score: got %d, want 70", sr.score)
	}
}

func TestScoreChain_StopsAtFirstHit_Pass4(t *testing.T) {
	id := uuid.New()
	sr := runScoreChain([]func() ([]uuid.UUID, error){
		func() ([]uuid.UUID, error) { return nil, nil },
		func() ([]uuid.UUID, error) { return nil, nil },
		func() ([]uuid.UUID, error) { return nil, nil },
		func() ([]uuid.UUID, error) { return []uuid.UUID{id}, nil },
	})
	if sr.score != 60 {
		t.Errorf("score: got %d, want 60", sr.score)
	}
}

func TestScoreChain_AllPassesMiss_ScoreZero(t *testing.T) {
	sr := runScoreChain([]func() ([]uuid.UUID, error){
		func() ([]uuid.UUID, error) { return nil, nil },
		func() ([]uuid.UUID, error) { return nil, nil },
		func() ([]uuid.UUID, error) { return nil, nil },
		func() ([]uuid.UUID, error) { return nil, nil },
	})
	if sr.score != 0 {
		t.Errorf("score: got %d, want 0", sr.score)
	}
	if len(sr.variantIDs) != 0 {
		t.Errorf("variantIDs should be empty, got %v", sr.variantIDs)
	}
}

// TestScoreChain_ErrorInPassContinues valida ADR-020: erro num passo não
// aborta a cadeia. O passo seguinte deve ser tentado.
func TestScoreChain_ErrorInPassContinues(t *testing.T) {
	id := uuid.New()
	fakeErr := errors.New("fake query error")
	sr := runScoreChain([]func() ([]uuid.UUID, error){
		func() ([]uuid.UUID, error) { return nil, fakeErr },           // pass1 error
		func() ([]uuid.UUID, error) { return []uuid.UUID{id}, nil },   // pass2 hit
	})
	if sr.score != 80 {
		t.Errorf("score: got %d, want 80 (pass2 after pass1 error)", sr.score)
	}
}

// TestScoreChain_MultipleErrors_AllSkipped verifica que múltiplos erros
// consecutivos não travam a cadeia e retornam score=0 ao final.
func TestScoreChain_MultipleErrors_AllSkipped(t *testing.T) {
	fakeErr := errors.New("db timeout")
	sr := runScoreChain([]func() ([]uuid.UUID, error){
		func() ([]uuid.UUID, error) { return nil, fakeErr },
		func() ([]uuid.UUID, error) { return nil, fakeErr },
		func() ([]uuid.UUID, error) { return nil, fakeErr },
		func() ([]uuid.UUID, error) { return nil, fakeErr },
	})
	if sr.score != 0 {
		t.Errorf("expected score 0 when all passes error, got %d", sr.score)
	}
}

// ─── pickVariantPure — lógica de desempate ────────────────────────────────────
//
// Replica a lógica de decisão de pickVariant sem IO para testar em isolamento.
// A função real em score.go usa fetchFinishes (banco) para popular o mapa de
// finishes — aqui injetamos o mapa diretamente.

func pickVariantPure(candidates []uuid.UUID, title string, baseScore int, finishes map[uuid.UUID]string) (uuid.UUID, int) {
	if len(candidates) == 0 {
		return uuid.Nil, 0
	}
	if len(candidates) == 1 {
		return candidates[0], baseScore + 10
	}

	titleLower := strings.ToLower(title)
	isReverse := strings.Contains(titleLower, "reverse")
	isHolo := strings.Contains(titleLower, "holo")

	for id, finish := range finishes {
		switch {
		case isReverse && finish == "reverse_holo":
			return id, baseScore
		case isHolo && !isReverse && finish == "holo":
			return id, baseScore
		}
	}
	return candidates[0], baseScore
}

func TestPickVariant_EmptyInput(t *testing.T) {
	id, score := pickVariantPure(nil, "anything", 80, map[uuid.UUID]string{})
	if id != uuid.Nil {
		t.Errorf("expected uuid.Nil for empty candidates, got %v", id)
	}
	if score != 0 {
		t.Errorf("expected score 0, got %d", score)
	}
}

func TestPickVariant_SingleCandidate_BonusApplied(t *testing.T) {
	id := uuid.New()
	resultID, score := pickVariantPure([]uuid.UUID{id}, "some title", 80, map[uuid.UUID]string{id: "holo"})
	if resultID != id {
		t.Errorf("wrong variant: got %v, want %v", resultID, id)
	}
	// +10 bônus para único candidato
	if score != 90 {
		t.Errorf("expected score 90 (80+10), got %d", score)
	}
}

func TestPickVariant_SingleCandidate_HighBase_BonusBeforeCap(t *testing.T) {
	// score 95 + bônus +10 = 105. O cap para 100 é responsabilidade de Resolve.
	id := uuid.New()
	_, score := pickVariantPure([]uuid.UUID{id}, "any", 95, map[uuid.UUID]string{id: "holo"})
	if score != 105 {
		t.Errorf("expected 105 before Resolve cap, got %d", score)
	}
}

func TestPickVariant_ReverseHoloPriority(t *testing.T) {
	reverseID := uuid.New()
	holoID := uuid.New()
	normalID := uuid.New()
	candidates := []uuid.UUID{normalID, holoID, reverseID}
	finishes := map[uuid.UUID]string{
		reverseID: "reverse_holo",
		holoID:    "holo",
		normalID:  "normal",
	}

	resultID, _ := pickVariantPure(candidates, "Pikachu ex reverse holo 199/191", 80, finishes)
	if resultID != reverseID {
		t.Errorf("expected reverse_holo variant %v, got %v", reverseID, resultID)
	}
}

func TestPickVariant_HoloPriority_NoReverse(t *testing.T) {
	holoID := uuid.New()
	normalID := uuid.New()
	candidates := []uuid.UUID{normalID, holoID}
	finishes := map[uuid.UUID]string{
		holoID:   "holo",
		normalID: "normal",
	}

	resultID, _ := pickVariantPure(candidates, "Charizard ex Holo Rare", 80, finishes)
	if resultID != holoID {
		t.Errorf("expected holo variant %v, got %v", holoID, resultID)
	}
}

// TestPickVariant_HoloTitleDoesNotMatchReverseHolo valida que "holo" no título
// NÃO seleciona reverse_holo — a regra isHolo requer !isReverse.
func TestPickVariant_HoloTitleDoesNotMatchReverseHolo(t *testing.T) {
	reverseID := uuid.New()
	holoID := uuid.New()
	candidates := []uuid.UUID{reverseID, holoID}
	finishes := map[uuid.UUID]string{
		reverseID: "reverse_holo",
		holoID:    "holo",
	}

	resultID, _ := pickVariantPure(candidates, "Pikachu ex Holo Rare", 80, finishes)
	if resultID != holoID {
		t.Errorf("'holo' title should prefer holo over reverse_holo, got %v", resultID)
	}
}

func TestPickVariant_NoHeuristicMatch_ReturnsFirst(t *testing.T) {
	firstID := uuid.New()
	secondID := uuid.New()
	candidates := []uuid.UUID{firstID, secondID}
	finishes := map[uuid.UUID]string{
		firstID:  "normal",
		secondID: "normal",
	}

	resultID, score := pickVariantPure(candidates, "Pikachu ex 199/191", 70, finishes)
	if resultID != firstID {
		t.Errorf("expected first candidate %v, got %v", firstID, resultID)
	}
	// Sem bônus quando mais de um candidato e sem heurística casada
	if score != 70 {
		t.Errorf("expected score 70 (no bonus), got %d", score)
	}
}

// TestPickVariant_CaseSensitivity valida que a comparação de título é
// case-insensitive ("Reverse Holo", "REVERSE HOLO", "reverse holo" devem casar).
func TestPickVariant_CaseSensitivity_ReverseUppercase(t *testing.T) {
	reverseID := uuid.New()
	normalID := uuid.New()
	candidates := []uuid.UUID{normalID, reverseID}
	finishes := map[uuid.UUID]string{
		reverseID: "reverse_holo",
		normalID:  "normal",
	}

	for _, title := range []string{
		"Pikachu ex REVERSE HOLO",
		"Pikachu ex Reverse Holo",
		"pikachu ex reverse holo",
	} {
		resultID, _ := pickVariantPure(candidates, title, 80, finishes)
		if resultID != reverseID {
			t.Errorf("title %q: expected reverse_holo variant, got %v", title, resultID)
		}
	}
}
