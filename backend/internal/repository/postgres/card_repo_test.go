// internal/repository/postgres/card_repo_test.go
//
// Testes unitários para CardRepo — foco em comportamentos que não requerem
// banco (guards, edge cases) e testes de integração marcados com t.Skip quando
// DATABASE_URL não estiver disponível.
//
// Testes de integração (via testcontainers) ficam em card_repo_integration_test.go
// quando testcontainers-go for adicionado ao go.mod.
package postgres_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/gustavojucoski/mercadotcg/backend/internal/repository/postgres"
)

// TestGetVariantDisplayBatch_EmptyInput verifica que passar um slice vazio
// retorna map vazio imediatamente, sem tentar executar a query no banco.
//
// Contexto: o handler ListStock constrói variantIDs a partir dos items
// retornados por ListItemsByStore. Se a loja não tem estoque, variantIDs
// fica vazio. Passar ANY($1) com array vazio para o banco poderia causar
// erro de sintaxe ou resultado inesperado em algumas versões de driver.
// O guard em GetVariantDisplayBatch previne esse caminho.
//
// Este teste chama GetVariantDisplayBatch com nil pool — se o guard não
// existisse, causaria panic/nil pointer. O teste PASSA se a função retorna
// antes de usar o pool.
func TestGetVariantDisplayBatch_EmptyInput(t *testing.T) {
	// Passa nil pool propositalmente: se o código tentar usar o pool, pânico.
	repo := postgres.NewCardRepo(nil)

	result, err := repo.GetVariantDisplayBatch(nil, []uuid.UUID{})
	if err != nil {
		t.Fatalf("unexpected error for empty slice: %v", err)
	}
	if result == nil {
		t.Error("expected non-nil map for empty slice, got nil")
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

// TestGetVariantDisplayBatch_NilInput verifica que nil é tratado igual a
// slice vazio — ambos devem retornar map vazio sem tocar no banco.
func TestGetVariantDisplayBatch_NilInput(t *testing.T) {
	repo := postgres.NewCardRepo(nil)

	result, err := repo.GetVariantDisplayBatch(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error for nil input: %v", err)
	}
	if result == nil {
		t.Error("expected non-nil map, got nil")
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}
